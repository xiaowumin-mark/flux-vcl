package flux

import (
	"strings"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

const fontOverrideProp = "_FontOverride"

type fontOverrideMask uint8

const (
	fontOverrideFamily fontOverrideMask = 1 << iota
	fontOverrideSize
	fontOverrideWeight
	fontOverrideItalic
	fontOverrideUnderline
	fontOverrideStrikeout
	fontOverrideAll = fontOverrideFamily | fontOverrideSize | fontOverrideWeight |
		fontOverrideItalic | fontOverrideUnderline | fontOverrideStrikeout
)

// fontOverride keeps atomic Font* options separate from the resolved FontSpec.
// This preserves style-layer fields that an atomic option did not override.
type fontOverride struct {
	Set  fontOverrideMask
	Font FontSpec
}

// defaultControlStyle is the compatibility baseline used until CD3's theme
// resolver supplies a complete style.  Keeping the old intrinsic metrics in
// this value preserves existing apps while making every metric explicit and
// overridable through ControlStyle/ControlStylePatch.
func defaultControlStyle(kind string) ControlStyle {
	style := ControlStyle{Font: FontSpec{Weight: FontWeightNormal}}
	switch kind {
	case "Button":
		style.Padding = Insets{Left: 16, Top: 6, Right: 16, Bottom: 6}
		style.MinSize = Size{W: 88, H: 32}
	case "Input":
		style.Padding = Insets{Left: 8, Top: 4, Right: 8, Bottom: 4}
		style.MinSize = Size{W: 180, H: 28}
	case "Text":
		// Text labels have no implicit padding or minimum size.
	case "CheckBox", "RadioButton":
		style.Padding = Insets{Left: 4, Top: 2, Right: 4, Bottom: 2}
	}
	return style
}

// effectiveControlStyle resolves a node's complete style before intrinsic
// measurement. It applies the style layer first, then explicit atomic options
// (Color/FontColor/Font/Padding/MinSize), matching the documented precedence.
func effectiveControlStyle(n *Node) ControlStyle {
	style := defaultControlStyle(n.Type)
	if raw, ok := n.Props.Get("Style"); ok {
		if value, ok := raw.(ControlStyle); ok {
			style = value
		}
	}
	if raw, ok := n.Props.Get("StylePatch"); ok {
		if patch, ok := raw.(ControlStylePatch); ok {
			style = patch.Apply(style)
		}
	}
	if raw, ok := n.Props.Get("Background"); ok {
		if value, ok := raw.(ColorValue); ok {
			style.Background = value
		}
	}
	if raw, ok := n.Props.Get("Color"); ok {
		if value, ok := raw.(ColorValue); ok {
			style.Background = value
		}
	}
	if raw, ok := n.Props.Get("Foreground"); ok {
		if value, ok := raw.(ColorValue); ok {
			style.Foreground = value
		}
	}
	if raw, ok := n.Props.Get("FontColor"); ok {
		if value, ok := raw.(ColorValue); ok {
			style.Foreground = value
		}
	}
	if raw, ok := n.Props.Get("Font"); ok {
		if value, ok := raw.(FontSpec); ok {
			style.Font = render.NormalizeFontSpec(value)
		}
	}
	if raw, ok := n.Props.Get(fontOverrideProp); ok {
		if override, ok := raw.(fontOverride); ok {
			style.Font = applyFontOverride(style.Font, override)
		}
	}
	if raw, ok := n.Props.Get("Border"); ok {
		if value, ok := raw.(BorderSpec); ok {
			style.Border = value
		}
	}
	if raw, ok := n.Props.Get("Padding"); ok {
		if value, ok := raw.(Insets); ok {
			style.Padding = value
		}
	}
	if raw, ok := n.Props.Get("MinSize"); ok {
		if value, ok := raw.(Size); ok {
			style.MinSize = value
		}
	}
	return style
}

// materializeCD2Fonts bridges the CD2 style values to the existing property
// diff until CD3 installs the full theme resolver pass. Atomic Font options
// keep their higher precedence; otherwise a complete style or a font-bearing
// patch is resolved once and written through the same Font property used by
// both intrinsic measurement and the native FontController.
func materializeCD2Fonts(n *Node) {
	if n == nil || n.Props == nil {
		return
	}
	_, hasFont := n.Props.Get("Font")
	if nodeDeclaresStyleFont(n) || nodeDeclaresFontOverride(n) {
		hasFont = true
	}
	if hasFont {
		n.Props.Set("Font", render.NormalizeFontSpec(effectiveControlStyle(n).Font))
	}
	for _, child := range n.Children {
		materializeCD2Fonts(child)
	}
}

func nodeDeclaresStyleFont(n *Node) bool {
	if raw, ok := n.Props.Get("Style"); ok {
		if _, valid := raw.(ControlStyle); valid {
			return true
		}
	}
	if raw, ok := n.Props.Get("StylePatch"); ok {
		if patch, valid := raw.(ControlStylePatch); valid {
			return patch.IsSet(StyleFieldFont)
		}
	}
	return false
}

func nodeDeclaresFontOverride(n *Node) bool {
	if raw, ok := n.Props.Get(fontOverrideProp); ok {
		if override, valid := raw.(fontOverride); valid {
			return override.Set != 0
		}
	}
	return false
}

func applyFontOverride(base FontSpec, override fontOverride) FontSpec {
	if override.Set&fontOverrideFamily != 0 {
		base.Family = override.Font.Family
	}
	if override.Set&fontOverrideSize != 0 {
		base.Size = override.Font.Size
	}
	if override.Set&fontOverrideWeight != 0 {
		base.Weight = override.Font.Weight
	}
	if override.Set&fontOverrideItalic != 0 {
		base.Italic = override.Font.Italic
	}
	if override.Set&fontOverrideUnderline != 0 {
		base.Underline = override.Font.Underline
	}
	if override.Set&fontOverrideStrikeout != 0 {
		base.Strikeout = override.Font.Strikeout
	}
	return render.NormalizeFontSpec(base)
}

func currentFontOverride(n *Node) fontOverride {
	if raw, ok := n.Props.Get(fontOverrideProp); ok {
		if override, ok := raw.(fontOverride); ok {
			return override
		}
	}
	return fontOverride{}
}

func setFontOverride(n *Node, override fontOverride) {
	n.Props.Set(fontOverrideProp, override)
}

func setFullFontOverride(n *Node, font FontSpec) {
	setFontOverride(n, fontOverride{Set: fontOverrideAll, Font: font})
}

func setFontField(n *Node, bit fontOverrideMask, update func(*FontSpec)) {
	override := currentFontOverride(n)
	override.Set |= bit
	update(&override.Font)
	setFontOverride(n, override)
}

func nodeGap(n *Node) int {
	if n != nil {
		if raw, ok := n.Props.Get("Gap"); ok {
			if value, ok := raw.(int); ok && value >= 0 {
				return value
			}
		}
	}
	return layoutGap
}

// measureStyledText is the one layout entry point for text metrics.  It sends
// the resolved FontSpec to optional renderers and uses TextExtent otherwise.
func measureStyledText(r render.Renderer, text string, font FontSpec, wrap render.TextWrap, c BoxConstraints) Size {
	request := render.TextMeasureRequest{
		Text: text,
		Font: render.NormalizeFontSpec(font),
		DPI:  rendererDPI(r),
		Wrap: wrap,
		Constraints: render.TextMeasureConstraints{
			MinW: c.MinW, MaxW: c.MaxW, MinH: c.MinH, MaxH: c.MaxH,
		},
		HasConstraints: true,
	}
	measured := render.MeasureText(r, request)
	return Size{W: measured.W, H: measured.H}
}

func rendererDPI(r render.Renderer) int {
	if provider, ok := r.(interface{ DPI() int }); ok {
		if dpi := provider.DPI(); dpi > 0 {
			return dpi
		}
	}
	return 96
}

func styledIntrinsic(r render.Renderer, n *Node, c BoxConstraints, baseW, baseH int, wrap render.TextWrap) Size {
	style := effectiveControlStyle(n)
	text := n.Props.String("Text")
	if text != "" || n.Type == "Text" || n.Type == "Button" || n.Type == "Input" {
		// Preserve the existing explicit-newline semantics while routing every
		// line through the same styled measurement request.
		normalized := strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
		lines := strings.Split(normalized, "\n")
		baseW, baseH = 0, 0
		for _, line := range lines {
			measured := measureStyledText(r, line, style.Font, wrap, Unbounded())
			if measured.W > baseW {
				baseW = measured.W
			}
			baseH += measured.H
		}
	}
	baseW += style.Padding.Horizontal()
	baseH += style.Padding.Vertical()
	if baseW < style.MinSize.W {
		baseW = style.MinSize.W
	}
	if baseH < style.MinSize.H {
		baseH = style.MinSize.H
	}
	return leafSize(baseW, baseH, n, c)
}

// Style applies a complete style or a presence-aware patch to a widget.  The
// variadic form keeps the public constructor ergonomic while accepting either
// ControlStyle, ControlStylePatch, or one or more StyleOption values.
func Style(values ...any) Opt {
	// Validate concrete style values at constructor time so an invalid public
	// FontSpec or geometry cannot remain latent until a widget is built.
	for _, value := range values {
		switch v := value.(type) {
		case ControlStyle:
			if err := v.Validate(); err != nil {
				panic(err)
			}
		case ControlStylePatch:
			if err := v.Validate(); err != nil {
				panic(err)
			}
		case StyleOption:
			// The option is applied below, where the merged patch is validated.
		case nil:
			// Preserve the existing no-op behavior for a conditional nil value.
		default:
			panic("flux: Style expects ControlStyle, ControlStylePatch, or StyleOption")
		}
	}
	return optFn(func(n *Node) {
		if len(values) == 0 {
			return
		}
		patch := ControlStylePatch{}
		if existing, ok := n.Props.Get("StylePatch"); ok {
			patch, _ = existing.(ControlStylePatch)
		}
		for _, value := range values {
			switch v := value.(type) {
			case nil:
				continue
			case ControlStyle:
				if err := v.Validate(); err != nil {
					panic(err)
				}
				patch = v.Patch()
			case ControlStylePatch:
				if err := v.Validate(); err != nil {
					panic(err)
				}
				patch = mergeStylePatches(patch, v)
			case StyleOption:
				patch.ApplyOptions(v)
			default:
				panic("flux: Style expects ControlStyle, ControlStylePatch, or StyleOption")
			}
		}
		if err := patch.Validate(); err != nil {
			panic(err)
		}
		n.Props.Set("StylePatch", patch)
	})
}

func mergeStylePatches(base, overlay ControlStylePatch) ControlStylePatch {
	if overlay.Set.Has(StyleFieldBackground) {
		base.Background = overlay.Background
	}
	if overlay.Set.Has(StyleFieldForeground) {
		base.Foreground = overlay.Foreground
	}
	if overlay.Set.Has(StyleFieldFont) {
		base.Font = overlay.Font
	}
	if overlay.Set.Has(StyleFieldBorder) {
		base.Border = overlay.Border
	}
	if overlay.Set.Has(StyleFieldRadius) {
		base.Radius = overlay.Radius
	}
	if overlay.Set.Has(StyleFieldPadding) {
		base.Padding = overlay.Padding
	}
	if overlay.Set.Has(StyleFieldMinSize) {
		base.MinSize = overlay.MinSize
	}
	base.Set |= overlay.Set
	return base
}

// FontSize sets only the effective font size (DIP), preserving other font
// attributes.  It is intentionally an Opt rather than a theme field.
func FontSize(value int) Opt {
	if value < 0 {
		panic("flux: font size must be non-negative")
	}
	return optFn(func(n *Node) {
		setFontField(n, fontOverrideSize, func(font *FontSpec) { font.Size = value })
	})
}

// FontFamily sets the effective font family; an empty family requests the
// system UI fallback.
func FontFamily(value string) Opt {
	if err := (FontSpec{Family: value}).Validate(); err != nil {
		panic(err)
	}
	return optFn(func(n *Node) {
		setFontField(n, fontOverrideFamily, func(font *FontSpec) { font.Family = value })
	})
}

// FontWeightOpt sets the effective font weight without colliding with the
// FontWeight value type.
func FontWeightOpt(value FontWeight) Opt {
	if err := (FontSpec{Weight: value}).Validate(); err != nil {
		panic(err)
	}
	return optFn(func(n *Node) {
		setFontField(n, fontOverrideWeight, func(font *FontSpec) { font.Weight = value })
	})
}
