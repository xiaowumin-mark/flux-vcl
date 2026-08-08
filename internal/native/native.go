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
	"syscall"

	"github.com/energye/lcl/api"
	"github.com/energye/lcl/api/libname"
	"github.com/energye/lcl/lcl"
	"github.com/energye/lcl/types"
	"github.com/energye/lcl/types/messages"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// GetDpiForWindow / GetDeviceCaps 无 energye 封装，用 syscall 直调（项目零 CGO）。
// 均为系统自带 DLL，进程内 lazy 加载一次。
var (
	procGetDpiForWindow = syscall.NewLazyDLL("user32.dll").NewProc("GetDpiForWindow")
	procGetDeviceCaps   = syscall.NewLazyDLL("gdi32.dll").NewProc("GetDeviceCaps")
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
	controls     map[render.Handle]lcl.IControl
	next         render.Handle
	form         lcl.IControl
	formRef      *engForm
	measureBmp   lcl.IBitmap        // 共享测量画布（布局在 diff 前，控件未创建）
	measureCache map[string][2]int32 // 文本测量缓存（字体随 DPI 变化时失效）
	dpi          int32               // 当前显示器 DPI（0=未查询，invalidateDPI 清零强制重查）
	canvasDpi    int32               // 测量 bitmap DC 的 DPI（进程内固定，缓存一次；0=未查询）
	resizeFn     func(w, h int)     // OnResize 统一回调（窗体 resize 与 WM_DPICHANGED 共用）
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
	switch widgetType {
	case "Window":
		c = r.form
	case "Button":
		c = lcl.NewButton(r.form)
	case "Text":
		c = lcl.NewLabel(r.form)
	case "Input":
		c = lcl.NewEdit(r.form)
	default:
		panic(fmt.Sprintf("native: 未知控件类型 %q", widgetType))
	}
	h := r.alloc()
	r.controls[h] = c
	return h
}

func (r *Renderer) Destroy(h render.Handle) {
	c := r.controls[h]
	if c == nil || c == r.form {
		return // 主窗体不显式 Free
	}
	c.Free()
	delete(r.controls, h)
}

func (r *Renderer) SetParent(child, parent render.Handle) {
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

func (r *Renderer) SetBounds(h render.Handle, b render.Rect) {
	dpi := int(r.currentDPI())
	r.controls[h].SetBounds(
		int32(render.DIPToPX(b.X, dpi)),
		int32(render.DIPToPX(b.Y, dpi)),
		int32(render.DIPToPX(b.W, dpi)),
		int32(render.DIPToPX(b.H, dpi)),
	)
}

func (r *Renderer) SetVisible(h render.Handle, visible bool) {
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
	}
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
func (r *Renderer) emitResize() {
	if r.resizeFn == nil {
		return
	}
	w, h := r.ClientSize()
	r.resizeFn(w, h)
}

func (r *Renderer) SetEvent(h render.Handle, event string, fn any) {
	c := r.controls[h]
	switch event {
	case "OnClick":
		c.SetOnClick(func(_ lcl.IObject) { fn.(func())() })
	case "OnChange":
		ed, ok := c.(lcl.ICustomEdit)
		if !ok {
			panic(fmt.Sprintf("native: 控件 %d 不支持 OnChange", h))
		}
		ed.SetOnChange(func(_ lcl.IObject) { fn.(func(string))(ed.Text()) })
	default:
		panic(fmt.Sprintf("native: 未知事件 %q", event))
	}
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
func (r *Renderer) RunOnUI(fn func()) {
	if api.CurrentThreadId() == api.MainThreadId() {
		fn()
		return
	}
	lcl.RunOnMainThreadSync(fn)
}

func (r *Renderer) HandleAllocated(h render.Handle) bool {
	_, ok := r.controls[h]
	return ok
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
