package flux

import (
	"fmt"
	"strings"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
	"github.com/xiaowumin-mark/flux-vcl/internal/widget"
)

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

// listScrollbarStrip 是 ListView 为可视滚动条预留的占位宽度（DIP）。
// 行内容宽度 = 视口宽 − 该占位，避免内容落到滚动条之下（Phase 6）。
const listScrollbarStrip = 17

const (
	pageControlDefaultWidth  = 320
	pageControlDefaultHeight = 220
	pageControlChromeWidth   = 8
	pageControlChromeHeight  = 32
)

// LayoutDiag 是一次 render 布局的溢出诊断（Phase 3.7 inspector 数据源之一）。
// OverflowW/H 为容器尺寸超出约束的量（0 表示该轴未溢出）。
type LayoutDiag struct {
	Type      string
	Key       string
	Path      string
	OverflowW int
	OverflowH int
}

// NodeDiag 是布局引擎对每个节点的诊断（Phase 3.7 inspector 数据源）：
// 节点收到的 Constraints、布局出的 Size、最终 Frame（Props["Bounds"]，DIP）
// 与 flex 因子（Expanded/Flexible 才 >0）。与 LayoutDiag（仅溢出）互补。
type NodeDiag struct {
	Type, Key   string
	Path        string
	Constraints BoxConstraints
	Size        Size
	Frame       render.Rect
	Flex        int
}

// layoutDiags 收集布局诊断（App 每次 render 新建，布局后读取）。
type layoutDiags struct {
	list          []LayoutDiag // 溢出诊断（LastLayoutDiags）
	overflowNodes []*Node
	nodes         []NodeDiag // 全节点诊断（Inspect）
	err           error
}

func (d *layoutDiags) overflow(n *Node, ow, oh int) {
	if d == nil || (ow <= 0 && oh <= 0) {
		return
	}
	d.list = append(d.list, LayoutDiag{Type: n.Type, Key: n.Key, OverflowW: ow, OverflowH: oh})
	d.overflowNodes = append(d.overflowNodes, n)
}

// record 收集一个节点的布局诊断。Frame 留空，由 finalize 在整棵布局完成后
// 统一填最终值（父容器 setPos 平移子树发生在递归内，record 时点可能早于平移）。
func (d *layoutDiags) record(n *Node, c BoxConstraints, sz Size) {
	if d == nil {
		return
	}
	d.nodes = append(d.nodes, NodeDiag{
		Type: n.Type, Key: n.Key,
		Constraints: c, Size: sz,
		Flex: flexFactor(n),
	})
}

// finalize 在布局完成后按后序（与 record 同序）回填每个节点的最终 Frame。
// 后序遍历与 layoutTree 的 record 时机（子先于父）严格一致。
func (d *layoutDiags) finalize(root *Node) {
	if d == nil {
		return
	}
	idx := 0
	paths := make(map[*Node]string)
	var walk func(n *Node, path string)
	walk = func(n *Node, path string) {
		paths[n] = path
		for i, c := range n.Children {
			walk(c, path+"/"+fmt.Sprint(i)+"/"+c.Type)
		}
		if idx < len(d.nodes) {
			d.nodes[idx].Path = path
			if b, ok := n.Props.Get("Bounds"); ok {
				d.nodes[idx].Frame = b.(render.Rect)
			}
			idx++
		}
	}
	walk(root, root.Type)
	for i, node := range d.overflowNodes {
		if i < len(d.list) {
			d.list[i].Path = paths[node]
		}
	}
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
	if d != nil && d.err != nil {
		return Size{}
	}
	var sz Size
	if runtime, ok := pluginRuntimeForNode(n); ok {
		sz = layoutPlugin(n, r, c, pos, d, runtime)
		d.record(n, c, sz)
		return sz
	}
	switch n.Type {
	case "Window":
		cw, ch := r.ClientSize()
		if cw <= 0 {
			cw = 400
		}
		if ch <= 0 {
			ch = 300
		}
		sz = layoutRoot(n, r, Tight(cw, ch), pos, d)
	case "Row":
		sz = layoutFlex(n, r, c, pos, d, true)
	case "Column":
		sz = layoutFlex(n, r, c, pos, d, false)
	case "Expanded", "Flexible":
		// 父容器已按 flex 语义算好约束 c，此处原样传给唯一子（tight/loose 已定）。
		sz = layoutTree(n.Children[0], r, c, pos, d)
		setBounds(n, pos, sz)
	case "Component":
		// 透明组件（Phase 5.4）：单子 passthrough —— 同一约束、同一位置传给
		// build() 产物，自身尺寸 = 子尺寸（Component 自身 Bounds 与子一致）。
		if len(n.Children) > 0 {
			sz = layoutTree(n.Children[0], r, c, pos, d)
		} else {
			sz = leafSize(0, 0, n, c)
		}
		setBounds(n, pos, sz)
	case "Text":
		w, h := multilineTextExtent(n.Props.String("Text"), r)
		sz = leafSize(w, h, n, c)
		setBounds(n, pos, sz)
	case "Button":
		w, _ := r.TextExtent(n.Props.String("Text"))
		bw := w + 32 // 左右 padding
		if bw < 88 {
			bw = 88
		}
		sz = leafSize(bw, 32, n, c)
		setBounds(n, pos, sz)
	case "CheckBox", "RadioButton":
		w, h := checkableIntrinsicSize(n.Props.String("Text"), r)
		sz = leafSize(w, h, n, c)
		setBounds(n, pos, sz)
	case "Input":
		sz = leafSize(180, 28, n, c)
		setBounds(n, pos, sz)
	case "Memo":
		w, h := memoIntrinsicSize(n.Props.String("Text"), r)
		sz = leafSize(w, h, n, c)
		setBounds(n, pos, sz)
	case "ComboBox":
		w, h := comboBoxIntrinsicSize(comboBoxItems(n), r)
		sz = leafSize(w, h, n, c)
		setBounds(n, pos, sz)
	case "ProgressBar":
		sz = leafSize(180, 20, n, c)
		setBounds(n, pos, sz)
	case "PageControl":
		sz = layoutPageControl(n, r, c, pos, d)
	case "TabPage":
		// TabPage 只能由 PageControl 布局；独立出现时安全退化为零尺寸，避免把
		// 页面内容错误地放进窗体坐标系。公开构造器已保证唯一子树。
		sz = leafSize(0, 0, n, c)
		setBounds(n, pos, sz)
	case "ScrollBox":
		sz = layoutScrollBox(n, r, c, pos, d)
	case "ListView":
		sz = layoutListView(n, r, c, pos, d)
	case "ListViewRow":
		// 虚拟列表行（Phase 6）：透明包装（diff 不建句柄、子挂祖父），身份靠
		// slot key（控件池槽位）；约束/位置原样传给唯一子（builder 产物）。
		if len(n.Children) > 0 {
			sz = layoutTree(n.Children[0], r, c, pos, d)
		} else {
			sz = leafSize(0, 0, n, c)
		}
		setBounds(n, pos, sz)
	default: // 未知类型（含第三方控件）：默认尺寸
		sz = leafSize(100, 32, n, c)
		setBounds(n, pos, sz)
	}
	d.record(n, c, sz)
	return sz
}

func pluginRuntimeForNode(n *Node) (*pluginRuntime, bool) {
	if n == nil || n.Props == nil {
		return nil, false
	}
	value, exists := n.Props.Get("_pluginRuntime")
	if !exists {
		return nil, false
	}
	runtime, ok := value.(*pluginRuntime)
	return runtime, ok && runtime != nil
}

func layoutPlugin(n *Node, r render.Renderer, c BoxConstraints, pos Point, d *layoutDiags, runtime *pluginRuntime) Size {
	if len(n.Children) != 1 {
		if d != nil {
			d.err = &PluginError{Name: runtime.name, Stage: "measure", Err: fmt.Errorf("%w: builder 必须产生唯一子树", ErrPluginInvalid)}
		}
		return Size{}
	}
	child := n.Children[0]
	childSize := layoutTree(child, r, c, pos, d)
	if d != nil && d.err != nil {
		return Size{}
	}
	layout := PluginLayout{Size: childSize}
	if runtime.descriptor.Measure != nil {
		ctx := PluginMeasureContext{
			PluginContext: runtime.context(),
			Type:          runtime.name, Key: n.Key,
			Properties: pluginPropertiesFromProps(n.Props), Constraints: c, ChildSize: childSize,
		}
		if err := pluginCall(runtime.name, "measure", func() error {
			var measureErr error
			layout, measureErr = runtime.descriptor.Measure(ctx)
			return measureErr
		}); err != nil {
			if d != nil {
				d.err = err
			}
			return Size{}
		}
	}
	w, h := layout.Size.W, layout.Size.H
	if value := n.Props.Int("Width"); value > 0 {
		w = value
	}
	if value := n.Props.Int("Height"); value > 0 {
		h = value
	}
	sz := c.Constrain(w, h)
	setPos(child, Point{X: pos.X + layout.ChildOffset.X, Y: pos.Y + layout.ChildOffset.Y})
	setBounds(n, pos, sz)
	return sz
}

// checkableIntrinsicSize 为 CheckBox（后续 RadioButton 复用）预留稳定的标签尺寸。
// 选中状态不影响布局；Width/Height Opt 与外部约束由调用方的 leafSize 处理。
func checkableIntrinsicSize(text string, r render.Renderer) (int, int) {
	w, h := r.TextExtent(text)
	w += 28 // 指示器 16 DIP + 标签间距 4 DIP + 两侧 padding 8 DIP
	if w < 32 {
		w = 32
	}
	if h < 24 {
		h = 24
	}
	return w, h
}

// multilineTextExtent 按显式换行逐行测量普通 Text 与 Memo，避免 Renderer 的
// 单行 TextExtent 把换行当普通字符，导致声明高度小于原生控件实际绘制高度。
func multilineTextExtent(text string, r render.Renderer) (int, int) {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	lines := strings.Split(strings.ReplaceAll(text, "\r", "\n"), "\n")
	maxW, totalH := 0, 0
	for _, line := range lines {
		w, h := r.TextExtent(line)
		if w > maxW {
			maxW = w
		}
		totalH += h
	}
	return maxW, totalH
}

// memoIntrinsicSize 按显式换行逐行量取 Memo 文本，避免后端将换行当普通字符。
// 编辑区至少保留 180×96 DIP；Width/Height Opt 与外部约束由调用方的 leafSize 处理。
func memoIntrinsicSize(text string, r render.Renderer) (int, int) {
	maxW, totalH := multilineTextExtent(text, r)
	if maxW < 180 {
		maxW = 180
	}
	if totalH < 96 {
		totalH = 96
	}
	return maxW, totalH
}

// comboBoxItems 返回已由 ComboBox 构造器规范化的 Items；手写 Node 的空值安全退化。
func comboBoxItems(n *Node) []string {
	items, _ := n.Props.Get("Items")
	values, _ := items.([]string)
	return values
}

// comboBoxIntrinsicSize 测量全部选项中最长的字符串，避免切换选中项导致宽度跳动。
func comboBoxIntrinsicSize(items []string, r render.Renderer) (int, int) {
	maxW, h := 0, 0
	if len(items) == 0 {
		maxW, h = r.TextExtent("")
	}
	for _, item := range items {
		w, itemH := r.TextExtent(item)
		if w > maxW {
			maxW = w
		}
		if itemH > h {
			h = itemH
		}
	}
	maxW += 36 // 下拉箭头 16 DIP + 内边距/边框 20 DIP
	if maxW < 100 {
		maxW = 100
	}
	if h < 28 {
		h = 28
	}
	return maxW, h
}

// layoutPageControl 以稳定 DIP 预算扣除原生页签表头与边框。各 TabPage 的唯一
// 子树都使用相同的紧客户区约束，并从页面自己的 (0,0) 客户区原点开始；inactive
// 页同样参与布局和保留，不通过 Visible/TabVisible 卸载。
func layoutPageControl(n *Node, r render.Renderer, c BoxConstraints, pos Point, d *layoutDiags) Size {
	sz := leafSize(pageControlDefaultWidth, pageControlDefaultHeight, n, c)
	contentW := max(0, sz.W-pageControlChromeWidth)
	contentH := max(0, sz.H-pageControlChromeHeight)
	pageConstraints := Tight(contentW, contentH)
	for _, page := range n.Children {
		if page == nil || page.Type != "TabPage" || len(page.Children) != 1 || page.Children[0] == nil {
			continue
		}
		child := page.Children[0]
		childSize := layoutTree(child, r, pageConstraints, Point{}, d)
		setBounds(page, Point{}, childSize)
		d.record(page, pageConstraints, childSize)
	}
	setBounds(n, pos, sz)
	return sz
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

// setPos 把节点定位到绝对位置 pos。
//
// 透明容器（Column/Row/Expanded/Flexible，diff 不建句柄、子挂祖父）平移整棵子树
// 保持内部相对结构；叶控件与真实容器（Window/ScrollBox）只定位自身 —— 真实容器
// 的子树坐标已相对其客户区（局部坐标空间），不能被父级平移破坏。
func setPos(n *Node, pos Point) {
	b, ok := n.Props.Get("Bounds")
	if !ok {
		return
	}
	br := b.(render.Rect)
	switch n.Type {
	case "Column", "Row", "Expanded", "Flexible", "Component", "ListViewRow":
		offsetSubtree(n, pos.X-br.X, pos.Y-br.Y)
	default:
		if _, plugin := pluginRuntimeForNode(n); plugin {
			offsetSubtree(n, pos.X-br.X, pos.Y-br.Y)
		} else {
			br.X, br.Y = pos.X, pos.Y
			n.Props.Set("Bounds", br)
		}
	}
}

// realContainer 报告类型是否为真实容器（拥有原生句柄）：其子树坐标相对自身
// 客户区（局部坐标空间），平移子树时在边界停止下钻（diff 层子控件 SetParent 挂
// 自身，SetBounds 需相对自身）。ListView（Phase 6）同为真实容器：行 slot 在
// 局部坐标空间布局（0..vh），父级定位只平移 ListView 自身 Bounds。
func realContainer(t string) bool {
	return t == "Window" || t == "ScrollBox" || t == "ListView" ||
		t == "PageControl" || t == "TabPage"
}

// offsetSubtree 平移节点及其子树的 Bounds（dx/dy）。遇真实容器边界停止 ——
// 其子树已是相对该容器的局部坐标，不应随外部坐标平移。
func offsetSubtree(n *Node, dx, dy int) {
	if b, ok := n.Props.Get("Bounds"); ok {
		br := b.(render.Rect)
		br.X += dx
		br.Y += dy
		n.Props.Set("Bounds", br)
	}
	if realContainer(n.Type) {
		return
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

// layoutScrollBox 布局垂直滚动容器（Phase 3.6，SingleChildScrollView 语义）。
//
// 内容（单子）用 {交叉轴 0..crossMax, 滚动轴(高) unbounded} 约束测量 → 内容总高
// contentH；自身 = viewport = c.Constrain(contentW, contentH)：内容超高被约束钳制
// （原生 TScrollBox 滚动条出现）、内容偏矮则收缩到内容（自适应）。内容在局部坐标
// 空间布局（pos=(0,0)，相对 ScrollBox 客户区原点 —— 原生 SetBounds 相对父）；父级
// 定位 ScrollBox 自身只平移其 Bounds（setPos 真实容器分支），内容不被破坏。
// 滚动轴溢出不记诊断（滚动是目的），交叉轴溢出记。
func layoutScrollBox(n *Node, r render.Renderer, c BoxConstraints, pos Point, d *layoutDiags) Size {
	_, crossMax := axisMax(c, false)
	contentW, contentH := 0, 0
	if len(n.Children) > 0 {
		cc := BoxConstraints{MinW: 0, MaxW: crossMax, MinH: 0, MaxH: unbounded}
		cs := layoutTree(n.Children[0], r, cc, Point{}, d) // 局部坐标：相对客户区原点
		contentW, contentH = cs.W, cs.H
	}
	sz := c.Constrain(contentW, contentH)
	setBounds(n, pos, sz)
	ow := 0
	if crossMax >= 0 && contentW > crossMax {
		ow = contentW - crossMax
	}
	d.overflow(n, ow, 0) // 滚动轴（垂直）永不记溢出
	return sz
}

// layoutListView 布局虚拟滚动列表（Phase 6 / design.md §16）。
//
// 虚拟化核心：只把"可见区 ± overscan"的数据行构建为 slot 子节点（控件池），
// diff 按 slot key（row-0..row-N）复用原生控件、属性级 patch 内容 —— 滚动时
// 已存在的 slot 原地更新（SetText/SetBounds），不创建/销毁控件（内存有界、
// 焦点/IME 不漂移）。行内容（builder 产物）挂在 slot 下，随 slot 一起 diff。
//
// 滚动位置：读取 ScrollTarget.Current()（DIP）→ 钳制到 [0, maxOffset] → 由
// 可见范围反推 slot 集合。写 ScrollConfig（内容总高/滚轮步长）与 ScrollPos
// 供 diff 应用（原生滚动条范围/位置）。
func layoutListView(n *Node, r render.Renderer, c BoxConstraints, pos Point, d *layoutDiags) Size {
	if c.IsUnboundedW() || c.IsUnboundedH() {
		panic("flux.ListView: 需要有界的宽高约束（虚拟列表必须有 viewport，请放在 Expanded/固定尺寸容器内）")
	}
	vw, vh := c.MaxW, c.MaxH
	if vw < 0 {
		vw = 0
	}
	if vh < 0 {
		vh = 0
	}
	count := n.Props.Int("ItemCount")
	itemH := n.Props.Int("ItemHeight")
	if count < 0 {
		count = 0
	}
	if itemH <= 0 {
		itemH = 24
	}
	contentH := count * itemH

	// 滚动位置（DIP）：读 ScrollTarget，钳制到 [0, maxOffset]。
	// 钳制结果回写 State（st.Apply，仅实际变化时）→ 触发一次 re-render 收敛：
	// State 始终与有效滚动位置一致（scroll.Get() 与原生滚动条读数不漂移）。
	// 值已合法时零 Apply → 无额外 render（D7c 兼容）。inRender 重入防护把该
	// 回写引发的 re-render 排队到当前 render 结束后 flush（无递归）。
	offset := 0
	maxOffset := contentH - vh
	if maxOffset < 0 {
		maxOffset = 0
	}
	if v, ok := n.Props.Get("Scroll"); ok {
		if st, ok := v.(render.ScrollTarget); ok {
			offset = st.Current()
			if offset < 0 {
				offset = 0
			}
			if offset > maxOffset {
				offset = maxOffset
			}
			if offset != st.Current() {
				st.Apply(offset)
			}
		}
	}

	// 可见范围（含 overscan 缓冲）：滚动时内容不立即换槽，仅平移/局部 patch
	const overscan = 3
	first, last := 0, -1
	if count > 0 {
		first = offset/itemH - overscan
		if first < 0 {
			first = 0
		}
		last = (offset+vh+itemH-1)/itemH - 1 + overscan
		if last >= count {
			last = count - 1
		}
	}

	// 构建可见区 slot 子节点（控件池：slot key = row-i，i 为槽位，非数据下标）。
	// 槽位稳定 → 滚动复用同一批原生控件，内容随槽位内 builder(index) 更新。
	bv, _ := n.Props.Get("Builder")
	builder := bv.(func(int) Widget)
	kids := make([]*Node, 0, last-first+1)
	for i, idx := 0, first; idx <= last; i, idx = i+1, idx+1 {
		slot := widget.NewNode("ListViewRow")
		slot.Key = fmt.Sprintf("row-%d", i) // 控件池槽位身份（D3）
		slot.Add(builder(idx).Create())
		kids = append(kids, slot)
	}
	n.Children = kids

	// 行宽 = 视口宽 − 滚动条占位；每行 tight (contentW, itemH)，局部坐标布局
	contentW := vw - listScrollbarStrip
	if contentW < 0 {
		contentW = 0
	}
	for i, idx := 0, first; idx <= last; i, idx = i+1, idx+1 {
		cc := BoxConstraints{MinW: contentW, MaxW: contentW, MinH: itemH, MaxH: itemH}
		layoutTree(kids[i], r, cc, Point{X: 0, Y: idx*itemH - offset}, d)
	}

	// 写滚动信息属性（diff 按值变化 patch；renderer 未实现 Scrollable 时忽略）
	n.Props.Set("ScrollConfig", render.ScrollConfig{Content: contentH, Step: 3 * itemH})
	n.Props.Set("ScrollPos", offset)

	setBounds(n, pos, Size{W: vw, H: vh})
	return Size{W: vw, H: vh}
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
