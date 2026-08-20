//go:build windows && !race

package native

import (
	"testing"

	"github.com/energye/lcl/types/colors"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

func TestStyledTextMeasureDPIAndFontColorProbe(t *testing.T) {
	t.Setenv(forceHighContrastEnv, "0")
	if err := Init(radioProbeDLL(t)); err != nil {
		t.Fatal(err)
	}
	r := NewRenderer()
	parent := r.Create("Window")
	labelHandle := r.Create("Text")
	r.SetParent(labelHandle, parent)

	declaredColor := render.Color(0xFF123456)
	r.SetFontColor(labelHandle, declaredColor)
	r.SetFont(labelHandle, render.FontSpec{Family: "Segoe UI", Size: 16, Weight: render.FontWeightSemibold})
	if got := r.controls[labelHandle].Font().Color(); got != colorToTColor(declaredColor) {
		t.Fatalf("SetFont reset declared FontColor: got %#x, want %#x", got, colorToTColor(declaredColor))
	}

	var widths, heights []int
	for _, dpi := range []int{96, 144, 192} {
		size := r.MeasureText(render.TextMeasureRequest{
			Text: "FluxVCL", DPI: dpi,
			Font: render.FontSpec{Family: "Segoe UI", Size: 16, Weight: render.FontWeightNormal},
		})
		widths = append(widths, size.W)
		heights = append(heights, size.H)
	}
	if dpiMetricDrift(widths) > 2 || dpiMetricDrift(heights) > 2 {
		t.Fatalf("DPI-normalized extent drifted: widths=%v heights=%v", widths, heights)
	}

	r.controls[labelHandle].Font().SetHeight(-1)
	r.invalidateDPI()
	if got := r.controls[labelHandle].Font().Height(); got == -1 {
		t.Fatal("DPI invalidation did not reapply the declared font")
	}
	resizeCount := 0
	r.OnResize(func(_, _ int) { resizeCount++ })
	r.refreshHighContrast()
	if resizeCount != 1 {
		t.Fatalf("system typography invalidation emitted %d layouts, want 1", resizeCount)
	}

	r.highContrast.Store(true)
	r.SetFont(labelHandle, render.FontSpec{Size: 18})
	if got := r.controls[labelHandle].Font().Color(); got != colors.ClDefault {
		t.Fatalf("SetFont did not preserve high-contrast FontColor: got %#x, want %#x", got, colors.ClDefault)
	}
}

func dpiMetricDrift(values []int) int {
	if len(values) == 0 {
		return 0
	}
	minimum, maximum := values[0], values[0]
	for _, value := range values[1:] {
		if value < minimum {
			minimum = value
		}
		if value > maximum {
			maximum = value
		}
	}
	return maximum - minimum
}
