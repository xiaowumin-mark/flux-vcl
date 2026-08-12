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
	measureBmp     lcl.IBitmap                   // 共享测量画布（布局在 diff 前，控件未创建）
	measureCache   map[string][2]int32           // 文本测量缓存（字体随 DPI 变化时失效）
	dpi            int32                         // 当前显示器 DPI（0=未查询，invalidateDPI 清零强制重查）
	canvasDpi      int32                         // 测量 bitmap DC 的 DPI（进程内固定，缓存一次；0=未查询）
	resizeFn       func(w, h int)                // OnResize 统一回调（窗体 resize 与 WM_DPICHANGED 共用）
	closeFn        func()                        // OnClose 回调（demo 停止后台轮询）
	closed         atomic.Bool                   // 窗体已进入关闭流程：拒绝后续 UI marshalling（关机竞态防护）
	pendingDestroy []lcl.IControl                // D4 延后销毁队列：render 完成时 DrainDestroy 统一 Free
	scrolls        map[render.Handle]*listScroll // Phase 6 ListView 滚动状态（Scrollable 实现）
	radios         map[render.Handle]*radioState // RadioButton 的逻辑分组元数据（不依赖缺失的 LCL setter）
	radioHosts     map[radioHostKey]*radioHost   // (原生父句柄, RadioButton 句柄) → 隔离用 TPanel
	pendingHosts   []*radioHost                  // 已脱离逻辑父级、待普通控件释放后销毁的内部 Panel
}

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
	c := r.controls[h]
	if c == nil || c == r.form {
		return // 主窗体不显式 Free
	}
	delete(r.controls, h)
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
	if radio := r.radios[child]; radio != nil {
		if parent == 0 {
			return // 顶层 Window；RadioButton 必须有实际 native parent。
		}
		radio.parent = parent
		r.attachRadioToHost(child, radio)
		return
	}
	if parent == 0 {
		return // 顶层（窗体）无父
	}
	cc := r.controls[child]
	pc, ok := r.controls[parent].(lcl.IWinControl)
	if !ok {
		panic(fmt.Sprintf("native: 父控件 %d 非 IWinControl", parent))
	}
	cc.SetParent(pc)
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
	SetMin(int32)
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
	c.SetMin(int32(minimum))
}

// SetMaximum 设置 ProgressBar 的最大值。
func (r *Renderer) SetMaximum(h render.Handle, maximum int) {
	c, ok := r.controls[h].(progressControl)
	if !ok {
		return
	}
	c.SetMax(int32(maximum))
}

// SetValue 设置 ProgressBar 的当前位置。
func (r *Renderer) SetValue(h render.Handle, value int) {
	c, ok := r.controls[h].(progressControl)
	if !ok {
		return
	}
	c.SetPosition(int32(value))
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
// 关机竞态防护（Phase 3.6）：窗体进入关闭流程后（OnClose 置 closed），
// 直接丢弃 —— 后台 goroutine 的 RunOnMainThreadSync 与窗体 teardown 竞争会在
// Application.Run() 内触发间歇性 0xC0000005（能量层复现：goroutine+ScrollBox）。
// 关闭后不再产生任何对 DLL 的 sync 调用，杜绝竞态窗口。
func (r *Renderer) RunOnUI(fn func()) {
	if r.closed.Load() {
		return
	}
	if api.CurrentThreadId() == api.MainThreadId() {
		render.Guard("RunOnUI", fn)
		return
	}
	lcl.RunOnMainThreadSync(func() { render.Guard("RunOnUI", fn) })
}

// OnClose 注册窗体关闭回调，并置 closed 门（此后 RunOnUI/invalidate 一律丢弃）。
// fn 在窗体销毁前于主线程触发 —— demo 用它停止后台轮询 goroutine，双保险。
func (r *Renderer) OnClose(fn func()) {
	r.closeFn = fn
	r.formRef.SetOnClose(func(_ lcl.IObject, _ *types.TCloseAction) {
		r.closed.Store(true)
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
// 清 DPI 缓存（下次边界换算用新 DPI）+ 清文本测量缓存（字体可能已变）+
// emitResize 触发全量 re-layout。
func (r *Renderer) setupDPIHook() {
	r.formRef.SetOnWndProc(func(msg *types.TLMessage) {
		r.formRef.InheritedWndProc(msg)
		if msg.Msg == messages.WM_DPICHANGED {
			r.invalidateDPI()
			r.measureCache = make(map[string][2]int32)
			r.emitResize()
		}
	})
}
