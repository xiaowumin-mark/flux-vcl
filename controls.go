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

// Column 垂直容器：children 自上而下排列。
//
// 接受混合参数：子节点（Widget，可含 Expanded/Flexible）与容器选项（Opt，
// 如 MainAxis/CrossAxis 对齐）。例如 Column(MainAxis(MainAxisCenter), Text("a")).
func Column(args ...any) Widget { return containerArgs("Column", args) }

// Row 水平容器：children 自左向右排列。
//
// 接受混合参数：子节点（Widget，可含 Expanded/Flexible）与容器选项（Opt，
// 如 MainAxis/CrossAxis 对齐）。例如 Row(Expanded(Text("a")), Text("b")).
func Row(args ...any) Widget { return containerArgs("Row", args) }

// Component 包装构建函数为 Widget（design.md §4.1 组件化，Phase 5.4）。
//
// 组件是透明分组节点（diff 不建原生控件）：子树 = build() 产物，按透明容器
// 参与 diff/layout（Element 句柄继承父，子控件挂祖父）。
//
// 组件身份靠外部 Key 稳定（D3）—— 绝不在 Build 内生成 key / 定义嵌套类型
// （React 教训：嵌套类型每次 render 是新类型，破坏 canUpdate 的原地复用）。
// 因此 Component 接受 Opts（典型只有 Key）：Key("card") 落在透明组件节点上，
// 使 diff 能按该 key 跨 render 复用子树（子控件原地 patch 不重建）。
//
//	Component(func() Widget {
//	    return Column(Text("name"), Button("OK", Key("ok")))
//	}, Key("login-card"))
func Component(build func() Widget, opts ...Opt) Widget {
	n := widget.NewNode("Component")
	for _, o := range opts {
		o.apply(n)
	}
	if w := build(); w != nil {
		n.Add(w.Create())
	}
	return widgetNode{n}
}

// ScrollBox 垂直滚动容器（对应绑定层 TScrollBox，Phase 3.6）。
//
// SingleChildScrollView 语义：单子内容（通常为 Column），内容超高时由原生
// TScrollBox 滚动条滚动；自身尺寸 = viewport（内容超高→钳制到约束出现滚动条，
// 内容偏矮→收缩到内容）。滚动内容在滚动轴（垂直）用 unbounded 约束测量；
// 已知限制：滚动内容内的 Expanded 会被压成 0（Flutter 同需 IntrinsicHeight）。
//
//	ScrollBox(Column(Text("a"), Text("b")))
func ScrollBox(child Widget) Widget { return containerArgs("ScrollBox", []any{child}) }

// PageControl 分页容器（对应绑定层 TPageControl）。
//
// args 只能混排 TabPage 与 Opt。每个 TabPage 必须带非空、唯一的稳定 Key，
// 以保证增删和重排时页面及其输入焦点、caret、IME 状态原地复用。SelectedIndex
// 是受控索引：非空页面默认选中第 0 页，-1 表示不选中，越界值会被钳制。
func PageControl(args ...any) Widget {
	n := widget.NewNode("PageControl")
	keys := make(map[string]struct{})
	for _, a := range args {
		switch v := a.(type) {
		case Widget:
			page := v.Create()
			if page == nil || page.Type != "TabPage" {
				panic("flux.PageControl: 子节点必须是 TabPage")
			}
			if len(page.Children) != 1 || page.Children[0] == nil {
				panic("flux.PageControl: TabPage 必须包含唯一非空子树")
			}
			if page.Key == "" {
				panic("flux.PageControl: TabPage 必须设置非空 Key")
			}
			if _, exists := keys[page.Key]; exists {
				panic("flux.PageControl: TabPage Key 必须唯一")
			}
			keys[page.Key] = struct{}{}
			n.Add(page)
		case Opt:
			v.apply(n)
		default:
			panic("flux.PageControl: 参数必须是 TabPage 或 Opt")
		}
	}
	selected := 0
	if _, exists := n.Props.Get("SelectedIndex"); exists {
		selected = n.Props.Int("SelectedIndex")
	}
	n.Props.Set("SelectedIndex", normalizePageSelectedIndex(len(n.Children), selected))
	return widgetNode{n}
}

// TabPage 创建 PageControl 的一个页面（对应绑定层 TTabSheet）。title 是页签标题，
// child 是该页唯一的内容子树。opts 必须包含 Key；动态页面的 Key 应来自业务模型，
// 不能使用数组下标或每次 render 临时生成的值。
func TabPage(title string, child Widget, opts ...Opt) Widget {
	if child == nil {
		panic("flux.TabPage: child 不能为空")
	}
	content := child.Create()
	if content == nil {
		panic("flux.TabPage: child.Create() 不能返回 nil")
	}
	n := widget.NewNode("TabPage")
	n.Props.Set("Text", title)
	applyOpts(n, opts)
	if n.Key == "" {
		panic("flux.TabPage: 必须设置非空 Key")
	}
	n.Add(content)
	return widgetNode{n}
}

// Expanded 把子控件在主轴上强制填满 flex 容器分配的剩余空间（tight，Phase 3.3）。
// 默认 flex=1；多个 flex 子按因子比例分配 freeSpace。Expanded(child, 2) 占双份。
func Expanded(child Widget, flex ...int) Widget { return flexNode("Expanded", child, flex) }

// Flexible 允许子控件小于 flex 分配的空间（loose），与 Expanded 相反（Phase 3.3）。
// 默认 flex=1；其余语义同 Expanded。
func Flexible(child Widget, flex ...int) Widget { return flexNode("Flexible", child, flex) }

// flexNode 构造 flex 透明包装节点：不创建原生控件，仅向布局引擎标记 flex 因子。
// diff 引擎按 transparentType 处理（Element 句柄继承父，children 挂祖父）。
func flexNode(t string, child Widget, flex []int) Widget {
	n := widget.NewNode(t)
	f := 1
	if len(flex) > 0 {
		f = flex[0]
	}
	if f <= 0 {
		panic("flux: flex 因子必须 > 0")
	}
	n.Props.Set("Flex", f)
	n.Add(child.Create())
	return widgetNode{n}
}

// containerArgs 构造容器节点，从混合参数中分离子节点与 Opt。
func containerArgs(t string, args []any) Widget {
	n := widget.NewNode(t)
	for _, a := range args {
		switch v := a.(type) {
		case Widget:
			n.Add(v.Create())
		case Opt:
			v.apply(n)
		default:
			panic("flux." + t + ": 参数必须是 Widget 或 Opt")
		}
	}
	return widgetNode{n}
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

// CheckBox 复选框（对应绑定层 TCheckBox）。text 为标签文本，可为字符串或 State 绑定。
//
// Checked 控制选中状态；OnCheckedChange 接收用户操作后的布尔值。本批不为
// State[bool] 添加隐式双向绑定，Bind(...) 仅绑定标签文本。
func CheckBox(text any, opts ...Opt) Widget {
	n := widget.NewNode("CheckBox")
	setTextProp(n, text)
	applyOpts(n, opts)
	return widgetNode{n}
}

// RadioButton 单选按钮（对应绑定层 TRadioButton）。text 为标签文本，可为字符串或
// State 绑定。Checked 与 OnCheckedChange 复用 CheckBox 的显式受控语义；支持
// RadioGroupable 的 Renderer 会在同一 resolved native parent 下维持 GroupIndex 的
// 逻辑互斥，默认组为 0。
func RadioButton(text any, opts ...Opt) Widget {
	n := widget.NewNode("RadioButton")
	setTextProp(n, text)
	applyOpts(n, opts)
	return widgetNode{n}
}

// Input 单行输入框（对应绑定层 TEdit）。
func Input(opts ...Opt) Widget {
	n := widget.NewNode("Input")
	applyOpts(n, opts)
	return widgetNode{n}
}

// Memo 多行输入框（对应绑定层 TMemo）。text 为初始文本，可为字符串或 State 绑定。
//
// Memo 的 intrinsic 尺寸按显式换行逐行测量，并保留 180×96 DIP 的最小编辑区域；
// 不自动根据软换行扩高，也不提供富文本语义。
func Memo(text any, opts ...Opt) Widget {
	n := widget.NewNode("Memo")
	setTextProp(n, text)
	// Bind(...) 同时实现 Opt 与 bindable：setTextProp 登记依赖和当前文本，
	// 再通过 Opt 写入 OnChange，实现与 Input 一致的双向文本绑定。
	if binding, ok := text.(Opt); ok {
		binding.apply(n)
	}
	applyOpts(n, opts)
	return widgetNode{n}
}

// ComboBox 下拉选择框（对应绑定层 TComboBox）。Items 提供字符串选项，
// SelectedIndex 控制选中索引，-1 表示未选择；OnSelectionChange 接收用户选择后的索引。
// 选择状态保持显式受控：调用方在回调中更新 State 后以 SelectedIndex 重新声明。
func ComboBox(opts ...Opt) Widget {
	n := widget.NewNode("ComboBox")
	applyOpts(n, opts)
	items, _ := n.Props.Get("Items")
	values, _ := items.([]string)
	values = canonicalItems(values)
	n.Props.Set("Items", values)
	n.Props.Set("SelectedIndex", normalizeSelectedIndex(values, n.Props.Int("SelectedIndex")))
	return widgetNode{n}
}

// ProgressBar 进度条（对应绑定层 TProgressBar）。Minimum、Maximum 与 Value 在所有
// Opt 应用后统一规范化：Maximum 不小于 Minimum，Value 钳制到该闭区间。
func ProgressBar(opts ...Opt) Widget {
	n := widget.NewNode("ProgressBar")
	applyOpts(n, opts)
	minimum := n.Props.Int("Minimum")
	maximum := 100
	if _, ok := n.Props.Get("Maximum"); ok {
		maximum = n.Props.Int("Maximum")
	}
	value := n.Props.Int("Value")
	minimum, maximum, value = normalizeProgress(minimum, maximum, value)
	n.Props.Set("Minimum", minimum)
	n.Props.Set("Maximum", maximum)
	n.Props.Set("Value", value)
	return widgetNode{n}
}

func normalizeProgress(minimum, maximum, value int) (int, int, int) {
	if maximum < minimum {
		maximum = minimum
	}
	if value < minimum {
		value = minimum
	}
	if value > maximum {
		value = maximum
	}
	return minimum, maximum, value
}

func canonicalItems(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	return append([]string(nil), items...)
}

func normalizeSelectedIndex(items []string, index int) int {
	if len(items) == 0 {
		return -1
	}
	if index < -1 {
		return -1
	}
	if index >= len(items) {
		return len(items) - 1
	}
	return index
}

func normalizePageSelectedIndex(pageCount, index int) int {
	if pageCount == 0 {
		return -1
	}
	if index < -1 {
		return -1
	}
	if index >= pageCount {
		return pageCount - 1
	}
	return index
}
