package flux_test

import (
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// TestBoxConstraintsConstrain BoxConstraints 协议：Tight/Loose/unbounded 与
// min/max 钳制（D5 constrain）。
func TestBoxConstraintsConstrain(t *testing.T) {
	cases := []struct {
		name string
		c    flux.BoxConstraints
		in   flux.Size
		want flux.Size
	}{
		{"Tight 强制", flux.Tight(100, 50), flux.Size{200, 20}, flux.Size{100, 50}},
		{"Loose 不强制", flux.Loose(200, 100), flux.Size{150, 60}, flux.Size{150, 60}},
		{"Loose 钳制到 Max", flux.Loose(100, 50), flux.Size{200, 60}, flux.Size{100, 50}},
		{"Loose 钳制到 Min(0)", flux.Loose(10, 5), flux.Size{-5, -3}, flux.Size{0, 0}},
		{"Min>0 钳制", flux.BoxConstraints{MinW: 10, MaxW: 100, MinH: 5, MaxH: 100}, flux.Size{3, 4}, flux.Size{10, 5}},
		{"Unbounded 原样", flux.Unbounded(), flux.Size{30, 40}, flux.Size{30, 40}},
	}
	for _, c := range cases {
		if got := c.c.Constrain(c.in.W, c.in.H); got != c.want {
			t.Errorf("%s: Constrain(%d,%d) = %+v，期望 %+v", c.name, c.in.W, c.in.H, got, c.want)
		}
	}
}

// TestFlexExpandedFillsFreeSpace Expanded（tight）拿满 flex 容器剩余主轴空间。
// Window client 400x300：Column 内 B 非 flex 高 20，Expanded A 分到 300-20-gap4=276。
func TestFlexExpandedFillsFreeSpace(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.Column(
		flux.Expanded(flux.Text("A", flux.Key("a"))),
		flux.Text("B", flux.Key("b")),
	)))

	elA := findByKey(t, app.Root(), "a")
	elB := findByKey(t, app.Root(), "b")
	if got := boundsOf(elA).H; got != 276 {
		t.Errorf("A.H = %d，期望 276（freeSpace = 300-20-4）", got)
	}
	if got := boundsOf(elB).Y; got != 280 {
		t.Errorf("B.Y = %d，期望 280（A.H 276 + gap 4）", got)
	}
}

// TestFlexFlexibleKeepsIntrinsic Flexible（loose）允许子小于分配空间：剩余空间
// 大时 A 仍保持 intrinsic 高度 20，不填满。
func TestFlexFlexibleKeepsIntrinsic(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.Column(
		flux.Flexible(flux.Text("A", flux.Key("a"))),
		flux.Text("B", flux.Key("b")),
	)))

	elA := findByKey(t, app.Root(), "a")
	if got := boundsOf(elA).H; got != 20 {
		t.Errorf("A.H = %d，期望 20（Flexible 不强制填满 freeSpace）", got)
	}
}

// TestFlexFactorsProportional flex 因子按比例分配：Expanded 1:2 → A:H 98、B:H 196。
// freeSpace = 300-gap4 = 296，perFlex = 296/3 = 98。
func TestFlexFactorsProportional(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.Column(
		flux.Expanded(flux.Text("A", flux.Key("a")), 1),
		flux.Expanded(flux.Text("B", flux.Key("b")), 2),
	)))

	elA := findByKey(t, app.Root(), "a")
	elB := findByKey(t, app.Root(), "b")
	if got := boundsOf(elA).H; got != 98 {
		t.Errorf("A.H = %d，期望 98（296/3）", got)
	}
	if got := boundsOf(elB).H; got != 196 {
		t.Errorf("B.H = %d，期望 196（296/3*2）", got)
	}
	if got := boundsOf(elB).Y; got != 102 {
		t.Errorf("B.Y = %d，期望 102（A.H 98 + gap 4）", got)
	}
}

// TestMainAxisAlignmentCenter 主轴居中：Row 内两固定宽子，leftover 380 → lead 190。
func TestMainAxisAlignmentCenter(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.Row(
		flux.MainAxis(flux.MainAxisCenter),
		flux.Text("A", flux.Key("a")),
		flux.Text("B", flux.Key("b")),
	)))

	elA := findByKey(t, app.Root(), "a")
	elB := findByKey(t, app.Root(), "b")
	if got := boundsOf(elA).X; got != 190 {
		t.Errorf("A.X = %d，期望 190（leftover 380 均分到首尾）", got)
	}
	if got := boundsOf(elB).X; got != 202 {
		t.Errorf("B.X = %d，期望 202（A.X 190 + A.W 8 + gap 4）", got)
	}
}

// TestMainAxisAlignmentSpaceBetween 两端对齐：首子贴 start，末子贴 end。
func TestMainAxisAlignmentSpaceBetween(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.Row(
		flux.MainAxis(flux.MainAxisSpaceBetween),
		flux.Text("A", flux.Key("a")),
		flux.Text("B", flux.Key("b")),
	)))

	elA := findByKey(t, app.Root(), "a")
	elB := findByKey(t, app.Root(), "b")
	if got := boundsOf(elA).X; got != 0 {
		t.Errorf("A.X = %d，期望 0（SpaceBetween 首贴 start）", got)
	}
	if got := boundsOf(elB).X; got != 392 {
		t.Errorf("B.X = %d，期望 392（末贴 end，右缘 400）", got)
	}
}

// TestMainAxisAlignmentSpaceAround 剩余空间均摊到每子两侧。
// per=380/4=95 → lead 95、between 194（gap4 + 2*95）。
func TestMainAxisAlignmentSpaceAround(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.Row(
		flux.MainAxis(flux.MainAxisSpaceAround),
		flux.Text("A", flux.Key("a")),
		flux.Text("B", flux.Key("b")),
	)))

	elA := findByKey(t, app.Root(), "a")
	elB := findByKey(t, app.Root(), "b")
	if got := boundsOf(elA).X; got != 95 {
		t.Errorf("A.X = %d，期望 95（380/(2*2)）", got)
	}
	if got := boundsOf(elB).X; got != 297 {
		t.Errorf("B.X = %d，期望 297", got)
	}
}

// TestMainAxisAlignmentSpaceEvenly 剩余空间均摊到包括首尾的每个间隙。
// per=380/3=126 → lead 126、between 130（gap4 + 126）。
func TestMainAxisAlignmentSpaceEvenly(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.Row(
		flux.MainAxis(flux.MainAxisSpaceEvenly),
		flux.Text("A", flux.Key("a")),
		flux.Text("B", flux.Key("b")),
	)))

	elA := findByKey(t, app.Root(), "a")
	elB := findByKey(t, app.Root(), "b")
	if got := boundsOf(elA).X; got != 126 {
		t.Errorf("A.X = %d，期望 126（380/3）", got)
	}
	if got := boundsOf(elB).X; got != 264 {
		t.Errorf("B.X = %d，期望 264", got)
	}
}

// TestCrossAxisAlignmentCenter 交叉轴居中：Row 内 A(高20) 相对 crossExtent(B 高32)
// 垂直居中 → A.Y=6、B.Y=0。
func TestCrossAxisAlignmentCenter(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.Row(
		flux.CrossAxis(flux.CrossAxisCenter),
		flux.Text("A", flux.Key("a")),
		flux.Button("B", flux.Key("b")),
	)))

	elA := findByKey(t, app.Root(), "a")
	elB := findByKey(t, app.Root(), "b")
	if got := boundsOf(elA).Y; got != 6 {
		t.Errorf("A.Y = %d，期望 6（(32-20)/2 居中）", got)
	}
	if got := boundsOf(elB).Y; got != 0 {
		t.Errorf("B.Y = %d，期望 0", got)
	}
}

// TestCrossAxisAlignmentStretch 交叉轴拉伸：Column 内子宽度填满容器（400）。
func TestCrossAxisAlignmentStretch(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.Column(
		flux.CrossAxis(flux.CrossAxisStretch),
		flux.Text("A", flux.Key("a")),
		flux.Text("B", flux.Key("b")),
	)))

	elA := findByKey(t, app.Root(), "a")
	elB := findByKey(t, app.Root(), "b")
	if got := boundsOf(elA).W; got != 400 {
		t.Errorf("A.W = %d，期望 400（stretch 填满交叉轴）", got)
	}
	if got := boundsOf(elB).W; got != 400 {
		t.Errorf("B.W = %d，期望 400", got)
	}
}

// TestOverflowDiagnostics 只增不缩 + 溢出诊断：20 个 Text（20*20+19*4=476）超出
// Window 高 300 → 溢出 176 记录到 App.LastLayoutDiags，子控件不被压缩。
func TestOverflowDiagnostics(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	var kids []any
	for i := 0; i < 20; i++ {
		kids = append(kids, flux.Text("x", flux.Key("t"+string(rune('a'+i)))))
	}
	app.Render(flux.Window(flux.Column(kids...)))

	diags := app.LastLayoutDiags()
	if len(diags) == 0 {
		t.Fatal("LastLayoutDiags 为空，期望溢出诊断")
	}
	if d := diags[0]; d.OverflowH != 176 {
		t.Errorf("溢出诊断 OverflowH = %d，期望 176（476-300）", d.OverflowH)
	}
	// 子控件未被压缩：第一个 Text 保持 intrinsic 高度。
	if got := boundsOf(findByKey(t, app.Root(), "ta")).H; got != 20 {
		t.Errorf("首个 Text.H = %d，期望 20（只增不缩）", got)
	}
}

// TestWindowUsesClientSize Window 布局用 renderer 客户区尺寸，而非固定 400x300。
func TestWindowUsesClientSize(t *testing.T) {
	m := render.NewMock()
	m.SetClientSize(600, 400)
	app := flux.NewApp(m)

	app.Render(flux.Window())

	if got := boundsOf(app.Root()); got != (render.Rect{W: 600, H: 400}) {
		t.Errorf("Window Bounds = %+v，期望 {0 0 600 400}", got)
	}
}

// TestResizeRerenders 窗体 resize → re-render → 布局即时更新，零控件重建
// （原地 patch Bounds）。模拟拖拽改变窗口尺寸。
func TestResizeRerenders(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Mount(func() flux.Widget {
		return flux.Window(flux.Column(
			flux.Expanded(flux.Text("A", flux.Key("a"))),
			flux.Text("B", flux.Key("b")),
		))
	})

	// 初始 400x300：freeSpace = 300-20-4 = 276
	if got := boundsOf(findByKey(t, app.Root(), "a")).H; got != 276 {
		t.Fatalf("初始 A.H = %d，期望 276", got)
	}
	base := len(m.Ops())
	m.SetClientSize(800, 500)
	m.TriggerResize(800, 500) // → app.invalidate → re-render

	// resize 后 800x500：freeSpace = 500-20-4 = 476 → A 的分配空间随窗体变化
	if got := boundsOf(findByKey(t, app.Root(), "a")).H; got != 476 {
		t.Errorf("resize 后 A.H = %d，期望 476（freeSpace 随窗体更新）", got)
	}
	ops := m.Ops()[base:]
	if n := countOps(ops, render.OpCreate); n != 0 {
		t.Errorf("resize re-render Create = %d，期望 0（零控件重建）", n)
	}
	if n := countOps(ops, render.OpDestroy); n != 0 {
		t.Errorf("resize re-render Destroy = %d，期望 0", n)
	}
}

// TestExpandedFlexibleTransparentNoHandle flex 包装是透明容器：不创建原生控件，
// 只创建 Window + Button 两个句柄，Button 直接挂在 Window 句柄下。
func TestExpandedFlexibleTransparentNoHandle(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.Column(
		flux.Expanded(flux.Button("B", flux.Key("b"))),
	)))

	if n := m.Count(render.OpCreate); n != 2 { // Window + Button
		t.Errorf("Create = %d，期望 2（Expanded 无句柄）", n)
	}
	// Button 的父是 Window Element（Column/Expanded 透明穿透）。
	elB := findByKey(t, app.Root(), "b")
	if elB.Parent == nil || elB.Parent.Type != "Expanded" {
		t.Errorf("Button.Parent.Type = %v，期望 Expanded（透明链）", elB.Parent.Type)
	}
}

// TestD7cSameTreeZeroMutation 相同树二次 diff 零 mutation（含 Expanded 包装）：
// 布局结果一致 → Bounds 相同 → 无任何 op（D7c 在 flex 场景落地）。
func TestD7cSameTreeZeroMutation(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	build := func() flux.Widget {
		return flux.Window(flux.Column(
			flux.Expanded(flux.Text("A", flux.Key("a"))),
			flux.Text("B", flux.Key("b")),
		))
	}
	app.Render(build())
	base := len(m.Ops())
	app.Render(build())

	ops := m.Ops()[base:]
	for _, op := range ops {
		t.Errorf("相同树不应产生 op：%+v", op)
	}
}
