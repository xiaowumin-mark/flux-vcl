package flux_test

import (
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// Phase 4 事件系统与生命周期无头测试：App+Mock 全链路（build → 布局 → diff →
// 注册回调），用 Mock.EventHandler 触发事件回调（mock 无原生消息循环）。
// 复用 flux_test.go 的 findByKey 辅助。

// TestUnifiedEventClick 统一事件：OnClick 收 func(Event)，diff 注入
// Source（"Type#Key"），mock 触发后字段齐全。
func TestUnifiedEventClick(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	var got flux.Event
	app.Render(flux.Window(flux.Column(
		flux.Button("OK", flux.Key("ok"), flux.OnClick(func(e flux.Event) {
			got = e
		})),
	)))

	btn := findByKey(t, app.Root(), "ok")
	fn, ok := m.EventHandler(btn.Handle, "OnClick").(func(flux.Event))
	if !ok {
		t.Fatal("OnClick 回调未注册（或类型不对）")
	}
	fn(flux.Event{Type: flux.EventClick})

	if got.Type != flux.EventClick {
		t.Errorf("got.Type = %v，期望 EventClick", got.Type)
	}
	if got.Source != "Button#ok" {
		t.Errorf("got.Source = %q，期望 Button#ok（diff 注入 Type#Key）", got.Source)
	}
}

// TestMouseEventMapping 鼠标按下事件：坐标/按键/修饰键与 Source 完整传递。
func TestMouseEventMapping(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	var got flux.Event
	app.Render(flux.Window(flux.Column(
		flux.Button("go", flux.Key("go"), flux.OnMouseDown(func(e flux.Event) {
			got = e
		})),
	)))

	btn := findByKey(t, app.Root(), "go")
	m.EventHandler(btn.Handle, "OnMouseDown").(func(flux.Event))(
		flux.Event{Type: flux.EventMouseDown, X: 5, Y: 6, Button: flux.ButtonLeft, Mods: flux.ModCtrl},
	)

	if got.X != 5 || got.Y != 6 {
		t.Errorf("坐标 = (%d,%d)，期望 (5,6)", got.X, got.Y)
	}
	if got.Button != flux.ButtonLeft || got.Mods != flux.ModCtrl {
		t.Errorf("Button/Mods = %v/%v，期望 ButtonLeft/ModCtrl", got.Button, got.Mods)
	}
	if got.Source != "Button#go" {
		t.Errorf("got.Source = %q，期望 Button#go", got.Source)
	}
}

// TestKeyPressTextRouted 字符输入（IME/中文）事件：OnKeyPress 的 Text 携带
// UTF-8 字符（native 经 OnUTF8KeyPress 提供组合结果，mock 直传）。
func TestKeyPressTextRouted(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	var got flux.Event
	app.Render(flux.Window(flux.Column(
		flux.Input(flux.Key("in"), flux.OnKeyPress(func(e flux.Event) {
			got = e
		})),
	)))

	in := findByKey(t, app.Root(), "in")
	m.EventHandler(in.Handle, "OnKeyPress").(func(flux.Event))(
		flux.Event{Type: flux.EventKeyPress, Text: "中"},
	)
	if got.Text != "中" {
		t.Errorf("got.Text = %q，期望 中（UTF-8 字符透传）", got.Text)
	}
	if got.Type != flux.EventKeyPress {
		t.Errorf("got.Type = %v，期望 EventKeyPress", got.Type)
	}
}

// TestLifecycleMountUpdateUnmount 生命周期钩子：挂载/更新/卸载触发时机。
// OnUpdate 为 didUpdateWidget 语义：仅当节点真实属性变化（文本等）才触发，
// 相同树不触发（避免每次 render 都回调 → 钩子内 Set State 无限循环）。
func TestLifecycleMountUpdateUnmount(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	var mount, update, unmount int
	tree := func(key, label string) flux.Widget {
		return flux.Window(flux.Column(
			flux.Button(label, flux.Key(key),
				flux.OnMount(func() { mount++ }),
				flux.OnUpdate(func() { update++ }),
				flux.OnUnmount(func() { unmount++ }),
			),
		))
	}

	app.Render(tree("a", "A"))
	if mount != 1 || update != 0 || unmount != 0 {
		t.Errorf("首次挂载后 mount/update/unmount = %d/%d/%d，期望 1/0/0", mount, update, unmount)
	}

	app.Render(tree("a", "A")) // 相同树：不重挂载，也无真实变化 → 不触发 OnUpdate
	if mount != 1 || update != 0 || unmount != 0 {
		t.Errorf("相同树二次渲染 mount/update/unmount = %d/%d/%d，期望 1/0/0", mount, update, unmount)
	}

	app.Render(tree("a", "B")) // 文本变化 → OnUpdate
	if mount != 1 || update != 1 || unmount != 0 {
		t.Errorf("属性变化 mount/update/unmount = %d/%d/%d，期望 1/1/0", mount, update, unmount)
	}

	app.Render(flux.Window(flux.Column(flux.Text("gone")))) // 移除 Button
	if mount != 1 || unmount != 1 {
		t.Errorf("移除后 mount/unmount = %d/%d，期望 1/1", mount, unmount)
	}

	app.Render(tree("b", "C")) // key 变化 → 重建：新挂载
	if mount != 2 || unmount != 1 {
		t.Errorf("重建后 mount/unmount = %d/%d，期望 2/1（旧节点卸载，新节点挂载）", mount, unmount)
	}
}

// TestOnUnmountBeforeDestroy OnUnmount 必须先于 Destroy op（卸载时入队销毁，
// D4：钩子在物理释放前清理资源）。
func TestOnUnmountBeforeDestroy(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	opsAtUnmount := -1
	app.Render(flux.Window(flux.Column(
		flux.Button("A", flux.Key("a"), flux.OnUnmount(func() {
			opsAtUnmount = len(m.Ops())
		})),
	)))
	app.Render(flux.Window(flux.Column(flux.Text("gone"))))

	ops := m.Ops()
	if opsAtUnmount < 0 || opsAtUnmount >= len(ops) {
		t.Fatalf("OnUnmount 未在销毁前触发（opsAtUnmount=%d, len=%d）", opsAtUnmount, len(ops))
	}
	if ops[opsAtUnmount].Type != render.OpDestroy {
		t.Errorf("OnUnmount 之后首条 op = %v，期望 OpDestroy（钩子先于销毁）", ops[opsAtUnmount])
	}
}

// TestLifecycleHooksZeroMutation 带生命周期钩子的相同树二次 diff 零 mutation
// （D7c 护栏：钩子是函数值、恒判为变化，但 applyProp 跳过不产生 op）。
func TestLifecycleHooksZeroMutation(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	tree := func() flux.Widget {
		return flux.Window(flux.Column(
			flux.Button("A", flux.Key("a"),
				flux.OnMount(func() {}),
				flux.OnUpdate(func() {}),
				flux.OnUnmount(func() {}),
			),
		))
	}
	app.Render(tree())
	base := len(m.Ops())
	app.Render(tree())
	if n := len(m.Ops()) - base; n != 0 {
		t.Errorf("带钩子相同树二次 diff 应零 mutation，实际 %d 条：%+v", n, m.Ops()[base:])
	}
}

// TestStateSetInsideLifecycleNoDeadlock 生命周期钩子内 Set State 不重入自锁：
// OnMount 触发 State.Set → invalidate → renderWidget 重入，靠 inRender 守卫
// 排队并由当前 render 结束 flush（Phase 4.3 工程发现：非重入 renderMu 自锁 +
// 无限递归）。测试能返回即通过（死锁会 timeout）。
//
// 必须用 Mount（有 build 函数）：Render 是单树手动路径，无 build，flush 为 no-op
// （App.render 里 build==nil 直接返回）—— State 自动更新语义只存在于 Mount 路径。
func TestStateSetInsideLifecycleNoDeadlock(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	life := flux.NewState("0")
	mounted := false
	app.Mount(func() flux.Widget {
		return flux.Window(flux.Column(
			flux.Button("A", flux.Key("a"), flux.OnMount(func() {
				if !mounted {
					mounted = true
					life.Set("1") // render 期间 Set State → 排队 flush，不重入
				}
			})),
			flux.Text(flux.Bind(life), flux.Key("life")),
		))
	})

	if !mounted {
		t.Fatal("OnMount 未触发")
	}
	// 收敛：flush render 把 life 文本从 "0" patch 为 "1"，且为该文本的最后一次
	// mutation（按钮无真实属性变化不触发 OnUpdate，无二次 Set；无结构变化不发
	// 重挂载 op）。
	ops := m.Ops()
	var lifeHandle render.Handle
	for _, op := range ops {
		if op.Type == render.OpSetText {
			if s, ok := op.Value.(string); ok && s == "0" {
				lifeHandle = op.Handle // 挂载期 Text 初值
			}
		}
	}
	if lifeHandle == 0 {
		t.Fatalf("未找到 life 文本初值 SetText：%+v", ops)
	}
	last := ops[len(ops)-1]
	if last.Type != render.OpSetText || last.Handle != lifeHandle {
		t.Fatalf("最后 op = %+v，期望 life(Handle=%d) 的 SetText", last, lifeHandle)
	}
	if s, _ := last.Value.(string); s != "1" {
		t.Errorf("life 文本 = %q，期望 flush 收敛到 1", s)
	}
}

// TestModifierComposition 修饰键按位组合可组合断言（ModCtrl|ModShift）。
func TestModifierComposition(t *testing.T) {
	if flux.ModShift|flux.ModCtrl == 0 {
		t.Error("修饰键常量组合为 0")
	}
	var ev flux.Event
	ev.Mods = flux.ModShift | flux.ModCtrl
	if ev.Mods&flux.ModCtrl == 0 || ev.Mods&flux.ModShift == 0 {
		t.Error("按位组合断言失败")
	}
	if ev.Mods&flux.ModAlt != 0 {
		t.Error("未设置的修饰键不应出现")
	}
}
