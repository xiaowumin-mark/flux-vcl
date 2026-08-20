package flux_test

import (
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/diff"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

func TestCD2IntrinsicUsesFontPaddingAndMinSize(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	font := flux.FontSpec{Family: "Test UI", Size: 24, Weight: flux.FontWeightBold}
	app.Render(flux.Window(flux.Text("abc", flux.Key("text"), flux.Font(font),
		flux.Padding(flux.InsetsAll(2)), flux.MinSize(flux.Size{W: 40, H: 30}))))

	text := findByKey(t, app.Root(), "text")
	if got := boundsOf(text); got.W != 40 || got.H != 34 {
		t.Fatalf("styled intrinsic bounds = %+v, want 40x34 after font/padding", got)
	}
	requests := m.MeasureRequests()
	if len(requests) == 0 {
		t.Fatal("layout did not issue a styled measurement request")
	}
	last := requests[len(requests)-1]
	if last.Font != font || last.DPI != 96 {
		t.Fatalf("styled request = %+v, want font=%+v dpi=96", last, font)
	}
}

func TestCD2StyleFontUsesSameMeasurementAndNativeProperty(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	font := flux.FontSpec{Family: "Styled UI", Size: 18, Weight: flux.FontWeightSemibold}
	build := func() flux.Widget {
		return flux.Window(flux.Text("styled", flux.Key("text"), flux.Style(flux.ControlStyle{Font: font})))
	}

	app.Render(build())
	request, ok := m.LastMeasureRequest()
	if !ok || request.Font != font {
		t.Fatalf("measurement font = %+v (present=%v), want %+v", request.Font, ok, font)
	}
	var nativeFont flux.FontSpec
	found := false
	for _, op := range m.Ops() {
		if op.Key == "Font" {
			nativeFont, found = op.Value.(flux.FontSpec)
		}
	}
	if !found || nativeFont != font {
		t.Fatalf("native font = %+v (present=%v), want %+v; ops=%+v", nativeFont, found, font, m.Ops())
	}

	base := len(m.Ops())
	app.Render(build())
	if ops := m.Ops()[base:]; len(ops) != 0 {
		t.Fatalf("identical resolved style should produce zero mutations: %+v", ops)
	}
}

func TestCD2FontRemovalRestoresSystemFontContract(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	font := flux.FontSpec{Family: "Styled UI", Size: 18, Weight: flux.FontWeightBold}

	app.Render(flux.Window(flux.Text("styled", flux.Key("text"), flux.Font(font))))
	base := len(m.Ops())
	app.Render(flux.Window(flux.Text("styled", flux.Key("text"))))

	want := flux.FontSpec{Weight: flux.FontWeightNormal}
	found := false
	for _, op := range m.Ops()[base:] {
		if op.Key != "Font" {
			continue
		}
		got, ok := op.Value.(flux.FontSpec)
		if !ok || got != want {
			t.Fatalf("removed font reset = %#v, want %+v", op.Value, want)
		}
		found = true
	}
	if !found {
		t.Fatalf("font removal did not reset native font: ops=%+v", m.Ops()[base:])
	}
	request, ok := m.LastMeasureRequest()
	if !ok || request.Font != want {
		t.Fatalf("font removal measurement = %+v (present=%v), want %+v", request.Font, ok, want)
	}
}

func TestCD2StyleAndAtomicFontPrecedence(t *testing.T) {
	styleFont := flux.FontSpec{Family: "Style UI", Size: 20, Weight: flux.FontWeightBold}
	atomicFont := flux.FontSpec{Family: "Atomic UI", Size: 14, Weight: flux.FontWeightSemibold}
	cases := []struct {
		name string
		opts []flux.Opt
		want flux.FontSpec
	}{
		{
			name: "style then family override",
			opts: []flux.Opt{flux.Style(flux.ControlStyle{Font: styleFont}), flux.FontFamily("Atomic UI")},
			want: flux.FontSpec{Family: "Atomic UI", Size: 20, Weight: flux.FontWeightBold},
		},
		{
			name: "family override then style",
			opts: []flux.Opt{flux.FontFamily("Atomic UI"), flux.Style(flux.ControlStyle{Font: styleFont})},
			want: flux.FontSpec{Family: "Atomic UI", Size: 20, Weight: flux.FontWeightBold},
		},
		{
			name: "full style replaces earlier patch",
			opts: []flux.Opt{flux.Style(flux.SetFont(atomicFont), flux.ControlStyle{Font: styleFont})},
			want: styleFont,
		},
		{
			name: "partial atomic size preserves style fields",
			opts: []flux.Opt{flux.Style(flux.ControlStyle{Font: styleFont}), flux.FontSize(24)},
			want: flux.FontSpec{Family: "Style UI", Size: 24, Weight: flux.FontWeightBold},
		},
		{
			name: "full atomic font then partial size",
			opts: []flux.Opt{flux.Font(atomicFont), flux.FontSize(24)},
			want: flux.FontSpec{Family: "Atomic UI", Size: 24, Weight: flux.FontWeightSemibold},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := render.NewMock()
			app := flux.NewApp(m)
			opts := append([]flux.Opt{flux.Key("text")}, tc.opts...)
			app.Render(flux.Window(flux.Text("styled", opts...)))
			request, ok := m.LastMeasureRequest()
			if !ok {
				t.Fatal("layout did not issue a styled measurement request")
			}
			want := tc.want
			if want.Weight == 0 {
				want.Weight = flux.FontWeightNormal
			}
			if request.Font != want {
				t.Fatalf("resolved font = %+v, want %+v", request.Font, want)
			}
		})
	}
}

func TestCD2StylePatchAffectsLayoutAcrossUpdateAndRemoval(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	build := func(insets flux.Insets, min flux.Size, styled bool) flux.Widget {
		var opts []flux.Opt
		if styled {
			opts = append(opts, flux.Style(flux.SetPadding(insets), flux.SetMinSize(min)))
		}
		opts = append(opts, flux.Key("text"))
		return flux.Window(flux.Text("x", opts...))
	}

	app.Render(build(flux.InsetsAll(2), flux.Size{W: 0, H: 0}, true))
	if got := boundsOf(findByKey(t, app.Root(), "text")); got.W != 12 || got.H != 24 {
		t.Fatalf("initial style patch bounds = %+v, want 12x24", got)
	}
	base := len(m.Ops())
	app.Render(build(flux.InsetsAll(4), flux.Size{W: 20, H: 30}, true))
	if got := boundsOf(findByKey(t, app.Root(), "text")); got.W != 20 || got.H != 30 {
		t.Fatalf("updated style patch bounds = %+v, want 20x30", got)
	}
	if len(m.Ops()) == base {
		t.Fatal("style patch update did not produce a bounds mutation")
	}
	app.Render(build(flux.Insets{}, flux.Size{}, false))
	if got := boundsOf(findByKey(t, app.Root(), "text")); got.W != 8 || got.H != 20 {
		t.Fatalf("removed style patch bounds = %+v, want 8x20", got)
	}
}

func TestCD2LayoutStyleChangesTriggerUpdateLifecycle(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	updates := 0
	build := func(insets flux.Insets, withStyle bool) flux.Widget {
		opts := []flux.Opt{
			flux.Key("text"),
			flux.OnUpdate(func() { updates++ }),
		}
		if withStyle {
			opts = append(opts, flux.Style(flux.SetPadding(insets)))
		}
		return flux.Window(flux.Text("x", opts...))
	}

	app.Render(build(flux.InsetsAll(2), true))
	app.Render(build(flux.InsetsAll(4), true))
	if updates != 1 {
		t.Fatalf("style update lifecycle count = %d, want 1", updates)
	}
	app.Render(build(flux.Insets{}, false))
	if updates != 2 {
		t.Fatalf("style removal lifecycle count = %d, want 2", updates)
	}
	app.Render(build(flux.Insets{}, false))
	if updates != 2 {
		t.Fatalf("identical style removal rerender changed lifecycle count = %d, want 2", updates)
	}
}

func TestCD2GapAndPaddingKeepChildFramesExplicit(t *testing.T) {
	m := render.NewMock()
	app := flux.NewApp(m)
	app.Render(flux.Window(flux.Padding(flux.Insets{Left: 2, Top: 3, Right: 4, Bottom: 5},
		flux.Column(flux.Gap(10),
			flux.Text("a", flux.Key("a")),
			flux.Text("b", flux.Key("b")),
		))))

	a := boundsOf(findByKey(t, app.Root(), "a"))
	b := boundsOf(findByKey(t, app.Root(), "b"))
	if a.X != 2 || a.Y != 3 || b.X != 2 || b.Y != 33 {
		t.Fatalf("child frames = a=%+v b=%+v, want a=(2,3) b=(2,33)", a, b)
	}
	padding := findCD2ByType(t, app.Root(), "Padding")
	if got := boundsOf(padding); got.W != 14 || got.H != 58 {
		t.Fatalf("padding frame = %+v, want 14x58", got)
	}
}

func findCD2ByType(t *testing.T, root *diff.Element, typ string) *diff.Element {
	t.Helper()
	if root == nil {
		return nil
	}
	if root.Type == typ {
		return root
	}
	for _, child := range root.Children {
		if found := findCD2ByType(t, child, typ); found != nil {
			return found
		}
	}
	return nil
}
