package flux_test

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

var pluginTestID atomic.Uint64

func pluginTestName() string {
	return fmt.Sprintf("test.widget.%d", pluginTestID.Add(1))
}

func registerTestPlugin(t *testing.T, descriptor flux.WidgetPlugin) string {
	t.Helper()
	name := pluginTestName()
	if err := flux.RegisterWidget(name, descriptor); err != nil {
		t.Fatalf("RegisterWidget(%q): %v", name, err)
	}
	t.Cleanup(func() {
		if err := flux.UnregisterWidget(name); err != nil && !errors.Is(err, flux.ErrPluginNotRegistered) {
			t.Errorf("UnregisterWidget(%q): %v", name, err)
		}
	})
	return name
}

func newPluginApp(t *testing.T, renderer render.Renderer) *flux.App {
	t.Helper()
	app := flux.NewApp(renderer)
	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Errorf("App.Close: %v", err)
		}
	})
	return app
}

func pluginTree(name, label string) flux.Widget {
	properties := flux.NewPluginProperties(flux.PluginString("label", label))
	return flux.Window(flux.Column(
		flux.PluginWidget(name, properties, flux.Key("badge")),
	))
}

type pluginCreateWidget struct {
	create func() *flux.Node
}

func (w pluginCreateWidget) Create() *flux.Node { return w.create() }

// TestPluginBuilderLayoutLifecycleAndD7 验证组合式 builder、DIP 布局、实例生命周期，
// 以及 D7a/D7c：属性变化只 patch 内建子控件，相同树零 mutation。
func TestPluginBuilderLayoutLifecycleAndD7(t *testing.T) {
	var events []string
	var gotBackend string
	var gotDPI int
	descriptor := flux.WidgetPlugin{
		Init: func(ctx flux.PluginContext) error {
			gotBackend, _ = flux.LookupCapability(ctx, flux.RendererBackend)
			gotDPI, _ = flux.LookupCapability(ctx, flux.RendererDPI)
			events = append(events, "init")
			return nil
		},
		Build: func(ctx flux.PluginBuildContext) (flux.Widget, error) {
			label, ok := ctx.Properties.String("label")
			if !ok {
				return nil, errors.New("缺少 label")
			}
			return flux.Text("[" + label + "]"), nil
		},
		Measure: func(ctx flux.PluginMeasureContext) (flux.PluginLayout, error) {
			return flux.PluginLayout{
				Size:        flux.Size{W: ctx.ChildSize.W + 16, H: ctx.ChildSize.H + 8},
				ChildOffset: flux.Point{X: 8, Y: 4},
			}, nil
		},
		OnMount: func(flux.PluginInstanceContext) error {
			events = append(events, "mount")
			return nil
		},
		OnUpdate: func(flux.PluginInstanceContext) error {
			events = append(events, "update")
			return nil
		},
		OnUnmount: func(flux.PluginInstanceContext) error {
			events = append(events, "unmount")
			return nil
		},
		Close: func(flux.PluginContext) error {
			events = append(events, "close")
			return nil
		},
	}
	name := registerTestPlugin(t, descriptor)
	mock := render.NewMock()
	app := newPluginApp(t, mock)

	if err := app.Render(pluginTree(name, "A")); err != nil {
		t.Fatalf("首次 Render: %v", err)
	}
	if gotBackend != "mock" || gotDPI != 96 {
		t.Fatalf("Renderer capability = %q/%d，期望 mock/96", gotBackend, gotDPI)
	}
	pluginType := "Plugin:" + name
	var pluginDiag, childDiag *flux.NodeDiag
	for i := range app.Inspect() {
		diag := &app.Inspect()[i]
		if diag.Type == pluginType {
			copy := *diag
			pluginDiag = &copy
		}
		if diag.Type == "Text" {
			copy := *diag
			childDiag = &copy
		}
	}
	if pluginDiag == nil || childDiag == nil {
		t.Fatalf("Inspector 缺少插件/子树诊断：%+v", app.Inspect())
	}
	if pluginDiag.Size.W != childDiag.Size.W+16 || pluginDiag.Size.H != childDiag.Size.H+8 {
		t.Errorf("插件布局=%+v，子树=%+v，期望增加 16x8 DIP", pluginDiag.Size, childDiag.Size)
	}
	if childDiag.Frame.X-pluginDiag.Frame.X != 8 || childDiag.Frame.Y-pluginDiag.Frame.Y != 4 {
		t.Errorf("子树偏移=%+v，插件 frame=%+v，期望 (8,4)", childDiag.Frame, pluginDiag.Frame)
	}
	for _, op := range mock.Ops() {
		if op.Type == render.OpCreate && op.Key == pluginType {
			t.Fatalf("组合式插件不应进入 Renderer.Create：%+v", op)
		}
	}

	creates := mock.Count(render.OpCreate)
	destroys := mock.Count(render.OpDestroy)
	base := len(mock.Ops())
	if err := app.Render(pluginTree(name, "B")); err != nil {
		t.Fatalf("属性更新 Render: %v", err)
	}
	if mock.Count(render.OpCreate) != creates || mock.Count(render.OpDestroy) != destroys {
		t.Fatal("D7a: 插件属性变化不应重建原生控件")
	}
	updated := mock.Ops()[base:]
	if len(updated) != 1 || updated[0].Type != render.OpSetText || updated[0].Value != "[B]" {
		t.Fatalf("插件属性变化应只 patch Text，实际 %+v", updated)
	}

	base = len(mock.Ops())
	if err := app.Render(pluginTree(name, "B")); err != nil {
		t.Fatalf("相同树 Render: %v", err)
	}
	if got := len(mock.Ops()) - base; got != 0 {
		t.Fatalf("D7c: 相同插件树应零 mutation，实际 %d: %+v", got, mock.Ops()[base:])
	}
	if got := strings.Join(events, ","); got != "init,mount,update" {
		t.Fatalf("关闭前生命周期=%q，期望 init,mount,update", got)
	}

	if err := flux.UnregisterWidget(name); !errors.Is(err, flux.ErrPluginInUse) {
		t.Fatalf("活跃 App 注销错误=%v，期望 ErrPluginInUse", err)
	}
	if err := app.Close(); err != nil {
		t.Fatalf("App.Close: %v", err)
	}
	if got := strings.Join(events, ","); got != "init,mount,update,unmount,close" {
		t.Fatalf("完整生命周期=%q，期望 init,mount,update,unmount,close", got)
	}
	if err := flux.UnregisterWidget(name); err != nil {
		t.Fatalf("关闭后注销: %v", err)
	}
}

// TestPluginRegistrationErrors 验证保留名称、缺失 Build、重复与未知类型均可 errors.Is。
func TestPluginRegistrationErrors(t *testing.T) {
	valid := flux.WidgetPlugin{Build: func(flux.PluginBuildContext) (flux.Widget, error) {
		return flux.Text("x"), nil
	}}
	for _, name := range []string{"Button", "PageControl", "TabPage"} {
		if err := flux.RegisterWidget(name, valid); !errors.Is(err, flux.ErrPluginReserved) {
			t.Fatalf("内建名称 %q 错误=%v，期望 ErrPluginReserved", name, err)
		}
	}
	if err := flux.RegisterWidget("bad name", valid); !errors.Is(err, flux.ErrPluginInvalid) {
		t.Fatalf("非法名称错误=%v，期望 ErrPluginInvalid", err)
	}
	if err := flux.RegisterWidget(pluginTestName(), flux.WidgetPlugin{}); !errors.Is(err, flux.ErrPluginInvalid) {
		t.Fatalf("缺少 Build 错误=%v，期望 ErrPluginInvalid", err)
	}

	name := registerTestPlugin(t, valid)
	if err := flux.RegisterWidget(name, valid); !errors.Is(err, flux.ErrPluginAlreadyRegistered) {
		t.Fatalf("重复注册错误=%v，期望 ErrPluginAlreadyRegistered", err)
	}
	mock := render.NewMock()
	app := newPluginApp(t, mock)
	unknown := pluginTestName()
	err := app.Render(flux.Window(flux.PluginWidget(unknown, flux.NewPluginProperties())))
	if !errors.Is(err, flux.ErrPluginNotRegistered) {
		t.Fatalf("未知插件错误=%v，期望 ErrPluginNotRegistered", err)
	}
	if len(mock.Ops()) != 0 {
		t.Fatalf("未知插件不得产生半提交 mutation：%+v", mock.Ops())
	}
}

func TestPluginPropertiesAndCapabilitiesValidation(t *testing.T) {
	properties := flux.NewPluginProperties(
		flux.PluginString("label", "first"),
		flux.PluginInt("count", 3),
		flux.PluginBool("active", true),
		flux.PluginFloat("ratio", 1.5),
		flux.PluginString("label", "last"),
	)
	if got := strings.Join(properties.Keys(), ","); got != "label,count,active,ratio" {
		t.Fatalf("Keys=%q，期望稳定去重顺序", got)
	}
	if got, ok := properties.String("label"); !ok || got != "last" {
		t.Fatalf("label=%q/%v，期望 last/true", got, ok)
	}
	if got, ok := properties.Int("count"); !ok || got != 3 {
		t.Fatalf("count=%d/%v，期望 3/true", got, ok)
	}
	if got, ok := properties.Bool("active"); !ok || !got {
		t.Fatalf("active=%v/%v，期望 true/true", got, ok)
	}
	if got, ok := properties.Float("ratio"); !ok || got != 1.5 {
		t.Fatalf("ratio=%v/%v，期望 1.5/true", got, ok)
	}
	keys := properties.Keys()
	keys[0] = "mutated"
	if got := properties.Keys()[0]; got != "label" {
		t.Fatalf("Keys 未防御性复制：%q", got)
	}

	capability := flux.NewCapability[int]("example.plugin.answer")
	if capability.Name() != "example.plugin.answer" {
		t.Fatalf("Capability.Name=%q", capability.Name())
	}
	assertPanics(t, "非法 property 名称", func() { flux.PluginString("bad name", "x") })
	assertPanics(t, "非法 capability 名称", func() { flux.NewCapability[int]("answer") })
}

func assertPanics(t *testing.T, label string, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Errorf("%s 应 panic", label)
		}
	}()
	fn()
}

// TestPluginConcurrentRegistration 同名并发注册只能有一个成功，注册表在 -race 下安全。
func TestPluginConcurrentRegistration(t *testing.T) {
	name := pluginTestName()
	descriptor := flux.WidgetPlugin{Build: func(flux.PluginBuildContext) (flux.Widget, error) {
		return flux.Text("x"), nil
	}}
	const workers = 32
	var wg sync.WaitGroup
	results := make(chan error, workers)
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- flux.RegisterWidget(name, descriptor)
		}()
	}
	wg.Wait()
	close(results)
	success, duplicate := 0, 0
	for err := range results {
		switch {
		case err == nil:
			success++
		case errors.Is(err, flux.ErrPluginAlreadyRegistered):
			duplicate++
		default:
			t.Fatalf("并发注册意外错误: %v", err)
		}
	}
	if success != 1 || duplicate != workers-1 {
		t.Fatalf("并发注册成功/重复=%d/%d，期望 1/%d", success, duplicate, workers-1)
	}
	if err := flux.UnregisterWidget(name); err != nil {
		t.Fatalf("UnregisterWidget: %v", err)
	}
}

// TestPluginPreparationFailuresRollback 验证 init/build/measure 失败或 panic 在 commit 前回滚。
func TestPluginPreparationFailuresRollback(t *testing.T) {
	tests := []struct {
		name       string
		descriptor func(closed *atomic.Int32) flux.WidgetPlugin
		want       error
		wantClose  int32
	}{
		{
			name: "init-error",
			descriptor: func(*atomic.Int32) flux.WidgetPlugin {
				return flux.WidgetPlugin{
					Init:  func(flux.PluginContext) error { return errors.New("init failed") },
					Build: func(flux.PluginBuildContext) (flux.Widget, error) { return flux.Text("x"), nil },
				}
			},
			want: errors.New("init failed"),
		},
		{
			name: "build-panic",
			descriptor: func(closed *atomic.Int32) flux.WidgetPlugin {
				return flux.WidgetPlugin{
					Build: func(flux.PluginBuildContext) (flux.Widget, error) { panic("build boom") },
					Close: func(flux.PluginContext) error { closed.Add(1); return nil },
				}
			},
			want: flux.ErrPluginPanic, wantClose: 1,
		},
		{
			name: "measure-panic",
			descriptor: func(closed *atomic.Int32) flux.WidgetPlugin {
				return flux.WidgetPlugin{
					Build: func(flux.PluginBuildContext) (flux.Widget, error) { return flux.Text("x"), nil },
					Measure: func(flux.PluginMeasureContext) (flux.PluginLayout, error) {
						panic("measure boom")
					},
					Close: func(flux.PluginContext) error { closed.Add(1); return nil },
				}
			},
			want: flux.ErrPluginPanic, wantClose: 1,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var closed atomic.Int32
			name := registerTestPlugin(t, test.descriptor(&closed))
			mock := render.NewMock()
			app := newPluginApp(t, mock)
			err := app.Render(pluginTree(name, "A"))
			if errors.Is(test.want, flux.ErrPluginPanic) {
				if !errors.Is(err, flux.ErrPluginPanic) {
					t.Fatalf("Render 错误=%v，期望 ErrPluginPanic", err)
				}
			} else if err == nil || !strings.Contains(err.Error(), test.want.Error()) {
				t.Fatalf("Render 错误=%v，期望包含 %q", err, test.want)
			}
			if len(mock.Ops()) != 0 {
				t.Fatalf("准备失败不得产生 mutation：%+v", mock.Ops())
			}
			if got := closed.Load(); got != test.wantClose {
				t.Fatalf("回滚 Close 次数=%d，期望 %d", got, test.wantClose)
			}
			if err := flux.UnregisterWidget(name); err != nil {
				t.Fatalf("回滚后应可注销: %v", err)
			}
		})
	}
}

// TestPluginPrepareFailureWithQueuedStateDoesNotRetry 验证 prepare 阶段触发 State.Set
// 后返回错误时只失败一次，不在同一栈里无限递归重试。
func TestPluginPrepareFailureWithQueuedStateDoesNotRetry(t *testing.T) {
	state := flux.NewState(0)
	prepareErr := errors.New("prepare failed after state update")
	var builds atomic.Int32
	name := registerTestPlugin(t, flux.WidgetPlugin{
		Build: func(flux.PluginBuildContext) (flux.Widget, error) {
			builds.Add(1)
			state.Set(state.Get() + 1)
			return nil, prepareErr
		},
	})
	app := newPluginApp(t, render.NewMock())

	err := app.Mount(func() flux.Widget {
		return flux.Window(flux.Column(
			flux.Text(flux.Bind(state)),
			flux.PluginWidget(name, flux.NewPluginProperties()),
		))
	})
	if !errors.Is(err, prepareErr) {
		t.Fatalf("Mount 错误=%v，期望 %v", err, prepareErr)
	}
	if got := builds.Load(); got != 1 {
		t.Fatalf("prepare 失败后 Build 次数=%d，期望 1（不得递归重试）", got)
	}
}

// TestPluginBuildWidgetCreateFailuresRollback verifies that the Widget returned
// by Build remains inside the build error boundary and cannot reach commit with
// an invalid Node.
func TestPluginBuildWidgetCreateFailuresRollback(t *testing.T) {
	tests := []struct {
		name   string
		create func() *flux.Node
		want   error
	}{
		{
			name: "create-panic",
			create: func() *flux.Node {
				panic("create boom")
			},
			want: flux.ErrPluginPanic,
		},
		{
			name:   "create-nil",
			create: func() *flux.Node { return nil },
			want:   flux.ErrPluginInvalid,
		},
		{
			name: "create-unknown-node-type",
			create: func() *flux.Node {
				node := flux.Text("x").Create()
				node.Type = "UnknownPluginNode"
				return node
			},
			want: flux.ErrPluginInvalid,
		},
		{
			name: "create-invalid-page-tree",
			create: func() *flux.Node {
				return flux.Window(
					flux.TabPage("orphan", flux.Text("x"), flux.Key("orphan")),
				).Create()
			},
			want: flux.ErrPluginInvalid,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var closed atomic.Int32
			name := registerTestPlugin(t, flux.WidgetPlugin{
				Build: func(flux.PluginBuildContext) (flux.Widget, error) {
					return pluginCreateWidget{create: test.create}, nil
				},
				Close: func(flux.PluginContext) error {
					closed.Add(1)
					return nil
				},
			})
			mock := render.NewMock()
			app := newPluginApp(t, mock)

			var recovered any
			var err error
			func() {
				defer func() { recovered = recover() }()
				err = app.Render(pluginTree(name, "A"))
			}()
			if recovered != nil {
				t.Fatalf("Render 逸出 panic=%v，Create 必须处于插件 build 错误边界内", recovered)
			}
			if !errors.Is(err, test.want) {
				t.Errorf("Render 错误=%v，期望 %v", err, test.want)
			}
			var pluginErr *flux.PluginError
			if !errors.As(err, &pluginErr) {
				t.Errorf("Render 错误类型=%T，期望 *PluginError", err)
			} else if pluginErr.Name != name || pluginErr.Stage != "build" {
				t.Errorf("PluginError=%+v，期望 Name=%q Stage=build", pluginErr, name)
			}
			if ops := mock.Ops(); len(ops) != 0 {
				t.Errorf("Create 失败不得产生 mutation：%+v", ops)
			}
			if got := closed.Load(); got != 1 {
				t.Errorf("回滚 Close 次数=%d，期望 1", got)
			}
			if err := flux.UnregisterWidget(name); err != nil {
				t.Errorf("回滚后应可注销: %v", err)
			}
		})
	}
}

func TestNestedPluginCanConsumeTabPageBeforePageValidation(t *testing.T) {
	lower := registerTestPlugin(t, flux.WidgetPlugin{
		Build: func(ctx flux.PluginBuildContext) (flux.Widget, error) {
			if len(ctx.Children) != 1 {
				return nil, fmt.Errorf("children=%d", len(ctx.Children))
			}
			return flux.PageControl(ctx.Children[0]), nil
		},
	})
	outer := registerTestPlugin(t, flux.WidgetPlugin{
		Build: func(ctx flux.PluginBuildContext) (flux.Widget, error) {
			if len(ctx.Children) != 1 {
				return nil, fmt.Errorf("children=%d", len(ctx.Children))
			}
			return flux.PluginWidget(lower, flux.NewPluginProperties(), ctx.Children[0]), nil
		},
	})
	mock := render.NewMock()
	app := newPluginApp(t, mock)
	root := flux.Window(flux.PluginWidget(
		outer,
		flux.NewPluginProperties(),
		flux.TabPage("nested", flux.Text("content"), flux.Key("nested")),
	))
	if err := app.Render(root); err != nil {
		t.Fatalf("嵌套插件消费 TabPage 后应形成合法 PageControl: %v", err)
	}
	if findByType(t, app.Root(), "PageControl") == nil || findByKey(t, app.Root(), "nested") == nil {
		t.Fatal("嵌套插件展开后缺少 PageControl/TabPage")
	}
}

// TestPluginUnmountErrorsAreAggregated verifies that removing multiple plugin
// instances preserves every OnUnmount failure in Render and LastError.
func TestPluginUnmountErrorsAreAggregated(t *testing.T) {
	firstErr := errors.New("first unmount failed")
	secondErr := errors.New("second unmount failed")
	name := registerTestPlugin(t, flux.WidgetPlugin{
		Build: func(ctx flux.PluginBuildContext) (flux.Widget, error) {
			return flux.Text(ctx.Key), nil
		},
		OnUnmount: func(ctx flux.PluginInstanceContext) error {
			switch ctx.Key {
			case "first":
				return firstErr
			case "second":
				return secondErr
			default:
				return nil
			}
		},
	})
	app := newPluginApp(t, render.NewMock())
	properties := flux.NewPluginProperties()
	if err := app.Render(flux.Window(flux.Column(
		flux.PluginWidget(name, properties, flux.Key("first")),
		flux.PluginWidget(name, properties, flux.Key("second")),
	))); err != nil {
		t.Fatalf("首次 Render: %v", err)
	}

	err := app.Render(flux.Window(flux.Column()))
	if !errors.Is(err, firstErr) || !errors.Is(err, secondErr) {
		t.Errorf("移除 Render 错误=%v，期望聚合两个 OnUnmount 错误", err)
	}
	lastErr := app.LastError()
	if !errors.Is(lastErr, firstErr) || !errors.Is(lastErr, secondErr) {
		t.Errorf("LastError=%v，期望聚合两个 OnUnmount 错误", lastErr)
	}
}

// TestPluginMountErrorSurvivesQueuedRender verifies that a lifecycle error is
// not cleared by the tail render queued by State.Set inside OnMount.
func TestPluginMountErrorSurvivesQueuedRender(t *testing.T) {
	state := flux.NewState("before")
	mountErr := errors.New("mount failed after state update")
	var mounted atomic.Bool
	name := registerTestPlugin(t, flux.WidgetPlugin{
		Build: func(flux.PluginBuildContext) (flux.Widget, error) {
			return flux.Text(flux.Bind(state), flux.Key("value")), nil
		},
		OnMount: func(flux.PluginInstanceContext) error {
			if mounted.CompareAndSwap(false, true) {
				state.Set("after")
				return mountErr
			}
			return nil
		},
	})
	app := newPluginApp(t, render.NewMock())

	err := app.Mount(func() flux.Widget {
		return flux.Window(flux.Column(
			flux.PluginWidget(name, flux.NewPluginProperties(), flux.Key("plugin")),
		))
	})
	if !errors.Is(err, mountErr) {
		t.Errorf("Mount 错误=%v，期望 %v", err, mountErr)
	}
	if got := findByKey(t, app.Root(), "value").Props.String("Text"); got != "after" {
		t.Errorf("尾部重入 render 后 Text=%q，期望 after", got)
	}
	if err := app.LastError(); !errors.Is(err, mountErr) {
		t.Errorf("尾部重入 render 后 LastError=%v，期望仍包含 mount 错误", err)
	}
}

// TestPluginLifecycleCloseIsRejected verifies that lifecycle callbacks cannot
// synchronously wait for the render that is currently invoking them.
func TestPluginLifecycleCloseIsRejected(t *testing.T) {
	var app *flux.App
	var closeErr error
	name := registerTestPlugin(t, flux.WidgetPlugin{
		Build: func(flux.PluginBuildContext) (flux.Widget, error) {
			return flux.Text("x"), nil
		},
		OnMount: func(flux.PluginInstanceContext) error {
			closeErr = app.Close()
			return nil
		},
	})
	app = flux.NewApp(render.NewMock())

	done := make(chan error, 1)
	go func() {
		done <- app.Render(pluginTree(name, "A"))
	}()
	select {
	case err := <-done:
		if !errors.Is(err, flux.ErrAppCloseDuringRender) {
			t.Errorf("Render 错误=%v，期望 ErrAppCloseDuringRender", err)
		}
	case <-time.After(time.Second):
		t.Fatal("生命周期内 App.Close 自锁")
	}
	if !errors.Is(closeErr, flux.ErrAppCloseDuringRender) {
		t.Errorf("OnMount Close 错误=%v，期望 ErrAppCloseDuringRender", closeErr)
	}
	if err := app.Close(); err != nil {
		t.Fatalf("render 返回后 App.Close: %v", err)
	}
}

// TestPluginKeyedReorderPreservesIdentity covers D7b: stable plugin keys keep
// both plugin Elements and their native child handles during list reordering.
func TestPluginKeyedReorderPreservesIdentity(t *testing.T) {
	var mounts, unmounts atomic.Int32
	name := registerTestPlugin(t, flux.WidgetPlugin{
		Build: func(ctx flux.PluginBuildContext) (flux.Widget, error) {
			return flux.Text(ctx.Key, flux.Key("text-"+ctx.Key)), nil
		},
		OnMount: func(flux.PluginInstanceContext) error {
			mounts.Add(1)
			return nil
		},
		OnUnmount: func(flux.PluginInstanceContext) error {
			unmounts.Add(1)
			return nil
		},
	})
	mock := render.NewMock()
	app := newPluginApp(t, mock)
	properties := flux.NewPluginProperties()
	tree := func(first, second string) flux.Widget {
		return flux.Window(flux.Column(
			flux.PluginWidget(name, properties, flux.Key(first)),
			flux.PluginWidget(name, properties, flux.Key(second)),
		))
	}

	if err := app.Render(tree("a", "b")); err != nil {
		t.Fatalf("首次 Render: %v", err)
	}
	pluginA := findByKey(t, app.Root(), "a")
	pluginB := findByKey(t, app.Root(), "b")
	textAHandle := findByKey(t, app.Root(), "text-a").Handle
	textBHandle := findByKey(t, app.Root(), "text-b").Handle
	creates := mock.Count(render.OpCreate)
	destroys := mock.Count(render.OpDestroy)

	if err := app.Render(tree("b", "a")); err != nil {
		t.Fatalf("重排 Render: %v", err)
	}
	if got := findByKey(t, app.Root(), "a"); got != pluginA {
		t.Errorf("key=a 的插件 Element 未保持 identity：%p -> %p", pluginA, got)
	}
	if got := findByKey(t, app.Root(), "b"); got != pluginB {
		t.Errorf("key=b 的插件 Element 未保持 identity：%p -> %p", pluginB, got)
	}
	if got := findByKey(t, app.Root(), "text-a").Handle; got != textAHandle {
		t.Errorf("key=a 子 Text 句柄=%d，期望保持 %d", got, textAHandle)
	}
	if got := findByKey(t, app.Root(), "text-b").Handle; got != textBHandle {
		t.Errorf("key=b 子 Text 句柄=%d，期望保持 %d", got, textBHandle)
	}
	if got := mock.Count(render.OpCreate); got != creates {
		t.Errorf("D7b 重排 Create 次数=%d，期望保持 %d", got, creates)
	}
	if got := mock.Count(render.OpDestroy); got != destroys {
		t.Errorf("D7b 重排 Destroy 次数=%d，期望保持 %d", got, destroys)
	}
	if got := mounts.Load(); got != 2 {
		t.Errorf("D7b 重排后 OnMount 次数=%d，期望 2", got)
	}
	if got := unmounts.Load(); got != 0 {
		t.Errorf("D7b 重排后 OnUnmount 次数=%d，期望 0", got)
	}
}

// TestPluginCloseReverseOrderAndErrors 验证实例先卸载、插件按 Init 逆序关闭，且关闭
// 错误/ panic 被聚合返回后仍释放注册占用。
func TestPluginCloseReverseOrderAndErrors(t *testing.T) {
	var events []string
	makePlugin := func(label string, closeFn func() error) flux.WidgetPlugin {
		return flux.WidgetPlugin{
			Init: func(flux.PluginContext) error { events = append(events, "init-"+label); return nil },
			Build: func(flux.PluginBuildContext) (flux.Widget, error) {
				return flux.Text(label), nil
			},
			OnUnmount: func(flux.PluginInstanceContext) error {
				events = append(events, "unmount-"+label)
				return nil
			},
			Close: func(flux.PluginContext) error {
				events = append(events, "close-"+label)
				return closeFn()
			},
		}
	}
	first := registerTestPlugin(t, makePlugin("a", func() error { return errors.New("close a") }))
	second := registerTestPlugin(t, makePlugin("b", func() error { panic("close b") }))
	app := flux.NewApp(render.NewMock())
	if err := app.Render(flux.Window(flux.Column(
		flux.PluginWidget(first, flux.NewPluginProperties()),
		flux.PluginWidget(second, flux.NewPluginProperties()),
	))); err != nil {
		t.Fatalf("Render: %v", err)
	}
	err := app.Close()
	if err == nil || !strings.Contains(err.Error(), "close a") || !errors.Is(err, flux.ErrPluginPanic) {
		t.Fatalf("Close 聚合错误=%v，期望 close a + ErrPluginPanic", err)
	}
	want := "init-a,init-b,unmount-a,unmount-b,close-b,close-a"
	if got := strings.Join(events, ","); got != want {
		t.Fatalf("关闭顺序=%q，期望 %q", got, want)
	}
	if err := flux.UnregisterWidget(first); err != nil {
		t.Fatalf("错误关闭后 first 仍应可注销: %v", err)
	}
	if err := flux.UnregisterWidget(second); err != nil {
		t.Fatalf("错误关闭后 second 仍应可注销: %v", err)
	}
}

type capabilitylessRenderer struct{ render.Renderer }

type changingCapabilityRenderer struct {
	*render.Mock
	dpi   atomic.Int64
	calls atomic.Int64
}

type panickingCapabilityRenderer struct{ *render.Mock }

func (r panickingCapabilityRenderer) PluginCapabilitySnapshot() map[string]any {
	panic("capability provider failed")
}

type mutableCapabilityRenderer struct{ *render.Mock }

func (r mutableCapabilityRenderer) PluginCapabilitySnapshot() map[string]any {
	return map[string]any{
		"example.mutable.slice": []int{1, 2, 3},
		"example.scalar.answer": 42,
	}
}

func newChangingCapabilityRenderer(dpi int64) *changingCapabilityRenderer {
	renderer := &changingCapabilityRenderer{Mock: render.NewMock()}
	renderer.dpi.Store(dpi)
	return renderer
}

func (r *changingCapabilityRenderer) PluginCapabilitySnapshot() map[string]any {
	r.calls.Add(1)
	return map[string]any{
		"flux.renderer.dpi":     int(r.dpi.Load()),
		"flux.renderer.backend": "changing",
	}
}

// TestPluginOptionalCapabilityMissing 验证不实现 capability provider 的 Renderer 安全退化。
func TestPluginOptionalCapabilityMissing(t *testing.T) {
	var backendOK, customOK bool
	custom := flux.NewCapability[int]("test.renderer.answer")
	name := registerTestPlugin(t, flux.WidgetPlugin{
		Build: func(ctx flux.PluginBuildContext) (flux.Widget, error) {
			_, backendOK = flux.LookupCapability(ctx.PluginContext, flux.RendererBackend)
			_, customOK = flux.LookupCapability(ctx.PluginContext, custom)
			return flux.Text("fallback"), nil
		},
	})
	mock := render.NewMock()
	app := newPluginApp(t, capabilitylessRenderer{Renderer: mock})
	if err := app.Render(pluginTree(name, "A")); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if backendOK || customOK {
		t.Fatalf("缺失能力不应命中：backend=%v custom=%v", backendOK, customOK)
	}
}

func TestPluginCapabilityProviderPanicFallsBackToMissing(t *testing.T) {
	var backendOK bool
	name := registerTestPlugin(t, flux.WidgetPlugin{
		Build: func(ctx flux.PluginBuildContext) (flux.Widget, error) {
			_, backendOK = flux.LookupCapability(ctx.PluginContext, flux.RendererBackend)
			return flux.Text("fallback"), nil
		},
	})
	app := newPluginApp(t, panickingCapabilityRenderer{Mock: render.NewMock()})
	if err := app.Render(pluginTree(name, "A")); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if backendOK {
		t.Fatal("provider panic 后 capability 应按缺失安全退化")
	}
}

func TestPluginCapabilityRejectsMutableValues(t *testing.T) {
	mutable := flux.NewCapability[[]int]("example.mutable.slice")
	scalar := flux.NewCapability[int]("example.scalar.answer")
	var mutableOK, scalarOK bool
	name := registerTestPlugin(t, flux.WidgetPlugin{
		Build: func(ctx flux.PluginBuildContext) (flux.Widget, error) {
			_, mutableOK = flux.LookupCapability(ctx.PluginContext, mutable)
			value, ok := flux.LookupCapability(ctx.PluginContext, scalar)
			scalarOK = ok && value == 42
			return flux.Text("fallback"), nil
		},
	})
	app := newPluginApp(t, mutableCapabilityRenderer{Mock: render.NewMock()})
	if err := app.Render(pluginTree(name, "A")); err != nil {
		t.Fatalf("Render: %v", err)
	}
	if mutableOK || !scalarOK {
		t.Fatalf("capability 过滤结果 mutable/scalar=%v/%v，期望 false/true", mutableOK, scalarOK)
	}
}

// TestPluginCapabilitySnapshotIsStableAndRefreshes 验证 capability 是回调边界的
// 不可变快照：保存的旧上下文可并发读取且不再调用 Renderer；下一次 Build 读取新值。
func TestPluginCapabilitySnapshotIsStableAndRefreshes(t *testing.T) {
	renderer := newChangingCapabilityRenderer(96)
	var contexts []flux.PluginContext
	name := registerTestPlugin(t, flux.WidgetPlugin{
		Build: func(ctx flux.PluginBuildContext) (flux.Widget, error) {
			contexts = append(contexts, ctx.PluginContext)
			return flux.Text("x"), nil
		},
	})
	app := newPluginApp(t, renderer)

	if err := app.Render(pluginTree(name, "A")); err != nil {
		t.Fatalf("首次 Render: %v", err)
	}
	if got, ok := flux.LookupCapability(contexts[0], flux.RendererDPI); !ok || got != 96 {
		t.Fatalf("首次 DPI=%d/%v，期望 96/true", got, ok)
	}
	callsAfterBuild := renderer.calls.Load()

	const readers = 32
	var wg sync.WaitGroup
	for i := 0; i < readers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				if got, ok := flux.LookupCapability(contexts[0], flux.RendererDPI); !ok || got != 96 {
					t.Errorf("并发读取旧快照 DPI=%d/%v，期望 96/true", got, ok)
					return
				}
			}
		}()
	}
	wg.Wait()
	if got := renderer.calls.Load(); got != callsAfterBuild {
		t.Fatalf("LookupCapability 再次调用 provider：calls=%d，期望 %d", got, callsAfterBuild)
	}

	renderer.dpi.Store(144)
	if err := app.Render(pluginTree(name, "B")); err != nil {
		t.Fatalf("第二次 Render: %v", err)
	}
	if len(contexts) != 2 {
		t.Fatalf("Build 上下文数=%d，期望 2", len(contexts))
	}
	if got, ok := flux.LookupCapability(contexts[1], flux.RendererDPI); !ok || got != 144 {
		t.Fatalf("新快照 DPI=%d/%v，期望 144/true", got, ok)
	}
	if got, _ := flux.LookupCapability(contexts[0], flux.RendererDPI); got != 96 {
		t.Fatalf("旧快照 DPI=%d，期望仍为 96", got)
	}
	if err := app.Close(); err != nil {
		t.Fatalf("App.Close: %v", err)
	}
	callsAfterClose := renderer.calls.Load()
	if got, ok := flux.LookupCapability(contexts[1], flux.RendererDPI); !ok || got != 144 {
		t.Fatalf("Close 后历史快照 DPI=%d/%v，期望 144/true", got, ok)
	}
	if got := renderer.calls.Load(); got != callsAfterClose {
		t.Fatalf("Close 后 LookupCapability 调用 provider：calls=%d，期望 %d", got, callsAfterClose)
	}
}

// TestPluginInstancePanicReported 验证提交期实例生命周期 panic 不崩溃，并由 Render/LastError 报告。
func TestPluginInstancePanicReported(t *testing.T) {
	name := registerTestPlugin(t, flux.WidgetPlugin{
		Build: func(flux.PluginBuildContext) (flux.Widget, error) { return flux.Text("x"), nil },
		OnMount: func(flux.PluginInstanceContext) error {
			panic("mount boom")
		},
	})
	mock := render.NewMock()
	app := newPluginApp(t, mock)
	err := app.Render(pluginTree(name, "A"))
	if !errors.Is(err, flux.ErrPluginPanic) || !errors.Is(app.LastError(), flux.ErrPluginPanic) {
		t.Fatalf("mount panic Render/LastError=%v/%v，期望 ErrPluginPanic", err, app.LastError())
	}
	if mock.Count(render.OpCreate) == 0 {
		t.Fatal("实例 mount 属提交边界：子树应已提交，错误必须可观测")
	}
}
