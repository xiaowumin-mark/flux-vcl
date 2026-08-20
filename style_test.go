package flux_test

import (
	"reflect"
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
)

func TestStylePatchPresenceCanOverrideWithZeroValues(t *testing.T) {
	base := flux.ControlStyle{
		Background: flux.RGB(10, 20, 30),
		Foreground: flux.RGB(240, 240, 240),
		Radius:     8,
		Padding:    flux.InsetsAll(6),
		MinSize:    flux.Size{W: 80, H: 24},
	}

	patch := flux.NewControlStylePatch(
		flux.SetBackground(0),
		flux.SetRadius(0),
		flux.SetPadding(flux.Insets{}),
		flux.SetMinSize(flux.Size{}),
	)
	got := patch.Apply(base)
	if got.Background != 0 || got.Radius != 0 || got.Padding != (flux.Insets{}) || got.MinSize != (flux.Size{}) {
		t.Fatalf("zero-valued fields were not applied: %+v", got)
	}
	if got.Foreground != base.Foreground {
		t.Fatalf("unset field changed: got foreground %#x, want %#x", got.Foreground, base.Foreground)
	}
	wantMask := flux.StyleFieldBackground | flux.StyleFieldRadius | flux.StyleFieldPadding | flux.StyleFieldMinSize
	if patch.Set != wantMask || !patch.Has(wantMask) || patch.IsSet(flux.StyleFieldForeground) {
		t.Fatalf("unexpected presence mask: got %#x, want %#x", patch.Set, wantMask)
	}
}

func TestStyleOptionsLastWriteWinsAndPatchRoundTrip(t *testing.T) {
	colorA := flux.RGB(1, 2, 3)
	colorB := flux.RGB(4, 5, 6)
	font := flux.FontSpec{Family: "Test UI", Size: 13, Weight: flux.FontWeightSemibold}
	style := flux.NewControlStyle(
		flux.StyleBackground(colorA),
		flux.WithBackground(colorB),
		flux.SetFont(font),
		flux.SetBorder(flux.NewBorderSpec(colorA, 2)),
	)
	if style.Background != colorB || style.Font != font || style.Border != flux.NewBorderSpec(colorA, 2) {
		t.Fatalf("options did not resolve deterministically: %+v", style)
	}
	full := style.Patch()
	if full.Set != flux.StyleFieldAll || full.Apply(flux.ControlStyle{}) != style {
		t.Fatalf("full patch did not round-trip style: patch=%+v style=%+v", full, style)
	}
}

func TestInsetsConstructorsAndGeometry(t *testing.T) {
	insets := flux.InsetsSymmetric(3, 5)
	if insets != (flux.Insets{Left: 3, Top: 5, Right: 3, Bottom: 5}) {
		t.Fatalf("unexpected symmetric insets: %+v", insets)
	}
	if insets.Horizontal() != 6 || insets.Vertical() != 10 {
		t.Fatalf("unexpected inset totals: horizontal=%d vertical=%d", insets.Horizontal(), insets.Vertical())
	}
	rect := flux.Rect{X: 10, Y: 20, W: 4, H: 4}
	if got := flux.InsetsAll(3).Deflate(rect); got.W != 0 || got.H != 0 || got.X != 13 || got.Y != 23 {
		t.Fatalf("unexpected deflate result: %+v", got)
	}
	if err := (flux.Insets{Left: -1}).Validate(); err == nil {
		t.Fatal("negative insets should fail validation")
	}
}

func TestStyleValueTypesContainNoMutableContainers(t *testing.T) {
	for _, typ := range []reflect.Type{
		reflect.TypeOf(flux.Insets{}),
		reflect.TypeOf(flux.BorderSpec{}),
		reflect.TypeOf(flux.ControlStyle{}),
		reflect.TypeOf(flux.FocusStyle{}),
		reflect.TypeOf(flux.ControlStylePatch{}),
	} {
		for i := 0; i < typ.NumField(); i++ {
			kind := typ.Field(i).Type.Kind()
			if kind == reflect.Map || kind == reflect.Slice || kind == reflect.Pointer || kind == reflect.Interface || kind == reflect.Func {
				t.Fatalf("%s field %s has mutable/reference kind %s", typ, typ.Field(i).Name, kind)
			}
		}
	}
}

func TestCD0FontSpecValidationAtStyleBoundaries(t *testing.T) {
	if err := (flux.FontSpec{}).Validate(); err != nil {
		t.Fatalf("zero FontSpec should be valid: %v", err)
	}
	cases := []struct {
		name string
		font flux.FontSpec
	}{
		{name: "negative size", font: flux.FontSpec{Size: -1}},
		{name: "unknown weight", font: flux.FontSpec{Weight: flux.FontWeight(450)}},
		{name: "invalid family", font: flux.FontSpec{Family: string([]byte{0xff})}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.font.Validate(); err == nil {
				t.Fatal("invalid FontSpec was accepted")
			}
			assertCD2Panics(t, func() { flux.Font(tc.font) })
			assertCD2Panics(t, func() { flux.SetFont(tc.font) })
			assertCD2Panics(t, func() { flux.Style(flux.ControlStyle{Font: tc.font}) })
		})
	}
	assertCD2Panics(t, func() { flux.FontFamily(string([]byte{0xff})) })
	assertCD2Panics(t, func() { flux.FontWeightOpt(flux.FontWeight(450)) })
}

func assertCD2Panics(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if recover() == nil {
			t.Fatal("expected panic")
		}
	}()
	fn()
}
