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
package native

import (
	"fmt"

	"github.com/energye/lcl/api"
	"github.com/energye/lcl/api/libname"
	"github.com/energye/lcl/lcl"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

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
	measureCache map[string][2]int32 // 文本测量缓存（Phase 3.5 DPI/字体变化时失效）
}

// NewRenderer 创建 LCL 渲染器并注册主窗体（须在 Init 之后调用）。
// 设默认窗体客户区 640x480（布局以客户区坐标系为准，Phase 3.5 前假设 96 DPI）。
func NewRenderer() *Renderer {
	f := &engForm{}
	lcl.Application.NewForms(f)
	r := &Renderer{
		controls:     make(map[render.Handle]lcl.IControl),
		measureCache: make(map[string][2]int32),
		formRef:      f,
		form:         f,
	}
	f.SetClientWidth(640)
	f.SetClientHeight(480)
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
	r.controls[h].SetBounds(int32(b.X), int32(b.Y), int32(b.W), int32(b.H))
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
// Phase 3.5 DPI/字体变化时需失效缓存。
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
	r.measureCache[text] = [2]int32{int32(w), int32(h)}
	return w, h
}

// ClientSize 返回窗体客户区尺寸（DIP）。子控件坐标以此为布局坐标系。
func (r *Renderer) ClientSize() (int, int) {
	return int(r.form.ClientWidth()), int(r.form.ClientHeight())
}

// OnResize 注册窗体 resize 回调。回调在 UI 线程触发（消息泵投递），
// 参数为新客户区尺寸（DIP）。幂等：重复调用覆盖。
func (r *Renderer) OnResize(fn func(w, h int)) {
	r.form.SetOnResize(func(_ lcl.IObject) {
		fn(int(r.form.ClientWidth()), int(r.form.ClientHeight()))
	})
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
