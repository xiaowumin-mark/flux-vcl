// Package native 实现默认 LCL 后端（energye/lcl + libenergy DLL）的 Renderer
// 适配层（design.md §5.2 / D6 绑定隔离）。
//
// 把 internal/render 窄接口映射到 LCL 控件：Window→TEngForm（Application.NewForms
// 注册的主窗体）、Text→TLabel、Button→TButton、Input→TEdit。事件经显式回调注册
// （D6：禁止反射方法名绑定）。
//
// 初始化序列（Phase 0 E2 结论，见 docs/phase0-e2-libenergy-mapping.md）：
//
//	Init(dllPath) → NewRenderer()（内部 Application.NewForms）→ 声明式 Render → Application.Run()
//
// 版本约束：lcl v1.0.3 ↔ libenergy-amd64.dll 严格一致，错位时窗口无法创建。
//
// DPI 策略（Phase 3.5，research.md §5.4）：Renderer 接口全 DIP，只有本层在边界做
// DIP↔物理像素换算（render.DIPToPX / render.PXToDIP）。WM_DPICHANGED 钩子先放行
// LCL 默认处理（窗体按建议矩形 resize、字体随 widgetset 缩放），再清缓存触发全量
// 重排。字体策略：不调 ScaleForPPI、不改 Application.Scaled —— TextExtent 经
// canvasDpi 归一化（bitmap DC 的 DPI 进程内固定，显示器无关）后测量结果自洽。
package native

import (
	"fmt"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"github.com/energye/lcl/api"
	"github.com/energye/lcl/api/libname"
	"github.com/energye/lcl/lcl"
	"github.com/energye/lcl/types"
	"github.com/energye/lcl/types/messages"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// GetDpiForWindow / GetDeviceCaps / DwmSetWindowAttribute 无 energye 封装，用
// syscall 直调（项目零 CGO）。均为系统自带 DLL，进程内 lazy 加载一次。
var (
	procGetDpiForWindow       = syscall.NewLazyDLL("user32.dll").NewProc("GetDpiForWindow")
	procGetDeviceCaps         = syscall.NewLazyDLL("gdi32.dll").NewProc("GetDeviceCaps")
	procDwmSetWindowAttribute = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")
)

// logPixelsX 是 GetDeviceCaps 的 LOGPIXELSX 索引（水平每英寸像素）。
const logPixelsX = 88

// engForm 是适配层拥有的主窗体类型（Application.NewForms 注册）。
type engForm struct {
	lcl.TEngForm
}

// Init 封装 Phase 0 E2 验证的标准初始化序列：加载 DLL → lcl.Init →
// Initialize → SetMainFormOnTaskBar。必须在创建任何控件前调用一次。
func Init(dllPath string) error {
	libname.LibName = dllPath
	lcl.Init(nil, nil)
	if api.Widget() != api.WtWIN32 {
		return fmt.Errorf("native: 期望 WtWIN32 widgetset，实际 = %v", api.Widget())
	}
	lcl.Application.Initialize()
	lcl.Application.SetMainFormOnTaskBar(true)
	return nil
}

// Renderer 是 energye/lcl 后端的 Renderer 实现：把窄接口调用映射到 LCL 控件。
type Renderer struct {
	controls       map[render.Handle]lcl.IControl
	next           render.Handle
	form           lcl.IControl
	formRef        *engForm
	measureBmp     lcl.IBitmap                    // 共享测量画布（布局在 diff 前，控件未创建）
	measureCache   map[string][2]int32            // 文本测量缓存（字体随 DPI 变化时失效）
	dpi            int32                          // 当前显示器 DPI（0=未查询，invalidateDPI 清零强制重查）
	canvasDpi      int32                          // 测量 bitmap DC 的 DPI（进程内固定，缓存一次；0=未查询）
	resizeFn       func(w, h int)                 // OnResize 统一回调（窗体 resize 与 WM_DPICHANGED 共用）
	closeFn        func()                         // OnClose 回调（demo 停止后台轮询）
	closed         atomic.Bool                    // 窗体已进入关闭流程：拒绝后续 UI marshalling（关机竞态防护）
	pendingDestroy []lcl.IControl                 // D4 延后销毁队列：render 完成时 DrainDestroy 统一 Free
	scrolls        map[render.Handle]*listScroll  // Phase 6 ListView 滚动状态（Scrollable 实现）
	radios         map[render.Handle]*radioState  // RadioButton 的逻辑分组元数据（不依赖缺失的 LCL setter）
	radioHosts     map[radioHostKey]*radioHost    // (原生父句柄, RadioButton 句柄) → 隔离用 TPanel
	pendingHosts   []*radioHost                   // 已脱离逻辑父级、待普通控件释放后销毁的内部 Panel
	pages          map[render.Handle]*pageState   // PageControl 的受控选择、页面顺序与事件状态
	texts          map[render.Handle]*textState   // Input/Memo 的程序化 SetText 应用门
	sliders        map[render.Handle]*sliderState // Slider 的程序化应用门与值变化回调
	paints         map[render.Handle]*paintState  // PaintBox 的稳定命令快照与原生绘制 surface
	grids          map[render.Handle]*gridState   // StringGrid 的受控矩阵、选择与编辑事件状态
	gridPollTimer  lcl.ITimer                     // Grid 选择轮询器；窗体拥有，空闲时禁用并复用
	gridPollStop   func()                         // 当前启用周期的幂等停止函数；nil 表示空闲
}

type textState struct {
	applying bool
}

type sliderState struct {
	applying bool
	onChange func(int)
}

type paintState struct {
	control  lcl.IPaintBox
	commands []render.PaintCommand
}

type gridEdit struct {
	cell  render.GridCell
	value string
}

type gridState struct {
	control   lcl.IStringGrid
	size      render.GridSize
	headers   []string
	widths    []int
	cells     [][]string
	editable  bool
	selection render.GridSelection
	applying  bool
	onSelect  func(render.GridCell)
	onEdit    func(render.GridCell, string)
	pending   *gridEdit
}

type pageState struct {
	selected int
	applying bool
	onSelect func(int)
	pages    []render.Handle
}

var (
	_ render.SliderController = (*Renderer)(nil)
	_ render.PaintController  = (*Renderer)(nil)
	_ render.GridController   = (*Renderer)(nil)
)

// radioState 保存 RadioButton 的 Flux 逻辑父级、坐标和受控属性。TRadioButton 在
// energye/lcl v1.0.3 中没有 GroupIndex setter，因此每个控件使用一个内部 TPanel
// 隔离 LCL 的同 parent 互斥；逻辑组由 Renderer 元数据维护，Panel 句柄绝不暴露给
// diff 或公开 API。
type radioState struct {
	parent   render.Handle
	group    int
	bounds   render.Rect
	visible  bool
	checked  bool
	applying bool // 抑制 SetChecked 触发的同步 OnChange 重入
	host     *radioHost
}

type radioHostKey struct {
	parent render.Handle
	handle render.Handle
}

type radioHost struct {
	key     radioHostKey
	panel   lcl.IPanel
	members map[render.Handle]struct{}
}

// listScroll 是 ListView 的原生滚动状态（Phase 6，design.md §16）：
// TScrollBox 视口（AutoScroll=false、隐藏内建滚动条、DoubleBuffered）＋ 独立
// TScrollBar（可视垂直滚动输入）。滚动位置由框架拥有（DIP），本层只把滚动条
// 范围/位置与滚轮/拖动事件换算成 DIP 回调（D6：滚动输入设备，不含业务逻辑）。
type listScroll struct {
	viewport lcl.IScrollBox // 视口容器：行控件池的宿主（diff 挂载/复用/延后销毁）
	bar      lcl.IScrollBar // 可视垂直滚动条（随视口右缘定位）
	content  int            // ScrollConfig.Content：内容总高（DIP）
	step     int            // ScrollConfig.Step：滚轮每档步长（DIP）
	pos      int            // 当前滚动位置（DIP，读入滚动条，防止事件回环）
	viewH    int            // 视口可见高（DIP，SetBounds 更新；滚动范围 = content-viewH）
	onScroll func(int)      // OnScroll 回调：滚动位置变化 → 回写 State → re-render
}

// NewRenderer 创建 LCL 渲染器并注册主窗体（须在 Init 之后调用）。
// 设默认窗体客户区 640x480 DIP，按当前 DPI 换算为物理像素。
func NewRenderer() *Renderer {
	f := &engForm{}
	lcl.Application.NewForms(f)
	r := &Renderer{
		controls:     make(map[render.Handle]lcl.IControl),
		measureCache: make(map[string][2]int32),
		formRef:      f,
		form:         f,
	}
	dpi := int(r.currentDPI())
	f.SetClientWidth(int32(render.DIPToPX(640, dpi)))
	f.SetClientHeight(int32(render.DIPToPX(480, dpi)))
	r.setupDPIHook()
	return r
}

func (r *Renderer) Create(widgetType string) render.Handle {
	var c lcl.IControl
	var ls *listScroll // ListView 的滚动状态（switch 内构建，h 分配后登记）
	var paint *paintState
	var grid *gridState
	switch widgetType {
	case "Window":
		c = r.form
	case "Button":
		c = lcl.NewButton(r.form)
	case "Text":
		c = lcl.NewLabel(r.form)
	case "Input":
		c = lcl.NewEdit(r.form)
	case "Memo":
		c = lcl.NewMemo(r.form)
	case "CheckBox":
		c = lcl.NewCheckBox(r.form)
	case "ComboBox":
		c = lcl.NewComboBox(r.form)
	case "RadioButton":
		c = lcl.NewRadioButton(r.form)
	case "ProgressBar":
		c = lcl.NewProgressBar(r.form)
	case "Slider":
		slider := lcl.NewTrackBar(r.form)
		slider.SetAlign(types.AlNone)
		slider.SetOrientation(types.TrHorizontal)
		slider.SetTickStyle(types.TsNone)
		c = slider
	case "PaintBox":
		box := lcl.NewPaintBox(r.form)
		box.SetAlign(types.AlNone)
		paint = &paintState{control: box, commands: []render.PaintCommand{}}
		box.SetOnPaint(func(_ lcl.IObject) {
			render.Guard("paint.OnPaint", func() { r.paint(paint) })
		})
		c = box
	case "StringGrid":
		control := lcl.NewStringGrid(r.form)
		control.SetAlign(types.AlNone)
		control.SetFixedCols(0)
		control.SetFixedRows(0)
		control.SetColCount(1)
		control.SetRowCount(1)
		grid = &gridState{
			control: control,
			size:    render.GridSize{Columns: 1},
			cells:   [][]string{},
			selection: render.GridSelection{
				Cell: render.GridCell{Row: -1, Column: -1},
			},
		}
		c = control
	case "PageControl":
		page := lcl.NewPageControl(r.form)
		page.SetAlign(types.AlNone)
		c = page
	case "TabPage":
		page := lcl.NewTabSheet(r.form)
		page.SetAlign(types.AlNone)
		c = page
	case "ScrollBox":
		// 垂直滚动容器（Phase 3.6）：AutoScroll=true 让 LCL 自动按子控件包围盒
		// 计算滚动范围、滚动条自动出现；DoubleBuffered 防闪烁（WM_SETREDRAW 批量
		// 防闪烁留 Phase 5 虚拟化）。
		sb := lcl.NewScrollBox(r.form)
		sb.SetAutoScroll(true)
		sb.SetDoubleBuffered(true)
		c = sb
	case "ListView":
		// 虚拟滚动列表（Phase 6，design.md §16）：TScrollBox 作视口（控件池宿主），
		// 隐藏内建双滚动条（滚动范围/位置由框架以 DIP 驱动），用独立 TScrollBar 作
		// 可视垂直滚动输入。可见区外的行不建控件 —— 虚拟化的原生侧不感知。
		view := lcl.NewScrollBox(r.form)
		view.SetAutoScroll(false) // 内容由布局引擎虚拟化定位，不用 LCL 自动滚动
		view.SetDoubleBuffered(true)
		view.HorzScrollBar().SetVisible(false)
		view.VertScrollBar().SetVisible(false)
		bar := lcl.NewScrollBar(view)
		bar.SetParent(view)
		bar.SetKind(types.SbVertical)
		c = view
		ls = &listScroll{viewport: view, bar: bar}
		wireScroll(ls)
	default:
		panic(fmt.Sprintf("native: 未知控件类型 %q", widgetType))
	}
	h := r.alloc()
	r.controls[h] = c
	if widgetType == "RadioButton" {
		if r.radios == nil {
			r.radios = make(map[render.Handle]*radioState)
		}
		r.radios[h] = &radioState{visible: true}
	}
	if ls != nil {
		if r.scrolls == nil {
			r.scrolls = make(map[render.Handle]*listScroll)
		}
		r.scrolls[h] = ls
	}
	if widgetType == "PageControl" {
		if r.pages == nil {
			r.pages = make(map[render.Handle]*pageState)
		}
		r.pages[h] = &pageState{selected: -1}
	}
	if widgetType == "Input" || widgetType == "Memo" {
		if r.texts == nil {
			r.texts = make(map[render.Handle]*textState)
		}
		r.texts[h] = &textState{}
	}
	if widgetType == "Slider" {
		if r.sliders == nil {
			r.sliders = make(map[render.Handle]*sliderState)
		}
		r.sliders[h] = &sliderState{}
	}
	if paint != nil {
		if r.paints == nil {
			r.paints = make(map[render.Handle]*paintState)
		}
		r.paints[h] = paint
	}
	if grid != nil {
		if r.grids == nil {
			r.grids = make(map[render.Handle]*gridState)
		}
		r.grids[h] = grid
	}
	return h
}

// wireScroll 接线 ListView 滚动输入设备（滚轮 + 滚动条拖动 → DIP 回调）。
// 滚轮步长与滚动范围来自 layoutListView 写入的 ScrollConfig（内容总高/步长，DIP）。
func wireScroll(ls *listScroll) {
	view, bar := ls.viewport, ls.bar

	// 滚轮：向上滚（delta>0）内容上移 → 位置减小。步长按比例换算（120 = 一格 ×
	// step），高精度滚轮（<120 的细分 delta）也按比例滚动而非截断为零。
	view.SetOnMouseWheel(func(_ lcl.IObject, _ types.TShiftState, wheelDelta int32, _ types.TPoint, handled *bool) {
		if handled != nil {
			*handled = true
		}
		ls.applyScroll(ls.pos - int(wheelDelta)*ls.step/120)
	})
	// 滚动条拖动/翻页：LCL 直接给绝对位置（scrollPos *int32，DIP 单位）。
	bar.SetOnScroll(func(_ lcl.IObject, _ types.TScrollCode, scrollPos *int32) {
		if scrollPos == nil {
			return
		}
		ls.applyScroll(int(*scrollPos))
	})
}

// applyScroll 统一处理滚动输入：钳制到 [0, content-viewH]，写回自身状态并回调。
// 事件回调同源（State.Set → re-render → SetScrollPos 把 pos 写回滚动条）时，
// 滚动条 Position 已是目标值，LCL 不再触发事件（无回环）。
func (ls *listScroll) applyScroll(pos int) {
	if ls.onScroll == nil {
		ls.pos = pos
		return
	}
	max := ls.content - ls.viewH
	if max < 0 {
		max = 0
	}
	if pos < 0 {
		pos = 0
	}
	if pos > max {
		pos = max
	}
	if ls.pos == pos {
		return
	}
	ls.pos = pos
	render.Guard("event.OnScroll", func() { ls.onScroll(pos) })
}

// applyBarRange 把内容总高/视口高/当前位置同步到原生滚动条（范围 = 内容−视口，
// 页尺寸 = 视口高）。内容或视口未就绪时跳过 —— SetScrollConfig/SetBounds 任一
// 到达且两者齐备后即生效（挂载时 diff 属性应用顺序不定，见 diff.applyProps）。
func (r *Renderer) applyBarRange(ls *listScroll) {
	if ls.content <= 0 || ls.viewH <= 0 {
		return
	}
	max := ls.content - ls.viewH
	if max < 0 {
		max = 0
	}
	page := ls.viewH
	if page < 1 {
		page = 1
	}
	ls.bar.SetParamsWithIntX4(int32(ls.pos), 0, int32(max), int32(page))
}

// Destroy 销毁控件。销毁入队延后（D4：绝不在事件回调内同步 Free，
// LCLRefCount>0 会崩溃）。句柄从映射表移除（后续 op 不再命中），LCL 对象
// 物理 Free 由 DrainDestroy 在 render 完成后统一执行（App 每次 render 后调用；
// 也是"事件回调内触发 render → 移除控件"的安全边界）。
func (r *Renderer) Destroy(h render.Handle) {
	if radio := r.radios[h]; radio != nil {
		r.removeRadioFromHost(h, radio)
		delete(r.radios, h)
	}
	grid, wasGrid := r.grids[h]
	c := r.controls[h]
	if c == nil || c == r.form {
		return // 主窗体不显式 Free
	}
	// TabSheet 必须先从 PageControl 的页面集合摘除，再进入延后 Free 队列；否则
	// keyed 删除后的 SyncPages 会在原生列表中看到已逻辑删除的旧页，索引会错位。
	if sheet, ok := c.(lcl.ITabSheet); ok {
		// LCL widgetset 可能在 SetPageControl(nil) 时同步派发 PageControl.OnChange。
		// 删除发生在 reconcile/render 栈内，必须把这类结构性变更视为程序化应用，
		// 避免用户回调重入 State/render；真正的用户切页仍由 OnChange 正常派发。
		r.setSheetPageControl(sheet, nil)
	}
	delete(r.controls, h)
	delete(r.pages, h)
	delete(r.texts, h)
	delete(r.sliders, h)
	delete(r.paints, h)
	delete(r.grids, h)
	if wasGrid {
		grid.onSelect = nil
		grid.onEdit = nil
		grid.pending = nil
		grid.control.SetOnAfterSelection(nil)
		grid.control.SetOnSetEditText(nil)
		grid.control.SetOnEditingDone(nil)
		r.stopGridSelectionPollerIfIdle()
	}
	delete(r.scrolls, h) // ListView 滚动状态随控件销毁；滚动条由视口 owner 级联 Free
	r.pendingDestroy = append(r.pendingDestroy, c)
}

// DrainDestroy 物理释放积压的待销毁控件（D4 延后销毁的落地点）。
// App 在每次 render/reconcile 完成后调用；进入关闭流程后也立即清空
// （teardown 期间不再保留待释放对象）。
func (r *Renderer) DrainDestroy() {
	for _, c := range r.pendingDestroy {
		c.Free()
	}
	r.pendingDestroy = nil
	for _, host := range r.pendingHosts {
		host.panel.Free()
	}
	r.pendingHosts = nil
}

func (r *Renderer) SetParent(child, parent render.Handle) {
	childControl := r.controls[child]
	if childControl == nil {
		panic(fmt.Sprintf("native: 未知子控件 %d", child))
	}
	if parent == 0 {
		if _, isSheet := childControl.(lcl.ITabSheet); isSheet {
			panic("native: TabPage 只能直接属于 PageControl")
		}
		return // 顶层（窗体）无父
	}
	parentControl := r.controls[parent]
	if parentControl == nil {
		panic(fmt.Sprintf("native: 未知父控件 %d", parent))
	}
	sheet, isSheet := childControl.(lcl.ITabSheet)
	page, isPageControl := parentControl.(lcl.IPageControl)
	if isSheet != isPageControl {
		if isSheet {
			panic(fmt.Sprintf("native: TabPage 父控件 %d 非 IPageControl", parent))
		}
		panic(fmt.Sprintf("native: PageControl %d 只能直接挂载 TabPage，子控件为 %d", parent, child))
	}
	if isSheet {
		// 页面 attach 是 reconciliation 的结构性操作，不是用户选择；同样屏蔽
		// widgetset 可能同步产生的 OnChange。跨 PageControl 移动时旧、新两侧
		// 都由 setSheetPageControl 进入 applying 状态。
		r.setSheetPageControl(sheet, page)
		return
	}
	if radio := r.radios[child]; radio != nil {
		radio.parent = parent
		r.attachRadioToHost(child, radio)
		return
	}
	pc, ok := parentControl.(lcl.IWinControl)
	if !ok {
		panic(fmt.Sprintf("native: 父控件 %d 非 IWinControl", parent))
	}
	childControl.SetParent(pc)
}

// radioHostFor 返回指定逻辑父级与 RadioButton 的内部 Panel。使用一控件一 host
// 避免 shared host 覆盖同一 bounds 区域内的非 RadioButton sibling；逻辑分组仍由
// Renderer 的 radioState 元数据维护。
func (r *Renderer) radioHostFor(parent, h render.Handle) *radioHost {
	key := radioHostKey{parent: parent, handle: h}
	if host := r.radioHosts[key]; host != nil {
		return host
	}
	pc, ok := r.controls[parent].(lcl.IWinControl)
	if !ok {
		panic(fmt.Sprintf("native: RadioButton 父控件 %d 非 IWinControl", parent))
	}
	panel := lcl.NewPanel(r.form)
	panel.SetParent(pc)
	panel.SetBevelInner(types.BvNone)
	panel.SetBevelOuter(types.BvNone)
	panel.SetBorderWidth(0)
	panel.SetParentColor(true)
	panel.SetParentBackground(true)
	panel.SetTabStop(false)
	host := &radioHost{key: key, panel: panel, members: make(map[render.Handle]struct{})}
	if r.radioHosts == nil {
		r.radioHosts = make(map[radioHostKey]*radioHost)
	}
	r.radioHosts[key] = host
	return host
}

func (r *Renderer) attachRadioToHost(h render.Handle, radio *radioState) {
	if radio.parent == 0 {
		return
	}
	if radio.host != nil {
		r.removeRadioFromHost(h, radio)
	}
	host := r.radioHostFor(radio.parent, h)
	host.members[h] = struct{}{}
	radio.host = host
	r.controls[h].SetParent(host.panel)
	r.layoutRadioHost(host)
	r.applyRadioChecked(h, radio.checked)
}

func (r *Renderer) removeRadioFromHost(h render.Handle, radio *radioState) {
	host := radio.host
	if host == nil {
		return
	}
	delete(host.members, h)
	radio.host = nil
	if len(host.members) != 0 {
		r.layoutRadioHost(host)
		return
	}
	delete(r.radioHosts, host.key)
	host.panel.SetVisible(false)
	r.pendingHosts = append(r.pendingHosts, host)
}

// layoutRadioHost keeps an internal host exactly under one radio. A host-per-radio
// design is necessary here: a shared rectangular Panel would change hit testing and
// z-order for arbitrary interleaved Flux siblings.
func (r *Renderer) layoutRadioHost(host *radioHost) {
	for h := range host.members {
		radio := r.radios[h]
		if radio == nil {
			continue
		}
		b := radio.bounds
		dpi := int(r.currentDPI())
		host.panel.SetBounds(
			int32(render.DIPToPX(b.X, dpi)),
			int32(render.DIPToPX(b.Y, dpi)),
			int32(render.DIPToPX(b.W, dpi)),
			int32(render.DIPToPX(b.H, dpi)),
		)
		r.controls[h].SetBounds(0, 0,
			int32(render.DIPToPX(b.W, dpi)),
			int32(render.DIPToPX(b.H, dpi)),
		)
		host.panel.SetVisible(radio.visible)
	}
}

// applyRadioChecked 必须在 RadioButton 已挂入最终 group host 后调用；否则 LCL
// 会按其创建时的 native parent 清除错误的兄弟控件。
func (r *Renderer) applyRadioChecked(h render.Handle, checked bool) {
	c, ok := r.controls[h].(checkableControl)
	if !ok {
		panic(fmt.Sprintf("native: 控件 %d 不支持 Checked", h))
	}
	radio := r.radios[h]
	if radio == nil {
		c.SetChecked(checked)
		return
	}
	radio.applying = true
	defer func() { radio.applying = false }()
	c.SetChecked(checked)
}

func (r *Renderer) SetBounds(h render.Handle, b render.Rect) {
	if _, ok := r.controls[h].(lcl.ITabSheet); ok {
		return // TabPage 客户区由 PageControl/widgetset 通过 TCM_AdjustRect 管理。
	}
	if radio := r.radios[h]; radio != nil {
		radio.bounds = b
		if radio.host != nil {
			r.layoutRadioHost(radio.host)
		}
		return
	}
	dpi := int(r.currentDPI())
	r.controls[h].SetBounds(
		int32(render.DIPToPX(b.X, dpi)),
		int32(render.DIPToPX(b.Y, dpi)),
		int32(render.DIPToPX(b.W, dpi)),
		int32(render.DIPToPX(b.H, dpi)),
	)
	// ListView 视口：内建滚动条贴视口右缘（宽 17 DIP = listScrollbarStrip，随视口
	// 大小/位置走）；并记 viewH 供滚动范围重算（内容−视口）。
	if ls := r.scrolls[h]; ls != nil {
		ls.viewH = b.H
		r.applyBarRange(ls)
		w := int32(render.DIPToPX(17, dpi))
		ls.bar.SetBounds(
			int32(render.DIPToPX(b.W, dpi))-w,
			0,
			w,
			int32(render.DIPToPX(b.H, dpi)),
		)
	}
}

func (r *Renderer) SetVisible(h render.Handle, visible bool) {
	if radio := r.radios[h]; radio != nil {
		radio.visible = visible
		r.controls[h].SetVisible(visible)
		if radio.host != nil {
			r.layoutRadioHost(radio.host)
		}
		return
	}
	r.controls[h].SetVisible(visible)
}

func (r *Renderer) SetEnabled(h render.Handle, enabled bool) {
	r.controls[h].SetEnabled(enabled)
}

func (r *Renderer) SetText(h render.Handle, text string) {
	c := r.controls[h]
	if ed, ok := c.(lcl.ICustomEdit); ok {
		state := r.texts[h]
		if state != nil {
			previous := state.applying
			state.applying = true
			defer func() { state.applying = previous }()
		}
		ed.SetText(text)
	} else {
		c.SetCaption(text)
		// 重绘保险：TLabel 无 HWND（自绘在父表面），caption 变化在 DoubleBuffered
		// 容器（ListView 视口）内不保证触发父容器重绘 —— 仅改文字不改尺寸时画面
		// 滞留旧文本，直到 resize 强制整窗重绘（Phase 6 实测）。显式 Invalidate
		// 强制下一帧把新文字画上（无效化会合并进同一次 WM_PAINT，无额外开销）。
		c.Invalidate()
	}
}

// SetColor 设置控件背景色（Phase 5.2 Theme）。ARGB 换算为 LCL TColor（BGR）：
// IControl 统一暴露 SetColor（TButton/TEdit/TLabel/TScrollBox/TEngForm 均有）。
func (r *Renderer) SetColor(h render.Handle, color render.Color) {
	r.controls[h].SetColor(colorToTColor(color))
}

// SetFontColor 设置控件文字颜色（经字体对象）。LCL 的 IControl 无 SetFontColor，
// 文字色统一走 Font().SetColor（TControl.Font() 返回 IFont）。无 Font 语义的
// 控件（极少数）忽略 —— 不 panic（与事件的结构化断言策略一致，D6）。
func (r *Renderer) SetFontColor(h render.Handle, color render.Color) {
	if f := r.controls[h].Font(); f != nil {
		f.SetColor(colorToTColor(color))
	}
}

// SetPaintCommands 替换 PaintBox 的不可变命令快照；invalidate 由调用方通过
// InvalidatePaint 独立控制。
func (r *Renderer) SetPaintCommands(h render.Handle, commands []render.PaintCommand) {
	paint := r.paints[h]
	if paint == nil {
		return
	}
	if err := render.ValidatePaintCommands(commands); err != nil {
		panic(fmt.Sprintf("native: invalid PaintCommands for control %d: %v", h, err))
	}
	paint.commands = render.ClonePaintCommands(commands)
}

// InvalidatePaint 在不重建 TPaintBox 的情况下请求一次 WM_PAINT。
func (r *Renderer) InvalidatePaint(h render.Handle) {
	if paint := r.paints[h]; paint != nil {
		paint.control.Invalidate()
	}
}

// paint 在 TPaintBox.OnPaint 内执行当前命令快照。LCL Canvas 使用物理像素，
// 因此所有 DIP 命令都在这里按 PaintBox 当前显示器 DPI 转换。
func (r *Renderer) paint(paint *paintState) {
	if paint == nil || paint.control == nil {
		return
	}
	canvas := paint.control.Canvas()
	if canvas == nil {
		return
	}
	dpi := r.dpiAt()
	brush := canvas.BrushToBrush()
	pen := canvas.PenToPen()
	for _, command := range paint.commands {
		switch command.Kind {
		case render.PaintClear:
			brush.SetStyle(types.BsSolid)
			brush.SetColor(colorToTColor(command.Color))
			canvas.FillRectWithIntX4(0, 0, paint.control.ClientWidth(), paint.control.ClientHeight())
		case render.PaintCircle:
			if command.FillColor == 0 {
				brush.SetStyle(types.BsClear)
			} else {
				brush.SetStyle(types.BsSolid)
				brush.SetColor(colorToTColor(command.FillColor))
			}
			if command.StrokeColor == 0 {
				pen.SetStyle(types.PsClear)
			} else {
				pen.SetStyle(types.PsSolid)
				pen.SetColor(colorToTColor(command.StrokeColor))
				width := render.DIPToPX(command.StrokeWidth, dpi)
				if width < 1 {
					width = 1
				}
				pen.SetWidth(int32(width))
			}
			x := render.DIPToPX(command.X, dpi)
			y := render.DIPToPX(command.Y, dpi)
			radius := render.DIPToPX(command.Radius, dpi)
			canvas.EllipseWithIntX4(
				int32(x-radius), int32(y-radius),
				int32(x+radius), int32(y+radius),
			)
		}
	}
}

// dwmwaUseImmersiveDarkMode 是 DWMWA_USE_IMMERSIVE_DARK_MODE 的属性号回退链：
// Win10 1809+ 为 20，更早（1607–1709）为 19。先试 20，E_INVALIDARG 回退 19。
const (
	dwmwaUseImmersiveDarkMode20 = 20
	dwmwaUseImmersiveDarkMode19 = 19
)

// SetTitleBarDark 设置窗体标题栏沉浸式暗色（win32 DWM，Phase 5.2 Theme）。
// dark=true → 暗色标题栏（标题文字变浅、背景变深）。仅 Window（窗体）有标题栏，
// h 恒为窗体句柄 —— 实现直接取 formRef.Handle()（与 currentDPI 同源）。
//
// DWM 属性即时生效（DWM 自动重绘标题栏），无需 Recreate/Redraw，属性仅值变化时
// diff 才调用。老系统不支持沉浸式暗色（20 返回 E_INVALIDARG）→ 回退属性 19；
// 仍失败则静默忽略（保持系统默认标题栏，属后端能力而非错误）。
func (r *Renderer) SetTitleBarDark(h render.Handle, dark bool) {
	if r.formRef == nil || !r.formRef.HandleAllocated() {
		return
	}
	hwnd := r.formRef.Handle()
	if hwnd == 0 {
		return
	}
	v := int32(0)
	if dark {
		v = 1
	}
	for _, attr := range []uint32{dwmwaUseImmersiveDarkMode20, dwmwaUseImmersiveDarkMode19} {
		hr, _, _ := procDwmSetWindowAttribute.Call(uintptr(hwnd), uintptr(attr), uintptr(unsafe.Pointer(&v)), 4)
		if hr == 0 { // S_OK
			return
		}
	}
}

// NewTimer 创建主线程定时器（Phase 5.1 动画 pump，TTimer/自定义 pump）。
//
// energye/lcl 的 TTimer 在应用消息泵上触发（主线程，无 goroutine/marshalling，
// 契合 D4 单一 UI 线程）。intervalMs 毫秒后每周期触发 fn，直至 stop 被调用
// （幂等：禁用 + Free）。关闭流程保护：窗体 teardown 后不再触发（closed 门，
// 与 RunOnUI 同策略，杜绝关机竞态）。
func (r *Renderer) NewTimer(intervalMs int, fn func()) (stop func()) {
	t := lcl.NewTimer(r.form)
	t.SetInterval(uint32(intervalMs))
	t.SetEnabled(true)
	t.SetOnTimer(func(_ lcl.IObject) {
		if r.closed.Load() || fn == nil {
			return
		}
		render.Guard("timer", fn)
	})
	var once sync.Once
	return func() {
		once.Do(func() {
			t.SetEnabled(false)
			t.Free()
		})
	}
}

// colorToTColor 把 ARGB（0xAARRGGBB）换算为 LCL TColor（$00BBGGRR，低 24 位
// BGR）。alpha 忽略（原生控件无透明合成）。独立纯函数便于无 DLL 单测
// （见 mapping_test.go）。
func colorToTColor(c render.Color) types.TColor {
	r := byte(c >> 16)
	g := byte(c >> 8)
	b := byte(c)
	return types.TColor(uint32(b)<<16 | uint32(g)<<8 | uint32(r))
}

// TextExtent 用共享 bitmap canvas 做 GDI 文本测量（design.md §6.2）。
//
// 布局在 diff 之前执行、控件未创建，因此测量不依赖控件句柄：分配一个
// 1x1 bitmap（得到有效 HDC）＋窗体默认字体（LCL 子控件默认继承窗体字体，
// 保证与真实渲染尺寸一致），调 TextExtentWithStr。结果按 text 缓存。
//
// Phase 3.5：bitmap DC 的 DPI（canvasDpi，GetDeviceCaps）进程内固定、显示器无关，
// 把测量到的物理像素经 PXToDIP 归一化为 DIP —— 因此测量结果是显示器无关的，
// measureCache 无需随显示器 DPI 变化而失效；只有字体点号随 DPI 变化时才失效
// （WM_DPICHANGED 钩子统一清缓存）。
func (r *Renderer) TextExtent(text string) (int, int) {
	if s, ok := r.measureCache[text]; ok {
		return int(s[0]), int(s[1])
	}
	if r.measureBmp == nil {
		bmp := lcl.NewBitmap()
		bmp.SetSize(1, 1) // 分配 DIB 段 → canvas HDC 有效
		bmp.Canvas().SetFontToFont(r.form.Font())
		r.measureBmp = bmp
	}
	sz := r.measureBmp.Canvas().TextExtentWithStr(text)
	w, h := int(sz.Cx), int(sz.Cy)
	if w <= 0 {
		w = len(text) * 8 // 兜底（空文本/极端字体）
	}
	if h <= 0 {
		h = 20
	}
	cdpi := int(r.canvasDPI())
	w, h = render.PXToDIP(w, cdpi), render.PXToDIP(h, cdpi)
	r.measureCache[text] = [2]int32{int32(w), int32(h)}
	return w, h
}

// ClientSize 返回窗体客户区尺寸（DIP）。子控件坐标以此为布局坐标系。
func (r *Renderer) ClientSize() (int, int) {
	dpi := int(r.currentDPI())
	return render.PXToDIP(int(r.form.ClientWidth()), dpi),
		render.PXToDIP(int(r.form.ClientHeight()), dpi)
}

// DPI 返回当前窗体所在显示器的 DPI（demo 读数/调试用）。
func (r *Renderer) DPI() int {
	return int(r.currentDPI())
}

// OnResize 注册窗体 resize 回调。回调在 UI 线程触发（消息泵投递），
// 参数为新客户区尺寸（DIP）。幂等：重复调用覆盖。
// 窗体 resize 与 WM_DPICHANGED 都经 emitResize 统一触发。
func (r *Renderer) OnResize(fn func(w, h int)) {
	r.resizeFn = fn
	r.form.SetOnResize(func(_ lcl.IObject) {
		r.emitResize()
	})
}

// emitResize 统一触发 resize 回调：读当前 ClientSize（DIP）→ 回调。
// 窗体进入关闭流程后丢弃（teardown 期间 ClientSize 可能已失效）。
func (r *Renderer) emitResize() {
	if r.closed.Load() || r.resizeFn == nil {
		return
	}
	w, h := r.ClientSize()
	render.Guard("resize", func() { r.resizeFn(w, h) })
}

// mouseEvents / keyEvents 是 LCL 鼠标/键盘事件的结构化接口。energye/lcl 的
// 具体控件（TButton/TLabel/TEdit/TScrollBox/TEngForm）各自实现这些方法，但
// IControl 接口只声明 OnClick —— 这里用结构化断言换取窄接口内的多态访问
// （D6：不引入对具体控件类型的依赖）。TLabel 无 HWND（非 TWinControl），
// 断言 keyEvents 失败 → 键盘事件 panic（可读错误）。
type mouseEvents interface {
	SetOnMouseDown(fn lcl.TMouseEvent)
	SetOnMouseUp(fn lcl.TMouseEvent)
	SetOnMouseMove(fn lcl.TMouseMoveEvent)
	SetOnMouseEnter(fn lcl.TNotifyEvent)
	SetOnMouseLeave(fn lcl.TNotifyEvent)
}

type keyEvents interface {
	SetOnKeyDown(fn lcl.TKeyEvent)
	SetOnKeyUp(fn lcl.TKeyEvent)
	SetOnKeyPress(fn lcl.TKeyPressEvent)
	SetOnUTF8KeyPress(fn lcl.TUTF8KeyPressEvent)
}

// checkableControl 是 TCheckBox/TRadioButton 暴露的最小布尔选择能力。
// 使用结构化接口让 Renderer 保持对具体 LCL 控件类型的低耦合（D6）。
type checkableControl interface {
	Checked() bool
	SetChecked(bool)
	SetOnChange(lcl.TNotifyEvent)
}

// progressControl 是 TProgressBar 暴露的最小范围和值能力。
type progressControl interface {
	Min() int32
	SetMin(int32)
	Max() int32
	SetMax(int32)
	SetPosition(int32)
}

// SetChecked 设置 CheckBox 等可选控件的受控选中状态。RadioButton 在尚未解析
// native parent 时只缓存值，待 SetParent 完成 host 挂载后才下发。同一逻辑父级和
// GroupIndex 的 RadioButton 在此处保持互斥；每个 radio 独立 host 则隔离 LCL 原生
// 的“同 parent 全部互斥”规则，且不会吞没相邻 sibling 的命中区域。
func (r *Renderer) SetChecked(h render.Handle, checked bool) {
	if radio := r.radios[h]; radio != nil {
		radio.checked = checked
		if radio.host == nil {
			return
		}
		if checked {
			for peerHandle, peer := range r.radios {
				if peerHandle == h || peer.host == nil || peer.parent != radio.parent || peer.group != radio.group || !peer.checked {
					continue
				}
				peer.checked = false
				r.applyRadioChecked(peerHandle, false)
			}
		}
		r.applyRadioChecked(h, checked)
		return
	}
	c, ok := r.controls[h].(checkableControl)
	if !ok {
		panic(fmt.Sprintf("native: 控件 %d 不支持 Checked", h))
	}
	c.SetChecked(checked)
}

// OnCheckedChange 绑定 CheckBox 等可选控件的布尔状态变化；nil 清除绑定。
func (r *Renderer) OnCheckedChange(h render.Handle, fn func(bool)) {
	c, ok := r.controls[h].(checkableControl)
	if !ok {
		panic(fmt.Sprintf("native: 控件 %d 不支持 OnCheckedChange", h))
	}
	if fn == nil {
		c.SetOnChange(nil)
		return
	}
	c.SetOnChange(func(_ lcl.IObject) {
		radio := r.radios[h]
		if radio != nil && radio.applying {
			return
		}
		// LCL 只会在同一真实 parent 内互斥。RadioButton 使用独立 host 后，
		// 用户点击也必须回到 Flux 的逻辑组规则，再通知受控状态所有者。
		if radio != nil {
			radio.checked = c.Checked()
			if radio.checked {
				r.SetChecked(h, true)
			}
		}
		render.Guard("event.OnCheckedChange", func() { fn(c.Checked()) })
	})
}

// SetGroupIndex 设置 RadioButton 的 Flux 逻辑组编号。互斥状态由本 Renderer 按
// resolved native parent + GroupIndex 维护；内部的逐控件 Panel 只隔离
// energye/lcl v1.0.3 的同 parent 原生互斥，不能替代逻辑分组元数据。
func (r *Renderer) SetGroupIndex(h render.Handle, groupIndex int) {
	radio := r.radios[h]
	if radio == nil {
		return
	}
	if radio.group == groupIndex {
		return
	}
	radio.group = groupIndex
	if radio.checked {
		r.SetChecked(h, true)
	}
}

// SetMinimum 设置 ProgressBar 的最小值。
func (r *Renderer) SetMinimum(h render.Handle, minimum int) {
	c, ok := r.controls[h].(progressControl)
	if !ok {
		return
	}
	r.withSliderApplying(h, func() {
		// Win32 clamps a new minimum to the current maximum. Expand the upper
		// bound first when a controlled range moves wholly above the old range;
		// the ordered Maximum patch immediately applies the final upper bound.
		if value := int32(minimum); value > c.Max() {
			c.SetMax(value)
		}
		c.SetMin(int32(minimum))
	})
}

// SetMaximum 设置 ProgressBar 的最大值。
func (r *Renderer) SetMaximum(h render.Handle, maximum int) {
	c, ok := r.controls[h].(progressControl)
	if !ok {
		return
	}
	r.withSliderApplying(h, func() { c.SetMax(int32(maximum)) })
}

// SetValue 设置 ProgressBar 的当前位置。
func (r *Renderer) SetValue(h render.Handle, value int) {
	c, ok := r.controls[h].(progressControl)
	if !ok {
		return
	}
	r.withSliderApplying(h, func() { c.SetPosition(int32(value)) })
}

func (r *Renderer) withSliderApplying(h render.Handle, fn func()) {
	state := r.sliders[h]
	if state == nil {
		fn()
		return
	}
	previous := state.applying
	state.applying = true
	defer func() { state.applying = previous }()
	fn()
}

// SetSliderStep 设置 TrackBar 的方向键/行步长。鼠标拖动仍可产生范围内任意整数。
func (r *Renderer) SetSliderStep(h render.Handle, step int) {
	track, ok := r.controls[h].(lcl.ITrackBar)
	if !ok {
		return
	}
	track.SetLineSize(int32(step))
}

// OnSliderValueChange 绑定真实用户 TrackBar 变化；nil 清除绑定。程序化属性
// 应用由 sliderState.applying 屏蔽，避免受控值回写形成事件环。
func (r *Renderer) OnSliderValueChange(h render.Handle, fn func(int)) {
	track, ok := r.controls[h].(lcl.ITrackBar)
	state := r.sliders[h]
	if !ok || state == nil {
		return
	}
	state.onChange = fn
	if fn == nil {
		track.SetOnChange(nil)
		return
	}
	track.SetOnChange(func(_ lcl.IObject) {
		if state.applying || state.onChange == nil {
			return
		}
		value := int(track.Position())
		render.Guard("event.OnValueChange", func() { state.onChange(value) })
	})
}

func (r *Renderer) withGridApplying(grid *gridState, fn func()) {
	if grid == nil || fn == nil {
		return
	}
	previous := grid.applying
	grid.applying = true
	defer func() { grid.applying = previous }()
	fn()
}

func gridFixedRows(grid *gridState) int {
	if grid != nil && len(grid.headers) > 0 {
		return 1
	}
	return 0
}

func emptyNativeGridCells(size render.GridSize) [][]string {
	values := make([][]string, size.Rows)
	for row := range values {
		values[row] = make([]string, size.Columns)
	}
	return values
}

func defaultGridSelection(size render.GridSize) render.GridSelection {
	selection := render.GridSelection{Cell: render.GridCell{Row: -1, Column: -1}}
	if size.Rows > 0 {
		selection.Cell = render.GridCell{}
	}
	return selection
}

func discardGridEdit(grid *gridState) {
	if grid != nil {
		grid.pending = nil
	}
}

func (r *Renderer) applyGridShape(grid *gridState) {
	fixedRows := gridFixedRows(grid)
	physicalRows := grid.size.Rows + fixedRows
	if physicalRows < 1 {
		physicalRows = 1
	}
	// FixedRows 不能大于 RowCount。缩小时先放开固定行，扩张时先建立物理行。
	if int(grid.control.FixedRows()) > fixedRows {
		grid.control.SetFixedRows(int32(fixedRows))
	}
	grid.control.SetColCount(int32(grid.size.Columns))
	grid.control.SetRowCount(int32(physicalRows))
	grid.control.SetFixedRows(int32(fixedRows))
}

func (r *Renderer) applyGridWidths(grid *gridState) {
	dpi := int(r.currentDPI())
	for column := 0; column < grid.size.Columns; column++ {
		width := 96
		if len(grid.widths) == grid.size.Columns {
			width = grid.widths[column]
		}
		grid.control.SetColWidths(int32(column), int32(render.DIPToPX(width, dpi)))
	}
}

func (r *Renderer) applyGridContents(grid *gridState) {
	fixedRows := gridFixedRows(grid)
	if fixedRows == 1 {
		for column, value := range grid.headers {
			grid.control.SetCells(int32(column), 0, value)
		}
	}
	for row := 0; row < grid.size.Rows; row++ {
		for column := 0; column < grid.size.Columns; column++ {
			grid.control.SetCells(int32(column), int32(row+fixedRows), grid.cells[row][column])
		}
	}
	if grid.size.Rows == 0 && fixedRows == 0 {
		for column := 0; column < grid.size.Columns; column++ {
			grid.control.SetCells(int32(column), 0, "")
		}
	}
}

func (r *Renderer) applyGridEditable(grid *gridState) {
	options := grid.control.Options()
	if grid.editable {
		options = options.Include(int32(types.GoEditing))
	} else {
		options = options.Exclude(int32(types.GoEditing))
	}
	grid.control.SetOptions(options)
	grid.control.SetAutoEdit(grid.editable)
}

func (r *Renderer) applyGridSelection(grid *gridState) {
	options := grid.control.Options()
	if grid.selection.RowOnly {
		options = options.Include(int32(types.GoRowSelect))
	} else {
		options = options.Exclude(int32(types.GoRowSelect))
	}
	grid.control.SetOptions(options)
	if grid.size.Rows == 0 {
		return
	}
	grid.control.SetColRow(types.Point(
		int32(grid.selection.Cell.Column),
		int32(grid.selection.Cell.Row+gridFixedRows(grid)),
	))
}

// SetGridSize 设置 StringGrid 的逻辑行列数，并在原生边界折算可选表头行。
func (r *Renderer) SetGridSize(h render.Handle, size render.GridSize) {
	grid := r.grids[h]
	if grid == nil {
		return
	}
	if size.Rows < 0 || size.Columns <= 0 {
		panic(fmt.Sprintf("native: invalid GridSize for control %d: %+v", h, size))
	}
	discardGridEdit(grid)
	grid.size = size
	grid.cells = emptyNativeGridCells(size)
	if len(grid.headers) != 0 && len(grid.headers) != size.Columns {
		grid.headers = []string{}
	}
	if len(grid.widths) != 0 && len(grid.widths) != size.Columns {
		grid.widths = []int{}
	}
	if !render.ValidGridSelection(size, grid.selection) {
		grid.selection = defaultGridSelection(size)
	}
	r.withGridApplying(grid, func() {
		r.applyGridShape(grid)
		r.applyGridWidths(grid)
		r.applyGridContents(grid)
		r.applyGridEditable(grid)
		r.applyGridSelection(grid)
	})
}

// SetGridHeaders 设置可选表头；表头不计入 GridSize.Rows。
func (r *Renderer) SetGridHeaders(h render.Handle, headers []string) {
	grid := r.grids[h]
	if grid == nil {
		return
	}
	if len(headers) != 0 && len(headers) != grid.size.Columns {
		panic(fmt.Sprintf("native: Headers length %d does not match %d columns", len(headers), grid.size.Columns))
	}
	discardGridEdit(grid)
	grid.headers = append([]string(nil), headers...)
	if len(grid.headers) == 0 {
		grid.headers = []string{}
	}
	r.withGridApplying(grid, func() {
		r.applyGridShape(grid)
		r.applyGridContents(grid)
		r.applyGridSelection(grid)
	})
}

// SetGridColumnWidths 设置每列的 DIP 宽度；空 slice 使用 96 DIP 默认值。
func (r *Renderer) SetGridColumnWidths(h render.Handle, widths []int) {
	grid := r.grids[h]
	if grid == nil {
		return
	}
	if len(widths) != 0 && len(widths) != grid.size.Columns {
		panic(fmt.Sprintf("native: ColumnWidths length %d does not match %d columns", len(widths), grid.size.Columns))
	}
	for _, width := range widths {
		if width <= 0 {
			panic("native: ColumnWidths must be > 0")
		}
	}
	grid.widths = append([]int(nil), widths...)
	if len(grid.widths) == 0 {
		grid.widths = []int{}
	}
	r.withGridApplying(grid, func() { r.applyGridWidths(grid) })
}

// SetGridCells 替换受控字符串矩阵；输入会在 native 边界再次深复制。
func (r *Renderer) SetGridCells(h render.Handle, cells [][]string) {
	grid := r.grids[h]
	if grid == nil {
		return
	}
	if err := render.ValidateGridCells(grid.size, cells); err != nil {
		panic(fmt.Sprintf("native: invalid Cells for control %d: %v", h, err))
	}
	discardGridEdit(grid)
	grid.cells = render.CloneGridCells(cells)
	r.withGridApplying(grid, func() { r.applyGridContents(grid) })
}

// SetGridEditable 设置是否启用 TStringGrid 原生编辑器。
func (r *Renderer) SetGridEditable(h render.Handle, editable bool) {
	grid := r.grids[h]
	if grid == nil {
		return
	}
	discardGridEdit(grid)
	grid.editable = editable
	r.withGridApplying(grid, func() { r.applyGridEditable(grid) })
}

// SetGridSelection 设置受控逻辑选择，并切换单元格或整行模式。
func (r *Renderer) SetGridSelection(h render.Handle, selection render.GridSelection) {
	grid := r.grids[h]
	if grid == nil {
		return
	}
	if !render.ValidGridSelection(grid.size, selection) {
		panic(fmt.Sprintf("native: invalid GridSelection for control %d: %+v", h, selection))
	}
	discardGridEdit(grid)
	grid.selection = selection
	r.withGridApplying(grid, func() { r.applyGridSelection(grid) })
}

func (r *Renderer) emitGridSelection(grid *gridState) {
	if r.closed.Load() || grid == nil || grid.applying || grid.onSelect == nil {
		return
	}
	cell := render.GridCell{
		Row:    int(grid.control.Row()) - gridFixedRows(grid),
		Column: int(grid.control.Col()),
	}
	if !render.ValidGridSelection(grid.size, render.GridSelection{Cell: cell}) ||
		cell == grid.selection.Cell {
		return
	}
	grid.selection.Cell = cell
	render.Guard("event.OnCellSelect", func() { grid.onSelect(cell) })
}

func (r *Renderer) ensureGridSelectionPoller() {
	if r.gridPollStop != nil {
		return
	}
	if r.gridPollTimer == nil {
		timer := lcl.NewTimer(r.form)
		timer.SetEnabled(false)
		timer.SetInterval(16)
		timer.SetOnTimer(func(_ lcl.IObject) {
			if r.closed.Load() {
				return
			}
			render.Guard("grid.selectionPoll", func() {
				handles := make([]render.Handle, 0, len(r.grids))
				for handle, grid := range r.grids {
					if grid != nil && grid.onSelect != nil {
						handles = append(handles, handle)
					}
				}
				for _, handle := range handles {
					if grid := r.grids[handle]; grid != nil {
						r.emitGridSelection(grid)
					}
				}
			})
		})
		r.gridPollTimer = timer
	}
	r.gridPollTimer.SetEnabled(true)
	stopped := false
	r.gridPollStop = func() {
		if stopped {
			return
		}
		stopped = true
		if r.gridPollTimer != nil {
			r.gridPollTimer.SetEnabled(false)
		}
	}
}

func (r *Renderer) hasGridSelectionSubscriber() bool {
	for _, grid := range r.grids {
		if grid != nil && grid.onSelect != nil {
			return true
		}
	}
	return false
}

func (r *Renderer) stopGridSelectionPollerIfIdle() {
	if !r.hasGridSelectionSubscriber() {
		r.stopGridSelectionPoller()
	}
}

func (r *Renderer) releaseGridSelectionPoller() {
	r.stopGridSelectionPoller()
	if r.gridPollTimer != nil {
		// TTimer 由 form 持有并随 form teardown 释放。这里只解除 Go 回调，
		// 避免 OnClose 从 timer 用户回调重入时在当前派发栈内 Free。
		r.gridPollTimer.SetOnTimer(nil)
		r.gridPollTimer = nil
	}
}

func (r *Renderer) stopGridSelectionPoller() {
	if r.gridPollStop == nil {
		return
	}
	stop := r.gridPollStop
	r.gridPollStop = nil
	stop()
}

// OnGridCellSelect 绑定逻辑单元格选择；nil 会从 TStringGrid 解除事件。
func (r *Renderer) OnGridCellSelect(h render.Handle, fn func(render.GridCell)) {
	grid := r.grids[h]
	if grid == nil {
		return
	}
	grid.onSelect = fn
	if fn == nil {
		grid.control.SetOnAfterSelection(nil)
		r.stopGridSelectionPollerIfIdle()
		return
	}
	r.ensureGridSelectionPoller()
	// LCL 的 OnAfterSelection 参数是移动前的坐标；提交后的焦点单元格必须从
	// 控件读取。锁定 DLL 不会为真实鼠标/键盘稳定桥接该事件，Renderer 级单一
	// 主线程轮询器会经过同一个去重出口，且不占用普通 Click/Mouse/Key 事件。
	// 空闲时只禁用、重绑时复用同一 TTimer：用户回调可能同步触发 render 并在
	// 当前 TTimer 回调栈中解绑/销毁 Grid，因此这里不能 Free 正在派发的对象。
	grid.control.SetOnAfterSelection(func(_ lcl.IObject, _, _ int32) {
		r.emitGridSelection(grid)
	})
}

// OnGridCellEdit 绑定原生编辑提交；nil 会解除输入和提交两个事件。
func (r *Renderer) OnGridCellEdit(h render.Handle, fn func(render.GridCell, string)) {
	grid := r.grids[h]
	if grid == nil {
		return
	}
	grid.onEdit = fn
	grid.pending = nil
	if fn == nil {
		grid.control.SetOnSetEditText(nil)
		grid.control.SetOnEditingDone(nil)
		return
	}
	grid.control.SetOnSetEditText(func(_ lcl.IObject, column, row int32, value string) {
		if grid.applying || grid.onEdit == nil {
			return
		}
		cell := render.GridCell{Row: int(row) - gridFixedRows(grid), Column: int(column)}
		if !render.ValidGridSelection(grid.size, render.GridSelection{Cell: cell}) {
			return
		}
		grid.pending = &gridEdit{cell: cell, value: value}
	})
	grid.control.SetOnEditingDone(func(_ lcl.IObject) {
		if grid.applying || grid.onEdit == nil || grid.pending == nil {
			return
		}
		edit := *grid.pending
		grid.pending = nil
		render.Guard("event.OnCellEdit", func() { grid.onEdit(edit.cell, edit.value) })
	})
}

// comboBoxControl 是 TComboBox 暴露的最小选择能力。
type comboBoxControl interface {
	Items() lcl.IStrings
	ItemIndex() int32
	SetItemIndex(int32)
	SetOnSelect(lcl.TNotifyEvent)
}

// SetItems 替换 ComboBox 的全部字符串选项，并保留合法的当前选中索引。
func (r *Renderer) SetItems(h render.Handle, values []string) {
	combo, ok := r.controls[h].(comboBoxControl)
	if !ok {
		return
	}
	items := combo.Items()
	items.Clear()
	for _, value := range values {
		items.Add(value)
	}
	combo.SetItemIndex(int32(normalizeComboIndex(len(values), int(combo.ItemIndex()))))
}

func normalizePageIndex(count, index int) int {
	if count == 0 {
		return -1
	}
	if index < -1 {
		return -1
	}
	if index >= count {
		return count - 1
	}
	return index
}

func samePageControl(left, right lcl.IPageControl) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return left.Instance() == right.Instance()
}

// setSheetPageControl is the only path that changes a TabSheet's PageControl.
// LCL can synchronously notify both the old and new PageControl, so both states
// must suppress OnChange while the structural mutation is in progress. The
// identity guard also keeps an unchanged attachment mutation-free.
func (r *Renderer) setSheetPageControl(sheet lcl.ITabSheet, page lcl.IPageControl) {
	previous := sheet.PageControl()
	if samePageControl(previous, page) {
		return
	}
	r.withPageApplying(previous, func() {
		r.withPageApplying(page, func() { sheet.SetPageControl(page) })
	})
}

// withPageApplying 在 LCL 页面集合发生结构性变化时抑制 OnChange。不同 widgetset
// 对 SetPageControl/SetPageIndex 是否同步通知的行为并不一致，统一在适配层兜底，
// 让 diff 只把真正的用户选择交给 OnPageSelectionChange。
func (r *Renderer) withPageApplying(page lcl.IPageControl, fn func()) {
	if fn == nil {
		return
	}
	if page == nil {
		fn()
		return
	}
	var state *pageState
	for handle, candidate := range r.controls {
		if candidate != nil && candidate.Instance() == page.Instance() {
			state = r.pages[handle]
			break
		}
	}
	if state == nil {
		fn()
		return
	}
	previous := state.applying
	state.applying = true
	defer func() { state.applying = previous }()
	fn()
}

// SyncPages 把 keyed reconciliation 得到的 TabPage 句柄顺序原地同步到 LCL，
// 再应用缓存的受控索引。SetPageIndex 可能同步触发 Change，因此整个过程抑制回调。
func (r *Renderer) SyncPages(parent render.Handle, pages []render.Handle) {
	state := r.pages[parent]
	page, ok := r.controls[parent].(lcl.IPageControl)
	if state == nil || !ok {
		panic(fmt.Sprintf("native: 分页父控件 %d 非 IPageControl", parent))
	}
	sheets := make([]lcl.ITabSheet, len(pages))
	seen := make(map[render.Handle]struct{}, len(pages))
	for index, handle := range pages {
		sheet, ok := r.controls[handle].(lcl.ITabSheet)
		if !ok {
			panic(fmt.Sprintf("native: PageControl 子控件 %d 非 ITabSheet", handle))
		}
		if _, exists := seen[handle]; exists {
			panic(fmt.Sprintf("native: PageControl 页面句柄 %d 重复", handle))
		}
		seen[handle] = struct{}{}
		sheets[index] = sheet
	}
	r.withPageApplying(page, func() {
		for index, sheet := range sheets {
			r.setSheetPageControl(sheet, page)
			if sheet.PageIndex() != int32(index) {
				sheet.SetPageIndex(int32(index))
			}
		}
		state.pages = append(state.pages[:0], pages...)
		index := normalizePageIndex(len(state.pages), state.selected)
		if page.ActivePageIndex() != int32(index) {
			page.SetActivePageIndex(int32(index))
		}
	})
}

// SetPageSelectedIndex 缓存并应用 PageControl 的受控选择。默认 LCL Options 不会
// 因程序化 SetActivePageIndex 派发 OnChange；applying 仍作为后端差异的防线。
func (r *Renderer) SetPageSelectedIndex(parent render.Handle, index int) {
	state := r.pages[parent]
	page, ok := r.controls[parent].(lcl.IPageControl)
	if state == nil || !ok {
		return
	}
	state.selected = index
	index = normalizePageIndex(len(state.pages), index)
	r.withPageApplying(page, func() {
		if page.ActivePageIndex() != int32(index) {
			page.SetActivePageIndex(int32(index))
		}
	})
}

// OnPageSelectionChange 绑定 PageControl 的用户选择事件；nil 清除绑定。
func (r *Renderer) OnPageSelectionChange(parent render.Handle, fn func(int)) {
	state := r.pages[parent]
	page, ok := r.controls[parent].(lcl.IPageControl)
	if state == nil || !ok {
		return
	}
	state.onSelect = fn
	if fn == nil {
		page.SetOnChange(nil)
		return
	}
	page.SetOnChange(func(_ lcl.IObject) {
		if state.applying || state.onSelect == nil {
			return
		}
		index := int(page.ActivePageIndex())
		render.Guard("event.OnPageSelectionChange", func() { state.onSelect(index) })
	})
}

// SetSelectedIndex 设置 ComboBox 的受控选中索引。TComboBox 自己维护 items，
// 因此只需以当前 Items.Count 为边界规范化。
func (r *Renderer) SetSelectedIndex(h render.Handle, index int) {
	combo, ok := r.controls[h].(comboBoxControl)
	if !ok {
		return
	}
	combo.SetItemIndex(int32(normalizeComboIndex(int(combo.Items().Count()), index)))
}

// OnSelectionChange 绑定 ComboBox 的 OnSelect；nil 清除绑定。
func (r *Renderer) OnSelectionChange(h render.Handle, fn func(int)) {
	combo, ok := r.controls[h].(comboBoxControl)
	if !ok {
		return
	}
	if fn == nil {
		combo.SetOnSelect(nil)
		return
	}
	combo.SetOnSelect(func(_ lcl.IObject) {
		render.Guard("event.OnSelectionChange", func() { fn(int(combo.ItemIndex())) })
	})
}

func normalizeComboIndex(count, index int) int {
	if count == 0 {
		return -1
	}
	if index < -1 {
		return -1
	}
	if index >= count {
		return count - 1
	}
	return index
}

// SetEvent 把统一事件回调（func(render.Event)）映射到 LCL 事件（Phase 4.2）。
//
// 坐标经 DIP 归一：LCL 鼠标回调的 X/Y 是物理像素（相对控件客户区），
// 用 render.PXToDIP 换算为 DIP（D5 全坐标 DIP）。Source 由 diff 引擎包装注入，
// 这里只组装事件负载。OnChange 保持 func(string)（Input 双向绑定路径）。
//
// fn 为 nil 时清除该事件绑定（D2 对称：diff 在属性移除时解绑，见 applyRemoved）。
// 所有用户回调包 D4 错误边界（render.Guard），回调 panic 不崩进程。
func (r *Renderer) SetEvent(h render.Handle, event string, fn any) {
	c := r.controls[h]
	if fn == nil {
		// 清除事件绑定（D2 对称：diff 在属性移除时解绑，见 internal/diff applyRemoved）。
		switch event {
		case "OnClick":
			c.SetOnClick(nil)
		case "OnMouseDown":
			c.(mouseEvents).SetOnMouseDown(nil)
		case "OnMouseUp":
			c.(mouseEvents).SetOnMouseUp(nil)
		case "OnMouseMove":
			c.(mouseEvents).SetOnMouseMove(nil)
		case "OnMouseEnter":
			c.(mouseEvents).SetOnMouseEnter(nil)
		case "OnMouseLeave":
			c.(mouseEvents).SetOnMouseLeave(nil)
		case "OnKeyDown":
			c.(keyEvents).SetOnKeyDown(nil)
		case "OnKeyUp":
			c.(keyEvents).SetOnKeyUp(nil)
		case "OnKeyPress":
			c.(keyEvents).SetOnUTF8KeyPress(nil)
		case "OnChange":
			c.(lcl.ICustomEdit).SetOnChange(nil)
		default:
			panic(fmt.Sprintf("native: 未知事件 %q", event))
		}
		return
	}
	switch event {
	case "OnClick":
		c.SetOnClick(func(_ lcl.IObject) {
			render.Guard("event.OnClick", func() {
				fn.(func(render.Event))(render.Event{Type: render.EventClick})
			})
		})
	case "OnMouseDown", "OnMouseUp":
		m, ok := c.(mouseEvents)
		if !ok {
			panic(fmt.Sprintf("native: 控件 %d 不支持鼠标事件 %q", h, event))
		}
		et := render.EventMouseDown
		if event == "OnMouseUp" {
			et = render.EventMouseUp
		}
		cb := func(_ lcl.IObject, button types.TMouseButton, shift types.TShiftState, x, y int32) {
			render.Guard("event."+event, func() {
				fn.(func(render.Event))(mouseEvent(et, button, shift, int(x), int(y), r.dpiAt()))
			})
		}
		if event == "OnMouseUp" {
			m.SetOnMouseUp(cb)
		} else {
			m.SetOnMouseDown(cb)
		}
	case "OnMouseMove":
		m, ok := c.(mouseEvents)
		if !ok {
			panic(fmt.Sprintf("native: 控件 %d 不支持鼠标事件 %q", h, event))
		}
		m.SetOnMouseMove(func(_ lcl.IObject, shift types.TShiftState, x, y int32) {
			render.Guard("event.OnMouseMove", func() {
				fn.(func(render.Event))(render.Event{
					Type: render.EventMouseMove,
					X:    render.PXToDIP(int(x), r.dpiAt()),
					Y:    render.PXToDIP(int(y), r.dpiAt()),
					Mods: mapShift(shift),
				})
			})
		})
	case "OnMouseEnter", "OnMouseLeave":
		m, ok := c.(mouseEvents)
		if !ok {
			panic(fmt.Sprintf("native: 控件 %d 不支持鼠标事件 %q", h, event))
		}
		et := render.EventMouseEnter
		if event == "OnMouseLeave" {
			et = render.EventMouseLeave
		}
		cb := func(_ lcl.IObject) {
			render.Guard("event."+event, func() {
				fn.(func(render.Event))(render.Event{Type: et})
			})
		}
		if event == "OnMouseLeave" {
			m.SetOnMouseLeave(cb)
		} else {
			m.SetOnMouseEnter(cb)
		}
	case "OnKeyDown", "OnKeyUp":
		k, ok := c.(keyEvents)
		if !ok {
			panic(fmt.Sprintf("native: 控件 %d 不支持键盘事件 %q（无 HWND 的控件无键盘焦点）", h, event))
		}
		et := render.EventKeyDown
		if event == "OnKeyUp" {
			et = render.EventKeyUp
		}
		cb := func(_ lcl.IObject, key *uint16, shift types.TShiftState) {
			render.Guard("event."+event, func() {
				fn.(func(render.Event))(render.Event{Type: et, Key: *key, Mods: mapShift(shift)})
			})
		}
		if event == "OnKeyUp" {
			k.SetOnKeyUp(cb)
		} else {
			k.SetOnKeyDown(cb)
		}
	case "OnKeyPress":
		// 4.4 IME/中文输入：走 SetOnUTF8KeyPress（energye/lcl v1.0.3 在
		// TWinControl 上可用，含 IME 组合结果；不依赖计划的"仅 TForm"担忧）。
		k, ok := c.(keyEvents)
		if !ok {
			panic(fmt.Sprintf("native: 控件 %d 不支持 OnKeyPress", h))
		}
		k.SetOnUTF8KeyPress(func(_ lcl.IObject, s *string) {
			render.Guard("event.OnKeyPress", func() {
				fn.(func(render.Event))(render.Event{Type: render.EventKeyPress, Text: *s})
			})
		})
	case "OnChange":
		ed, ok := c.(lcl.ICustomEdit)
		if !ok {
			panic(fmt.Sprintf("native: 控件 %d 不支持 OnChange", h))
		}
		ed.SetOnChange(func(_ lcl.IObject) {
			if state := r.texts[h]; state != nil && state.applying {
				return
			}
			render.Guard("event.OnChange", func() { fn.(func(string))(ed.Text()) })
		})
	default:
		panic(fmt.Sprintf("native: 未知事件 %q", event))
	}
}

// dpiAt 返回事件构造时使用的 DPI（等价 currentDPI，语义别名：事件坐标换算用）。
func (r *Renderer) dpiAt() int {
	return int(r.currentDPI())
}

// mouseEvent 组装鼠标按下/释放事件负载（DIP 坐标 + 按键 + 修饰键）。
// 独立纯函数便于无 DLL 单测（见 mapping_test.go）。
func mouseEvent(et render.EventType, button types.TMouseButton, shift types.TShiftState, x, y, dpi int) render.Event {
	return render.Event{
		Type:   et,
		X:      render.PXToDIP(x, dpi),
		Y:      render.PXToDIP(y, dpi),
		Button: mapButton(button),
		Mods:   mapShift(shift),
	}
}

// mapButton 把 LCL TMouseButton 映射到统一 MouseButton。
func mapButton(b types.TMouseButton) render.MouseButton {
	switch b {
	case types.MbLeft:
		return render.ButtonLeft
	case types.MbRight:
		return render.ButtonRight
	case types.MbMiddle:
		return render.ButtonMiddle
	default:
		return render.ButtonNone
	}
}

// mapShift 把 LCL TShiftState（位集合）映射到统一 Modifier 掩码。
func mapShift(s types.TShiftState) render.Modifier {
	var m render.Modifier
	if s.In(types.SsShift) {
		m |= render.ModShift
	}
	if s.In(types.SsCtrl) {
		m |= render.ModCtrl
	}
	if s.In(types.SsAlt) {
		m |= render.ModAlt
	}
	if s.In(types.SsMeta) || s.In(types.SsSuper) {
		m |= render.ModWin
	}
	return m
}

func (r *Renderer) AttachRef(h render.Handle, ref render.Ref) {
	ref.SetNative(r.controls[h])
}

func (r *Renderer) ApplyNative(h render.Handle, fn func(obj any)) {
	fn(r.controls[h])
}

// RunOnUI 把 fn marshal 到 UI 线程执行（D4 marshalling）。
// 已在主线程（事件回调内）则直接执行；否则经 lcl.RunOnMainThreadSync 阻塞
// 等待主线程消费 —— State 从任意 goroutine 触发 re-render 的规范路径。
//
// 关机竞态防护（Phase 3.6）：窗体进入关闭流程后（OnClose 置 closed），后台
// goroutine 的新任务直接丢弃，避免 RunOnMainThreadSync 与窗体 teardown 竞争；
// 已在主线程的关闭回调仍可同步执行 App.Close，完成 Element/plugin 清理且不产生
// 新的跨线程 DLL sync 调用。
func (r *Renderer) RunOnUI(fn func()) {
	if api.CurrentThreadId() == api.MainThreadId() {
		render.Guard("RunOnUI", fn)
		return
	}
	if r.closed.Load() {
		return
	}
	lcl.RunOnMainThreadSync(func() { render.Guard("RunOnUI", fn) })
}

// PluginCapabilitySnapshot 在插件回调开始前于 UI 线程捕获 LCL 后端的只读能力快照。
// 返回值不得含 Renderer、原生对象、句柄或其他可变后端状态。
func (r *Renderer) PluginCapabilitySnapshot() map[string]any {
	return map[string]any{
		"flux.renderer.dpi":     r.DPI(),
		"flux.renderer.backend": "lcl",
	}
}

// OnClose 注册窗体关闭回调，并置 closed 门（此后后台 RunOnUI/invalidate 丢弃；
// 当前主线程回调仍可执行 App.Close 清理）。
// fn 在窗体销毁前于主线程触发 —— demo 用它停止后台轮询 goroutine，双保险。
func (r *Renderer) OnClose(fn func()) {
	r.closeFn = fn
	r.formRef.SetOnClose(func(_ lcl.IObject, _ *types.TCloseAction) {
		r.closed.Store(true)
		r.releaseGridSelectionPoller()
		if fn != nil {
			render.Guard("OnClose", fn)
		}
	})
}

func (r *Renderer) HandleAllocated(h render.Handle) bool {
	_, ok := r.controls[h]
	return ok
}

// InspectNative 返回 Inspector 使用的全部原生类型、逻辑父级与分配状态。
// ClassName 只读，不创建 HWND，也不改变被检查控件。
func (r *Renderer) InspectNative() render.NativeSnapshot {
	out := make(render.NativeSnapshot, len(r.controls))
	for h, c := range r.controls {
		if c == nil {
			continue
		}
		info := render.NativeInfo{Type: c.ClassName(), Allocated: true}
		if radio := r.radios[h]; radio != nil {
			info.Parent = radio.parent
		} else if parent := c.Parent(); parent != nil {
			for parentHandle, candidate := range r.controls {
				if candidate != nil && candidate.Instance() == parent.Instance() {
					info.Parent = parentHandle
					break
				}
			}
		}
		out[h] = info
	}
	return out
}

// —— Phase 6 滚动（Scrollable 实现）与多窗口 ——

// SetScrollConfig 配置 ListView 滚动（DIP）：内容总高 + 滚轮步长。
// 更新滚动条范围（内容−视口），内容 <= 视口时范围为 0（滚动条到顶，无可滚）。
func (r *Renderer) SetScrollConfig(h render.Handle, cfg render.ScrollConfig) {
	ls := r.scrolls[h]
	if ls == nil {
		return
	}
	ls.content = cfg.Content
	ls.step = cfg.Step
	r.applyBarRange(ls)
}

// SetScrollPos 设置滚动位置（DIP）。滚动条 Position 更新；pos 回写自身状态
// （applyScroll 事件同源不触发二次回调 —— LCL 相同 Position 不派发事件）。
func (r *Renderer) SetScrollPos(h render.Handle, pos int) {
	ls := r.scrolls[h]
	if ls == nil {
		return
	}
	if pos < 0 {
		pos = 0
	}
	ls.pos = pos
	if ls.bar != nil {
		ls.bar.SetPosition(int32(pos))
	}
}

// OnScroll 绑定滚动位置变化回调（DIP，UI 线程）。覆盖式注册（单回调）。
func (r *Renderer) OnScroll(h render.Handle, fn func(int)) {
	ls := r.scrolls[h]
	if ls == nil {
		return
	}
	ls.onScroll = fn
}

// Show 显示窗体（多窗口，Phase 6.3）。主窗体由 Application.Run() 自动显示；
// 次要窗体（第二个 NewRenderer → 第二个 Application.NewForms）须显式 Show 才出现。
// 幂等：已可见时 LCL 忽略；窗体进入关闭流程后丢弃。
func (r *Renderer) Show() {
	if r.closed.Load() || r.formRef == nil {
		return
	}
	r.formRef.Show()
}

func (r *Renderer) alloc() render.Handle {
	r.next++
	return r.next
}

// —— DPI 源与钩子（Phase 3.5）——

// currentDPI 返回当前显示器 DPI，fallback 链：
// GetDpiForWindow（窗口句柄已分配时，perMonitorV2 下返回真实 DPI）→
// formRef.Monitor().PixelsPerInch()（LCL 兜底）→ 96。结果缓存，DPI 变化时
// 由 invalidateDPI 清零强制重查。
func (r *Renderer) currentDPI() int32 {
	if r.dpi > 0 {
		return r.dpi
	}
	dpi := int32(96)
	if r.formRef != nil {
		if r.formRef.HandleAllocated() {
			if hwnd := r.formRef.Handle(); hwnd != 0 {
				if v, _, _ := procGetDpiForWindow.Call(uintptr(hwnd)); v != 0 {
					dpi = int32(v)
				}
			}
		}
		if dpi == 96 { // 主链路未取到（或确为 96），用 LCL monitor 兜底
			if m := r.formRef.Monitor(); m != nil {
				if p := m.PixelsPerInch(); p > 0 {
					dpi = p
				}
			}
		}
	}
	if dpi <= 0 {
		dpi = 96
	}
	r.dpi = dpi
	return dpi
}

// invalidateDPI 清零 DPI 缓存，强制下次 currentDPI 重查（WM_DPICHANGED 触发）。
func (r *Renderer) invalidateDPI() {
	r.dpi = 0
}

func (r *Renderer) refreshDPISensitiveControls() {
	for _, grid := range r.grids {
		grid := grid
		if grid != nil {
			r.withGridApplying(grid, func() { r.applyGridWidths(grid) })
		}
	}
	for _, paint := range r.paints {
		if paint != nil && paint.control != nil {
			paint.control.Invalidate()
		}
	}
}

// canvasDPI 返回测量 bitmap DC 的 DPI。bitmap DC 的 DPI 与显示器相关但进程内固定
// （不随 WM_DPICHANGED 变化），缓存一次即可。
func (r *Renderer) canvasDPI() int32 {
	if r.canvasDpi > 0 {
		return r.canvasDpi
	}
	dpi := int32(96)
	if r.measureBmp != nil {
		if hdc := r.measureBmp.Canvas().Handle(); hdc != 0 {
			if v, _, _ := procGetDeviceCaps.Call(uintptr(hdc), logPixelsX); v != 0 {
				dpi = int32(v)
			}
		}
	}
	if dpi <= 0 {
		dpi = 96
	}
	r.canvasDpi = dpi
	return dpi
}

// setupDPIHook 注册 WM_DPICHANGED 钩子。每条窗口消息先 InheritedWndProc 放行
// （保留 LCL 默认：窗体按建议矩形 resize、字体随 widgetset 缩放；不会递归 ——
// InheritedWndProc 直接走 Pascal 父类实现）。收到 WM_DPICHANGED 后：
// 清 DPI 缓存（下次边界换算用新 DPI）+ 清文本测量缓存（字体可能已变），
// 重施 Grid 的 DIP 列宽并重绘 PaintBox，最后 emitResize 触发全量 re-layout。
func (r *Renderer) setupDPIHook() {
	r.formRef.SetOnWndProc(func(msg *types.TLMessage) {
		r.formRef.InheritedWndProc(msg)
		if msg.Msg == messages.WM_DPICHANGED {
			r.invalidateDPI()
			r.measureCache = make(map[string][2]int32)
			r.refreshDPISensitiveControls()
			r.emitResize()
		}
	})
}
