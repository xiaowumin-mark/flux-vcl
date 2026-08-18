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

// Title 设置窗体标题（Window 用；内部走 Text 属性 → 绑定层 SetCaption）。
func Title(s string) Opt {
	return optFn(func(n *Node) { n.Props.Set("Text", s) })
}

// Key 设置稳定身份（D3）—— 用于 reconciliation 的 identity，不是寻址。
//
// 只在以下场景必须：
//   - 动态列表/可变子节点：key 必须来自模型（ID），绝不用数组 index、绝不每次
//     render 随机 —— 否则重排时 VCL 焦点/caret/IME 会漂移（index key 会迁到错行）；
//   - Component 身份（透明分组跨 render 复用子树）；
//   - App.SetBounds 动画目标（跨 render 保持同一控件）；
//   - 同类型多 handler 需用事件 e.Source 区分（无 key 时 Source 回落为树路径，
//     结构变动会漂移 —— 需要稳定身份请用 Key）。
//
// 静态树（结构固定、不重排）可不写 Key：无 key 控件按位置匹配，且可用
// App.FindByPath（隐式寻址）定位 —— 寻址与身份解耦（见 diff 包文档）。
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

// MainAxis 设置 flex 容器（Row/Column）的主轴对齐方式（Phase 3.3）。
// 缺省 MainAxisStart（与占位堆叠一致）。
func MainAxis(a MainAxisAlignment) Opt {
	return optFn(func(n *Node) { n.Props.Set("MainAxisAlignment", int(a)) })
}

// CrossAxis 设置 flex 容器（Row/Column）的交叉轴对齐方式。
// CrossAxisStretch 使子控件填满交叉轴；缺省 CrossAxisStart。
func CrossAxis(a CrossAxisAlignment) Opt {
	return optFn(func(n *Node) { n.Props.Set("CrossAxisAlignment", int(a)) })
}

// Enabled 设置初始可用状态（缺省 true）。
func Enabled(v bool) Opt {
	return optFn(func(n *Node) { n.Props.Set("Enabled", v) })
}

// Checked 设置 CheckBox 等可选控件的选中状态（缺省 false）。
func Checked(v bool) Opt {
	return optFn(func(n *Node) { n.Props.Set("Checked", v) })
}

// Items 设置 ComboBox 的字符串选项。输入 slice 会被防御性复制；nil 与空 slice
// 统一为语义等价的空列表，避免调用方后续修改绕过 diff。
func Items(items []string) Opt {
	values := canonicalItems(items)
	return optFn(func(n *Node) { n.Props.Set("Items", values) })
}

// SelectedIndex 设置 ComboBox 或 PageControl 的受控选中索引。-1 表示未选择；
// ComboBox 按 Items、PageControl 按页面数规范化到合法范围，空集合恒为 -1。
func SelectedIndex(index int) Opt {
	return optFn(func(n *Node) { n.Props.Set("SelectedIndex", index) })
}

// Minimum 设置 ProgressBar 或 Slider 的最小值（缺省 0）。最终范围与 Value 在
// 控件构造时统一规范化，因此多个 Opt 的声明顺序不影响结果。
func Minimum(value int) Opt {
	return optFn(func(n *Node) { n.Props.Set("Minimum", value) })
}

// Maximum 设置 ProgressBar 或 Slider 的最大值（缺省 100）。小于 Minimum 时回落为 Minimum。
func Maximum(value int) Opt {
	return optFn(func(n *Node) { n.Props.Set("Maximum", value) })
}

// Value 设置 ProgressBar 或 Slider 的当前值；最终值被钳制到 [Minimum, Maximum]。
func Value(value int) Opt {
	return optFn(func(n *Node) { n.Props.Set("Value", value) })
}

// GroupIndex 设置 RadioButton 的逻辑单选组编号，缺省为 0。支持
// RadioGroupable 的 Renderer 会让同一 resolved native parent 内的同组控件互斥，
// 不同组保持独立；它不是对某个绑定库属性 setter 的直接转发。
func GroupIndex(index int) Opt {
	return optFn(func(n *Node) { n.Props.Set("GroupIndex", index) })
}

// DarkTitleBar 设置窗体标题栏沉浸式暗色（Window 用；内部走 "TitleBarDark" 属性 →
// 绑定层 DwmSetWindowAttribute）。dark=true → 暗色标题栏，false → 系统默认亮色。
// Phase 5.2 Theme：主题切换时随 Theme.DarkTitleBar 传入，让标题栏与客户区一起变暗。
func DarkTitleBar(dark bool) Opt {
	return optFn(func(n *Node) { n.Props.Set("TitleBarDark", dark) })
}
