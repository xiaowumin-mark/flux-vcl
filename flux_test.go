package flux_test

import (
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/diff"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// TestConstructorsBuildNode 各构造器生成正确的节点：类型、文本属性、key。
func TestConstructorsBuildNode(t *testing.T) {
	cases := []struct {
		name string
		w    flux.Widget
		typ  string
		text string
	}{
		{"Button", flux.Button("OK"), "Button", "OK"},
		{"Text", flux.Text("hi"), "Text", "hi"},
		{"Input", flux.Input(), "Input", ""},
		{"Memo", flux.Memo("line 1\nline 2"), "Memo", "line 1\nline 2"},
		{"CheckBox", flux.CheckBox("accept"), "CheckBox", "accept"},
		{"ComboBox", flux.ComboBox(), "ComboBox", ""},
		{"ProgressBar", flux.ProgressBar(), "ProgressBar", ""},
		{"RadioButton", flux.RadioButton("choice"), "RadioButton", "choice"},
		{"Column", flux.Column(), "Column", ""},
		{"Row", flux.Row(), "Row", ""},
		{"Window", flux.Window(), "Window", ""},
	}
	for _, c := range cases {
		n := c.w.Create()
		if n.Type != c.typ {
			t.Errorf("%s.Type = %q，期望 %q", c.name, n.Type, c.typ)
		}
		if c.text != "" && n.Props.String("Text") != c.text {
			t.Errorf("%s.Text = %q，期望 %q", c.name, n.Props.String("Text"), c.text)
		}
	}
}

// TestOptionsApply OnClick/Key/Width/Height 正确写入节点。
func TestOptionsApply(t *testing.T) {
	var clicked bool
	b := flux.Button("go",
		flux.OnClick(func(_ flux.Event) { clicked = true }),
		flux.Key("ok"),
		flux.Width(200),
		flux.Height(50),
	)
	n := b.Create()
	if n.Key != "ok" {
		t.Errorf("Key = %q，期望 ok", n.Key)
	}
	fn, ok := n.Props.Get("OnClick")
	if !ok {
		t.Fatal("OnClick 属性缺失")
	}
	fn.(func(flux.Event))(flux.Event{})
	if !clicked {
		t.Error("OnClick 回调未执行")
	}
	if n.Props.Int("Width") != 200 || n.Props.Int("Height") != 50 {
		t.Errorf("Width/Height = %d/%d，期望 200/50", n.Props.Int("Width"), n.Props.Int("Height"))
	}
}

// TestCheckBoxOptions Checked 与 OnCheckedChange 写入独立的布尔状态属性，
// 不与 Input/Memo 的文本 OnChange 混用。
func TestCheckBoxOptions(t *testing.T) {
	var got bool
	n := flux.CheckBox("accept", flux.Checked(true), flux.OnCheckedChange(func(checked bool) {
		got = checked
	})).Create()

	if !n.Props.Bool("Checked") {
		t.Error("Checked(true) 未写入 Checked 属性")
	}
	v, ok := n.Props.Get("OnCheckedChange")
	if !ok {
		t.Fatal("OnCheckedChange 属性缺失")
	}
	v.(func(bool))(true)
	if !got {
		t.Error("OnCheckedChange 回调未接收 true")
	}
}

// TestComboBoxOptions 选择控件使用最小字符串列表和显式受控索引；输入 slice
// 在构造时复制，Items/SelectedIndex 的 Opt 顺序不影响规范化结果。
func TestComboBoxOptions(t *testing.T) {
	input := []string{"one", "two"}
	var got int
	n := flux.ComboBox(
		flux.SelectedIndex(9),
		flux.Items(input),
		flux.OnSelectionChange(func(index int) { got = index }),
	).Create()
	input[0] = "mutated"

	items, _ := n.Props.Get("Items")
	values := items.([]string)
	if values[0] != "one" {
		t.Errorf("Items 未防御性复制，实际 %v", values)
	}
	if index := n.Props.Int("SelectedIndex"); index != 1 {
		t.Errorf("越界 SelectedIndex = %d，期望 1", index)
	}
	v, ok := n.Props.Get("OnSelectionChange")
	if !ok {
		t.Fatal("OnSelectionChange 属性缺失")
	}
	v.(func(int))(0)
	if got != 0 {
		t.Errorf("OnSelectionChange 回调收到 %d，期望 0", got)
	}

	empty := flux.ComboBox(flux.Items(nil), flux.SelectedIndex(0)).Create()
	items, _ = empty.Props.Get("Items")
	if got := items.([]string); got == nil || len(got) != 0 {
		t.Errorf("nil Items 应规范为空非 nil slice，实际 %#v", got)
	}
	if index := empty.Props.Int("SelectedIndex"); index != -1 {
		t.Errorf("空 Items 的 SelectedIndex = %d，期望 -1", index)
	}
}

func TestProgressBarAndRadioButtonOptions(t *testing.T) {
	progress := flux.ProgressBar(flux.Value(180), flux.Maximum(120), flux.Minimum(40)).Create()
	if got := progress.Props.Int("Minimum"); got != 40 {
		t.Errorf("Minimum = %d，期望 40", got)
	}
	if got := progress.Props.Int("Maximum"); got != 120 {
		t.Errorf("Maximum = %d，期望 120", got)
	}
	if got := progress.Props.Int("Value"); got != 120 {
		t.Errorf("Value = %d，期望钳制到 120", got)
	}
	defaults := flux.ProgressBar().Create()
	if defaults.Props.Int("Minimum") != 0 || defaults.Props.Int("Maximum") != 100 || defaults.Props.Int("Value") != 0 {
		t.Errorf("ProgressBar 默认值 = %d/%d/%d，期望 0/100/0", defaults.Props.Int("Minimum"), defaults.Props.Int("Maximum"), defaults.Props.Int("Value"))
	}

	var checked bool
	radio := flux.RadioButton("one", flux.Checked(true), flux.GroupIndex(3), flux.OnCheckedChange(func(value bool) {
		checked = value
	})).Create()
	if !radio.Props.Bool("Checked") || radio.Props.Int("GroupIndex") != 3 {
		t.Errorf("RadioButton 属性 = Checked(%v), GroupIndex(%d)", radio.Props.Bool("Checked"), radio.Props.Int("GroupIndex"))
	}
	callback, ok := radio.Props.Get("OnCheckedChange")
	if !ok {
		t.Fatal("RadioButton OnCheckedChange 属性缺失")
	}
	callback.(func(bool))(true)
	if !checked {
		t.Error("RadioButton OnCheckedChange 未接收 true")
	}
}

func TestLayoutColumnStacks(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.Column(
		flux.Text("A", flux.Key("t")),
		flux.Button("B", flux.Key("b")),
	)))

	root := app.Root()
	tb := boundsOf(findByKey(t, root, "t"))
	if tb.X != 0 || tb.Y != 0 || tb.H != 20 {
		t.Errorf("Text Bounds = %+v，期望 {0 0 w 20}（Column 顶行）", tb)
	}
	bb := boundsOf(findByKey(t, root, "b"))
	if bb.Y != 24 { // Text H=20 + gap 4
		t.Errorf("Button Y = %d，期望 24（紧接 Text 下方）", bb.Y)
	}
	if bb.W != 88 { // TextWidth("B")+32 = 8+32=40 < 88 → 最小 88
		t.Errorf("Button W = %d，期望 88（intrinsic 下限）", bb.W)
	}
}

// TestAppRenderMockEndToEnd App+Mock 端到端：首次挂载后，点击按钮触发
// re-render（文本变化），diff 引擎零重建 —— 纯属性 patch（D7a 在 flux 层落地）。
func TestAppRenderMockEndToEnd(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	var text string
	var build func() flux.Widget // 闭包自引用需先声明再赋值
	build = func() flux.Widget {
		return flux.Window(flux.Column(
			flux.Text(text, flux.Key("t")),
			flux.Button("Click me", flux.Key("b"), flux.OnClick(func(_ flux.Event) {
				text = "Clicked!"
				app.Render(build())
			})),
		))
	}

	app.Render(build())
	base := len(m.Ops())
	if n := m.Count(render.OpCreate); n != 3 { // Window + Text + Button
		t.Errorf("首次挂载 Create = %d，期望 3", n)
	}

	// 模拟点击：执行按钮事件回调 → 修改 text → re-render
	onClickOf(t, build().Create(), "Button")(flux.Event{})

	ops := m.Ops()[base:]
	if n := countOps(ops, render.OpCreate); n != 0 {
		t.Errorf("re-render Create = %d，期望 0（零控件重建）", n)
	}
	if n := countOps(ops, render.OpDestroy); n != 0 {
		t.Errorf("re-render Destroy = %d，期望 0", n)
	}
	if n := countOps(ops, render.OpSetText); n != 1 {
		t.Errorf("re-render SetText = %d，期望 1（Text 属性 patch）", n)
	}
}

// TestNativeRefEscapes 逃逸口 Opt 正确写入节点 Props。
func TestNativeRefEscapes(t *testing.T) {
	r := &flux.Ref{}
	b := flux.Button("OK", flux.BindRef(r), flux.Native(func(any) {}))
	n := b.Create()
	if got, ok := n.Props.Get("Ref"); !ok || got != r {
		t.Error("BindRef 未写入 Ref 属性")
	}
	if _, ok := n.Props.Get("Native"); !ok {
		t.Error("Native 未写入 Native 属性")
	}
}

// —— 测试辅助 ——

func countOps(ops []render.Op, t render.OpType) int {
	n := 0
	for _, op := range ops {
		if op.Type == t {
			n++
		}
	}
	return n
}

func findByKey(t *testing.T, e *diff.Element, key string) *diff.Element {
	t.Helper()
	if e.Key == key {
		return e
	}
	for _, c := range e.Children {
		if found := findByKey(t, c, key); found != nil {
			return found
		}
	}
	return nil
}

func boundsOf(e *diff.Element) render.Rect {
	v, _ := e.Props.Get("Bounds")
	b, _ := v.(render.Rect)
	return b
}

// onClickOf 在节点树中查找指定类型控件的 OnClick 回调。
func onClickOf(t *testing.T, n *flux.Node, typ string) func(flux.Event) {
	t.Helper()
	if n.Type == typ {
		if fn, ok := n.Props.Get("OnClick"); ok {
			return fn.(func(flux.Event))
		}
	}
	for _, c := range n.Children {
		if f := onClickOf(t, c, typ); f != nil {
			return f
		}
	}
	return nil
}

// TestAppFindByPath 隐式寻址（App 层）：静态树零 Key，按树路径定位 Element。
// 寻址与身份解耦（D3 补充）—— 测试/排查不再被迫给每个控件起 Key。
func TestAppFindByPath(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	app.Render(flux.Window(flux.Column(
		flux.Text("a"),
		flux.Button("b"),
	)))

	btn := app.FindByPath("Window/0/Column/1/Button")
	if btn == nil || btn.Type != "Button" {
		t.Fatalf("FindByPath(Button) = %+v，期望命中 Button", btn)
	}
	if btn.Path != "Window/0/Column/1/Button" {
		t.Errorf("btn.Path = %q，期望 Window/0/Column/1/Button", btn.Path)
	}
	if got := app.FindByPath("Window/0/Column/0/Text"); got == nil || got.Type != "Text" {
		t.Errorf("FindByPath(Text) = %+v，期望命中 Text", got)
	}
	// 越界 / 根类型不符 → nil
	if got := app.FindByPath("Window/0/Column/9/Button"); got != nil {
		t.Errorf("越界路径应返回 nil，实际命中 %s", got.Type)
	}
	if got := app.FindByPath("Row/0"); got != nil {
		t.Errorf("根类型不符应返回 nil，实际命中 %s", got.Type)
	}
	if got := app.FindByPath(""); got != nil {
		t.Errorf("空路径应返回 nil，实际 %+v", got)
	}
}
