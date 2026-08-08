package flux_test

import (
	"errors"
	"math"
	"testing"
	"time"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/diff"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// Phase 5 高级特性无头测试：动画（Curve/Tween/Controller/App.Animate/SetBounds）、
// Theme（Color/FontColor Opt + 属性 patch）、Component（透明分组）、Async（marshalling）。
// 全部走 App+Mock，不依赖 DLL/显示环境（与 event_test.go 同一驱动）。

// —— 5.1 动画 ——

// TestCurves 内置缓动曲线的端点与单调性。
func TestCurves(t *testing.T) {
	if got := flux.LinearCurve(0); got != 0 {
		t.Errorf("LinearCurve(0) = %v，期望 0", got)
	}
	if got := flux.LinearCurve(1); got != 1 {
		t.Errorf("LinearCurve(1) = %v，期望 1", got)
	}
	if got := flux.EaseIn(0.5); got != 0.25 {
		t.Errorf("EaseIn(0.5) = %v，期望 0.25", got)
	}
	if got := flux.EaseOut(0.5); got != 0.75 {
		t.Errorf("EaseOut(0.5) = %v，期望 0.75", got)
	}
	if got := flux.EaseInOut(0.25); got != 0.125 {
		t.Errorf("EaseInOut(0.25) = %v，期望 0.125", got)
	}
	if got := flux.EaseInOut(0.75); got != 0.875 {
		t.Errorf("EaseInOut(0.75) = %v，期望 0.875", got)
	}
	// 非单调：ElasticOut 中段可 >1（过冲）
	if got := flux.ElasticOut(0.5); got <= 0 || got > 1.2 {
		t.Errorf("ElasticOut(0.5) = %v，期望过冲（0<v<=1.2）", got)
	}
	if got := flux.ElasticOut(0); got != 0 || flux.ElasticOut(1) != 1 {
		t.Errorf("ElasticOut 端点 = %v/%v，期望 0/1", flux.ElasticOut(0), flux.ElasticOut(1))
	}
}

// TestTween 数值插值（int 与 float64 类型推断）。
func TestTween(t *testing.T) {
	if got := flux.Tween(0, 160, 0.5); got != 80 {
		t.Errorf("Tween(0,160,0.5) = %d，期望 80", got)
	}
	if got := flux.Tween(10, 20, 0.25); got != 12 {
		t.Errorf("Tween(10,20,0.25) = %d，期望 12", got)
	}
	if got := flux.Tween(0.0, 1.0, 0.25); got != 0.25 {
		t.Errorf("Tween(0.0,1.0,0.25) = %v，期望 0.25", got)
	}
	if got := flux.Tween(100, 0, 0.0); got != 100 {
		t.Errorf("Tween(100,0,0) = %d，期望 100（起点）", got)
	}
}

// TestAnimationController 控制器状态机：Step 推进 → 曲线进度 → 终点 onEnd + done。
func TestAnimationController(t *testing.T) {
	ctrl := flux.NewAnimationController(100*time.Millisecond, flux.EaseInOut)
	var got []float64
	var ended bool
	ctrl.Start(func(v float64) { got = append(got, v) }, func() { ended = true })

	if !ctrl.Running() {
		t.Fatal("Start 后应 running")
	}
	v, done := ctrl.Step(50 * time.Millisecond)
	if done {
		t.Errorf("50/100ms 不应 done，实际 done=%v", done)
	}
	if !ctrl.Running() {
		t.Error("50/100ms 后仍应 running")
	}
	// EaseInOut(0.5) = 1-2*(1-0.5)^2 = 0.5（二次曲线中点为 0.5）
	if math.Abs(v-0.5) > 1e-9 {
		t.Errorf("EaseInOut(0.5) = %v，期望 0.5", v)
	}

	_, done = ctrl.Step(50 * time.Millisecond)
	if !done {
		t.Error("100/100ms 应 done")
	}
	if ctrl.Running() {
		t.Error("终点后应停止 running")
	}
	if !ended {
		t.Error("onEnd 应触发一次")
	}
	if got[len(got)-1] != 1.0 {
		t.Errorf("终点回调应收到 1.0，实际 %v", got[len(got)-1])
	}

	// 停止后再 Step：no-op（不回调、不 done）
	n := len(got)
	_, done = ctrl.Step(time.Second)
	if done || len(got) != n {
		t.Errorf("停止后 Step 应 no-op（done=%v, 回调数 %d→%d）", done, n, len(got))
	}
}

// TestAnimationControllerStop 提前 Stop：onEnd 不触发，Step 不再推进。
func TestAnimationControllerStop(t *testing.T) {
	ctrl := flux.NewAnimationController(time.Second, nil) // nil curve → linear
	var called bool
	ctrl.Start(func(float64) {}, func() { called = true })
	ctrl.Stop()
	ctrl.Step(time.Second)
	if called {
		t.Error("Stop 后不应触发 onEnd")
	}
	if ctrl.Running() {
		t.Error("Stop 后不应 running")
	}
}

// TestAppAnimate 动画 pump：Mock.NewTimer 手动 FireTimer 驱动，每帧收到线性进度，
// 到终点自动停表（后续 FireTimer no-op）。
func TestAppAnimate(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	var vals []float64
	stop := app.Animate(80*time.Millisecond, flux.LinearCurve, func(v float64) {
		vals = append(vals, v)
	})
	defer stop()

	for i := 0; i < 6; i++ {
		m.FireTimer() // 每帧 +16ms
	}
	want := []float64{0.2, 0.4, 0.6, 0.8, 1.0}
	if len(vals) != len(want) {
		t.Fatalf("onStep 次数 = %d，期望 %d：%v", len(vals), len(want), vals)
	}
	for i, w := range want {
		if math.Abs(vals[i]-w) > 1e-9 {
			t.Errorf("第 %d 帧 v = %v，期望 %v", i, vals[i], w)
		}
	}
	// 终点已停表：再 FireTimer 应无回调
	n := len(vals)
	m.FireTimer()
	if len(vals) != n {
		t.Errorf("动画结束后 FireTimer 不应回调（%d→%d）", n, len(vals))
	}
}

// TestAppAnimateStop 提前 stop：立即停表，不再回调。
func TestAppAnimateStop(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	var n int
	stop := app.Animate(time.Second, nil, func(float64) { n++ })
	m.FireTimer()
	if n != 1 {
		t.Fatalf("首帧应回调 1 次，实际 %d", n)
	}
	stop()
	m.FireTimer()
	if n != 1 {
		t.Errorf("stop 后不应再回调（%d）", n)
	}
}

// TestAppSetBoundsByKey 直接应用几何逃逸口（D2）：命中真实控件句柄 → SetBounds op；
// 透明容器 / Window / 未知 key 跳过。
func TestAppSetBoundsByKey(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	app.Render(flux.Window(flux.Column(
		flux.Button("A", flux.Key("btn")),
		flux.Column(flux.Text("x"), flux.Key("wrap")),
		flux.Text("t"),
	)))

	btn := findByKey(t, app.Root(), "btn")
	target := render.Rect{X: 10, Y: 20, W: 100, H: 40}
	app.SetBounds("btn", target)
	ops := m.Ops()
	last := ops[len(ops)-1]
	if last.Type != render.OpSetProperty || last.Key != "Bounds" || last.Handle != btn.Handle {
		t.Fatalf("SetBounds(btn) 后最后 op = %+v，期望 Bounds op 命中句柄 %d", last, btn.Handle)
	}
	if r, _ := last.Value.(render.Rect); r != target {
		t.Errorf("Bounds 值 = %+v，期望 %+v", r, target)
	}

	// 透明容器（Column key=wrap）无独立句柄 → 跳过
	base := len(m.Ops())
	app.SetBounds("wrap", target)
	if len(m.Ops()) != base {
		t.Errorf("SetBounds 透明容器不应产生 op")
	}
	// 未知 key → 跳过
	app.SetBounds("nope", target)
	if len(m.Ops()) != base {
		t.Errorf("SetBounds 未知 key 不应产生 op")
	}
}

// —— 5.2 Theme ——

// TestThemeColorOpts Color/FontColor Opt 写入 Props，diff 应用 SetColor/SetFontColor op，
// 且属性不变时零 mutation（D7c 兼容：Color 是可比 uint32）。
func TestThemeColorOpts(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	red := flux.RGB(0xFF, 0x00, 0x00)

	tree := func() flux.Widget {
		return flux.Window(
			flux.Column(
				flux.Button("A", flux.Key("btn"), flux.Color(red), flux.FontColor(red)),
			))
	}
	app.Render(tree())
	btn := findByKey(t, app.Root(), "btn")

	findOp := func(k string) *render.Op {
		for i := range m.Ops() {
			if m.Ops()[i].Key == k && m.Ops()[i].Handle == btn.Handle {
				return &m.Ops()[i]
			}
		}
		return nil
	}
	if op := findOp("Color"); op == nil || op.Value.(render.Color) != red {
		t.Errorf("Color op 缺失或值错：%+v", op)
	}
	if op := findOp("FontColor"); op == nil || op.Value.(render.Color) != red {
		t.Errorf("FontColor op 缺失或值错：%+v", op)
	}

	// 相同树二次 render：零 mutation（Color 可比 → ValuesEqual 命中）
	base := len(m.Ops())
	app.Render(tree())
	if len(m.Ops()) != base {
		t.Errorf("相同树二次 render 应零 mutation，实际 %d 条", len(m.Ops())-base)
	}
}

// TestThemeData 主题数据：Light/Dark 调色板齐全、RGB 构造正确。
func TestThemeData(t *testing.T) {
	lt, dt := flux.LightTheme, flux.DarkTheme
	for name, c := range map[string]flux.ColorValue{ // 关键字段非零
		"light.Primary": lt.Primary, "light.Background": lt.Background,
		"light.Text": lt.Text, "dark.Primary": dt.Primary, "dark.Text": dt.Text,
	} {
		if c == 0 {
			t.Errorf("%s 不应为零", name)
		}
	}
	if lt.Text == dt.Text {
		t.Error("Light/Dark 文字色应不同")
	}
	if got := flux.RGB(0x12, 0x34, 0x56); got != render.Color(0xFF123456) {
		t.Errorf("RGB(12,34,56) = %#x，期望 0xFF123456", got)
	}
}

// —— 5.4 Component ——

// TestComponent 组件透明分组：不产生原生控件（无 Create op），子树 = build 结果，
// 按外部 Key 稳定复用（重建时子控件原地 patch 不重建）。
func TestComponent(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)

	comp := func(label string) flux.Widget {
		return flux.Component(func() flux.Widget {
			return flux.Column(
				flux.Button(label, flux.Key("inner")),
			)
		}, flux.Key("card"))
	}

	app.Render(flux.Window(flux.Column(comp("A"))))
	// Component 透明：无 "Component" 的 Create op；其子按钮正常创建
	for _, op := range m.Ops() {
		if op.Type == render.OpCreate && op.Key == "Component" {
			t.Errorf("Component 不应创建原生控件：%+v", op)
		}
	}
	inner := findByKey(t, app.Root(), "inner")
	if inner == nil {
		t.Fatal("组件内子控件未挂载")
	}

	// 相同 key 组件、文本变化 → 子按钮原地 patch（不重建：Create 数不变）
	createCount := 0
	for _, op := range m.Ops() {
		if op.Type == render.OpCreate {
			createCount++
		}
	}
	app.Render(flux.Window(flux.Column(comp("B"))))
	after := 0
	for _, op := range m.Ops() {
		if op.Type == render.OpCreate {
			after++
		}
	}
	if after != createCount {
		t.Errorf("组件 key 不变时子控件应原地 patch，Create %d→%d", createCount, after)
	}
}

// —— 5.3 Async ——

// TestAsyncSuccess 后台 goroutine 执行 load → RunOnUI marshal → onSuccess。
// Mock.RunOnUI 内联执行，测试用 channel 同步断言（-race 验证跨 goroutine 安全）。
func TestAsyncSuccess(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	var got string
	done := make(chan struct{})
	flux.Async(app, func() (string, error) {
		return "result", nil
	}, func(s string) {
		got = s
		close(done)
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("async 未在 2s 内完成")
	}
	if got != "result" {
		t.Errorf("onSuccess 收到 %q，期望 result", got)
	}
}

// TestAsyncError load 返回 err → onError（不调 onSuccess）。
func TestAsyncError(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	boom := errors.New("boom")
	var gotErr error
	var successCalled bool
	done := make(chan struct{})
	flux.Async(app, func() (string, error) {
		return "", boom
	}, func(string) {
		successCalled = true
		close(done)
	}, func(err error) {
		gotErr = err
		close(done)
	})
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("async 未在 2s 内完成")
	}
	if successCalled {
		t.Error("load 出错时不应调用 onSuccess")
	}
	if gotErr != boom {
		t.Errorf("onError 收到 %v，期望 boom", gotErr)
	}
}

// diff.Element 引用仅用于类型检查（findByKey 定义于 flux_test.go）。
var _ = diff.Element{}
