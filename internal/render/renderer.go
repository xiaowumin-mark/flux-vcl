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
	// TextWidth 测量给定文本在当前控件字体下的宽度（DIP）。
	// 用于布局引擎的 intrinsic-size 测量（design.md §6.2）。
	TextWidth(h Handle, text string) int
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
