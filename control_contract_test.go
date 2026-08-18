package flux_test

import (
	"fmt"
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/diff"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

type controlContractCase struct {
	name       string
	targetType string
	targetKey  string
	native     bool
	build      func(updated bool) flux.Widget
}

// controlContractCases 是 7.3a 的公开控件清单。事件函数不进入这些树，保证
// D7c 断言只观察声明值相同的树；事件重绑/解绑由独立矩阵覆盖。
func controlContractCases() []controlContractCase {
	return []controlContractCase{
		{"Window", "Window", "subject", true, func(updated bool) flux.Widget {
			title := "before"
			if updated {
				title = "after"
			}
			return flux.Window(flux.Key("subject"), flux.Title(title))
		}},
		{"Column", "Column", "subject", false, func(updated bool) flux.Widget {
			return flux.Window(flux.Column(flux.Text(contractText(updated)), flux.Key("subject")))
		}},
		{"Row", "Row", "subject", false, func(updated bool) flux.Widget {
			return flux.Window(flux.Row(flux.Text(contractText(updated)), flux.Key("subject")))
		}},
		{"Component", "Component", "subject", false, func(updated bool) flux.Widget {
			return flux.Window(flux.Component(func() flux.Widget {
				return flux.Text(contractText(updated), flux.Key("leaf"))
			}, flux.Key("subject")))
		}},
		{"Expanded", "Expanded", "", false, func(updated bool) flux.Widget {
			return flux.Window(flux.Column(flux.Expanded(flux.Text(contractText(updated)))))
		}},
		{"Flexible", "Flexible", "", false, func(updated bool) flux.Widget {
			return flux.Window(flux.Column(flux.Flexible(flux.Text(contractText(updated)))))
		}},
		{"Text", "Text", "subject", true, func(updated bool) flux.Widget {
			return flux.Window(flux.Text(contractText(updated), flux.Key("subject")))
		}},
		{"Button", "Button", "subject", true, func(updated bool) flux.Widget {
			return flux.Window(flux.Button(contractText(updated), flux.Key("subject")))
		}},
		{"Input", "Input", "subject", true, func(updated bool) flux.Widget {
			return flux.Window(flux.Input(flux.Key("subject"), flux.Enabled(!updated)))
		}},
		{"Memo", "Memo", "subject", true, func(updated bool) flux.Widget {
			return flux.Window(flux.Memo(contractText(updated), flux.Key("subject")))
		}},
		{"CheckBox", "CheckBox", "subject", true, func(updated bool) flux.Widget {
			return flux.Window(flux.CheckBox("check", flux.Key("subject"), flux.Checked(updated)))
		}},
		{"RadioButton", "RadioButton", "subject", true, func(updated bool) flux.Widget {
			return flux.Window(flux.RadioButton("radio", flux.Key("subject"), flux.Checked(updated)))
		}},
		{"ComboBox", "ComboBox", "subject", true, func(updated bool) flux.Widget {
			selected := 0
			if updated {
				selected = 1
			}
			return flux.Window(flux.ComboBox(
				flux.Key("subject"), flux.Items([]string{"a", "b"}), flux.SelectedIndex(selected),
			))
		}},
		{"ProgressBar", "ProgressBar", "subject", true, func(updated bool) flux.Widget {
			value := 10
			if updated {
				value = 20
			}
			return flux.Window(flux.ProgressBar(flux.Key("subject"), flux.Value(value)))
		}},
		{"ScrollBox", "ScrollBox", "", true, func(updated bool) flux.Widget {
			return flux.Window(flux.ScrollBox(flux.Text(contractText(updated))))
		}},
		{"ListView", "ListView", "subject", true, func(updated bool) flux.Widget {
			prefix := contractText(updated)
			return flux.Window(flux.Expanded(flux.ListView(100, 20, func(index int) flux.Widget {
				return flux.Text(fmt.Sprintf("%s-%d", prefix, index))
			}, flux.Key("subject"))))
		}},
		{"PageControl", "PageControl", "subject", true, func(updated bool) flux.Widget {
			selected := 0
			if updated {
				selected = 1
			}
			return flux.Window(flux.PageControl(
				flux.TabPage("A", flux.Input(), flux.Key("page-a")),
				flux.TabPage("B", flux.Input(), flux.Key("page-b")),
				flux.Key("subject"), flux.SelectedIndex(selected),
			))
		}},
		{"TabPage", "TabPage", "subject", true, func(updated bool) flux.Widget {
			return flux.Window(flux.PageControl(
				flux.TabPage(contractText(updated), flux.Input(), flux.Key("subject")),
			))
		}},
	}
}

func contractText(updated bool) string {
	if updated {
		return "after"
	}
	return "before"
}

func contractTarget(t *testing.T, root *diff.Element, tc controlContractCase) *diff.Element {
	t.Helper()
	if tc.targetKey != "" {
		return findByKey(t, root, tc.targetKey)
	}
	return findByType(t, root, tc.targetType)
}

// TestControlContractMatrixInventory 固定 7.3a 的 18 项基线，防止意外删除、重命名
// 或重复；新增公开构造器时必须同步此清单。PluginWidget 的动态注册契约由
// plugin_test.go 单独覆盖；ListViewRow 不是公开控件。
func TestControlContractMatrixInventory(t *testing.T) {
	want := []string{
		"Window", "Column", "Row", "Component", "Expanded", "Flexible", "Text", "Button",
		"Input", "Memo", "CheckBox", "RadioButton", "ComboBox", "ProgressBar", "ScrollBox",
		"ListView", "PageControl", "TabPage",
	}
	cases := controlContractCases()
	if len(cases) != len(want) {
		t.Fatalf("控件矩阵项数=%d，期望 %d", len(cases), len(want))
	}
	seen := make(map[string]struct{}, len(cases))
	for i, tc := range cases {
		if tc.name != want[i] {
			t.Fatalf("控件矩阵第 %d 项=%q，期望 %q", i, tc.name, want[i])
		}
		if _, exists := seen[tc.name]; exists {
			t.Fatalf("控件矩阵重复项 %q", tc.name)
		}
		seen[tc.name] = struct{}{}
	}
}

// TestControlContractMountPatchAndD7cMatrix 对 7.3a 的 18 个内建公开控件统一验证：
// mount 完整、相同树零 mutation、属性变化原地 patch 且不重建。
func TestControlContractMountPatchAndD7cMatrix(t *testing.T) {
	for _, tc := range controlContractCases() {
		t.Run(tc.name, func(t *testing.T) {
			mock := render.NewMock()
			app := flux.NewApp(mock)
			if err := app.Render(tc.build(false)); err != nil {
				t.Fatal(err)
			}
			target := contractTarget(t, app.Root(), tc)
			if target == nil {
				t.Fatalf("mount 后缺少 %s Element", tc.targetType)
			}
			mountedHandle := target.Handle
			created := false
			for _, op := range mock.Ops() {
				if op.Type == render.OpCreate && op.Handle == mountedHandle && op.Key == tc.targetType {
					created = true
				}
			}
			if created != tc.native {
				t.Fatalf("%s native create=%v，期望 %v", tc.targetType, created, tc.native)
			}

			base := len(mock.Ops())
			if err := app.Render(tc.build(false)); err != nil {
				t.Fatal(err)
			}
			if ops := mock.Ops()[base:]; len(ops) != 0 {
				t.Fatalf("D7c 相同树产生 %d 条 mutation: %+v", len(ops), ops)
			}

			base = len(mock.Ops())
			if err := app.Render(tc.build(true)); err != nil {
				t.Fatal(err)
			}
			ops := mock.Ops()[base:]
			if countOps(ops, render.OpCreate) != 0 || countOps(ops, render.OpDestroy) != 0 {
				t.Fatalf("D7a 属性 patch 重建了控件: %+v", ops)
			}
			if len(ops) == 0 {
				t.Fatal("属性变化未产生 patch mutation")
			}
			if got := contractTarget(t, app.Root(), tc).Handle; got != mountedHandle {
				t.Fatalf("属性 patch 改变句柄: %d -> %d", mountedHandle, got)
			}
		})
	}
}

type removableControlCase struct {
	name  string
	build func(configured bool) flux.Widget
}

// TestControlContractRemovedPropertyResetMatrix 用所有公开可配置 native 控件的
// Visible Opt 验证配置移除回落到挂载默认值。ScrollBox 没有公开 Opt，透明容器
// 则按契约不得把重置错误应用到继承的父句柄，因此不在此 native 属性矩阵中。
func TestControlContractRemovedPropertyResetMatrix(t *testing.T) {
	cases := []removableControlCase{
		{"Window", func(c bool) flux.Widget {
			args := []any{flux.Key("subject")}
			if c {
				args = append(args, flux.Visible(false))
			}
			return flux.Window(args...)
		}},
		{"Text", func(c bool) flux.Widget { return flux.Window(contractVisibleText(c)) }},
		{"Button", func(c bool) flux.Widget { return flux.Window(contractVisibleButton(c)) }},
		{"Input", func(c bool) flux.Widget { return flux.Window(contractVisibleInput(c)) }},
		{"Memo", func(c bool) flux.Widget { return flux.Window(contractVisibleMemo(c)) }},
		{"CheckBox", func(c bool) flux.Widget { return flux.Window(contractVisibleCheckBox(c)) }},
		{"RadioButton", func(c bool) flux.Widget { return flux.Window(contractVisibleRadioButton(c)) }},
		{"ComboBox", func(c bool) flux.Widget { return flux.Window(contractVisibleComboBox(c)) }},
		{"ProgressBar", func(c bool) flux.Widget { return flux.Window(contractVisibleProgressBar(c)) }},
		{"ListView", func(c bool) flux.Widget {
			opts := []flux.Opt{flux.Key("subject")}
			if c {
				opts = append(opts, flux.Visible(false))
			}
			return flux.Window(flux.Expanded(flux.ListView(10, 20, func(i int) flux.Widget {
				return flux.Text(fmt.Sprint(i))
			}, opts...)))
		}},
		{"PageControl", func(c bool) flux.Widget {
			args := []any{flux.TabPage("A", flux.Input(), flux.Key("a")), flux.Key("subject")}
			if c {
				args = append(args, flux.Visible(false))
			}
			return flux.Window(flux.PageControl(args...))
		}},
		{"TabPage", func(c bool) flux.Widget {
			opts := []flux.Opt{flux.Key("subject")}
			if c {
				opts = append(opts, flux.Visible(false))
			}
			return flux.Window(flux.PageControl(flux.TabPage("A", flux.Input(), opts...)))
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := render.NewMock()
			app := flux.NewApp(mock)
			if err := app.Render(tc.build(true)); err != nil {
				t.Fatal(err)
			}
			h := findByKey(t, app.Root(), "subject").Handle
			base := len(mock.Ops())
			if err := app.Render(tc.build(false)); err != nil {
				t.Fatal(err)
			}
			ops := mock.Ops()[base:]
			if countOps(ops, render.OpCreate)+countOps(ops, render.OpDestroy) != 0 {
				t.Fatalf("移除属性重建了控件: %+v", ops)
			}
			if !hasOp(ops, render.OpSetProperty, h, "Visible", true) {
				t.Fatalf("Visible 移除后未重置为 true: %+v", ops)
			}
		})
	}
}

func visibleOpts(configured bool) []flux.Opt {
	opts := []flux.Opt{flux.Key("subject")}
	if configured {
		opts = append(opts, flux.Visible(false))
	}
	return opts
}

func contractVisibleText(c bool) flux.Widget        { return flux.Text("x", visibleOpts(c)...) }
func contractVisibleButton(c bool) flux.Widget      { return flux.Button("x", visibleOpts(c)...) }
func contractVisibleInput(c bool) flux.Widget       { return flux.Input(visibleOpts(c)...) }
func contractVisibleMemo(c bool) flux.Widget        { return flux.Memo("x", visibleOpts(c)...) }
func contractVisibleCheckBox(c bool) flux.Widget    { return flux.CheckBox("x", visibleOpts(c)...) }
func contractVisibleRadioButton(c bool) flux.Widget { return flux.RadioButton("x", visibleOpts(c)...) }
func contractVisibleComboBox(c bool) flux.Widget    { return flux.ComboBox(visibleOpts(c)...) }
func contractVisibleProgressBar(c bool) flux.Widget { return flux.ProgressBar(visibleOpts(c)...) }

type clickEventControlCase struct {
	name  string
	build func(handler func(flux.Event)) flux.Widget
}

// TestControlContractEventRemovalMatrix 覆盖所有公开且接受 Opt 的 native 控件。
// 先证明事件可触发，再移除 Opt，要求旧 handler 变为 nil 且全程零重建。
func TestControlContractEventRemovalMatrix(t *testing.T) {
	cases := []clickEventControlCase{
		{"Window", func(h func(flux.Event)) flux.Widget { return contractEventWindow(h) }},
		{"Text", func(h func(flux.Event)) flux.Widget { return flux.Window(contractEventText(h)) }},
		{"Button", func(h func(flux.Event)) flux.Widget { return flux.Window(contractEventButton(h)) }},
		{"Input", func(h func(flux.Event)) flux.Widget { return flux.Window(contractEventInput(h)) }},
		{"Memo", func(h func(flux.Event)) flux.Widget { return flux.Window(contractEventMemo(h)) }},
		{"CheckBox", func(h func(flux.Event)) flux.Widget { return flux.Window(contractEventCheckBox(h)) }},
		{"RadioButton", func(h func(flux.Event)) flux.Widget { return flux.Window(contractEventRadioButton(h)) }},
		{"ComboBox", func(h func(flux.Event)) flux.Widget { return flux.Window(contractEventComboBox(h)) }},
		{"ProgressBar", func(h func(flux.Event)) flux.Widget { return flux.Window(contractEventProgressBar(h)) }},
		{"ListView", func(h func(flux.Event)) flux.Widget { return contractEventListView(h) }},
		{"PageControl", func(h func(flux.Event)) flux.Widget { return contractEventPageControl(h) }},
		{"TabPage", func(h func(flux.Event)) flux.Widget { return contractEventTabPage(h) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mock := render.NewMock()
			app := flux.NewApp(mock)
			calls := 0
			if err := app.Render(tc.build(func(flux.Event) { calls++ })); err != nil {
				t.Fatal(err)
			}
			target := findByKey(t, app.Root(), "subject")
			h := target.Handle
			bound, ok := mock.EventHandler(h, "OnClick").(func(render.Event))
			if !ok {
				t.Fatal("OnClick 未绑定到 Mock")
			}
			bound(render.Event{})
			if calls != 1 {
				t.Fatalf("OnClick calls=%d，期望 1", calls)
			}

			base := len(mock.Ops())
			if err := app.Render(tc.build(nil)); err != nil {
				t.Fatal(err)
			}
			ops := mock.Ops()[base:]
			if countOps(ops, render.OpCreate)+countOps(ops, render.OpDestroy) != 0 {
				t.Fatalf("事件解绑重建了控件: %+v", ops)
			}
			if mock.EventHandler(h, "OnClick") != nil {
				t.Fatal("移除 OnClick 后旧 handler 仍存在")
			}
			if !hasOp(ops, render.OpSetEvent, h, "OnClick", nil) {
				t.Fatalf("缺少 OnClick=nil mutation: %+v", ops)
			}
		})
	}
}

// TestControlContractTypedEventRemovalMatrix 覆盖各窄 Renderer 能力的专属事件，
// 防止通用 OnClick 解绑通过而文本/选择/勾选回调仍残留。
func TestControlContractTypedEventRemovalMatrix(t *testing.T) {
	for _, tc := range []struct {
		name string
		memo bool
	}{{"Input.OnChange", false}, {"Memo.OnChange", true}} {
		t.Run(tc.name, func(t *testing.T) {
			mock := render.NewMock()
			app := flux.NewApp(mock)
			calls := 0
			build := func(bound bool) flux.Widget {
				opts := []flux.Opt{flux.Key("subject")}
				if bound {
					opts = append(opts, flux.OnChange(func(string) { calls++ }))
				}
				if tc.memo {
					return flux.Window(flux.Memo("text", opts...))
				}
				return flux.Window(flux.Input(opts...))
			}
			if err := app.Render(build(true)); err != nil {
				t.Fatal(err)
			}
			h := findByKey(t, app.Root(), "subject").Handle
			mock.EventHandler(h, "OnChange").(func(string))("before")
			if calls != 1 {
				t.Fatalf("OnChange calls=%d，期望 1", calls)
			}
			base := len(mock.Ops())
			if err := app.Render(build(false)); err != nil {
				t.Fatal(err)
			}
			ops := mock.Ops()[base:]
			if mock.EventHandler(h, "OnChange") != nil || !hasOp(ops, render.OpSetEvent, h, "OnChange", nil) {
				t.Fatalf("OnChange 未解绑: %+v", ops)
			}
			assertNoContractRebuild(t, ops)
		})
	}

	for _, tc := range []struct {
		name  string
		radio bool
	}{{"CheckBox.OnCheckedChange", false}, {"RadioButton.OnCheckedChange", true}} {
		t.Run(tc.name, func(t *testing.T) {
			mock := render.NewMock()
			app := flux.NewApp(mock)
			calls := 0
			build := func(bound bool) flux.Widget {
				opts := []flux.Opt{flux.Key("subject")}
				if bound {
					opts = append(opts, flux.OnCheckedChange(func(bool) { calls++ }))
				}
				if tc.radio {
					return flux.Window(flux.RadioButton("choice", opts...))
				}
				return flux.Window(flux.CheckBox("choice", opts...))
			}
			if err := app.Render(build(true)); err != nil {
				t.Fatal(err)
			}
			h := findByKey(t, app.Root(), "subject").Handle
			mock.FireCheckedChange(h, true)
			if calls != 1 {
				t.Fatalf("OnCheckedChange calls=%d，期望 1", calls)
			}
			base := len(mock.Ops())
			if err := app.Render(build(false)); err != nil {
				t.Fatal(err)
			}
			ops := mock.Ops()[base:]
			mock.FireCheckedChange(h, false)
			if calls != 1 || !hasOp(ops, render.OpSetEvent, h, "OnCheckedChange", nil) {
				t.Fatalf("OnCheckedChange 未解绑: calls=%d ops=%+v", calls, ops)
			}
			assertNoContractRebuild(t, ops)
		})
	}

	t.Run("ComboBox.OnSelectionChange", func(t *testing.T) {
		mock := render.NewMock()
		app := flux.NewApp(mock)
		calls := 0
		build := func(bound bool) flux.Widget {
			opts := []flux.Opt{flux.Key("subject"), flux.Items([]string{"a", "b"})}
			if bound {
				opts = append(opts, flux.OnSelectionChange(func(int) { calls++ }))
			}
			return flux.Window(flux.ComboBox(opts...))
		}
		if err := app.Render(build(true)); err != nil {
			t.Fatal(err)
		}
		h := findByKey(t, app.Root(), "subject").Handle
		mock.FireSelectionChange(h, 1)
		base := len(mock.Ops())
		if err := app.Render(build(false)); err != nil {
			t.Fatal(err)
		}
		ops := mock.Ops()[base:]
		mock.FireSelectionChange(h, 0)
		if calls != 1 || !hasOp(ops, render.OpSetEvent, h, "OnSelectionChange", nil) {
			t.Fatalf("ComboBox OnSelectionChange 未解绑: calls=%d ops=%+v", calls, ops)
		}
		assertNoContractRebuild(t, ops)
	})

	t.Run("PageControl.OnSelectionChange", func(t *testing.T) {
		mock := render.NewMock()
		app := flux.NewApp(mock)
		calls := 0
		build := func(bound bool) flux.Widget {
			args := []any{
				flux.TabPage("A", flux.Input(), flux.Key("a")),
				flux.TabPage("B", flux.Input(), flux.Key("b")),
				flux.Key("subject"),
			}
			if bound {
				args = append(args, flux.OnSelectionChange(func(int) { calls++ }))
			}
			return flux.Window(flux.PageControl(args...))
		}
		if err := app.Render(build(true)); err != nil {
			t.Fatal(err)
		}
		h := findByKey(t, app.Root(), "subject").Handle
		mock.FirePageSelectionChange(h, 1)
		base := len(mock.Ops())
		if err := app.Render(build(false)); err != nil {
			t.Fatal(err)
		}
		ops := mock.Ops()[base:]
		mock.FirePageSelectionChange(h, 0)
		if calls != 1 || !hasOp(ops, render.OpSetEvent, h, "OnSelectionChange", nil) {
			t.Fatalf("PageControl OnSelectionChange 未解绑: calls=%d ops=%+v", calls, ops)
		}
		assertNoContractRebuild(t, ops)
	})
}

func eventOpts(h func(flux.Event)) []flux.Opt {
	opts := []flux.Opt{flux.Key("subject")}
	if h != nil {
		opts = append(opts, flux.OnClick(h))
	}
	return opts
}

func contractEventWindow(h func(flux.Event)) flux.Widget {
	args := []any{flux.Key("subject")}
	if h != nil {
		args = append(args, flux.OnClick(h))
	}
	return flux.Window(args...)
}
func contractEventText(h func(flux.Event)) flux.Widget   { return flux.Text("x", eventOpts(h)...) }
func contractEventButton(h func(flux.Event)) flux.Widget { return flux.Button("x", eventOpts(h)...) }
func contractEventInput(h func(flux.Event)) flux.Widget  { return flux.Input(eventOpts(h)...) }
func contractEventMemo(h func(flux.Event)) flux.Widget   { return flux.Memo("x", eventOpts(h)...) }
func contractEventCheckBox(h func(flux.Event)) flux.Widget {
	return flux.CheckBox("x", eventOpts(h)...)
}
func contractEventRadioButton(h func(flux.Event)) flux.Widget {
	return flux.RadioButton("x", eventOpts(h)...)
}
func contractEventComboBox(h func(flux.Event)) flux.Widget { return flux.ComboBox(eventOpts(h)...) }
func contractEventProgressBar(h func(flux.Event)) flux.Widget {
	return flux.ProgressBar(eventOpts(h)...)
}
func contractEventListView(h func(flux.Event)) flux.Widget {
	return flux.Window(flux.Expanded(flux.ListView(10, 20, func(i int) flux.Widget {
		return flux.Text(fmt.Sprint(i))
	}, eventOpts(h)...)))
}
func contractEventPageControl(h func(flux.Event)) flux.Widget {
	args := []any{flux.TabPage("A", flux.Input(), flux.Key("a")), flux.Key("subject")}
	if h != nil {
		args = append(args, flux.OnClick(h))
	}
	return flux.Window(flux.PageControl(args...))
}
func contractEventTabPage(h func(flux.Event)) flux.Widget {
	return flux.Window(flux.PageControl(flux.TabPage("A", flux.Input(), eventOpts(h)...)))
}

// TestControlContractScrollEventRemoval 验证 ListView 移除 ScrollOffset 后必须解绑
// 原生回调，防止已移除的旧 State 继续收到滚动回写。
func TestControlContractScrollEventRemoval(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	scroll := flux.NewState(0)
	build := func(bound bool) flux.Widget {
		opts := []flux.Opt{flux.Key("list")}
		if bound {
			opts = append(opts, flux.ScrollOffset(scroll))
		}
		return flux.Window(flux.Expanded(flux.ListView(100, 20, func(i int) flux.Widget {
			return flux.Text(fmt.Sprint(i))
		}, opts...)))
	}
	if err := app.Render(build(true)); err != nil {
		t.Fatal(err)
	}
	h := findByKey(t, app.Root(), "list").Handle
	base := len(mock.Ops())
	if err := app.Render(build(false)); err != nil {
		t.Fatal(err)
	}
	ops := mock.Ops()[base:]
	unbinds := 0
	for _, op := range ops {
		if op.Type == render.OpSetEvent && op.Handle == h {
			if op.Key != "Scroll" {
				t.Fatalf("移除 ScrollOffset 产生伪事件解绑 %q: %+v", op.Key, ops)
			}
			unbinds++
		}
	}
	if unbinds != 1 {
		t.Fatalf("移除 ScrollOffset 的 OnScroll 解绑次数=%d，期望 1: %+v", unbinds, ops)
	}
	mock.FireScroll(h, 80)
	if got := scroll.Get(); got != 0 {
		t.Fatalf("解绑后旧 State 仍收到滚动回写: %d", got)
	}
}

// TestControlContractStateWritebackMatrix 统一覆盖全部有用户输入回写语义的控件；
// 每个回写都必须更新 State，同时保持目标 native handle 不变。
func TestControlContractStateWritebackMatrix(t *testing.T) {
	t.Run("Button", func(t *testing.T) {
		mock := render.NewMock()
		app := flux.NewApp(mock)
		state := flux.NewState(0)
		if err := app.Mount(func() flux.Widget {
			return flux.Window(flux.Button(flux.Bind(state), flux.Key("subject"), flux.OnClick(func(flux.Event) {
				state.Set(state.Get() + 1)
			})))
		}); err != nil {
			t.Fatal(err)
		}
		h := findByKey(t, app.Root(), "subject").Handle
		base := len(mock.Ops())
		mock.EventHandler(h, "OnClick").(func(render.Event))(render.Event{})
		ops := mock.Ops()[base:]
		assertContractWriteback(t, mock, app, h,
			state.Get() == 1 && findByKey(t, app.Root(), "subject").Props.String("Text") == "1")
		if !hasOp(ops, render.OpSetText, h, "", "1") {
			t.Fatalf("Button State 回写缺少 Text patch: %+v", ops)
		}
		assertNoContractRebuild(t, ops)
	})

	for _, tc := range []struct {
		name string
		memo bool
	}{{"Input", false}, {"Memo", true}} {
		t.Run(tc.name, func(t *testing.T) {
			mock := render.NewMock()
			app := flux.NewApp(mock)
			state := flux.NewState("before")
			if err := app.Mount(func() flux.Widget {
				if tc.memo {
					return flux.Window(flux.Memo(flux.Bind(state), flux.Key("subject")))
				}
				return flux.Window(flux.Input(flux.Bind(state), flux.Key("subject")))
			}); err != nil {
				t.Fatal(err)
			}
			h := findByKey(t, app.Root(), "subject").Handle
			base := len(mock.Ops())
			mock.EventHandler(h, "OnChange").(func(string))("after")
			ops := mock.Ops()[base:]
			assertContractWriteback(t, mock, app, h,
				state.Get() == "after" && findByKey(t, app.Root(), "subject").Props.String("Text") == "after")
			if !hasOp(ops, render.OpSetText, h, "", "after") {
				t.Fatalf("%s State 回写缺少 Text patch: %+v", tc.name, ops)
			}
			assertNoContractRebuild(t, ops)
		})
	}

	for _, tc := range []struct {
		name  string
		radio bool
	}{{"CheckBox", false}, {"RadioButton", true}} {
		t.Run(tc.name, func(t *testing.T) {
			mock := render.NewMock()
			app := flux.NewApp(mock)
			state := flux.NewState(false)
			if err := app.Mount(func() flux.Widget {
				opts := []flux.Opt{flux.Key("subject"), flux.Checked(state.Get()), flux.OnCheckedChange(state.Set)}
				var control flux.Widget = flux.CheckBox("choice", opts...)
				if tc.radio {
					control = flux.RadioButton("choice", opts...)
				}
				return flux.Window(flux.Column(control, flux.Text(flux.Bind(state))))
			}); err != nil {
				t.Fatal(err)
			}
			h := findByKey(t, app.Root(), "subject").Handle
			base := len(mock.Ops())
			mock.FireCheckedChange(h, true)
			assertContractWriteback(t, mock, app, h, state.Get() && findByKey(t, app.Root(), "subject").Props.Bool("Checked"))
			assertNoContractRebuild(t, mock.Ops()[base:])
		})
	}

	t.Run("ComboBox", func(t *testing.T) {
		mock := render.NewMock()
		app := flux.NewApp(mock)
		state := flux.NewState(0)
		if err := app.Mount(func() flux.Widget {
			return flux.Window(flux.Column(
				flux.ComboBox(flux.Key("subject"), flux.Items([]string{"a", "b"}),
					flux.SelectedIndex(state.Get()), flux.OnSelectionChange(state.Set)),
				flux.Text(flux.Bind(state)),
			))
		}); err != nil {
			t.Fatal(err)
		}
		h := findByKey(t, app.Root(), "subject").Handle
		base := len(mock.Ops())
		mock.FireSelectionChange(h, 1)
		assertContractWriteback(t, mock, app, h, state.Get() == 1 && findByKey(t, app.Root(), "subject").Props.Int("SelectedIndex") == 1)
		assertNoContractRebuild(t, mock.Ops()[base:])
	})

	t.Run("PageControl", func(t *testing.T) {
		mock := render.NewMock()
		app := flux.NewApp(mock)
		state := flux.NewState(0)
		if err := app.Mount(func() flux.Widget {
			return flux.Window(flux.Column(
				flux.PageControl(
					flux.TabPage("A", flux.Input(), flux.Key("a")),
					flux.TabPage("B", flux.Input(), flux.Key("b")),
					flux.Key("subject"), flux.SelectedIndex(state.Get()), flux.OnSelectionChange(state.Set),
				),
				flux.Text(flux.Bind(state)),
			))
		}); err != nil {
			t.Fatal(err)
		}
		h := findByKey(t, app.Root(), "subject").Handle
		base := len(mock.Ops())
		mock.FirePageSelectionChange(h, 1)
		ops := mock.Ops()[base:]
		assertContractWriteback(t, mock, app, h,
			state.Get() == 1 && mock.PageSelectedIndex(h) == 1 &&
				findByKey(t, app.Root(), "subject").Props.Int("SelectedIndex") == 1)
		if !hasOp(ops, render.OpSetProperty, h, "SelectedIndex", 1) {
			t.Fatalf("PageControl State 回写缺少 SelectedIndex patch: %+v", ops)
		}
		assertNoContractRebuild(t, ops)
	})

	t.Run("ListView", func(t *testing.T) {
		mock := render.NewMock()
		app := flux.NewApp(mock)
		// 两个偏移都处于列表中段，overscan 控件池基数保持不变。
		state := flux.NewState(5000)
		if err := app.Mount(func() flux.Widget {
			return flux.Window(flux.Expanded(flux.ListView(100000, 20, func(i int) flux.Widget {
				return flux.Text(fmt.Sprint(i))
			}, flux.Key("subject"), flux.ScrollOffset(state))))
		}); err != nil {
			t.Fatal(err)
		}
		h := findByKey(t, app.Root(), "subject").Handle
		base := len(mock.Ops())
		mock.FireScroll(h, 5200)
		assertContractWriteback(t, mock, app, h, state.Get() == 5200 && mock.ScrollPos(h) == 5200)
		assertNoContractRebuild(t, mock.Ops()[base:])
	})
}

func assertContractWriteback(t *testing.T, mock *render.Mock, app *flux.App, handle render.Handle, ok bool) {
	t.Helper()
	if !ok {
		t.Fatal("State 回写未收敛到声明树/native 状态")
	}
	if got := findByKey(t, app.Root(), "subject").Handle; got != handle {
		t.Fatalf("State 回写改变句柄: %d -> %d", handle, got)
	}
	if !mock.HandleAllocated(handle) {
		t.Fatalf("State 回写后句柄 %d 无效", handle)
	}
}

func assertNoContractRebuild(t *testing.T, ops []render.Op) {
	t.Helper()
	if countOps(ops, render.OpCreate)+countOps(ops, render.OpDestroy) != 0 {
		t.Fatalf("State 回写重建了控件: %+v", ops)
	}
}

type capabilitylessControlRenderer struct{ render.Renderer }

// TestControlContractMissingCapabilitiesAreSafe 一次覆盖 7.3a 新控件使用的全部
// 可选 Renderer 能力；缺失能力只能退化，不能 panic 或破坏 Element 树。
func TestControlContractMissingCapabilitiesAreSafe(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(capabilitylessControlRenderer{Renderer: mock})
	scroll := flux.NewState(0)
	defer func() {
		if recovered := recover(); recovered != nil {
			t.Fatalf("可选 Renderer 能力缺失时 panic: %v", recovered)
		}
	}()
	err := app.Render(flux.Window(flux.Column(
		flux.CheckBox("check", flux.Key("check"), flux.Checked(true), flux.OnCheckedChange(func(bool) {})),
		flux.RadioButton("radio", flux.Key("radio"), flux.Checked(true), flux.GroupIndex(1), flux.OnCheckedChange(func(bool) {})),
		flux.ComboBox(flux.Key("combo"), flux.Items([]string{"a"}), flux.SelectedIndex(0), flux.OnSelectionChange(func(int) {})),
		flux.ProgressBar(flux.Key("progress"), flux.Value(10)),
		flux.Expanded(flux.ListView(100, 20, func(i int) flux.Widget { return flux.Text(fmt.Sprint(i)) }, flux.Key("list"), flux.ScrollOffset(scroll))),
		flux.PageControl(
			flux.TabPage("A", flux.Input(), flux.Key("page")),
			flux.Key("pages"), flux.SelectedIndex(0), flux.OnSelectionChange(func(int) {}),
		),
	)))
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"check", "radio", "combo", "progress", "list", "pages", "page"} {
		if findByKey(t, app.Root(), key) == nil {
			t.Fatalf("能力缺失后 Element %q 丢失", key)
		}
	}
}
