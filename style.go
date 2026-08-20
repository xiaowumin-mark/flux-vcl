package flux

import "fmt"

// Insets describes the four sides of a control's content inset in DIP.
//
// Insets is deliberately a plain value.  A zero value means no inset; a
// negative side is representable for compatibility with direct struct
// literals, but is rejected by Validate when a layout boundary is checked.
type Insets struct {
	Left, Top, Right, Bottom int
}

// NewInsets creates an Insets value in left/top/right/bottom order.
func NewInsets(left, top, right, bottom int) Insets {
	return Insets{Left: left, Top: top, Right: right, Bottom: bottom}
}

// InsetsLTRB is an explicitly named alias for NewInsets.  It is useful at
// call sites where the side ordering should be self-documenting.
func InsetsLTRB(left, top, right, bottom int) Insets {
	return NewInsets(left, top, right, bottom)
}

// InsetsAll creates equal insets on every side.
func InsetsAll(value int) Insets {
	return Insets{Left: value, Top: value, Right: value, Bottom: value}
}

// InsetsUniform is an alias for InsetsAll.
func InsetsUniform(value int) Insets { return InsetsAll(value) }

// InsetsSymmetric creates horizontal and vertical insets.  The first
// argument is applied to left/right, the second to top/bottom.
func InsetsSymmetric(horizontal, vertical int) Insets {
	return Insets{Left: horizontal, Top: vertical, Right: horizontal, Bottom: vertical}
}

// InsetsXY is an alias for InsetsSymmetric.
func InsetsXY(horizontal, vertical int) Insets { return InsetsSymmetric(horizontal, vertical) }

// InsetsHorizontal creates equal left/right insets.
func InsetsHorizontal(value int) Insets {
	return Insets{Left: value, Right: value}
}

// InsetsVertical creates equal top/bottom insets.
func InsetsVertical(value int) Insets {
	return Insets{Top: value, Bottom: value}
}

// Horizontal returns the total horizontal inset.
func (i Insets) Horizontal() int { return i.Left + i.Right }

// Vertical returns the total vertical inset.
func (i Insets) Vertical() int { return i.Top + i.Bottom }

// IsZero reports whether all four sides are zero.
func (i Insets) IsZero() bool { return i == Insets{} }

// Add adds each side of two Insets values.
func (i Insets) Add(other Insets) Insets {
	return Insets{
		Left: i.Left + other.Left, Top: i.Top + other.Top,
		Right: i.Right + other.Right, Bottom: i.Bottom + other.Bottom,
	}
}

// Deflate removes the insets from a rectangle.  Width and height are clamped
// to zero if the inset is larger than the rectangle.
func (i Insets) Deflate(rect Rect) Rect {
	result := Rect{
		X: rect.X + i.Left,
		Y: rect.Y + i.Top,
		W: rect.W - i.Horizontal(),
		H: rect.H - i.Vertical(),
	}
	if result.W < 0 {
		result.W = 0
	}
	if result.H < 0 {
		result.H = 0
	}
	return result
}

// Inflate adds the insets around a rectangle.
func (i Insets) Inflate(rect Rect) Rect {
	return Rect{
		X: rect.X - i.Left,
		Y: rect.Y - i.Top,
		W: rect.W + i.Horizontal(),
		H: rect.H + i.Vertical(),
	}
}

// Validate reports whether every side is a non-negative DIP value.
func (i Insets) Validate() error {
	if i.Left < 0 || i.Top < 0 || i.Right < 0 || i.Bottom < 0 {
		return fmt.Errorf("flux: insets must be non-negative: %+v", i)
	}
	return nil
}

// BorderSpec describes a control border in DIP.  Width zero means no border.
// ColorValue zero retains the style resolver's default/no-paint semantics.
type BorderSpec struct {
	Color ColorValue
	Width int
}

// NewBorderSpec creates a border value from color and DIP width.
func NewBorderSpec(color ColorValue, width int) BorderSpec {
	return BorderSpec{Color: color, Width: width}
}

// Border is a concise alias for NewBorderSpec.
func Border(color ColorValue, width int) BorderSpec { return NewBorderSpec(color, width) }

// NoBorder returns the zero border value.
func NoBorder() BorderSpec { return BorderSpec{} }

// IsZero reports whether the border has no configured color or width.
func (b BorderSpec) IsZero() bool { return b == BorderSpec{} }

// IsVisible reports whether the border has a positive width and a paint
// color.  A zero color remains a valid unresolved/default value, but is not
// considered visible by this convenience predicate.
func (b BorderSpec) IsVisible() bool { return b.Width > 0 && b.Color != 0 }

// Validate reports whether the border width is a valid non-negative DIP value.
func (b BorderSpec) Validate() error {
	if b.Width < 0 {
		return fmt.Errorf("flux: border width must be non-negative: %d", b.Width)
	}
	return nil
}

// FocusStyle describes the focus ring border and its distance from the
// control bounds.  Inset is measured in DIP and must be non-negative.
type FocusStyle struct {
	Border BorderSpec
	Inset  int
}

// NewFocusStyle creates a focus style value.
func NewFocusStyle(border BorderSpec, inset int) FocusStyle {
	return FocusStyle{Border: border, Inset: inset}
}

// Validate reports whether the focus border and inset are valid.
func (f FocusStyle) Validate() error {
	if err := f.Border.Validate(); err != nil {
		return err
	}
	if f.Inset < 0 {
		return fmt.Errorf("flux: focus inset must be non-negative: %d", f.Inset)
	}
	return nil
}

// StyleFieldMask identifies fields present in a ControlStylePatch.  Bits are
// assigned in the same order as ControlStyle's fields and are stable once
// published.  Unknown bits are retained by value but ignored by Apply.
type StyleFieldMask uint64

const (
	StyleFieldBackground StyleFieldMask = 1 << iota
	StyleFieldForeground
	StyleFieldFont
	StyleFieldBorder
	StyleFieldRadius
	StyleFieldPadding
	StyleFieldMinSize

	// StyleFieldAll is the set of fields defined by ControlStyle.
	StyleFieldAll = StyleFieldBackground | StyleFieldForeground | StyleFieldFont |
		StyleFieldBorder | StyleFieldRadius | StyleFieldPadding | StyleFieldMinSize
	// StyleFieldNone is an empty presence mask.
	StyleFieldNone StyleFieldMask = 0
)

// Compatibility aliases make the field-oriented names convenient without
// requiring callers to hand-write bit shifts.
const (
	ControlStyleFieldBackground = StyleFieldBackground
	ControlStyleFieldForeground = StyleFieldForeground
	ControlStyleFieldFont       = StyleFieldFont
	ControlStyleFieldBorder     = StyleFieldBorder
	ControlStyleFieldRadius     = StyleFieldRadius
	ControlStyleFieldPadding    = StyleFieldPadding
	ControlStyleFieldMinSize    = StyleFieldMinSize
	ControlStyleFieldAll        = StyleFieldAll
)

// More explicit aliases are provided for callers that name the value type in
// their resolver code.  They intentionally share the same stable bits.
const (
	StyleFieldBackgroundColor = StyleFieldBackground
	StyleFieldForegroundColor = StyleFieldForeground
	StyleFieldFontSpec        = StyleFieldFont
	StyleFieldBorderSpec      = StyleFieldBorder
	StyleFieldMinSizeValue    = StyleFieldMinSize
)

// Short field aliases are useful when composing masks in internal resolver
// tables.  They are aliases, rather than new bits.
const (
	StyleBackgroundField = StyleFieldBackground
	StyleForegroundField = StyleFieldForeground
	StyleFontField       = StyleFieldFont
	StyleBorderField     = StyleFieldBorder
	StyleRadiusField     = StyleFieldRadius
	StylePaddingField    = StyleFieldPadding
	StyleMinSizeField    = StyleFieldMinSize
	BackgroundField      = StyleFieldBackground
	ForegroundField      = StyleFieldForeground
	FontField            = StyleFieldFont
	BorderField          = StyleFieldBorder
	RadiusField          = StyleFieldRadius
	PaddingField         = StyleFieldPadding
	MinSizeField         = StyleFieldMinSize
)

// Has reports whether all bits in field are present.  As with conventional
// bit masks, Has(0) is true.
func (m StyleFieldMask) Has(field StyleFieldMask) bool { return m&field == field }

// Contains is an alias for Has.
func (m StyleFieldMask) Contains(field StyleFieldMask) bool { return m.Has(field) }

// Any reports whether at least one of the requested bits is present.
func (m StyleFieldMask) Any(field StyleFieldMask) bool { return m&field != 0 }

// Known removes bits not defined by ControlStyle.
func (m StyleFieldMask) Known() StyleFieldMask { return m & StyleFieldAll }

// Unknown returns bits not defined by ControlStyle.
func (m StyleFieldMask) Unknown() StyleFieldMask { return m &^ StyleFieldAll }

// With adds fields to a mask.
func (m StyleFieldMask) With(field StyleFieldMask) StyleFieldMask { return m | field }

// Without removes fields from a mask.
func (m StyleFieldMask) Without(field StyleFieldMask) StyleFieldMask { return m &^ field }

// ControlStyle is a fully resolved, pure-value control appearance.  Zero
// values are meaningful: zero padding/radius/min-size explicitly request no
// value after resolution, while zero colors retain the resolver's default
// color semantics.
type ControlStyle struct {
	Background ColorValue
	Foreground ColorValue
	Font       FontSpec
	Border     BorderSpec
	Radius     int
	Padding    Insets
	MinSize    Size
}

// NewControlStyle resolves options over a zero style.  Use ResolveStyle when
// applying a partial override to an existing theme value.
func NewControlStyle(options ...StyleOption) ControlStyle {
	return ControlStylePatchFrom(options...).Apply(ControlStyle{})
}

// ResolveStyle applies options in declaration order, with later options
// replacing earlier values for the same field.
func ResolveStyle(base ControlStyle, options ...StyleOption) ControlStyle {
	return ControlStylePatchFrom(options...).Apply(base)
}

// Apply resolves options over the receiver and returns a new value.
func (s ControlStyle) Apply(options ...StyleOption) ControlStyle {
	return ResolveStyle(s, options...)
}

// With is an alias for Apply.
func (s ControlStyle) With(options ...StyleOption) ControlStyle { return s.Apply(options...) }

// Patch returns a complete patch that sets every ControlStyle field to the
// receiver's value.  It is useful when crossing resolver/layout boundaries.
func (s ControlStyle) Patch() ControlStylePatch {
	return ControlStylePatch{
		Set:        StyleFieldAll,
		Background: s.Background,
		Foreground: s.Foreground,
		Font:       s.Font,
		Border:     s.Border,
		Radius:     s.Radius,
		Padding:    s.Padding,
		MinSize:    s.MinSize,
	}
}

// Validate checks all dimensions and the CD0 FontSpec contract for a complete
// style. Color alpha remains a renderer concern and is not duplicated here.
func (s ControlStyle) Validate() error {
	if err := s.Font.Validate(); err != nil {
		return err
	}
	if err := s.Padding.Validate(); err != nil {
		return err
	}
	if err := s.Border.Validate(); err != nil {
		return err
	}
	if s.Radius < 0 {
		return fmt.Errorf("flux: style radius must be non-negative: %d", s.Radius)
	}
	if s.MinSize.W < 0 || s.MinSize.H < 0 {
		return fmt.Errorf("flux: style min-size must be non-negative: %+v", s.MinSize)
	}
	return nil
}

// StyleOption sets one field in a ControlStylePatch.  Options mutate only the
// temporary patch supplied by the resolver; the public style values remain
// structs with no map, slice, or pointer storage.
type StyleOption func(*ControlStylePatch)

// ControlStylePatch carries a partial style and an explicit presence mask.
// A set field is applied even when its value is the type's zero value.
type ControlStylePatch struct {
	Set        StyleFieldMask
	Background ColorValue
	Foreground ColorValue
	Font       FontSpec
	Border     BorderSpec
	Radius     int
	Padding    Insets
	MinSize    Size
}

// ControlStylePatchFrom builds a patch by applying options in order.
func ControlStylePatchFrom(options ...StyleOption) ControlStylePatch {
	var patch ControlStylePatch
	patch.ApplyOptions(options...)
	if err := patch.Validate(); err != nil {
		panic(err)
	}
	return patch
}

// NewControlStylePatch is the constructor form of ControlStylePatchFrom.
func NewControlStylePatch(options ...StyleOption) ControlStylePatch {
	return ControlStylePatchFrom(options...)
}

// ApplyOptions applies options in order.  A nil option is ignored so callers
// can conditionally append an option without branching at the call site.
func (p *ControlStylePatch) ApplyOptions(options ...StyleOption) {
	if p == nil {
		return
	}
	for _, option := range options {
		if option != nil {
			option(p)
		}
	}
}

// With returns a copy with options applied.
func (p ControlStylePatch) With(options ...StyleOption) ControlStylePatch {
	p.ApplyOptions(options...)
	if err := p.Validate(); err != nil {
		panic(err)
	}
	return p
}

// Has reports whether all requested fields are present in the patch.
func (p ControlStylePatch) Has(field StyleFieldMask) bool { return p.Set.Has(field) }

// IsSet reports whether a single non-zero field bit is present.
func (p ControlStylePatch) IsSet(field StyleFieldMask) bool {
	return field != StyleFieldNone && p.Set.Has(field)
}

// Empty reports whether no field is present.
func (p ControlStylePatch) Empty() bool { return p.Set == StyleFieldNone }

// Mask returns the patch's presence mask.
func (p ControlStylePatch) Mask() StyleFieldMask { return p.Set }

// Apply overlays all known fields in p over base and returns a new complete
// style.  Unknown mask bits are ignored, allowing forward-compatible values
// to pass through older resolvers without changing known fields.
func (p ControlStylePatch) Apply(base ControlStyle) ControlStyle {
	if p.Set.Has(StyleFieldBackground) {
		base.Background = p.Background
	}
	if p.Set.Has(StyleFieldForeground) {
		base.Foreground = p.Foreground
	}
	if p.Set.Has(StyleFieldFont) {
		base.Font = p.Font
	}
	if p.Set.Has(StyleFieldBorder) {
		base.Border = p.Border
	}
	if p.Set.Has(StyleFieldRadius) {
		base.Radius = p.Radius
	}
	if p.Set.Has(StyleFieldPadding) {
		base.Padding = p.Padding
	}
	if p.Set.Has(StyleFieldMinSize) {
		base.MinSize = p.MinSize
	}
	return base
}

// Resolve is an alias for Apply.
func (p ControlStylePatch) Resolve(base ControlStyle) ControlStyle { return p.Apply(base) }

// ApplyTo and Merge are compatibility spellings for Apply.
func (p ControlStylePatch) ApplyTo(base ControlStyle) ControlStyle { return p.Apply(base) }
func (p ControlStylePatch) Merge(base ControlStyle) ControlStyle   { return p.Apply(base) }

// Validate checks the mask and every field that is present.  Unset fields are
// deliberately not validated because their values are ignored by Apply.
func (p ControlStylePatch) Validate() error {
	if unknown := p.Set.Unknown(); unknown != 0 {
		return fmt.Errorf("flux: unknown style field mask 0x%x", uint64(unknown))
	}
	if p.IsSet(StyleFieldPadding) {
		if err := p.Padding.Validate(); err != nil {
			return err
		}
	}
	if p.IsSet(StyleFieldFont) {
		if err := p.Font.Validate(); err != nil {
			return err
		}
	}
	if p.IsSet(StyleFieldBorder) {
		if err := p.Border.Validate(); err != nil {
			return err
		}
	}
	if p.IsSet(StyleFieldRadius) && p.Radius < 0 {
		return fmt.Errorf("flux: style radius must be non-negative: %d", p.Radius)
	}
	if p.IsSet(StyleFieldMinSize) && (p.MinSize.W < 0 || p.MinSize.H < 0) {
		return fmt.Errorf("flux: style min-size must be non-negative: %+v", p.MinSize)
	}
	return nil
}

// IsEmpty is an alias for Empty.
func (p ControlStylePatch) IsEmpty() bool { return p.Empty() }

// With* methods provide an immutable, value-oriented alternative to applying
// a StyleOption slice directly.
func (p ControlStylePatch) WithBackground(value ColorValue) ControlStylePatch {
	return p.With(SetBackground(value))
}

func (p ControlStylePatch) WithForeground(value ColorValue) ControlStylePatch {
	return p.With(SetForeground(value))
}

func (p ControlStylePatch) WithFont(value FontSpec) ControlStylePatch {
	return p.With(SetFont(value))
}

func (p ControlStylePatch) WithBorder(value BorderSpec) ControlStylePatch {
	return p.With(SetBorder(value))
}

func (p ControlStylePatch) WithRadius(value int) ControlStylePatch {
	return p.With(SetRadius(value))
}

func (p ControlStylePatch) WithPadding(value Insets) ControlStylePatch {
	return p.With(SetPadding(value))
}

func (p ControlStylePatch) WithMinSize(value Size) ControlStylePatch {
	return p.With(SetMinSize(value))
}

// The Set* constructors are the canonical presence-safe style options.
func SetBackground(value ColorValue) StyleOption {
	return func(p *ControlStylePatch) { p.Set |= StyleFieldBackground; p.Background = value }
}

func SetForeground(value ColorValue) StyleOption {
	return func(p *ControlStylePatch) { p.Set |= StyleFieldForeground; p.Foreground = value }
}

func SetFont(value FontSpec) StyleOption {
	if err := value.Validate(); err != nil {
		panic(err)
	}
	return func(p *ControlStylePatch) { p.Set |= StyleFieldFont; p.Font = value }
}

func SetBorder(value BorderSpec) StyleOption {
	return func(p *ControlStylePatch) { p.Set |= StyleFieldBorder; p.Border = value }
}

func SetRadius(value int) StyleOption {
	return func(p *ControlStylePatch) { p.Set |= StyleFieldRadius; p.Radius = value }
}

func SetPadding(value Insets) StyleOption {
	return func(p *ControlStylePatch) { p.Set |= StyleFieldPadding; p.Padding = value }
}

func SetMinSize(value Size) StyleOption {
	return func(p *ControlStylePatch) { p.Set |= StyleFieldMinSize; p.MinSize = value }
}

// With* and Style* aliases read naturally in both resolver and theme code.
func WithBackground(value ColorValue) StyleOption { return SetBackground(value) }
func WithForeground(value ColorValue) StyleOption { return SetForeground(value) }
func WithFont(value FontSpec) StyleOption         { return SetFont(value) }
func WithBorder(value BorderSpec) StyleOption     { return SetBorder(value) }
func WithRadius(value int) StyleOption            { return SetRadius(value) }
func WithPadding(value Insets) StyleOption        { return SetPadding(value) }
func WithMinSize(value Size) StyleOption          { return SetMinSize(value) }

func StyleBackground(value ColorValue) StyleOption { return SetBackground(value) }
func StyleForeground(value ColorValue) StyleOption { return SetForeground(value) }
func StyleFont(value FontSpec) StyleOption         { return SetFont(value) }
func StyleBorder(value BorderSpec) StyleOption     { return SetBorder(value) }
func StyleRadius(value int) StyleOption            { return SetRadius(value) }
func StylePadding(value Insets) StyleOption        { return SetPadding(value) }
func StyleMinSize(value Size) StyleOption          { return SetMinSize(value) }

// Descriptive aliases for integrations that prefer the noun-first spelling.
func BackgroundStyle(value ColorValue) StyleOption { return SetBackground(value) }
func ForegroundStyle(value ColorValue) StyleOption { return SetForeground(value) }
func FontStyle(value FontSpec) StyleOption         { return SetFont(value) }
func BorderStyle(value BorderSpec) StyleOption     { return SetBorder(value) }
func RadiusStyle(value int) StyleOption            { return SetRadius(value) }
func PaddingStyle(value Insets) StyleOption        { return SetPadding(value) }
func MinSizeStyle(value Size) StyleOption          { return SetMinSize(value) }
