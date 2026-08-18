package flux

import "github.com/xiaowumin-mark/flux-vcl/internal/widget"

// Slider 创建水平整数滑块（对应绑定层 TTrackBar）。
//
// Minimum、Maximum、Value 与 Step 是显式受控值；默认分别为 0、100、0、1。
// Maximum 小于 Minimum 时收敛到 Minimum，Value 钳制到闭区间。Step 只控制
// 键盘/行步进，不把鼠标拖动结果强制吸附到步长，也不扩张 Bind 的隐式语义。
func Slider(opts ...Opt) Widget {
	n := widget.NewNode("Slider")
	applyOpts(n, opts)
	minimum := n.Props.Int("Minimum")
	maximum := 100
	if _, ok := n.Props.Get("Maximum"); ok {
		maximum = n.Props.Int("Maximum")
	}
	value := n.Props.Int("Value")
	step := 1
	if _, ok := n.Props.Get("Step"); ok {
		step = n.Props.Int("Step")
	}
	if step <= 0 {
		panic("flux.Slider: Step 必须 > 0")
	}
	minimum, maximum, value = normalizeProgress(minimum, maximum, value)
	n.Props.Set("Minimum", minimum)
	n.Props.Set("Maximum", maximum)
	n.Props.Set("Step", step)
	n.Props.Set("Value", value)
	return widgetNode{n}
}

// Step 设置 Slider 的正整数键盘步长，缺省为 1。它不影响鼠标拖动精度。
func Step(value int) Opt {
	if value <= 0 {
		panic("flux.Step: value 必须 > 0")
	}
	return optFn(func(n *Node) { n.Props.Set("Step", value) })
}

// OnValueChange 绑定 Slider 的用户值变化事件。程序化 Value patch 不触发回调；
// 调用方应在回调中更新 State，并在下一次 render 继续声明受控 Value。
func OnValueChange(fn func(value int)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnValueChange", fn) })
}
