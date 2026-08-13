package flux_test

import (
	"fmt"
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

func TestPageControlConstructors(t *testing.T) {
	pageA := flux.TabPage("A", flux.Input(flux.Key("input-a")), flux.Key("a"))
	pageB := flux.TabPage("B", flux.Text("B", flux.Key("text-b")), flux.Key("b"))
	n := flux.PageControl(pageA, pageB, flux.SelectedIndex(9)).Create()
	if n.Type != "PageControl" || len(n.Children) != 2 {
		t.Fatalf("PageControl = type %q children=%d", n.Type, len(n.Children))
	}
	if got := n.Props.Int("SelectedIndex"); got != 1 {
		t.Fatalf("SelectedIndex = %d，期望钳制到 1", got)
	}
	if n.Children[0].Props.String("Text") != "A" || n.Children[1].Props.String("Text") != "B" {
		t.Fatalf("页面标题未保留: %q / %q", n.Children[0].Props.String("Text"), n.Children[1].Props.String("Text"))
	}
	if n.Children[0].Key != "a" || len(n.Children[0].Children) != 1 {
		t.Fatalf("页面 identity/唯一子树错误: key=%q children=%d", n.Children[0].Key, len(n.Children[0].Children))
	}
	if got := flux.PageControl().Create().Props.Int("SelectedIndex"); got != -1 {
		t.Fatalf("空 PageControl SelectedIndex = %d，期望 -1", got)
	}
}

func TestPageControlRejectsInvalidPages(t *testing.T) {
	assertPanic := func(name string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s 未 panic", name)
			}
		}()
		fn()
	}
	assertPanic("缺少 Key", func() { flux.TabPage("A", flux.Text("x")) })
	assertPanic("错误子类型", func() { flux.PageControl(flux.Text("x")) })
	page := flux.TabPage("A", flux.Text("x", flux.Key("x")), flux.Key("a"))
	assertPanic("重复 Key", func() { flux.PageControl(page, flux.TabPage("B", flux.Text("y"), flux.Key("a"))) })
}

func TestPageControlMockIdentityAndSelection(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	build := func(index int, order ...string) flux.Widget {
		pages := make([]flux.Widget, 0, len(order))
		for _, key := range order {
			pages = append(pages, flux.TabPage(key, flux.Input(flux.Key("input-"+key)), flux.Key(key)))
		}
		args := make([]any, 0, len(pages)+1)
		for _, page := range pages {
			args = append(args, page)
		}
		args = append(args, flux.SelectedIndex(index))
		return flux.Window(flux.PageControl(args...))
	}
	app.Render(build(0, "a", "b", "c"))
	pc := findByType(t, app.Root(), "PageControl")
	if pc == nil {
		t.Fatal("未找到 PageControl")
	}
	pageHandles := m.Pages(pc.Handle)
	if len(pageHandles) != 3 || m.PageSelectedIndex(pc.Handle) != 0 {
		t.Fatalf("初始页面状态 handles=%v selected=%d", pageHandles, m.PageSelectedIndex(pc.Handle))
	}
	aInput := findByKey(t, app.Root(), "input-a").Handle
	bInput := findByKey(t, app.Root(), "input-b").Handle
	base := len(m.Ops())
	app.Render(build(2, "c", "a", "b"))
	if got := countOps(m.Ops()[base:], render.OpCreate) + countOps(m.Ops()[base:], render.OpDestroy); got != 0 {
		t.Fatalf("页面重排不应创建/销毁，mutation=%v", m.Ops()[base:])
	}
	if findByKey(t, app.Root(), "input-a").Handle != aInput || findByKey(t, app.Root(), "input-b").Handle != bInput {
		t.Fatal("页面重排迁移了页内控件 identity")
	}
	if got := m.PageSelectedIndex(pc.Handle); got != 2 {
		t.Fatalf("重排后 SelectedIndex=%d，期望 2", got)
	}
}

func TestPageControlControlledEventRemovalAndTitlePatch(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	selected := -2
	build := func(title string, index int, onSelection func(int)) flux.Widget {
		args := []any{
			flux.TabPage(title, flux.Input(flux.Key("input-a")), flux.Key("a")),
			flux.TabPage("B", flux.Input(flux.Key("input-b")), flux.Key("b")),
			flux.SelectedIndex(index),
		}
		if onSelection != nil {
			args = append(args, flux.OnSelectionChange(onSelection))
		}
		return flux.Window(flux.PageControl(args...))
	}

	if err := app.Render(build("A", 0, func(index int) { selected = index })); err != nil {
		t.Fatal(err)
	}
	pc := findByType(t, app.Root(), "PageControl")
	pageA := findByKey(t, app.Root(), "a")
	inputA := findByKey(t, app.Root(), "input-a")
	if pc == nil || pageA == nil || inputA == nil {
		t.Fatal("首次挂载缺少分页 Element")
	}

	base := len(m.Ops())
	m.FirePageSelectionChange(pc.Handle, 1)
	if selected != 1 || m.PageSelectedIndex(pc.Handle) != 1 {
		t.Fatalf("用户选择未分派：callback=%d selected=%d", selected, m.PageSelectedIndex(pc.Handle))
	}
	if len(m.Ops()) != base {
		t.Fatalf("用户选择不应伪造程序化 mutation：%+v", m.Ops()[base:])
	}

	pageHandle, inputHandle := pageA.Handle, inputA.Handle
	base = len(m.Ops())
	if err := app.Render(build("A renamed", 1, func(index int) { selected = index })); err != nil {
		t.Fatal(err)
	}
	ops := m.Ops()[base:]
	if countOps(ops, render.OpCreate)+countOps(ops, render.OpDestroy) != 0 {
		t.Fatalf("标题/选择 patch 不应重建：%+v", ops)
	}
	if findByKey(t, app.Root(), "a").Handle != pageHandle || findByKey(t, app.Root(), "input-a").Handle != inputHandle {
		t.Fatal("标题 patch 改变了页面或页内控件 identity")
	}
	if !hasOp(ops, render.OpSetText, pageHandle, "", "A renamed") {
		t.Fatalf("标题 patch 未原地更新 TabPage：%+v", ops)
	}

	if err := app.Render(build("A renamed", 1, nil)); err != nil {
		t.Fatal(err)
	}
	selected = -2
	m.FirePageSelectionChange(pc.Handle, 0)
	if selected != -2 {
		t.Fatal("移除 OnSelectionChange 后仍触发旧回调")
	}
}

func TestPageControlAddRemovePagesAndIndexClamp(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	build := func(index int, order ...string) flux.Widget {
		args := make([]any, 0, len(order)+1)
		for _, key := range order {
			args = append(args, flux.TabPage(key, flux.Input(flux.Key("input-"+key)), flux.Key(key)))
		}
		args = append(args, flux.SelectedIndex(index))
		return flux.Window(flux.PageControl(args...))
	}

	if err := app.Render(build(1, "a", "b")); err != nil {
		t.Fatal(err)
	}
	pc := findByType(t, app.Root(), "PageControl")
	pageA := findByKey(t, app.Root(), "a").Handle
	inputA := findByKey(t, app.Root(), "input-a").Handle

	base := len(m.Ops())
	if err := app.Render(build(9, "a", "c")); err != nil {
		t.Fatal(err)
	}
	ops := m.Ops()[base:]
	if countOps(ops, render.OpCreate) != 2 || countOps(ops, render.OpDestroy) != 2 {
		t.Fatalf("替换一页应只创建/销毁该页及唯一子树：%+v", ops)
	}
	if findByKey(t, app.Root(), "a").Handle != pageA || findByKey(t, app.Root(), "input-a").Handle != inputA {
		t.Fatal("增删页面破坏了保留页 identity")
	}
	if got := m.PageSelectedIndex(pc.Handle); got != 1 {
		t.Fatalf("页面数变化后越界索引=%d，期望钳制到 1", got)
	}
	if pages := m.Pages(pc.Handle); len(pages) != 2 || pages[0] != pageA || pages[1] != findByKey(t, app.Root(), "c").Handle {
		t.Fatalf("页面顺序未同步：%v", pages)
	}

	base = len(m.Ops())
	if err := app.Render(build(-1)); err != nil {
		t.Fatal(err)
	}
	ops = m.Ops()[base:]
	if countOps(ops, render.OpCreate) != 0 || countOps(ops, render.OpDestroy) != 4 {
		t.Fatalf("清空页面应只销毁两页及其子树：%+v", ops)
	}
	if got := m.PageSelectedIndex(pc.Handle); got != -1 {
		t.Fatalf("空 PageControl SelectedIndex=%d，期望 -1", got)
	}
}

type pageCapabilitylessRenderer struct{ render.Renderer }

func TestPageControlMissingRendererCapabilityIsSafe(t *testing.T) {
	m := render.NewMock()
	r := pageCapabilitylessRenderer{Renderer: m}
	app := flux.NewApp(r)
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("Renderer 缺少 PageController 时不应 panic：%v", recovered)
		}
	}()
	if err := app.Render(flux.Window(flux.PageControl(
		flux.TabPage("A", flux.Text("content"), flux.Key("a")),
		flux.SelectedIndex(0),
		flux.OnSelectionChange(func(int) {}),
	))); err != nil {
		t.Fatal(err)
	}
	if findByType(t, app.Root(), "PageControl") == nil || findByKey(t, app.Root(), "a") == nil {
		t.Fatal("能力缺失时 Element 树仍应完整挂载")
	}
}

func hasOp(ops []render.Op, typ render.OpType, handle render.Handle, key string, value any) bool {
	for _, op := range ops {
		if op.Type == typ && op.Handle == handle && op.Key == key && fmt.Sprint(op.Value) == fmt.Sprint(value) {
			return true
		}
	}
	return false
}

func findByType(t *testing.T, e *diff.Element, typ string) *diff.Element {
	t.Helper()
	if e.Type == typ {
		return e
	}
	for _, c := range e.Children {
		if found := findByType(t, c, typ); found != nil {
			return found
		}
	}
	return nil
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
