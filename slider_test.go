package flux_test

import (
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

func TestSliderCanonicalRangeAndStep(t *testing.T) {
	n := flux.Slider(
		flux.Minimum(20), flux.Maximum(10), flux.Value(99), flux.Step(5),
	).Create()
	if got := n.Props.Int("Minimum"); got != 20 {
		t.Fatalf("Minimum=%d，期望 20", got)
	}
	if got := n.Props.Int("Maximum"); got != 20 {
		t.Fatalf("Maximum=%d，期望规范为 20", got)
	}
	if got := n.Props.Int("Value"); got != 20 {
		t.Fatalf("Value=%d，期望钳制为 20", got)
	}
	if got := n.Props.Int("Step"); got != 5 {
		t.Fatalf("Step=%d，期望 5", got)
	}

	defaults := flux.Slider().Create()
	for key, want := range map[string]int{"Minimum": 0, "Maximum": 100, "Value": 0, "Step": 1} {
		if got := defaults.Props.Int(key); got != want {
			t.Fatalf("默认 %s=%d，期望 %d", key, got, want)
		}
	}
}

func TestSliderRejectsNonPositiveStep(t *testing.T) {
	for _, step := range []int{0, -1} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("Step(%d) 未 panic", step)
				}
			}()
			_ = flux.Step(step)
		}()
	}
}

func TestSliderMountPatchD7cAndIntrinsic(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	build := func(value, step int) flux.Widget {
		return flux.Window(flux.Slider(
			flux.Key("subject"), flux.Minimum(10), flux.Maximum(80),
			flux.Value(value), flux.Step(step),
		))
	}
	if err := app.Render(build(20, 2)); err != nil {
		t.Fatal(err)
	}
	target := findByKey(t, app.Root(), "subject")
	h := target.Handle
	minimum, maximum, value := mock.Progress(h)
	if minimum != 10 || maximum != 80 || value != 20 || mock.SliderStep(h) != 2 {
		t.Fatalf("mount 状态=(%d,%d,%d step=%d)", minimum, maximum, value, mock.SliderStep(h))
	}
	if bounds, ok := target.Props.Get("Bounds"); !ok || bounds != (render.Rect{W: 180, H: 32}) {
		t.Fatalf("Slider intrinsic Bounds=%v", bounds)
	}

	base := len(mock.Ops())
	if err := app.Render(build(20, 2)); err != nil {
		t.Fatal(err)
	}
	if ops := mock.Ops()[base:]; len(ops) != 0 {
		t.Fatalf("D7c 相同树产生 mutation: %+v", ops)
	}

	base = len(mock.Ops())
	if err := app.Render(build(70, 5)); err != nil {
		t.Fatal(err)
	}
	ops := mock.Ops()[base:]
	if countOps(ops, render.OpCreate) != 0 || countOps(ops, render.OpDestroy) != 0 {
		t.Fatalf("D7a Slider patch 重建控件: %+v", ops)
	}
	if got := findByKey(t, app.Root(), "subject").Handle; got != h {
		t.Fatalf("Slider handle=%d，期望保持 %d", got, h)
	}
	_, _, value = mock.Progress(h)
	if value != 70 || mock.SliderStep(h) != 5 {
		t.Fatalf("patch value=%d step=%d", value, mock.SliderStep(h))
	}
}

func TestSliderValueChangeWritesControlledStateAndUnbinds(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	value := flux.NewState(10)
	bound := true
	build := func() flux.Widget {
		opts := []flux.Opt{
			flux.Key("slider"), flux.Value(value.Get()), flux.Step(2),
		}
		if bound {
			opts = append(opts, flux.OnValueChange(value.Set))
		}
		return flux.Window(flux.Column(
			flux.Text(flux.Bind(value)),
			flux.Slider(opts...),
		))
	}
	if err := app.Mount(build); err != nil {
		t.Fatal(err)
	}
	h := findByKey(t, app.Root(), "slider").Handle
	mock.FireValueChange(h, 42)
	if got := value.Get(); got != 42 {
		t.Fatalf("State=%d，期望 42", got)
	}
	if got := findByKey(t, app.Root(), "slider").Props.Int("Value"); got != 42 {
		t.Fatalf("受控 Value=%d，期望 42", got)
	}

	bound = false
	if err := app.Render(build()); err != nil {
		t.Fatal(err)
	}
	mock.FireValueChange(h, 60)
	if got := value.Get(); got != 42 {
		t.Fatalf("事件解绑后 State=%d，期望仍为 42", got)
	}
}

type rendererWithoutSlider struct{ render.Renderer }

func TestSliderCapabilityMissingSafelyDegrades(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(rendererWithoutSlider{Renderer: mock})
	if err := app.Render(flux.Window(flux.Slider(
		flux.Minimum(1), flux.Maximum(9), flux.Value(4), flux.Step(2),
		flux.OnValueChange(func(int) {}),
	))); err != nil {
		t.Fatal(err)
	}
}
