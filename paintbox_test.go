package flux_test

import (
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

func paintCommands(radius int) []flux.PaintCommand {
	return []flux.PaintCommand{
		{Kind: flux.PaintClear, Color: flux.RGB(250, 250, 250)},
		{
			Kind: flux.PaintCircle, X: 80, Y: 70, Radius: radius,
			FillColor: flux.RGB(40, 120, 220), StrokeColor: flux.RGB(20, 30, 40), StrokeWidth: 2,
		},
	}
}

func TestPaintBoxDefensiveCopyAndValidation(t *testing.T) {
	commands := paintCommands(20)
	node := flux.PaintBox(commands).Create()
	commands[1].Radius = 99
	value, ok := node.Props.Get("PaintCommands")
	stored, valid := value.([]render.PaintCommand)
	if !ok || !valid || len(stored) != 2 || stored[1].Radius != 20 {
		t.Fatalf("PaintCommands 未防御性复制: %#v", value)
	}
}

func TestPaintBoxMountPatchD7cAndInvalidate(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	build := func(radius int) flux.Widget {
		return flux.Window(flux.PaintBox(paintCommands(radius), flux.Key("paint")))
	}
	if err := app.Render(build(20)); err != nil {
		t.Fatal(err)
	}
	target := findByKey(t, app.Root(), "paint")
	h := target.Handle
	if got := mock.PaintCommands(h); len(got) != 2 || got[1].Radius != 20 {
		t.Fatalf("mount commands=%+v", got)
	}
	if got := mock.PaintInvalidations(h); got != 1 {
		t.Fatalf("mount invalidations=%d，期望 1", got)
	}
	if bounds, ok := target.Props.Get("Bounds"); !ok || bounds != (render.Rect{W: 360, H: 260}) {
		t.Fatalf("PaintBox intrinsic Bounds=%v", bounds)
	}

	base := len(mock.Ops())
	if err := app.Render(build(20)); err != nil {
		t.Fatal(err)
	}
	if ops := mock.Ops()[base:]; len(ops) != 0 {
		t.Fatalf("D7c 相同命令产生 mutation: %+v", ops)
	}

	base = len(mock.Ops())
	if err := app.Render(build(36)); err != nil {
		t.Fatal(err)
	}
	ops := mock.Ops()[base:]
	if countOps(ops, render.OpCreate) != 0 || countOps(ops, render.OpDestroy) != 0 {
		t.Fatalf("PaintBox invalidate 重建控件: %+v", ops)
	}
	if got := findByKey(t, app.Root(), "paint").Handle; got != h {
		t.Fatalf("PaintBox handle=%d，期望保持 %d", got, h)
	}
	if got := mock.PaintCommands(h); got[1].Radius != 36 {
		t.Fatalf("patch commands=%+v", got)
	}
	if got := mock.PaintInvalidations(h); got != 2 {
		t.Fatalf("patch invalidations=%d，期望 2", got)
	}
}

func TestPaintBoxRejectsInvalidCommand(t *testing.T) {
	tests := []struct {
		name    string
		command flux.PaintCommand
	}{
		{name: "unknown kind", command: flux.PaintCommand{Kind: flux.PaintCommandKind(99)}},
		{name: "clear without color", command: flux.PaintCommand{Kind: flux.PaintClear}},
		{name: "circle without radius", command: flux.PaintCommand{Kind: flux.PaintCircle, FillColor: flux.RGB(1, 2, 3)}},
		{name: "circle without paint", command: flux.PaintCommand{Kind: flux.PaintCircle, Radius: 10}},
		{name: "stroke without width", command: flux.PaintCommand{Kind: flux.PaintCircle, Radius: 10, StrokeColor: flux.RGB(1, 2, 3)}},
		{name: "width without stroke", command: flux.PaintCommand{Kind: flux.PaintCircle, Radius: 10, FillColor: flux.RGB(1, 2, 3), StrokeWidth: 1}},
		{name: "negative width", command: flux.PaintCommand{Kind: flux.PaintCircle, Radius: 10, FillColor: flux.RGB(1, 2, 3), StrokeWidth: -1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("非法 PaintCommand 未 panic")
				}
			}()
			_ = flux.PaintBox([]flux.PaintCommand{test.command})
		})
	}
}

type rendererWithoutPaint struct{ render.Renderer }

func TestPaintBoxCapabilityMissingSafelyDegrades(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(rendererWithoutPaint{Renderer: mock})
	if err := app.Render(flux.Window(flux.PaintBox(paintCommands(20)))); err != nil {
		t.Fatal(err)
	}
}

func TestPaintBoxGenericMouseEventSupportsHitTesting(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	commands := paintCommands(20)
	hit := false
	var received flux.Event
	if err := app.Render(flux.Window(flux.PaintBox(commands,
		flux.Key("paint"),
		flux.OnMouseDown(func(event flux.Event) {
			received = event
			circle := commands[1]
			dx, dy := event.X-circle.X, event.Y-circle.Y
			hit = dx*dx+dy*dy <= circle.Radius*circle.Radius
		}),
	))); err != nil {
		t.Fatal(err)
	}
	h := findByKey(t, app.Root(), "paint").Handle
	handler, ok := mock.EventHandler(h, "OnMouseDown").(func(render.Event))
	if !ok {
		t.Fatal("PaintBox did not bind the generic mouse handler")
	}
	handler(render.Event{Type: render.EventMouseDown, X: 84, Y: 75, Button: render.ButtonLeft})
	if !hit || received.Source != "PaintBox#paint" || received.X != 84 || received.Y != 75 {
		t.Fatalf("mouse event/hit mismatch: hit=%v event=%+v", hit, received)
	}
}

func TestPaintBoxExplicitSizeAndConstraints(t *testing.T) {
	commands := paintCommands(20)
	for _, test := range []struct {
		name     string
		clientW  int
		clientH  int
		opts     []flux.Opt
		wantRect render.Rect
	}{
		{name: "explicit", clientW: 400, clientH: 300, opts: []flux.Opt{flux.Width(180), flux.Height(140)}, wantRect: render.Rect{W: 180, H: 140}},
		{name: "constrained", clientW: 120, clientH: 90, wantRect: render.Rect{W: 120, H: 90}},
	} {
		t.Run(test.name, func(t *testing.T) {
			mock := render.NewMock()
			mock.SetClientSize(test.clientW, test.clientH)
			app := flux.NewApp(mock)
			opts := append([]flux.Opt{flux.Key("paint")}, test.opts...)
			if err := app.Render(flux.Window(flux.PaintBox(commands, opts...))); err != nil {
				t.Fatal(err)
			}
			value, _ := findByKey(t, app.Root(), "paint").Props.Get("Bounds")
			if got := value.(render.Rect); got != test.wantRect {
				t.Fatalf("Bounds=%+v, want %+v", got, test.wantRect)
			}
		})
	}
}
