package flux

import "github.com/xiaowumin-mark/flux-vcl/internal/render"

// 布局引擎（Phase 3.1/3.3/3.4：design.md §6.2 / D5 / research.md §5.1）。
//
// 协议：constraints 下传 / size 上抛 / 父定 offset。每节点最终 frame 写入
// Props["Bounds"]（render.Rect，DIP），diff 引擎按普通属性应用（透明容器
// 与 Window 的 Bounds 只用于定位/诊断，diff 跳过，见 internal/diff）。
//
// 单遍 RenderFlex 递归：flex 容器先量非 flex 子（主轴 unbounded）→ freeSpace
// 按 flex 因子分配（Expanded=tight / Flexible=loose）→ 主轴对齐分布 → 交叉轴
// 对齐。只增不缩：非 flex 子恒 intrinsic，溢出记入 layoutDiags。

// layoutGap 是 flex 容器子控件间的基础间距（DIP）。
const layoutGap = 4

// LayoutDiag 是一次 render 布局的溢出诊断（Phase 3.7 inspector 的前身，
// 本轮供测试断言）。OverflowW/H 为容器尺寸超出约束的量（0 表示该轴未溢出）。
type LayoutDiag struct {
	Type      string
	Key       string
	OverflowW int
	OverflowH int
}

// layoutDiags 收集布局诊断（App 每次 render 新建，布局后读取）。
type layoutDiags struct {
	list []LayoutDiag
}

func (d *layoutDiags) overflow(n *Node, ow, oh int) {
	if d == nil || (ow <= 0 && oh <= 0) {
		return
	}
	d.list = append(d.list, LayoutDiag{Type: n.Type, Key: n.Key, OverflowW: ow, OverflowH: oh})
}

// flexKid 是 flex 容器的一个子项（测量/定位工作单元）。
type flexKid struct {
	node   *Node
	flex   int // >0 表示 flex 子（Expanded/Flexible），否则非 flex
	isFlex bool
	size   Size
}

// layoutTree 布局一棵子树，返回其在约束 c 下的内容尺寸，并把绝对 frame 写入
// 每个节点的 Props["Bounds"]。pos 为绝对坐标（窗体客户区坐标系）。
func layoutTree(n *Node, r render.Renderer, c BoxConstraints, pos Point, d *layoutDiags) Size {
	switch n.Type {
	case "Window":
		cw, ch := r.ClientSize()
		if cw <= 0 {
			cw = 400
		}
		if ch <= 0 {
			ch = 300
		}
		return layoutRoot(n, r, Tight(cw, ch), pos, d)
	case "Row":
		return layoutFlex(n, r, c, pos, d, true)
	case "Column":
		return layoutFlex(n, r, c, pos, d, false)
	case "Expanded", "Flexible":
		// 父容器已按 flex 语义算好约束 c，此处原样传给唯一子（tight/loose 已定）。
		sz := layoutTree(n.Children[0], r, c, pos, d)
		setBounds(n, pos, sz)
		return sz
	case "Text":
		w, h := r.TextExtent(n.Props.String("Text"))
		sz := leafSize(w, h, n, c)
		setBounds(n, pos, sz)
		return sz
	case "Button":
		w, _ := r.TextExtent(n.Props.String("Text"))
		bw := w + 32 // 左右 padding
		if bw < 88 {
			bw = 88
		}
		sz := leafSize(bw, 32, n, c)
		setBounds(n, pos, sz)
		return sz
	case "Input":
		sz := leafSize(180, 28, n, c)
		setBounds(n, pos, sz)
		return sz
	default: // 未知类型（含第三方控件）：默认尺寸
		sz := leafSize(100, 32, n, c)
		setBounds(n, pos, sz)
		return sz
	}
}

// leafSize 用 Width/Height Opt 覆盖 intrinsic 尺寸后钳制到约束（D5 constrain）。
func leafSize(w, h int, n *Node, c BoxConstraints) Size {
	if v := n.Props.Int("Width"); v > 0 {
		w = v
	}
	if v := n.Props.Int("Height"); v > 0 {
		h = v
	}
	return c.Constrain(w, h)
}

// setBounds 把节点 frame 写为绝对位置 pos + 尺寸 sz。
func setBounds(n *Node, pos Point, sz Size) {
	n.Props.Set("Bounds", render.Rect{X: pos.X, Y: pos.Y, W: sz.W, H: sz.H})
}

// setPos 把节点定位到绝对位置 pos（平移其 Bounds）；透明容器则平移整棵子树
// （嵌套 flex 时保持子树内部相对结构）。
func setPos(n *Node, pos Point) {
	b, ok := n.Props.Get("Bounds")
	if !ok {
		return
	}
	br := b.(render.Rect)
	offsetSubtree(n, pos.X-br.X, pos.Y-br.Y)
}

// offsetSubtree 平移节点及其子树的 Bounds（dx/dy）。
func offsetSubtree(n *Node, dx, dy int) {
	if b, ok := n.Props.Get("Bounds"); ok {
		br := b.(render.Rect)
		br.X += dx
		br.Y += dy
		n.Props.Set("Bounds", br)
	}
	for _, c := range n.Children {
		offsetSubtree(c, dx, dy)
	}
}

// layoutRoot 布局根容器（Window）。
//
// 与 layoutFlex 的差别：根容器的子控件收到"有界的主轴约束"（0..mainMax），
// 而非非 flex 子惯用的 unbounded。布局根必须给内容一个有界盒子（Flutter
// Scaffold body 语义）：否则根 Column 在 unbounded 主轴下 mainMax=-1、
// freeSpace=0，内部 Expanded 全部被压成 0（嵌套 flex + Expanded 的根）。
// 子控件在根容器内垂直堆叠。
func layoutRoot(n *Node, r render.Renderer, c BoxConstraints, pos Point, d *layoutDiags) Size {
	mainAxis := MainAxisAlignment(n.Props.Int("MainAxisAlignment"))
	crossAxis := CrossAxisAlignment(n.Props.Int("CrossAxisAlignment"))
	mainMax, crossMax := axisMax(c, false)

	// 每个子都收到有界的主轴约束（交叉轴 0..crossMax），量出内容尺寸。
	kids := make([]flexKid, 0, len(n.Children))
	for _, ch := range n.Children {
		cc := BoxConstraints{MinW: 0, MaxW: crossMax, MinH: 0, MaxH: mainMax}
		sz := layoutTree(ch, r, cc, pos, d)
		kids = append(kids, flexKid{node: ch, size: sz})
	}

	mainUsed := totalUsed(kids, false)
	leftover := max(0, mainMax-mainUsed)
	lead, between := mainDistribution(mainAxis, leftover, len(kids))

	crossExtent := 0
	if crossAxis == CrossAxisStretch {
		crossExtent = crossMax
	} else {
		for _, k := range kids {
			if w := k.size.W; w > crossExtent {
				crossExtent = w
			}
		}
	}

	cursor := lead
	for i, k := range kids {
		kp := pos
		kp.Y += cursor
		kp.X += crossOffset(crossAxis, k.size.W, crossExtent)
		setPos(kids[i].node, kp)
		cursor += k.size.H + between
	}

	sz := c.Constrain(crossExtent, mainUsed)
	setBounds(n, pos, sz)

	ow, oh := 0, 0
	if mainUsed > mainMax {
		oh = mainUsed - mainMax
	}
	if crossExtent > crossMax {
		ow = crossExtent - crossMax
	}
	d.overflow(n, ow, oh)
	return sz
}

// layoutFlex 实现 RenderFlex 算法（research.md §5.1）。isRow 时主轴为宽。
//
// 流程：
//  1. 非 flex 子用主轴 unbounded、交叉轴（stretch 时）tight 约束测量，累加主轴；
//  2. freeSpace = max(0, maxMain - usedMain)，spacePerFlex = freeSpace/totalFlex，
//     Expanded（tight）强制填满分配空间，Flexible（loose）允许更小；
//  3. 主轴按 MainAxisAlignment 分布（start/center/end/spaceBetween/Around/Evenly）；
//  4. 交叉轴按 CrossAxisAlignment 对齐（stretch 时子已填满）。
//
// 只增不缩：子控件恒 intrinsic，容器超约束时钳制自身、不压缩子，溢出记诊断。
func layoutFlex(n *Node, r render.Renderer, c BoxConstraints, pos Point, d *layoutDiags, isRow bool) Size {
	mainAxis := MainAxisAlignment(n.Props.Int("MainAxisAlignment"))
	crossAxis := CrossAxisAlignment(n.Props.Int("CrossAxisAlignment"))
	mainMax, crossMax := axisMax(c, isRow)

	kids, totalFlex := collectKids(n, r, c, pos, d, isRow, crossAxis, crossMax)

	// Phase 2：flex 空间分配（Expanded=tight / Flexible=loose）
	if totalFlex > 0 {
		freeSpace := 0
		if mainMax >= 0 {
			freeSpace = max(0, mainMax-totalUsed(kids, isRow))
		}
		perFlex := freeSpace / totalFlex
		for i := range kids {
			if !kids[i].isFlex {
				continue
			}
			alloc := perFlex * kids[i].flex
			childC := flexConstraints(alloc, isRow, crossAxis, crossMax, kids[i].node.Type == "Expanded")
			kids[i].size = layoutTree(kids[i].node, r, childC, pos, d)
		}
	}

	// 主轴对齐（leftover 为 flex 未吸收的剩余空间）
	mainUsed := totalUsed(kids, isRow)
	leftover := 0
	if mainMax >= 0 {
		leftover = max(0, mainMax-mainUsed)
	}
	lead, between := mainDistribution(mainAxis, leftover, len(kids))

	// 交叉轴范围：stretch 时为约束值，否则为子最大值
	crossExtent := 0
	if crossAxis == CrossAxisStretch && crossMax >= 0 {
		crossExtent = crossMax
	} else {
		for _, k := range kids {
			if kc := axis(k.size, isRow, false); kc > crossExtent {
				crossExtent = kc
			}
		}
	}

	// 布局：按主轴游标 + 交叉轴对齐写每个子的绝对位置
	cursor := lead
	for i, k := range kids {
		kp := pos
		if isRow {
			kp.X += cursor
			kp.Y += crossOffset(crossAxis, axis(k.size, isRow, false), crossExtent)
		} else {
			kp.Y += cursor
			kp.X += crossOffset(crossAxis, axis(k.size, isRow, false), crossExtent)
		}
		setPos(kids[i].node, kp)
		cursor += axis(k.size, isRow, true) + between
	}

	// 容器尺寸 + 溢出诊断
	var sz Size
	if isRow {
		sz = c.Constrain(mainUsed, crossExtent)
	} else {
		sz = c.Constrain(crossExtent, mainUsed)
	}
	setBounds(n, pos, sz)

	ow, oh := 0, 0
	if mainMax >= 0 && mainUsed > mainMax {
		if isRow {
			ow = mainUsed - mainMax
		} else {
			oh = mainUsed - mainMax
		}
	}
	if crossMax >= 0 && crossExtent > crossMax {
		if isRow {
			oh = crossExtent - crossMax
		} else {
			ow = crossExtent - crossMax
		}
	}
	d.overflow(n, ow, oh)

	return sz
}

// collectKids 识别 flex/非 flex 子：非 flex 子立即测量（主轴 unbounded），
// flex 子只登记 flex 因子（尺寸在 Phase 2 分配后测量）。返回 kids 与 totalFlex。
func collectKids(n *Node, r render.Renderer, c BoxConstraints, pos Point, d *layoutDiags, isRow bool, crossAxis CrossAxisAlignment, crossMax int) ([]flexKid, int) {
	kids := make([]flexKid, 0, len(n.Children))
	totalFlex := 0
	for _, ch := range n.Children {
		if f := flexFactor(ch); f > 0 {
			kids = append(kids, flexKid{node: ch, flex: f, isFlex: true})
			totalFlex += f
			continue
		}
		childC := childConstraints(c, isRow, crossAxis, crossMax)
		sz := layoutTree(ch, r, childC, pos, d)
		kids = append(kids, flexKid{node: ch, size: sz})
	}
	return kids, totalFlex
}

// flexFactor 返回节点的 flex 因子（Expanded/Flexible 包装节点读取 Props["Flex"]）。
func flexFactor(n *Node) int {
	if n.Type == "Expanded" || n.Type == "Flexible" {
		return n.Props.Int("Flex")
	}
	return 0
}

// axisMax 取约束在主轴/交叉轴的 Max（主轴在前）。
func axisMax(c BoxConstraints, isRow bool) (int, int) {
	if isRow {
		return c.MaxW, c.MaxH
	}
	return c.MaxH, c.MaxW
}

// axis 取 Size 的主轴（main=true）或交叉轴分量。
func axis(s Size, isRow, main bool) int {
	if isRow {
		if main {
			return s.W
		}
		return s.H
	}
	if main {
		return s.H
	}
	return s.W
}

// totalUsed 计算 kids 主轴尺寸和（含基础间距）。
func totalUsed(kids []flexKid, isRow bool) int {
	main := 0
	for _, k := range kids {
		main += axis(k.size, isRow, true)
	}
	if len(kids) > 1 {
		main += (len(kids) - 1) * layoutGap
	}
	return main
}

// childConstraints 非 flex 子的约束：主轴 unbounded、交叉轴按对齐
// （stretch 时 tight，否则 loose 到 crossMax）。crossMax 无界时交叉轴也 unbounded。
func childConstraints(c BoxConstraints, isRow bool, crossAxis CrossAxisAlignment, crossMax int) BoxConstraints {
	stretch := crossAxis == CrossAxisStretch && crossMax >= 0
	if isRow {
		if stretch {
			return BoxConstraints{MinW: 0, MaxW: unbounded, MinH: crossMax, MaxH: crossMax}
		}
		return BoxConstraints{MinW: 0, MaxW: unbounded, MinH: 0, MaxH: crossMax}
	}
	if stretch {
		return BoxConstraints{MinW: crossMax, MaxW: crossMax, MinH: 0, MaxH: unbounded}
	}
	return BoxConstraints{MinW: 0, MaxW: crossMax, MinH: 0, MaxH: unbounded}
}

// flexConstraints flex 子的约束：主轴 Tight(alloc)（Expanded）或 Loose(alloc)
// （Flexible，Min=0 允许更小）；交叉轴同非 flex 子。
func flexConstraints(alloc int, isRow bool, crossAxis CrossAxisAlignment, crossMax int, tight bool) BoxConstraints {
	stretch := crossAxis == CrossAxisStretch && crossMax >= 0
	if isRow {
		minW := alloc
		if !tight {
			minW = 0
		}
		cc := BoxConstraints{MinW: minW, MaxW: alloc, MinH: 0, MaxH: crossMax}
		if stretch {
			cc.MinH, cc.MaxH = crossMax, crossMax
		}
		return cc
	}
	minH := alloc
	if !tight {
		minH = 0
	}
	cc := BoxConstraints{MinW: 0, MaxW: crossMax, MinH: minH, MaxH: alloc}
	if stretch {
		cc.MinW, cc.MaxW = crossMax, crossMax
	}
	return cc
}

// mainDistribution 按主轴对齐方式返回起始偏移 lead 与子间距 between。
func mainDistribution(a MainAxisAlignment, leftover, n int) (lead, between int) {
	if n <= 1 {
		switch a {
		case MainAxisCenter:
			return leftover / 2, 0
		case MainAxisEnd:
			return leftover, 0
		default:
			return 0, 0
		}
	}
	switch a {
	case MainAxisCenter:
		return leftover / 2, layoutGap
	case MainAxisEnd:
		return leftover, layoutGap
	case MainAxisSpaceBetween:
		per := 0
		if n > 1 {
			per = leftover / (n - 1)
		}
		return 0, layoutGap + per
	case MainAxisSpaceAround:
		per := leftover / (2 * n)
		return per, layoutGap + 2*per
	case MainAxisSpaceEvenly:
		per := leftover / (n + 1)
		return per, layoutGap + per
	default: // MainAxisStart
		return 0, layoutGap
	}
}

// crossOffset 计算子控件在交叉轴范围 extent 内的偏移（Start/Stretch=0）。
func crossOffset(a CrossAxisAlignment, size, extent int) int {
	switch a {
	case CrossAxisCenter:
		return max(0, (extent-size)/2)
	case CrossAxisEnd:
		return max(0, extent-size)
	default: // Start / Stretch（stretch 时 size==extent）
		return 0
	}
}
