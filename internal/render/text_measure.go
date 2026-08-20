package render

import (
	"math"
	"strings"
	"unicode/utf8"
)

// Size is the result of an intrinsic measurement.  Like Rect and all other
// render geometry, its units are integer DIP values.
type Size struct {
	W int
	H int
}

// TextMeasureConstraints describes optional bounds supplied to a text
// measurement request.  A negative maximum means that the corresponding axis
// is unbounded.  Zero is a useful, explicit bound and is therefore preserved.
// The type deliberately lives in render rather than importing the public
// layout package; renderer implementations must remain usable by third-party
// clients without creating an import cycle.
type TextMeasureConstraints struct {
	MinW int
	MaxW int
	MinH int
	MaxH int
}

// TextMeasureConstraint is a singular spelling retained for callers that use
// the terminology from the layout protocol.
type TextMeasureConstraint = TextMeasureConstraints

// TextMeasureRequest is the complete input to a styled text measurement.
//
// Font is the same value passed to DrawText.  DPI and Revision are part of the
// request rather than ambient renderer state so a measurement cache cannot
// accidentally reuse a result from another monitor or typography revision.
// MaxWidth/MaxHeight are convenience bounds; when supplied they are merged
// with Constraints.  Wrap and Overflow are included in the key even when a
// backend elects to use a native single-line measurement for them.
type TextMeasureRequest struct {
	Text string
	Font FontSpec
	DPI  int

	// Revision identifies the current system/theme font revision.  FontRevision
	// is an explicit alias used by integrations that distinguish it from other
	// renderer revisions; when both are set, FontRevision wins.
	Revision     uint64
	FontRevision uint64

	Wrap     TextWrap
	Overflow TextOverflow

	Constraints TextMeasureConstraints
	// Constraint is a singular compatibility spelling.  Non-zero fields are
	// merged with Constraints (the latter wins when both specify a field).
	Constraint TextMeasureConstraints
	MaxWidth   int
	MaxHeight  int
	// HasConstraints disambiguates an explicitly zero-sized box from the
	// zero-value request, whose axes are unbounded. Layout sets this when it
	// forwards BoxConstraints so a max of zero remains meaningful.
	HasConstraints bool
}

// StyledTextMeasurer is an optional narrow renderer capability.  Renderer
// intentionally does not embed this interface: existing third-party
// renderers continue to work and callers use MeasureText's legacy fallback.
type StyledTextMeasurer interface {
	MeasureText(TextMeasureRequest) Size
}

// MeasureRequest builds the exact request used by a TextPaint. Keeping this
// conversion next to Draw Core prevents layout and paint from inventing
// separate font/fallback normalization rules.
func (paint TextPaint) MeasureRequest(text string, dpi int, constraints TextMeasureConstraints) TextMeasureRequest {
	return TextMeasureRequest{
		Text: text, Font: NormalizeFontSpec(paint.Font), DPI: dpi,
		Wrap: paint.Wrap, Overflow: paint.Overflow,
		Constraints: constraints, HasConstraints: true,
	}
}

// TextMeasureRequest is a descriptive alias for MeasureRequest.
func (paint TextPaint) TextMeasureRequest(text string, dpi int, constraints TextMeasureConstraints) TextMeasureRequest {
	return paint.MeasureRequest(text, dpi, constraints)
}

// TextMeasureCacheKey is the comparable, canonical cache identity for a text
// request.  It includes every input that can change intrinsic dimensions.  A
// backend may use this key directly or serialize it as part of a bounded cache.
type TextMeasureCacheKey struct {
	Text       string
	Font       FontSpec
	DPI        int
	Revision   uint64
	Wrap       TextWrap
	Overflow   TextOverflow
	MinW, MaxW int
	MinH, MaxH int
}

// CacheKey canonicalizes a request and returns its complete cache identity.
func (r TextMeasureRequest) CacheKey() TextMeasureCacheKey {
	n := NormalizeTextMeasureRequest(r)
	c := n.Constraints
	return TextMeasureCacheKey{
		Text:     n.Text,
		Font:     n.Font,
		DPI:      n.DPI,
		Revision: n.revision(),
		Wrap:     n.Wrap,
		Overflow: n.Overflow,
		MinW:     c.MinW,
		MaxW:     c.MaxW,
		MinH:     c.MinH,
		MaxH:     c.MaxH,
	}
}

// NewTextMeasureCacheKey is the function form of TextMeasureRequest.CacheKey,
// useful to backends that keep cache keys in a separate package.
func NewTextMeasureCacheKey(r TextMeasureRequest) TextMeasureCacheKey { return r.CacheKey() }

func (r TextMeasureRequest) revision() uint64 {
	if r.FontRevision != 0 {
		return r.FontRevision
	}
	return r.Revision
}

// NormalizeTextMeasureRequest applies the shared request contract used by
// measurement and drawing.  Invalid negative bounds are treated as absent;
// negative font sizes are reset to the documented inherited (zero) size so a
// capability cannot panic while handling an untrusted request.
func NormalizeTextMeasureRequest(r TextMeasureRequest) TextMeasureRequest {
	if !utf8.ValidString(r.Text) {
		// Go strings can contain arbitrary bytes.  Native text APIs generally
		// expect UTF-8, so replace invalid sequences before a cache key is made.
		r.Text = strings.ToValidUTF8(r.Text, "\uFFFD")
	}
	r.Font = NormalizeFontSpec(r.Font)
	if r.DPI <= 0 {
		r.DPI = 96
	}
	if r.Wrap != TextNoWrap && r.Wrap != TextWrapWord {
		r.Wrap = TextNoWrap
	}
	if r.Overflow != TextOverflowClip && r.Overflow != TextOverflowEllipsis {
		r.Overflow = TextOverflowClip
	}
	r.Constraints = mergeTextConstraints(r.Constraints, r.Constraint)
	// The convenience scalar uses zero as "not supplied" unless
	// HasConstraints is set; callers that need an explicit zero-sized box set
	// HasConstraints=true (or use a negative maximum for an unbounded axis).
	if r.MaxWidth > 0 {
		r.Constraints.MaxW = r.MaxWidth
	}
	if r.MaxHeight > 0 {
		r.Constraints.MaxH = r.MaxHeight
	}
	if r.Constraints.MinW < 0 {
		r.Constraints.MinW = 0
	}
	if r.Constraints.MinH < 0 {
		r.Constraints.MinH = 0
	}
	if r.Constraints.MaxW == 0 && !r.HasConstraints {
		r.Constraints.MaxW = -1
	} else if r.Constraints.MaxW < 0 {
		r.Constraints.MaxW = -1
	}
	if r.Constraints.MaxH == 0 && !r.HasConstraints {
		r.Constraints.MaxH = -1
	} else if r.Constraints.MaxH < 0 {
		r.Constraints.MaxH = -1
	}
	if r.Constraints.MaxW >= 0 && r.Constraints.MaxW < r.Constraints.MinW {
		r.Constraints.MaxW = r.Constraints.MinW
	}
	if r.Constraints.MaxH >= 0 && r.Constraints.MaxH < r.Constraints.MinH {
		r.Constraints.MaxH = r.Constraints.MinH
	}
	return r
}

func mergeTextConstraints(base, overlay TextMeasureConstraints) TextMeasureConstraints {
	if overlay.MinW != 0 {
		base.MinW = overlay.MinW
	}
	if overlay.MaxW != 0 {
		base.MaxW = overlay.MaxW
	}
	if overlay.MinH != 0 {
		base.MinH = overlay.MinH
	}
	if overlay.MaxH != 0 {
		base.MaxH = overlay.MaxH
	}
	return base
}

// NormalizeFontSpec is the canonical FontSpec normalization shared by Draw
// validation and styled measurement.  It intentionally does not resolve a
// family name: an empty family means the current system UI font and a missing
// explicit family is resolved by the native font fallback chain.
func NormalizeFontSpec(font FontSpec) FontSpec {
	if font.Size < 0 {
		font.Size = 0
	}
	if font.Weight == 0 {
		font.Weight = FontWeightNormal
	}
	return font
}

// NormalizeTextPaint applies the same FontSpec/fallback normalization used by
// TextMeasureRequest. It is intentionally value-only so callers can retain a
// DrawText snapshot without sharing mutable native font state.
func NormalizeTextPaint(paint TextPaint) TextPaint {
	paint.Font = NormalizeFontSpec(paint.Font)
	return paint
}

// ResolveFontSpec is a descriptive alias for NormalizeFontSpec.  Native
// backends may apply their platform fallback after this value normalization.
func ResolveFontSpec(font FontSpec) FontSpec { return NormalizeFontSpec(font) }

// CanonicalFontSpec validates and canonicalizes a FontSpec using the exact
// rules applied by DrawText.  It is useful to backends that need an error
// rather than the permissive inherited-font behavior of NormalizeFontSpec.
func CanonicalFontSpec(font FontSpec) (FontSpec, error) { return canonicalizeFont(font) }

// CanonicalizeFontSpec is an equivalent verb spelling retained for adapter
// code that follows Go's usual Canonicalize naming convention.
func CanonicalizeFontSpec(font FontSpec) (FontSpec, error) {
	return CanonicalFontSpec(font)
}

// MeasureText asks for a styled measurement when the renderer exposes the
// optional capability.  Renderers from before CD2 transparently fall back to
// TextExtent, preserving existing layout behavior.
func MeasureText(r Renderer, request TextMeasureRequest) Size {
	request = NormalizeTextMeasureRequest(request)
	if r == nil {
		return Size{}
	}
	if styled, ok := r.(StyledTextMeasurer); ok && styled != nil {
		return sanitizeMeasuredSize(styled.MeasureText(request))
	}
	return sanitizeMeasuredSize(fallbackTextMeasure(r, request))
}

// MeasureTextWithFallback is an explicit spelling for code that wants to make
// the optional-capability fallback visible at call sites.
func MeasureTextWithFallback(r Renderer, request TextMeasureRequest) Size {
	return MeasureText(r, request)
}

// FallbackTextMeasurer adapts a legacy Renderer to StyledTextMeasurer.  It is
// useful when a layout/plugin receives a capability object independently of
// the renderer itself.
type FallbackTextMeasurer struct{ Renderer Renderer }

func (f FallbackTextMeasurer) MeasureText(request TextMeasureRequest) Size {
	return MeasureTextWithoutCapability(f.Renderer, request)
}

// MeasureTextWithoutCapability performs the deterministic legacy fallback and
// deliberately bypasses a StyledTextMeasurer assertion to avoid recursion.
func MeasureTextWithoutCapability(r Renderer, request TextMeasureRequest) Size {
	request = NormalizeTextMeasureRequest(request)
	if r == nil {
		return Size{}
	}
	return sanitizeMeasuredSize(fallbackTextMeasure(r, request))
}

// StyledTextExtent is a compact compatibility helper used by intrinsic
// layout code.  It is equivalent to MeasureText with a no-wrap request.
func StyledTextExtent(r Renderer, text string, font FontSpec) Size {
	return MeasureText(r, TextMeasureRequest{Text: text, Font: font})
}

func fallbackTextMeasure(r Renderer, request TextMeasureRequest) Size {
	if r == nil {
		return Size{}
	}

	// Legacy TextExtent has no font argument.  Keep the old metric exactly for
	// inherited fonts; for an explicit size use a proportional DIP estimate so
	// the fallback still responds to style changes and never silently reuses a
	// differently sized result.
	extent := func(text string) Size {
		w, h := r.TextExtent(text)
		if request.Font.Size > 0 {
			base := 16
			w = scaleMetric(w, request.Font.Size, base)
			h = scaleMetric(h, request.Font.Size, base)
		}
		return Size{W: max0(w), H: max0(h)}
	}

	lines := strings.Split(request.Text, "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	lineHeight := 0
	maxWidth := 0
	for _, line := range lines {
		m := extent(line)
		if m.H > lineHeight {
			lineHeight = m.H
		}
		if m.W > maxWidth {
			maxWidth = m.W
		}
	}
	if lineHeight == 0 {
		lineHeight = extent("").H
	}

	if request.Wrap == TextWrapWord && request.Constraints.MaxW >= 0 {
		maxWidth, lineCount := wrapFallbackLines(r, request, lines, request.Constraints.MaxW, extent)
		if lineCount > 0 {
			return applyTextConstraints(Size{W: maxWidth, H: lineCount * lineHeight}, request.Constraints)
		}
	}

	return applyTextConstraints(Size{W: maxWidth, H: lineHeight * len(lines)}, request.Constraints)
}

func wrapFallbackLines(r Renderer, request TextMeasureRequest, lines []string, maxW int, extent func(string) Size) (int, int) {
	if maxW < 0 {
		return 0, 0
	}
	lineCount := 0
	maxWidth := 0
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
			if current != "" && extent(candidate).W > maxW {
				m := extent(current)
				if m.W > maxWidth {
					maxWidth = m.W
				}
				lineCount++
				current = word
			} else {
				current = candidate
			}
			// A single word wider than the bound is clipped by native DrawText;
			// retaining its measured width keeps intrinsic size honest.
		}
		m := extent(current)
		if m.W > maxWidth {
			maxWidth = m.W
		}
		lineCount++
	}
	return maxWidth, lineCount
}

func applyTextConstraints(size Size, c TextMeasureConstraints) Size {
	if size.W < c.MinW {
		size.W = c.MinW
	}
	if size.H < c.MinH {
		size.H = c.MinH
	}
	if c.MaxW >= 0 && size.W > c.MaxW {
		size.W = c.MaxW
	}
	if c.MaxH >= 0 && size.H > c.MaxH {
		size.H = c.MaxH
	}
	return size
}

func sanitizeMeasuredSize(size Size) Size {
	if size.W < 0 {
		size.W = 0
	}
	if size.H < 0 {
		size.H = 0
	}
	return size
}

func scaleMetric(value, size, base int) int {
	if value <= 0 || size <= 0 || base <= 0 {
		return max0(value)
	}
	return int(math.Max(0, math.Round(float64(value*size)/float64(base))))
}

func max0(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
