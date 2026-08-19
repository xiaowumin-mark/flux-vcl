package native

import (
	"os"
	"strings"

	"github.com/energye/lcl/pkgs/win"
	"github.com/energye/lcl/types"
	"github.com/energye/lcl/types/colors"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

const forceHighContrastEnv = "FLUXVCL_FORCE_HIGH_CONTRAST"

func detectHighContrast() bool {
	if value, exists := os.LookupEnv(forceHighContrastEnv); exists {
		return parseHighContrastOverride(value)
	}
	return win.IsCurrentlyHighContrastMode()
}

func parseHighContrastOverride(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

// HighContrast 报告默认后端当前是否让系统高对比度覆盖应用主题。环境变量
// FLUXVCL_FORCE_HIGH_CONTRAST 可用于自动化验证，实际系统设置仍是默认来源。
func (r *Renderer) HighContrast() bool { return r.highContrast.Load() }

func (r *Renderer) applyRequestedColor(h render.Handle) {
	control := r.controls[h]
	if control == nil {
		return
	}
	requested := r.requestedColors[h]
	control.SetColor(resolveThemeColor(requested, r.HighContrast()))
}

func (r *Renderer) applyRequestedFontColor(h render.Handle) {
	control := r.controls[h]
	if control == nil || control.Font() == nil {
		return
	}
	requested := r.requestedFontColors[h]
	control.Font().SetColor(resolveThemeColor(requested, r.HighContrast()))
}

func resolveThemeColor(requested render.Color, highContrast bool) types.TColor {
	if highContrast || requested == 0 {
		return types.TColor(colors.ClDefault)
	}
	return colorToTColor(requested)
}

func (r *Renderer) applyRequestedTitleBar() {
	if !r.titleBarConfigured {
		return
	}
	r.applyTitleBarDark(r.requestedTitleBarDark && !r.HighContrast())
}

func (r *Renderer) refreshHighContrast() {
	r.highContrast.Store(detectHighContrast())
	for h := range r.requestedColors {
		r.applyRequestedColor(h)
	}
	for h := range r.requestedFontColors {
		r.applyRequestedFontColor(h)
	}
	r.applyRequestedTitleBar()
	for _, paint := range r.paints {
		if paint != nil && paint.control != nil {
			paint.control.Invalidate()
		}
	}
}

type contrastPaintPart uint8

const (
	contrastPaintBackground contrastPaintPart = iota
	contrastPaintFill
	contrastPaintStroke
)

func (r *Renderer) paintColor(requested render.Color, part contrastPaintPart) types.TColor {
	if !r.HighContrast() {
		return colorToTColor(requested)
	}
	switch part {
	case contrastPaintBackground:
		return types.TColor(colors.ClWindow)
	case contrastPaintFill:
		return types.TColor(colors.ClHighlight)
	default:
		return types.TColor(colors.ClWindowText)
	}
}
