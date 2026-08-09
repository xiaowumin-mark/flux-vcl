// Package render 定义 FluxVCL 的 Renderer 抽象与 Mutation op 集（design.md §5.1）。
//
// D6 绑定隔离：Renderer 面向窄接口（Create/SetBounds/SetVisible/TextWidth/…），
// 默认 LCL 绑定（energye/lcl）藏在适配层后，保留切 govcl v1.2.10（B 计划）的余地。
// diff 引擎（Phase 1.4）只面向本接口与 Op 集，因此可在无显示环境下测试
// （0.6 无头测试驱动：用 Mock 替换真实绑定，state/diff 纯逻辑跑 go test）。
package render

// Handle 标识一个原生控件实例。在真实绑定下是对原生对象的引用；
// 在 Mock 下是模拟句柄。不透明类型，外部只作相等比较。
type Handle uintptr

// Rect 控件几何（DIP，与 design.md D5 一致：全坐标用 DIP）。
type Rect struct {
	X, Y, W, H int
}

// Ref 是控件引用接收者（逃逸口，design.md §11.2）。
//
// 绑定层在控件创建后把原生控件对象经 SetNative 注入；用户侧（flux.Ref）
// 实现本接口，在事件回调/外部代码中读取该对象（类型断言到具体后端类型，
// 如 *lcl.TButton）。D6 隔离：本接口是 flux 与绑定层之间唯一的知识点。
type Ref interface {
	SetNative(obj any)
}

// Renderer 是 FluxVCL 对原生控件后端的最小依赖面（D6 窄接口）。
//
// 所有方法在主 UI 线程调用（D4）；diff 引擎生成的 op 经调度器落到这里。
type Renderer interface {
	// Create 创建指定类型控件，返回句柄。widgetType 为 FluxVCL 控件类型名
	// （如 "Form"、"Button"），由适配层映射到原生控件。
	Create(widgetType string) Handle
	// Destroy 销毁控件。销毁必须入队延后，绝不在事件回调内同步调用（D4）。
	Destroy(h Handle)
	// SetParent 将 child 挂到 parent 下。parent 为零值时表示顶层（窗体）。
	SetParent(child, parent Handle)
	// SetBounds 设置控件几何（DIP）。
	SetBounds(h Handle, bounds Rect)
	// SetVisible 设置可见性。
	SetVisible(h Handle, visible bool)
	// SetText 设置控件文本（Caption）。
	SetText(h Handle, text string)
	// SetEnabled 设置可用状态。
	SetEnabled(h Handle, enabled bool)
	// SetColor 设置控件背景色（ARGB；透明合成由后端决定，原生控件无 alpha）。
	// Phase 5.2 Theme 的颜色落点（diff 按 "Color" 属性分发）。
	SetColor(h Handle, color Color)
	// SetFontColor 设置控件文字颜色（经字体对象，ARGB）。TLabel/TEdit/TButton
	// 等有 Font 的控件均可；无 Font 的控件忽略。Phase 5.2 Theme。
	SetFontColor(h Handle, color Color)
	// SetTitleBarDark 设置窗体标题栏沉浸式暗色（win32 DWM，Phase 5.2 Theme）。
	// dark=true → 暗色标题栏（Win10 1809+ DwmSetWindowAttribute）。仅 Window
	// 有标题栏；实现方忽略 h 直接用主窗体句柄。主题切换时随 Theme 应用。
	SetTitleBarDark(h Handle, dark bool)
	// NewTimer 创建主线程定时器：intervalMs 毫秒后每 intervalMs 周期触发 fn，
	// 直至返回的停止函数被调用（幂等）。主线程定时器（D4）：
	// 真实绑定用 TTimer（消息泵上触发，无 goroutine/marshalling）；
	// Mock 不真实调度 —— 保存回调，由测试经 FireTimer 手动驱动（确定性断言）。
	NewTimer(intervalMs int, fn func()) (stop func())
	// TextExtent 测量给定文本在当前默认字体下的宽高（DIP）。
	// intrinsic-size 测量（design.md §6.2）：不因测量而实现控件，无句柄依赖
	// （实现方用 bitmap canvas / 字体对象测量）。实现方内部按 text 缓存；
	// 字体/DPI 变化时失效（Phase 3.5）。
	TextExtent(text string) (w, h int)
	// ClientSize 返回窗体客户区尺寸（DIP）—— 子控件布局坐标系。
	// Window 节点布局时查询；resize 后 render 现查最新值。
	ClientSize() (w, h int)
	// OnResize 注册窗体 resize 回调（UI 线程调用，参数为新客户区尺寸 DIP）。
	// 幂等：重复调用覆盖；App 在 NewApp 注册一次，仅用于触发 re-render。
	OnResize(fn func(w, h int))
	// SetEvent 绑定事件回调（如 "OnClick"）。fn 为绑定层可识别的事件回调
	// （Phase 1 约定 func() / func(string)）。函数值无法比较相等性，
	// diff 引擎每次 render 均重新绑定（D2 逃逸口行为，React 同款）。
	SetEvent(h Handle, event string, fn any)
	// AttachRef 把控件句柄关联到引用（逃逸口，design.md §11.2）。
	// 绑定层把原生控件对象经 ref.SetNative(obj) 注入。
	AttachRef(h Handle, ref Ref)
	// ApplyNative 在控件创建后调用逃逸函数（逃逸口，design.md §11.1）。
	// fn 接收绑定层原生控件对象（flux.Native 已断言到具体类型）。
	ApplyNative(h Handle, fn func(obj any))
	// RunOnUI 把 fn 投递到 UI 线程执行（D4 marshalling）。
	// State 从任意 goroutine 触发 re-render 时经此 marshal；已在 UI 线程则直接执行。
	// Mock 实现直接调用（测试在调用 goroutine 内同步执行）；
	// 真实绑定用 ThreadSync（如 lcl.RunOnMainThreadSync，阻塞当前 goroutine）。
	RunOnUI(fn func())
	// HandleAllocated 报告句柄是否已分配真实原生控件。
	HandleAllocated(h Handle) bool
}

// OpType 是 mutation 操作类型（Dioxus 风格 op 集，design.md §5.1）。
type OpType int

const (
	// OpCreate 创建控件。Value 为 widgetType 字符串。
	OpCreate OpType = iota
	// OpDestroy 销毁控件。
	OpDestroy
	// OpAppendChild 将 child 追加到 parent 末尾。
	OpAppendChild
	// OpInsertChildBefore 将 child 插入到 parent 中 before 之前（Index 为位置）。
	OpInsertChildBefore
	// OpRemoveChild 从 parent 移除 child。
	OpRemoveChild
	// OpSetProperty 设置单个属性（Key 为属性名，Value 为值，如 "Visible"）。
	OpSetProperty
	// OpSetText 设置文本（Value 为 string）。独立于通用属性，便于高频批量。
	OpSetText
	// OpSetEvent 绑定事件回调（Key 为事件名，Value 为 fn）。函数值无法
	// 比较相等性，每次 diff 重新绑定（D2 逃逸口行为）。
	OpSetEvent
)

// Op 是一条针对某个控件的 mutation。diff 引擎批量生成，
// 由调度器在 UI 线程上逐条应用（D2 批量提交 + D4 线程纪律）。
type Op struct {
	Type   OpType
	Handle Handle // 目标控件
	Parent Handle // AppendChild/InsertChildBefore/RemoveChild 的父控件
	Before Handle // InsertChildBefore 的参考控件（零值=追加到末尾）
	Key    string // Create 的 widgetType；SetProperty 的属性名
	Value  any    // Create 无；SetProperty/SetText 为值
}
