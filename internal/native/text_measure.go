package native

import (
	"strings"

	"github.com/energye/lcl/lcl"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

type nativeFontCacheKey struct {
	Font     render.FontSpec
	DPI      int
	Revision uint64
}

// MeasureText implements the optional styled capability.  The same resolved
// FontSpec is materialized on the shared bitmap canvas for every request;
// cache identity includes the text style, target DPI, wrapping constraints and
// font revision through TextMeasureRequest.CacheKey.
func (r *Renderer) MeasureText(request render.TextMeasureRequest) render.Size {
	if r.measureCache == nil {
		r.measureCache = make(map[render.TextMeasureCacheKey]render.Size)
	}
	if request.DPI <= 0 {
		request.DPI = int(r.currentDPI())
	}
	if request.FontRevision == 0 && request.Revision == 0 {
		request.FontRevision = r.fontRevision
	}
	request = render.NormalizeTextMeasureRequest(request)
	key := request.CacheKey()
	if size, ok := r.measureCache[key]; ok {
		return size
	}

	// A zero Renderer is useful in package-level contract tests and during
	// teardown. Do not allocate a native bitmap when no form exists; retain the
	// same deterministic fallback as the headless renderer.
	if r.measureBmp == nil && r.form == nil {
		size := render.Size{W: len([]rune(request.Text)) * 8, H: 20}
		size = constrainNativeSize(size, request)
		r.measureCache[key] = size
		return size
	}
	if r.measureBmp == nil {
		bmp := lcl.NewBitmap()
		bmp.SetSize(1, 1)
		r.measureBmp = bmp
	}
	if font := r.measureBmp.Canvas().FontToFont(); font != nil {
		// Configure the canvas-owned font directly. Assigning a separately
		// materialized font through SetFontToFont makes LCL rescale it to the
		// bitmap DC's fixed DPI, losing the request DPI before TextExtent runs.
		r.configureFont(font, request.Font, request.DPI)
	}

	measure := func(text string) render.Size {
		sz := r.measureBmp.Canvas().TextExtentWithStr(text)
		w, h := int(sz.Cx), int(sz.Cy)
		if w <= 0 {
			w = len([]rune(text)) * 8
		}
		if h <= 0 {
			h = 20
		}
		// The font was materialized for request.DPI, so its physical extent must
		// be normalized by that same target DPI. The bitmap DC can retain a
		// process-wide DPI that differs from the window's current monitor.
		return render.Size{W: render.PXToDIP(w, request.DPI), H: render.PXToDIP(h, request.DPI)}
	}

	lines := strings.Split(request.Text, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	lineHeight, maxWidth := 0, 0
	for _, line := range lines {
		m := measure(line)
		if m.H > lineHeight {
			lineHeight = m.H
		}
		if m.W > maxWidth {
			maxWidth = m.W
		}
	}
	if lineHeight <= 0 {
		lineHeight = measure("").H
	}

	lineCount := len(lines)
	if request.Wrap == render.TextWrapWord && request.Constraints.MaxW >= 0 {
		maxWidth, lineCount = wrapNativeLines(lines, request.Constraints.MaxW, measure)
	}
	size := render.Size{W: maxWidth, H: lineHeight * lineCount}
	size = constrainNativeSize(size, request)
	r.measureCache[key] = size
	return size
}

func constrainNativeSize(size render.Size, request render.TextMeasureRequest) render.Size {
	if size.W < request.Constraints.MinW {
		size.W = request.Constraints.MinW
	}
	if size.H < request.Constraints.MinH {
		size.H = request.Constraints.MinH
	}
	if request.Constraints.MaxW >= 0 && size.W > request.Constraints.MaxW {
		size.W = request.Constraints.MaxW
	}
	if request.Constraints.MaxH >= 0 && size.H > request.Constraints.MaxH {
		size.H = request.Constraints.MaxH
	}
	return size
}

func wrapNativeLines(lines []string, maxWidth int, measure func(string) render.Size) (int, int) {
	lineCount, widest := 0, 0
	for _, source := range lines {
		words := strings.Fields(source)
		if len(words) == 0 {
			lineCount++
			continue
		}
		current := ""
		for _, word := range words {
			candidate := word
			if current != "" {
				candidate = current + " " + word
			}
			if current != "" && measure(candidate).W > maxWidth {
				if w := measure(current).W; w > widest {
					widest = w
				}
				lineCount++
				current = word
			} else {
				current = candidate
			}
		}
		if w := measure(current).W; w > widest {
			widest = w
		}
		lineCount++
	}
	return widest, lineCount
}

func (r *Renderer) measureFont(spec render.FontSpec, dpi int, revision uint64) lcl.IFont {
	if dpi <= 0 {
		dpi = int(r.canvasDPI())
	}
	spec = render.ResolveFontSpec(spec)
	key := nativeFontCacheKey{Font: spec, DPI: dpi, Revision: revision}
	if r.fontCache == nil {
		r.fontCache = make(map[nativeFontCacheKey]lcl.IFont)
	}
	if font := r.fontCache[key]; font != nil {
		return font
	}
	if r.form == nil || r.form.Font() == nil {
		return nil
	}
	font := lcl.NewFont()
	r.configureFont(font, spec, dpi)
	r.fontCache[key] = font
	return font
}

func (r *Renderer) configureFont(font lcl.IFont, spec render.FontSpec, dpi int) {
	if font == nil || r.form == nil || r.form.Font() == nil {
		return
	}
	font.Assign(r.form.Font())
	font.SetPixelsPerInch(int32(dpi))
	if spec.Family != "" {
		font.SetName(spec.Family)
	}
	if spec.Size > 0 {
		font.SetHeight(-int32(render.DIPToPX(spec.Size, dpi)))
	}
	font.SetBold(spec.Weight >= render.FontWeightSemibold)
	font.SetItalic(spec.Italic)
	font.SetUnderline(spec.Underline)
	font.SetStrikeThrough(spec.Strikeout)
}

// SetFont applies the same resolved font used by MeasureText to a native
// control. The cached font is copied into the control-owned object so later
// cache invalidation cannot mutate a live control.
func (r *Renderer) SetFont(h render.Handle, spec render.FontSpec) {
	if r.requestedFonts == nil {
		r.requestedFonts = make(map[render.Handle]render.FontSpec)
	}
	r.requestedFonts[h] = render.NormalizeFontSpec(spec)
	r.applyRequestedFont(h)
}

func (r *Renderer) applyRequestedFont(h render.Handle) {
	control := r.controls[h]
	if control == nil || control.Font() == nil {
		return
	}
	dpi := int(r.currentDPI())
	font := r.measureFont(r.requestedFonts[h], dpi, r.fontRevision)
	if font == nil {
		return
	}
	control.Font().Assign(font)
	// Assign copies the cached font's color too. Restore the independently
	// declared FontColor so a font-only patch cannot silently reset it.
	if _, configured := r.requestedFontColors[h]; configured {
		r.applyRequestedFontColor(h)
	}
}

func (r *Renderer) reapplyRequestedFonts() {
	for h := range r.requestedFonts {
		r.applyRequestedFont(h)
	}
}

func (r *Renderer) releaseTextMeasureCaches() {
	if r.measureBmp != nil && r.form != nil && r.form.Font() != nil {
		r.measureBmp.Canvas().SetFontToFont(r.form.Font())
	}
	for key, font := range r.fontCache {
		if font != nil {
			font.Free()
		}
		delete(r.fontCache, key)
	}
	r.measureCache = make(map[render.TextMeasureCacheKey]render.Size)
	r.fontRevision++
}

func (r *Renderer) releaseTextMeasureResources() {
	r.releaseTextMeasureCaches()
	if r.measureBmp != nil {
		r.measureBmp.Free()
		r.measureBmp = nil
		r.canvasDpi = 0
	}
}
