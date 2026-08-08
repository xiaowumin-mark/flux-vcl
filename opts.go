package flux

// Opt 是控件属性选项（design.md §7 Modifier）。构造器可叠加：
//
//	Button("OK", OnClick(fn), Width(120), Height(40), Key("ok"))
type Opt interface {
	apply(n *Node)
}

type optFn func(*Node)

func (f optFn) apply(n *Node) { f(n) }

func applyOpts(n *Node, opts []Opt) {
	for _, o := range opts {
		o.apply(n)
	}
}

// OnClick 绑定点击事件。事件回调每次 render 重新绑定（函数值无法比较相等性）。
// Phase 1 事件签名为 func()（无 sender/坐标，Phase 4 引入统一 Event）。
func OnClick(fn func()) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnClick", fn) })
}

// OnChange 绑定文本变化事件（Input 等可编辑控件）。fn 接收新文本。
func OnChange(fn func(text string)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnChange", fn) })
}

// Key 设置稳定身份（D3）。列表/可变子节点必须用模型来源的稳定 key，
// 绝不用数组 index、绝不每次 render 随机 —— 否则 VCL 焦点/caret/IME 会漂移。
func Key(k string) Opt {
	return optFn(func(n *Node) { n.Key = k })
}

// Width 覆盖 intrinsic 布局宽度（DIP）。缺省按控件类型 intrinsic 尺寸。
func Width(v int) Opt {
	return optFn(func(n *Node) { n.Props.Set("Width", v) })
}

// Height 覆盖 intrinsic 布局高度（DIP）。
func Height(v int) Opt {
	return optFn(func(n *Node) { n.Props.Set("Height", v) })
}

// Visible 设置初始可见性（缺省 true）。
func Visible(v bool) Opt {
	return optFn(func(n *Node) { n.Props.Set("Visible", v) })
}

// Enabled 设置初始可用状态（缺省 true）。
func Enabled(v bool) Opt {
	return optFn(func(n *Node) { n.Props.Set("Enabled", v) })
}
