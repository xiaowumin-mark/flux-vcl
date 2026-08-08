package flux

import "github.com/xiaowumin-mark/flux-vcl/internal/widget"

// 控件构造器（design.md §4.2 / Phase 1.5 基础控件集）。
//
// 每个构造器构建一次节点树并包装为不可变 Widget；每次 render 重新调用。
// Column/Row 为透明容器：不映射原生控件，仅表达布局分组（diff 引擎直接
// 把子控件挂到祖父容器）。布局为占位实现（见 layout.go，Phase 3 精修）。

// Window 顶层窗体（对应绑定层 TEngForm）。children 按列堆叠。
func Window(children ...Widget) Widget {
	return containerNode("Window", children)
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
func Text(text string, opts ...Opt) Widget {
	n := widget.NewNode("Text")
	n.Props.Set("Text", text)
	applyOpts(n, opts)
	return widgetNode{n}
}

// Button 按钮（对应绑定层 TButton）。text 为按钮文本。
func Button(text string, opts ...Opt) Widget {
	n := widget.NewNode("Button")
	n.Props.Set("Text", text)
	applyOpts(n, opts)
	return widgetNode{n}
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
