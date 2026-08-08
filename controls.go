package flux

import "github.com/xiaowumin-mark/flux-vcl/internal/widget"

// 控件构造器（design.md §4.2 / Phase 1.5 基础控件集）。
//
// 每个构造器构建一次节点树并包装为不可变 Widget；每次 render 重新调用。
// Column/Row 为透明容器：不映射原生控件，仅表达布局分组（diff 引擎直接
// 把子控件挂到祖父容器）。布局为占位实现（见 layout.go，Phase 3 精修）。

// Window 顶层窗体（对应绑定层 TEngForm）。子节点按列堆叠。
//
// 接受混合参数：子节点（Widget，如 Column/Text/Button）与窗体选项（Opt，
// 如 Title/Width/Height）。例如 Window(Title("hi"), Column(Text("x"))).
func Window(args ...any) Widget {
	n := widget.NewNode("Window")
	for _, a := range args {
		switch v := a.(type) {
		case Widget:
			n.Add(v.Create())
		case Opt:
			v.apply(n)
		default:
			panic("flux.Window: 参数必须是 Widget 或 Opt")
		}
	}
	return widgetNode{n}
}

// Column 垂直容器：children 自上而下排列（占位布局，Phase 3 精修）。
func Column(children ...Widget) Widget {
	return containerNode("Column", children)
}

// Row 水平容器：children 自左向右排列（占位布局，Phase 3 精修）。
func Row(children ...Widget) Widget {
	return containerNode("Row", children)
}

// Text 文本标签（对应绑定层 TLabel）。布局宽度按 intrinsic 文本测量。
// text 为字符串或 State 绑定（Text(Bind(state))，渲染值随 State 变化）。
func Text(text any, opts ...Opt) Widget {
	n := widget.NewNode("Text")
	setTextProp(n, text)
	applyOpts(n, opts)
	return widgetNode{n}
}

// Button 按钮（对应绑定层 TButton）。text 为按钮文本，可为字符串或 State 绑定。
func Button(text any, opts ...Opt) Widget {
	n := widget.NewNode("Button")
	setTextProp(n, text)
	applyOpts(n, opts)
	return widgetNode{n}
}

// setTextProp 把文本参数（string 或 bindable）写入节点 Props。
func setTextProp(n *Node, text any) {
	switch v := text.(type) {
	case string:
		n.Props.Set("Text", v)
	case bindable:
		n.Props.Set("Text", v.renderText())
		n.Props.Set(bindKey, v) // 登记绑定依赖（collectBindings 订阅）
	default:
		panic("flux: 文本参数必须是 string 或 Bind(...)")
	}
}

// Input 单行输入框（对应绑定层 TEdit）。
func Input(opts ...Opt) Widget {
	n := widget.NewNode("Input")
	applyOpts(n, opts)
	return widgetNode{n}
}

func containerNode(t string, children []Widget) Widget {
	n := widget.NewNode(t)
	for _, c := range children {
		n.Add(c.Create())
	}
	return widgetNode{n}
}
