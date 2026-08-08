package flux

// 布局协议类型（design.md §6.2 / D5，Phase 3.1）。
//
// constraints 下传 / size 上抛 / 父定 offset：父用 BoxConstraints 约束子，
// 子返回内容 Size，父决定最终位置。所有坐标/尺寸为 DIP（D5，Phase 3.5 前假设 96 DPI）。

// Size 是控件内容尺寸（DIP）。
type Size struct{ W, H int }

// Point 是布局坐标（DIP，相对窗体客户区）。
type Point struct{ X, Y int }

// BoxConstraints 是父向子下传的布局约束（Flutter BoxConstraints 的最小集）。
//
// Max 字段用 -1 表示 unbounded（∞）；Min 恒非负。constraints 的语义：
// 子控件的最终尺寸必须落在 [Min, Max] 区间（由 Constrain 钳制）。
type BoxConstraints struct {
	MinW, MaxW int
	MinH, MaxH int
}

// unbounded 表示主轴/交叉轴无上界（-1）。
const unbounded = -1

// Tight 构造上下界相同的紧约束：子必须精确取 (w, h)。
func Tight(w, h int) BoxConstraints { return BoxConstraints{w, w, h, h} }

// Loose 构造宽松约束：子可取 [0, w]×[0, h] 内任意尺寸（不强制填满）。
func Loose(w, h int) BoxConstraints { return BoxConstraints{0, w, 0, h} }

// Unbounded 构造两轴均无上界的约束（flex 容器测量非 flex 子、滚动容器内部用）。
func Unbounded() BoxConstraints { return BoxConstraints{0, unbounded, 0, unbounded} }

// IsUnboundedW 报告宽方向是否无上界。
func (c BoxConstraints) IsUnboundedW() bool { return c.MaxW < 0 }

// IsUnboundedH 报告高方向是否无上界。
func (c BoxConstraints) IsUnboundedH() bool { return c.MaxH < 0 }

// Constrain 把 (w, h) 钳制到 [Min, Max] 区间（D5：结果必须 constrain，否则溢出）。
func (c BoxConstraints) Constrain(w, h int) Size {
	if c.MaxW >= 0 && w > c.MaxW {
		w = c.MaxW
	}
	if w < c.MinW {
		w = c.MinW
	}
	if c.MaxH >= 0 && h > c.MaxH {
		h = c.MaxH
	}
	if h < c.MinH {
		h = c.MinH
	}
	return Size{W: w, H: h}
}

// MainAxisAlignment 是 flex 容器（Row/Column）的主轴对齐方式。
type MainAxisAlignment int

const (
	MainAxisStart MainAxisAlignment = iota
	MainAxisCenter
	MainAxisEnd
	MainAxisSpaceBetween
	MainAxisSpaceAround
	MainAxisSpaceEvenly
)

// CrossAxisAlignment 是 flex 容器（Row/Column）的交叉轴对齐方式。
type CrossAxisAlignment int

const (
	CrossAxisStart CrossAxisAlignment = iota
	CrossAxisCenter
	CrossAxisEnd
	CrossAxisStretch // 子控件填满交叉轴
)
