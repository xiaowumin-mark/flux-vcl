package flux_test

import (
	"sync"
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// TestStateGetSet State 基本读写（线程安全，-race 验证）。
func TestStateGetSet(t *testing.T) {
	s := flux.NewState(10)
	if s.Get() != 10 {
		t.Fatalf("初值 = %d，期望 10", s.Get())
	}
	s.Set(11)
	if s.Get() != 11 {
		t.Fatalf("Set 后 = %d，期望 11", s.Get())
	}
}

// TestStateOneWayBind 单向绑定（2.2）：State 变化 → 文本随之刷新，仅 patch
// 属性（零控件重建）。Mock RunOnUI 同步执行，两次 Set 各触发一次 render。
func TestStateOneWayBind(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	count := flux.NewState(0)

	app.Mount(func() flux.Widget {
		return flux.Window(flux.Text(flux.Bind(count), flux.Key("t")))
	})
	if n := m.Count(render.OpCreate); n != 2 { // Window + Text
		t.Fatalf("挂载 Create = %d，期望 2", n)
	}

	base := len(m.Ops())
	count.Set(1)
	count.Set(2)

	ops := m.Ops()[base:]
	if n := countOps(ops, render.OpCreate); n != 0 {
		t.Errorf("re-render Create = %d，期望 0（零重建）", n)
	}
	if n := countOps(ops, render.OpDestroy); n != 0 {
		t.Errorf("re-render Destroy = %d，期望 0", n)
	}
	if n := countOps(ops, render.OpSetText); n != 2 {
		t.Errorf("SetText = %d，期望 2（0→1→2 两次属性 patch）", n)
	}
	if got := findByKey(t, app.Root(), "t").Props.String("Text"); got != "2" {
		t.Errorf("Text = %q，期望 2", got)
	}
}

// TestStateInvalidateMerge 失效合并（2.5）：同一周期内多次 Set 只触发一次
// render。用一个延迟执行的 RunOnUI 制造"同周期"窗口。
func TestStateInvalidateMerge(t *testing.T) {
	q := &queuedMock{Mock: render.NewMock()}
	app := flux.NewApp(q)
	count := flux.NewState(0)

	app.Mount(func() flux.Widget {
		return flux.Window(flux.Text(flux.Bind(count), flux.Key("t")))
	})

	// 周期内两次 Set：pending 标志合并，只入队一次 render。
	count.Set(1)
	count.Set(2)
	if len(q.queue) != 1 {
		t.Fatalf("排队 render = %d，期望 1（pending 合并）", len(q.queue))
	}
	q.flush()
	if got := findByKey(t, app.Root(), "t").Props.String("Text"); got != "2" {
		t.Errorf("合并后 Text = %q，期望 2", got)
	}
}

// TestStateTwoWayBind 双向绑定（2.3）：Input(Bind(name)) 初始显示初值；
// 触发 OnChange（模拟用户输入）→ State 更新 → re-render 文本同步。
func TestStateTwoWayBind(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	name := flux.NewState("flux")

	app.Mount(func() flux.Widget {
		return flux.Window(flux.Input(flux.Bind(name), flux.Key("i")))
	})

	el := findByKey(t, app.Root(), "i")
	if got := el.Props.String("Text"); got != "flux" {
		t.Errorf("Input 初值 = %q，期望 flux", got)
	}
	cb, ok := el.Props.Get("OnChange")
	if !ok {
		t.Fatal("OnChange 回调缺失（双向绑定未设置回写）")
	}
	cb.(func(string))("wumin") // 模拟用户输入

	if got := name.Get(); got != "wumin" {
		t.Errorf("State = %q，期望 wumin", got)
	}
	// 双向：State 变化又触发 render，Input 文本同步为最新值。
	if got := findByKey(t, app.Root(), "i").Props.String("Text"); got != "wumin" {
		t.Errorf("Input 同步后 = %q，期望 wumin", got)
	}
}

// TestMemoTwoWayBind Memo 复用文本绑定：用户输入通过 OnChange 回写 State，
// 后续 render 原地更新 Text，不创建或销毁原生控件。
func TestMemoTwoWayBind(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	text := flux.NewState("first\nline")

	app.Mount(func() flux.Widget {
		return flux.Window(flux.Memo(flux.Bind(text), flux.Key("memo")))
	})

	el := findByKey(t, app.Root(), "memo")
	if got := el.Props.String("Text"); got != "first\nline" {
		t.Errorf("Memo 初值 = %q，期望 first\\nline", got)
	}
	cb, ok := el.Props.Get("OnChange")
	if !ok {
		t.Fatal("Memo OnChange 回调缺失（双向绑定未设置回写）")
	}
	base := len(m.Ops())
	cb.(func(string))("updated\ntext")

	if got := text.Get(); got != "updated\ntext" {
		t.Errorf("State = %q，期望 updated\\ntext", got)
	}
	if got := findByKey(t, app.Root(), "memo").Props.String("Text"); got != "updated\ntext" {
		t.Errorf("Memo 同步后 = %q，期望 updated\\ntext", got)
	}
	ops := m.Ops()[base:]
	if n := countOps(ops, render.OpCreate); n != 0 {
		t.Errorf("Memo 回写后 Create = %d，期望 0", n)
	}
	if n := countOps(ops, render.OpDestroy); n != 0 {
		t.Errorf("Memo 回写后 Destroy = %d，期望 0", n)
	}
	if n := countOps(ops, render.OpSetText); n != 1 {
		t.Errorf("Memo 回写后 SetText = %d，期望 1", n)
	}
}

// TestComboBoxExplicitControlledState ComboBox 不扩张 Bind 语义：回调显式写 State，
// 后续构建以 SelectedIndex 受控回写，控件句柄保持不变。
func TestComboBoxExplicitControlledState(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	selected := flux.NewState(-1)

	app.Mount(func() flux.Widget {
		return flux.Window(flux.Column(
			flux.ComboBox(
				flux.Key("combo"),
				flux.Items([]string{"red", "green", "blue"}),
				flux.SelectedIndex(selected.Get()),
				flux.OnSelectionChange(func(index int) { selected.Set(index) }),
			),
			// 显式受控状态仍需订阅 App；回显同时证明回调后的 build 确实执行。
			flux.Text(flux.Bind(selected), flux.Key("selected-value")),
		))
	})
	h := findByKey(t, app.Root(), "combo").Handle
	base := len(m.Ops())
	m.FireSelectionChange(h, 2)

	if got := selected.Get(); got != 2 {
		t.Fatalf("选择回调后的 State = %d，期望 2", got)
	}
	el := findByKey(t, app.Root(), "combo")
	if el.Handle != h {
		t.Fatalf("受控更新后 ComboBox 句柄变化：%d → %d", h, el.Handle)
	}
	if got := m.SelectedIndex(h); got != 2 {
		t.Errorf("受控回写后 SelectedIndex = %d，期望 2", got)
	}
	if got := el.Props.Int("SelectedIndex"); got != 2 {
		t.Errorf("受控回写后 Element SelectedIndex = %d，期望 2", got)
	}
	ops := m.Ops()[base:]
	if n := countOps(ops, render.OpCreate); n != 0 {
		t.Errorf("选择回写 Create = %d，期望 0", n)
	}
	if n := countOps(ops, render.OpDestroy); n != 0 {
		t.Errorf("选择回写 Destroy = %d，期望 0", n)
	}
}

// TestPageControlExplicitControlledState 验证用户切页只通过类型化回调写 State，
// 随后的受控 SelectedIndex patch 原地更新，不重建页面或页内输入控件。
func TestPageControlExplicitControlledState(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	selected := flux.NewState(0)
	status := flux.NewState("first")

	if err := app.Mount(func() flux.Widget {
		return flux.Window(flux.Column(
			flux.PageControl(
				flux.TabPage("first", flux.Input(flux.Key("first-input")), flux.Key("first")),
				flux.TabPage("second", flux.Input(flux.Key("second-input")), flux.Key("second")),
				flux.Key("pages"),
				flux.SelectedIndex(selected.Get()),
				flux.OnSelectionChange(func(index int) {
					selected.Set(index)
					status.Set([]string{"first", "second"}[index])
				}),
			),
			flux.Text(flux.Bind(status), flux.Key("status")),
		))
	}); err != nil {
		t.Fatal(err)
	}
	pc := findByKey(t, app.Root(), "pages")
	firstPage := findByKey(t, app.Root(), "first").Handle
	secondPage := findByKey(t, app.Root(), "second").Handle
	firstInput := findByKey(t, app.Root(), "first-input").Handle
	secondInput := findByKey(t, app.Root(), "second-input").Handle
	base := len(m.Ops())

	m.FirePageSelectionChange(pc.Handle, 1)
	if selected.Get() != 1 || status.Get() != "second" || m.PageSelectedIndex(pc.Handle) != 1 {
		t.Fatalf("切页 State 未收敛：selected=%d status=%q native=%d",
			selected.Get(), status.Get(), m.PageSelectedIndex(pc.Handle))
	}
	if findByKey(t, app.Root(), "first").Handle != firstPage ||
		findByKey(t, app.Root(), "second").Handle != secondPage ||
		findByKey(t, app.Root(), "first-input").Handle != firstInput ||
		findByKey(t, app.Root(), "second-input").Handle != secondInput {
		t.Fatal("受控切页重建了页面或页内控件")
	}
	ops := m.Ops()[base:]
	if countOps(ops, render.OpCreate) != 0 || countOps(ops, render.OpDestroy) != 0 {
		t.Fatalf("受控切页不应创建/销毁：%+v", ops)
	}
}

// TestRadioButtonExplicitControlledState 验证 RadioButton 沿用显式受控模式：
// 原生事件只通知调用方，调用方写入 State 后由下一次 render 回写 Checked。
func TestRadioButtonExplicitControlledState(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	checked := flux.NewState(false)

	var build func() flux.Widget
	build = func() flux.Widget {
		return flux.Window(flux.RadioButton(
			"option",
			flux.Key("radio"),
			flux.Checked(checked.Get()),
			flux.OnCheckedChange(func(value bool) {
				checked.Set(value)
				app.Render(build())
			}),
		))
	}
	app.Mount(build)
	h := findByKey(t, app.Root(), "radio").Handle
	base := len(m.Ops())
	m.FireCheckedChange(h, true)

	if got := checked.Get(); !got {
		t.Fatal("选中回调后的 State = false，期望 true")
	}
	el := findByKey(t, app.Root(), "radio")
	if el.Handle != h {
		t.Fatalf("受控更新后 RadioButton 句柄变化：%d → %d", h, el.Handle)
	}
	if !m.Checked(h) || !el.Props.Bool("Checked") {
		t.Errorf("受控回写后 Checked = mock(%v) props(%v)，期望 true/true", m.Checked(h), el.Props.Bool("Checked"))
	}
	ops := m.Ops()[base:]
	if n := countOps(ops, render.OpCreate); n != 0 {
		t.Errorf("选中回写 Create = %d，期望 0", n)
	}
	if n := countOps(ops, render.OpDestroy); n != 0 {
		t.Errorf("选中回写 Destroy = %d，期望 0", n)
	}
}

// UI 正确刷新。mock RunOnUI 同步执行，render 在各自 goroutine 内完成；
// -race 下验证 State/App 的锁纪律。
func TestStateSetFromGoroutine(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	count := flux.NewState(0)

	app.Mount(func() flux.Widget {
		return flux.Window(flux.Text(flux.Bind(count), flux.Key("t")))
	})

	var wg sync.WaitGroup
	for i := 1; i <= 5; i++ {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			count.Set(v)
		}(i)
	}
	wg.Wait()

	// 写序不确定，最终值必为 1..5 之一；核心是并发 Set 不崩溃、无数据竞争。
	got := findByKey(t, app.Root(), "t").Props.String("Text")
	switch got {
	case "1", "2", "3", "4", "5":
	default:
		t.Errorf("Text = %q，期望 1..5 之一（并发写）", got)
	}
}

// TestStateScopeInvalidation 作用域失效（2.4）：只 Set 一个 State 时，diff
// 只 patch 受影响子树 —— SetText 恰一条且指向该子树，未变子树零 mutation。
func TestStateScopeInvalidation(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	a := flux.NewState("a0")
	b := flux.NewState("b0")

	app.Mount(func() flux.Widget {
		return flux.Window(flux.Column(
			flux.Text(flux.Bind(a), flux.Key("a")),
			flux.Text(flux.Bind(b), flux.Key("b")),
		))
	})

	base := len(m.Ops())
	a.Set("a1")

	ops := m.Ops()[base:]
	if n := countOps(ops, render.OpCreate); n != 0 {
		t.Errorf("a 变化 Create = %d，期望 0（零重建）", n)
	}
	if n := countOps(ops, render.OpSetText); n != 1 {
		t.Fatalf("a 变化 SetText = %d，期望 1（只 patch 受影响子树）", n)
	}
	elA := findByKey(t, app.Root(), "a")
	if ops[0].Handle != elA.Handle {
		t.Errorf("SetText 目标句柄 = %d，期望 a 子树的 %d（作用域限定）", ops[0].Handle, elA.Handle)
	}
	if got := elA.Props.String("Text"); got != "a1" {
		t.Errorf("a.Text = %q，期望 a1", got)
	}
	// 未变子树 b：零 mutation，值保持。
	if got := findByKey(t, app.Root(), "b").Props.String("Text"); got != "b0" {
		t.Errorf("b.Text = %q，期望 b0（未变子树保持）", got)
	}
}

// queuedMock 把 RunOnUI 改为入队，制造"同周期"窗口验证 invalidate 合并。
type queuedMock struct {
	*render.Mock
	queue []func()
}

func (q *queuedMock) RunOnUI(fn func()) { q.queue = append(q.queue, fn) }

func (q *queuedMock) flush() {
	for len(q.queue) > 0 {
		fn := q.queue[0]
		q.queue = q.queue[1:]
		fn()
	}
}
