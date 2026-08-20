package render

import "testing"

type legacyMeasureRenderer struct {
	*Mock
}

// Hide Mock's optional capability while retaining the legacy Renderer
// implementation.  This models a third-party backend compiled before CD2.
type legacyOnlyRenderer struct{ Renderer }

func TestMeasureTextFallsBackForLegacyRenderer(t *testing.T) {
	legacy := legacyOnlyRenderer{Renderer: NewMock()}
	got := MeasureText(legacy, TextMeasureRequest{Text: "abcd", Font: FontSpec{Size: 16}, DPI: 96})
	if got != (Size{W: 32, H: 20}) {
		t.Fatalf("legacy styled fallback = %#v, want 32x20", got)
	}
}

func TestMeasureTextUsesStyledCapabilityAndCanonicalizesRequest(t *testing.T) {
	m := NewMock()
	got := MeasureText(m, TextMeasureRequest{Text: "abc", Font: FontSpec{}, DPI: 0})
	if got.W != 24 || got.H != 20 {
		t.Fatalf("mock styled measurement = %#v, want 24x20", got)
	}
	req, ok := m.LastMeasureRequest()
	if !ok {
		t.Fatal("styled request was not recorded")
	}
	if req.DPI != 96 || req.Font.Weight != FontWeightNormal {
		t.Fatalf("request normalization = %#v, want DPI=96 normal weight", req)
	}
}

func TestTextMeasureCacheKeyIncludesStyleAndConstraints(t *testing.T) {
	base := TextMeasureRequest{Text: "same", Font: FontSpec{Family: "Segoe UI", Size: 14}, DPI: 96}
	a := base.CacheKey()
	b := base
	b.Font.Weight = FontWeightBold
	if a == b.CacheKey() {
		t.Fatal("font weight must distinguish text cache keys")
	}
	b = base
	b.DPI = 144
	if a == b.CacheKey() {
		t.Fatal("DPI must distinguish text cache keys")
	}
	b = base
	b.Wrap = TextWrapWord
	b.MaxWidth = 80
	if a == b.CacheKey() {
		t.Fatal("wrap/constraint mode must distinguish text cache keys")
	}
}

func TestNormalizeTextMeasureRequestMergesBounds(t *testing.T) {
	r := NormalizeTextMeasureRequest(TextMeasureRequest{
		Text:       "x",
		Constraint: TextMeasureConstraints{MinW: 4, MaxW: 40},
		MaxHeight:  12,
	})
	if r.Constraints.MinW != 4 || r.Constraints.MaxW != 40 || r.Constraints.MaxH != 12 {
		t.Fatalf("merged constraints = %#v", r.Constraints)
	}
}
