package flux_test

import (
	"fmt"
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// —— 3.6 滚动容器（ScrollBox） ——

// scrollContent 构造 20 行文本的滚动内容（mock TextExtent = len*8 x 20，行高 20）。
// 内容总高 = 20*20 + 19*4 = 476，超 Mock 客户区高 300。
func scrollContent() flux.Widget {
	var kids []any
	for i := range 20 {
		kids = append(kids, flux.Text(fmt.Sprintf("item %d", i), flux.Key(fmt.Sprintf("si%d", i))))
	}
	return flux.Column(kids...)
}

// TestScrollBoxContentUnbounded 滚动内容在滚动轴（垂直）用 unbounded 约束测量：
// 20 行（高 476）超 viewport 后 ScrollBox 自身钳制到约束高 300、出现滚动语义，
// 内容行不压缩、末行 Y 超出 ScrollBox 底部（由原生 TScrollBox 滚动裁切）。
func TestScrollBoxContentUnbounded(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.ScrollBox(scrollContent())))

	elScroll := app.Root().Children[0] // Window 直接子 = ScrollBox
	if got := boundsOf(elScroll).H; got != 300 {
		t.Errorf("ScrollBox.H = %d，期望 300（viewport 钳制到 Window 客户区高）", got)
	}

	// 内容行不压缩：首行/末行高度仍 20
	if got := boundsOf(findByKey(t, app.Root(), "si0")).H; got != 20 {
		t.Errorf("首行 H = %d，期望 20（滚动内容不压缩）", got)
	}
	if got := boundsOf(findByKey(t, app.Root(), "si19")).H; got != 20 {
		t.Errorf("末行 H = %d，期望 20（滚动内容不压缩）", got)
	}

	// 末行 Y 超出 ScrollBox 底部（内容坐标相对 ScrollBox 客户区原点）
	// 20 行：行高 20*20 + 间距 4*19 = 476；末行 Y = 19*20 + 19*4 = 456
	if got := boundsOf(findByKey(t, app.Root(), "si19")).Y; got != 456 {
		t.Errorf("末行 Y = %d，期望 456（内容总高 476 超 viewport 300）", got)
	}
}

// TestScrollBoxShrinksToContent 内容小于约束时 ScrollBox 收缩到内容尺寸（自适应）。
func TestScrollBoxShrinksToContent(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.ScrollBox(flux.Column(flux.Text("hi", flux.Key("hi"))))))

	elScroll := app.Root().Children[0] // Window 直接子 = ScrollBox
	if got := boundsOf(elScroll).H; got != 20 {
		t.Errorf("ScrollBox.H = %d，期望 20（内容偏矮收缩到内容）", got)
	}
	if got := boundsOf(elScroll).W; got != 16 { // "hi" len 2 → 2*8
		t.Errorf("ScrollBox.W = %d，期望 16", got)
	}
}

// TestScrollBoxNoOverflowDiag 滚动吸收溢出：内容超高不进 LastLayoutDiags
// （与 flex 容器溢出诊断不同 —— 滚动是目的，不是 overflow）。
func TestScrollBoxNoOverflowDiag(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.ScrollBox(scrollContent())))

	if diags := app.LastLayoutDiags(); len(diags) != 0 {
		t.Errorf("LastLayoutDiags = %+v，期望空（滚动吸收溢出）", diags)
	}
}

// TestScrollBoxCreatesHandle ScrollBox 是非透明真实控件：mock 产生 Create op
// （对照透明容器 Column/Row/Expanded 不建句柄）。
func TestScrollBoxCreatesHandle(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.ScrollBox(flux.Column(flux.Text("x", flux.Key("x"))))))

	ops := m.Ops()
	// Window + ScrollBox + Text = 3 个 Create（Column 透明不建）
	if got := countOps(ops, render.OpCreate); got != 3 {
		t.Errorf("Create 次数 = %d，期望 3（Window/ScrollBox/Text；Column 透明）", got)
	}
	found := false
	for _, op := range ops {
		if op.Type == render.OpCreate && op.Key == "ScrollBox" {
			found = true
			break
		}
	}
	if !found {
		t.Error("缺失 ScrollBox 的 Create op（应映射原生 TScrollBox 句柄）")
	}
}

// TestScrollBoxContentCoordsRelative ScrollBox 位于非原点时，内容子 Bounds 相对
// ScrollBox 客户区原点（0,0）而非窗体绝对坐标 —— 原生 SetBounds 相对父。
func TestScrollBoxContentCoordsRelative(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	// Column：Text("pad") 高 20 + gap 4 → ScrollBox 绝对 Y=24
	app.Render(flux.Window(flux.Column(
		flux.Text("pad", flux.Key("pad")),
		flux.ScrollBox(flux.Column(flux.Text("in", flux.Key("sin")))),
	)))

	elScroll := app.Root().Children[0].Children[1] // Window>Column 的第二个子 = ScrollBox
	if got := boundsOf(elScroll).Y; got != 24 {
		t.Errorf("ScrollBox.Y = %d，期望 24（pad 20 + gap 4）", got)
	}
	if got := boundsOf(findByKey(t, app.Root(), "sin")).Y; got != 0 {
		t.Errorf("内容子 Y = %d，期望 0（相对 ScrollBox 客户区原点）", got)
	}
	if got := boundsOf(findByKey(t, app.Root(), "sin")).X; got != 0 {
		t.Errorf("内容子 X = %d，期望 0", got)
	}
}

// —— 3.7 布局调试（Inspect） ——

// findNodeDiag 在 Inspect() 结果中按 Type/Key 查节点诊断。
func findNodeDiag(t *testing.T, nodes []flux.NodeDiag, typ, key string) (flux.NodeDiag, bool) {
	t.Helper()
	for _, nd := range nodes {
		if nd.Type == typ && nd.Key == key {
			return nd, true
		}
	}
	return flux.NodeDiag{}, false
}

// TestInspectCollectsNodeInfo Inspect() 提供每个节点的 constraints/size/frame/flex
// 因子（Phase 3.7 inspector 数据源）。
func TestInspectCollectsNodeInfo(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.Column(
		flux.Text("A", flux.Key("a")),
		flux.Expanded(flux.Button("B", flux.Key("b"))),
	)))

	nodes := app.Inspect()
	if len(nodes) == 0 {
		t.Fatal("Inspect() 为空，应有节点诊断")
	}

	// Window：约束为 App 传入的 Tight(client)，尺寸即客户区。
	win, ok := findNodeDiag(t, nodes, "Window", "")
	if !ok {
		t.Fatal("缺 Window 节点诊断")
	}
	if win.Size != (flux.Size{W: 400, H: 300}) {
		t.Errorf("Window.Size = %+v，期望 400x300", win.Size)
	}
	if win.Frame != (render.Rect{X: 0, Y: 0, W: 400, H: 300}) {
		t.Errorf("Window.Frame = %+v", win.Frame)
	}

	// Column：layoutRoot 给的子约束 {0,400,0,300}。
	col, ok := findNodeDiag(t, nodes, "Column", "")
	if !ok {
		t.Fatal("缺 Column 节点诊断")
	}
	if col.Constraints != (flux.BoxConstraints{MinW: 0, MaxW: 400, MinH: 0, MaxH: 300}) {
		t.Errorf("Column.Constraints = %+v，期望 {0,400,0,300}", col.Constraints)
	}

	// Text A：非 flex 子约束（交叉轴 loose、主轴 unbounded），intrinsic 尺寸。
	ta, ok := findNodeDiag(t, nodes, "Text", "a")
	if !ok {
		t.Fatal("缺 Text a 节点诊断")
	}
	if ta.Flex != 0 {
		t.Errorf("Text a.Flex = %d，期望 0（非 flex）", ta.Flex)
	}
	if !ta.Constraints.IsUnboundedH() {
		t.Errorf("Text a.Constraints = %+v，期望主轴(高) unbounded", ta.Constraints)
	}
	if ta.Size != (flux.Size{W: 8, H: 20}) { // "A" len 1 → 1*8
		t.Errorf("Text a.Size = %+v，期望 8x20", ta.Size)
	}
	if ta.Frame != (render.Rect{X: 0, Y: 0, W: 8, H: 20}) {
		t.Errorf("Text a.Frame = %+v", ta.Frame)
	}

	// Expanded：flex 因子记录（flex 分配后 tight 约束。freeSpace 含 gap：
	// 300-20-4 = 276 → {0,400,276,276}，主轴 tight）。
	ex, ok := findNodeDiag(t, nodes, "Expanded", "")
	if !ok {
		t.Fatal("缺 Expanded 节点诊断")
	}
	if ex.Flex != 1 {
		t.Errorf("Expanded.Flex = %d，期望 1", ex.Flex)
	}
	if ex.Constraints != (flux.BoxConstraints{MinW: 0, MaxW: 400, MinH: 276, MaxH: 276}) {
		t.Errorf("Expanded.Constraints = %+v，期望 {0,400,276,276}（freeSpace 含 gap：300-20-4）", ex.Constraints)
	}
	if ex.Size != (flux.Size{W: 88, H: 276}) {
		t.Errorf("Expanded.Size = %+v，期望 88x276（Button 交叉轴 intrinsic 88、主轴撑到 276）", ex.Size)
	}

	// Button B：与 boundsOf 一致（diff 应用的就是同一 Frame）。
	btn, ok := findNodeDiag(t, nodes, "Button", "b")
	if !ok {
		t.Fatal("缺 Button b 节点诊断")
	}
	if btn.Frame != boundsOf(findByKey(t, app.Root(), "b")) {
		t.Errorf("Button b.Frame = %+v，与 Element Bounds %+v 不一致",
			btn.Frame, boundsOf(findByKey(t, app.Root(), "b")))
	}
}
