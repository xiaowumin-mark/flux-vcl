package flux_test

import (
	"errors"
	"strings"
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/diff"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
	"github.com/xiaowumin-mark/flux-vcl/internal/widget"
)

func drawFill(color flux.ColorValue) flux.DrawOp {
	return flux.FillRect(flux.Rect{X: 1, Y: 2, W: 30, H: 20}, flux.FillStyle{Color: color})
}

func requireDrawValidation(t *testing.T, err error, id flux.MessageID, index int, field string) {
	t.Helper()
	if !errors.Is(err, flux.ErrInvalidDrawList) {
		t.Fatalf("errors.Is(%v, ErrInvalidDrawList) = false", err)
	}
	var validation *flux.DrawValidationError
	if !errors.As(err, &validation) {
		t.Fatalf("errors.As(%T) did not return DrawValidationError", err)
	}
	if validation.ID != id || validation.OpIndex != index || validation.Field != field {
		t.Fatalf("validation = %+v, want ID=%q index=%d field=%q", validation, id, index, field)
	}
}

func TestDrawListValidationAndValueSemantics(t *testing.T) {
	color := flux.RGB(0x12, 0x34, 0x56)
	list, err := flux.NewDrawList(
		flux.Clear(color),
		flux.PushClip(flux.Rect{X: 0, Y: 0, W: 20, H: 20}),
		drawFill(color),
		flux.PopClip(),
		flux.DrawText("text", flux.Rect{W: 10, H: 10}, flux.TextPaint{Color: color}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if list.Len() != 5 {
		t.Fatalf("Len() = %d, want 5", list.Len())
	}
	copy := list.Ops()
	copy[0] = flux.Clear(flux.RGB(1, 2, 3))
	if list.Equal(flux.MustDrawList(flux.Clear(flux.RGB(1, 2, 3)))) {
		t.Fatal("mutating Ops changed list")
	}
	if !list.Equal(list.Clone()) {
		t.Fatal("cloned list is not equal")
	}
	input := []flux.DrawOp{flux.Clear(color)}
	snapshot := flux.MustDrawList(input...)
	input[0] = flux.Clear(flux.RGB(9, 8, 7))
	if !snapshot.Equal(flux.MustDrawList(flux.Clear(color))) {
		t.Fatal("mutating constructor input changed DrawList snapshot")
	}
	if (flux.DrawList{}).Len() != 0 || !(flux.DrawList{}).Equal(flux.DrawList{}) {
		t.Fatal("zero DrawList must be a valid empty value")
	}
}

func TestDrawListAllPrimitiveOpsAndFontCanonicalization(t *testing.T) {
	color := flux.RGB(10, 20, 30)
	fill := flux.FillStyle{Color: color}
	stroke := flux.StrokeStyle{Color: color, Width: 2, Style: flux.StrokeSolid}
	rect := flux.Rect{X: -10, Y: -10, W: 20, H: 20}
	list, err := flux.NewDrawList(
		flux.Clear(color),
		flux.FillRect(rect, fill), flux.StrokeRect(rect, stroke),
		flux.FillRoundRect(rect, 3, fill), flux.StrokeRoundRect(rect, 3, stroke),
		flux.DrawLine(flux.Point{X: -2, Y: 0}, flux.Point{X: 2, Y: 0}, stroke),
		flux.FillEllipse(rect, fill), flux.StrokeEllipse(rect, stroke),
		flux.DrawText("hello", rect, flux.TextPaint{Color: color}),
		flux.PushClip(rect), flux.PopClip(),
	)
	if err != nil || list.Len() != 11 {
		t.Fatalf("all primitive ops: list=%v err=%v", list, err)
	}
	zeroWeight := flux.MustDrawList(flux.DrawText("hello", rect, flux.TextPaint{Color: color}))
	normalWeight := flux.MustDrawList(flux.DrawText("hello", rect, flux.TextPaint{
		Color: color, Font: flux.FontSpec{Weight: flux.FontWeightNormal},
	}))
	if !zeroWeight.Equal(normalWeight) {
		t.Fatal("zero font weight was not canonicalized to FontWeightNormal")
	}
}

func TestDrawListRejectsStructuredBoundaryErrors(t *testing.T) {
	color := flux.RGB(1, 2, 3)
	tests := []struct {
		name  string
		ops   []flux.DrawOp
		id    flux.MessageID
		index int
		field string
	}{
		{"color required", []flux.DrawOp{flux.Clear(0)}, flux.DiagnosticDrawColorRequired, 0, "color"},
		{"partial alpha", []flux.DrawOp{flux.Clear(flux.ColorValue(0x80112233))}, flux.DiagnosticDrawAlphaUnsupported, 0, "color"},
		{"negative rect", []flux.DrawOp{flux.FillRect(flux.Rect{W: -1}, flux.FillStyle{Color: color})}, flux.DiagnosticDrawNegativeSize, 0, "rect"},
		{"radius", []flux.DrawOp{flux.FillRoundRect(flux.Rect{}, -1, flux.FillStyle{Color: color})}, flux.DiagnosticDrawRadiusNegative, 0, "radius"},
		{"stroke", []flux.DrawOp{flux.StrokeEllipse(flux.Rect{}, flux.StrokeStyle{Color: color})}, flux.DiagnosticDrawStrokeWidth, 0, "width"},
		{"clip underflow", []flux.DrawOp{flux.PopClip()}, flux.DiagnosticDrawClipUnderflow, 0, "clip"},
		{"clip unbalanced", []flux.DrawOp{flux.PushClip(flux.Rect{})}, flux.DiagnosticDrawClipUnbalanced, -1, ""},
		{"font", []flux.DrawOp{flux.DrawText("x", flux.Rect{}, flux.TextPaint{Color: color, Font: flux.FontSpec{Size: -1}})}, flux.DiagnosticDrawFontSize, 0, "size"},
		{"enum", []flux.DrawOp{flux.DrawText("x", flux.Rect{}, flux.TextPaint{Color: color, Wrap: flux.TextWrap(9)})}, flux.DiagnosticDrawEnumUnknown, 0, "wrap"},
		{"nil op", []flux.DrawOp{nil}, flux.DiagnosticDrawEnumUnknown, 0, "op"},
		{"text too long", []flux.DrawOp{flux.DrawText(strings.Repeat("x", 65537), flux.Rect{}, flux.TextPaint{Color: color})}, flux.DiagnosticDrawTextTooLong, 0, "text"},
		{"family too long", []flux.DrawOp{flux.DrawText("x", flux.Rect{}, flux.TextPaint{Color: color, Font: flux.FontSpec{Family: strings.Repeat("x", 256)}})}, flux.DiagnosticDrawFontSize, 0, "family"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := flux.NewDrawList(test.ops...)
			requireDrawValidation(t, err, test.id, test.index, test.field)
		})
	}

	ops := make([]flux.DrawOp, 4097)
	for i := range ops {
		ops[i] = flux.Clear(color)
	}
	_, err := flux.NewDrawList(ops...)
	requireDrawValidation(t, err, flux.DiagnosticDrawTooManyOps, -1, "")

	clips := make([]flux.DrawOp, 33)
	for i := range clips {
		clips[i] = flux.PushClip(flux.Rect{})
	}
	_, err = flux.NewDrawList(clips...)
	requireDrawValidation(t, err, flux.DiagnosticDrawClipUnbalanced, 32, "clip_depth")
}

func TestMustDrawListPanicsWithErrorValue(t *testing.T) {
	defer func() {
		value := recover()
		err, ok := value.(error)
		if !ok || !errors.Is(err, flux.ErrInvalidDrawList) {
			t.Fatalf("panic = %T %v, want invalid DrawList error", value, value)
		}
	}()
	_ = flux.MustDrawList(flux.Clear(0))
}

func TestDrawValidationErrorUsesDiagnosticCatalog(t *testing.T) {
	catalog, err := flux.NewCatalog("test", flux.Resources{"test": {
		flux.DiagnosticDrawColorRequired: "DRAW COLOR",
	}})
	if err != nil {
		t.Fatal(err)
	}
	restore := flux.SetDiagnosticCatalog(catalog, "test")
	defer restore()
	_, err = flux.NewDrawList(flux.Clear(0))
	if err == nil || !strings.Contains(err.Error(), "DRAW COLOR") {
		t.Fatalf("localized Draw error = %v", err)
	}
}

func TestDrawSurfaceDiffMountPatchRemoveAndD7c(t *testing.T) {
	mock := render.NewMock()
	rc := diff.New(mock)
	color := flux.RGB(20, 40, 60)
	list := flux.MustDrawList(drawFill(color))
	tree := func(value flux.DrawList, include bool) *widget.Node {
		root := widget.NewNode("Window")
		surface := widget.NewNode("DrawSurface")
		surface.Key = "draw"
		if include {
			surface.Props.Set("DrawList", value)
		}
		return root.Add(surface)
	}

	rc.Render(tree(list, true))
	element := rc.Lookup("draw")
	if element == nil {
		t.Fatal("DrawSurface was not mounted")
	}
	h := element.Handle
	if !mock.DrawList(h).Equal(list) || mock.DrawInvalidations(h) != 1 {
		t.Fatalf("mount list=%v invalidations=%d", mock.DrawList(h), mock.DrawInvalidations(h))
	}
	if ops := rc.Render(tree(list, true)); len(ops) != 0 || mock.DrawInvalidations(h) != 1 {
		t.Fatalf("same list D7c ops=%+v invalidations=%d", ops, mock.DrawInvalidations(h))
	}
	changed := flux.MustDrawList(drawFill(flux.RGB(80, 100, 120)))
	ops := rc.Render(tree(changed, true))
	if len(ops) != 2 || !mock.DrawList(h).Equal(changed) || mock.DrawInvalidations(h) != 2 {
		t.Fatalf("patch ops=%+v invalidations=%d", ops, mock.DrawInvalidations(h))
	}
	ops = rc.Render(tree(flux.DrawList{}, false))
	if len(ops) != 2 || mock.DrawList(h).Len() != 0 || mock.DrawInvalidations(h) != 3 {
		t.Fatalf("remove ops=%+v list=%v invalidations=%d", ops, mock.DrawList(h), mock.DrawInvalidations(h))
	}
}

type rendererWithoutDraw struct{ render.Renderer }

func TestDrawListCapabilityMissingSafelyDegrades(t *testing.T) {
	mock := render.NewMock()
	rc := diff.New(rendererWithoutDraw{Renderer: mock})
	root := widget.NewNode("Window")
	surface := widget.NewNode("DrawSurface")
	surface.Props.Set("DrawList", flux.MustDrawList(drawFill(flux.RGB(1, 2, 3))))
	root.Add(surface)
	if ops := rc.Render(root); len(ops) == 0 {
		t.Fatal("mount should still create the logical native identity")
	}
}

func TestPaintCommandsToDrawList(t *testing.T) {
	list, err := render.PaintCommandsToDrawList([]render.PaintCommand{
		{Kind: flux.PaintClear, Color: flux.RGB(255, 255, 255)},
		{Kind: flux.PaintCircle, X: 10, Y: 12, Radius: 4, FillColor: flux.RGB(1, 2, 3)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if list.Len() != 2 {
		t.Fatalf("adapted list length = %d, want 2", list.Len())
	}
}
