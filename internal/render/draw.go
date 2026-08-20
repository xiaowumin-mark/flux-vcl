package render

import (
	"errors"
	"fmt"
	"unicode/utf8"
)

// MessageID is a stable, language-independent diagnostic key shared with the
// public package. It lives in render so the headless renderer does not import
// the root package.
type MessageID string

// ErrInvalidDrawList identifies every DrawList validation failure.
var ErrInvalidDrawList = errors.New("invalid draw list")

const (
	drawMaxOps        = 4096
	drawMaxTextBytes  = 65536
	drawMaxClipDepth  = 32
	drawMaxCoordinate = 1 << 20
	drawMaxFamilyByte = 255
)

const (
	drawIDTooManyOps       MessageID = "flux.draw.too_many_ops"
	drawIDTextTooLong      MessageID = "flux.draw.text_too_long"
	drawIDClipUnderflow    MessageID = "flux.draw.clip_underflow"
	drawIDClipUnbalanced   MessageID = "flux.draw.clip_unbalanced"
	drawIDCoordinateRange  MessageID = "flux.draw.coordinate_range"
	drawIDNegativeSize     MessageID = "flux.draw.negative_size"
	drawIDRadiusNegative   MessageID = "flux.draw.radius_negative"
	drawIDStrokeWidth      MessageID = "flux.draw.stroke_width"
	drawIDColorRequired    MessageID = "flux.draw.color_required"
	drawIDAlphaUnsupported MessageID = "flux.draw.alpha_unsupported"
	drawIDFontSize         MessageID = "flux.draw.font_size"
	drawIDEnumUnknown      MessageID = "flux.draw.enum_unknown"
)

// DrawValidationError carries structured DrawList validation details.
type DrawValidationError struct {
	ID      MessageID
	OpIndex int
	Field   string
}

func (e *DrawValidationError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.OpIndex < 0 {
		return string(e.ID)
	}
	return fmt.Sprintf("%s: op %d field %s", e.ID, e.OpIndex, e.Field)
}

func (e *DrawValidationError) Unwrap() error { return ErrInvalidDrawList }

// DrawOp is the renderer-owned, sealed value operation interface.
type DrawOp interface{ drawOp() }

// FillStyle describes an opaque fill color.
type FillStyle struct{ Color Color }

// StrokeKind identifies a stroke pattern.
type StrokeKind uint8

// StrokeSolid is the only stroke pattern supported by CD1.
const StrokeSolid StrokeKind = iota

// StrokeStyle describes an opaque DIP-width stroke.
type StrokeStyle struct {
	Color Color
	Width int
	Style StrokeKind
}

// FontWeight is a CSS/Win32-compatible font weight.
type FontWeight uint16

const (
	// FontWeightNormal is the regular weight.
	FontWeightNormal FontWeight = 400
	// FontWeightMedium is the medium weight.
	FontWeightMedium FontWeight = 500
	// FontWeightSemibold is the semibold weight.
	FontWeightSemibold FontWeight = 600
	// FontWeightBold is the bold weight.
	FontWeightBold FontWeight = 700
)

// FontSpec describes a system or explicit UI font in DIP units.
type FontSpec struct {
	Family    string
	Size      int
	Weight    FontWeight
	Italic    bool
	Underline bool
	Strikeout bool
}

// Validate checks the public CD0 FontSpec contract without changing the
// receiver. Zero weight is the documented normal-weight sentinel; negative
// sizes, invalid UTF-8/oversized family names, and unknown weights are errors.
func (font FontSpec) Validate() error {
	_, err := canonicalizeFont(font)
	return err
}

// TextAlignment controls horizontal or vertical alignment.
type TextAlignment uint8

const (
	// TextAlignStart aligns to the leading edge.
	TextAlignStart TextAlignment = iota
	// TextAlignCenter centers content.
	TextAlignCenter
	// TextAlignEnd aligns to the trailing edge.
	TextAlignEnd
)

// TextWrap controls line wrapping.
type TextWrap uint8

const (
	// TextNoWrap keeps text on one line.
	TextNoWrap TextWrap = iota
	// TextWrapWord wraps at word boundaries.
	TextWrapWord
)

// TextOverflow controls clipped or ellipsized overflow.
type TextOverflow uint8

const (
	// TextOverflowClip clips at the text rectangle.
	TextOverflowClip TextOverflow = iota
	// TextOverflowEllipsis uses an ellipsis for overflow.
	TextOverflowEllipsis
)

// TextPaint combines font, color, alignment, wrapping, overflow, and mnemonic policy.
type TextPaint struct {
	Font       FontSpec
	Color      Color
	Horizontal TextAlignment
	Vertical   TextAlignment
	Wrap       TextWrap
	Overflow   TextOverflow
	Mnemonic   bool
}

type clearOp struct{ Color Color }

func (clearOp) drawOp() {}

type fillRectOp struct {
	Rect Rect
	Fill FillStyle
}

func (fillRectOp) drawOp() {}

type strokeRectOp struct {
	Rect   Rect
	Stroke StrokeStyle
}

func (strokeRectOp) drawOp() {}

type fillRoundRectOp struct {
	Rect   Rect
	Radius int
	Fill   FillStyle
}

func (fillRoundRectOp) drawOp() {}

type strokeRoundRectOp struct {
	Rect   Rect
	Radius int
	Stroke StrokeStyle
}

func (strokeRoundRectOp) drawOp() {}

type lineOp struct {
	From, To Point
	Stroke   StrokeStyle
}

func (lineOp) drawOp() {}

type fillEllipseOp struct {
	Rect Rect
	Fill FillStyle
}

func (fillEllipseOp) drawOp() {}

type strokeEllipseOp struct {
	Rect   Rect
	Stroke StrokeStyle
}

func (strokeEllipseOp) drawOp() {}

type textOp struct {
	Text  string
	Rect  Rect
	Paint TextPaint
}

func (textOp) drawOp() {}

type pushClipOp struct{ Rect Rect }

func (pushClipOp) drawOp() {}

type popClipOp struct{}

func (popClipOp) drawOp() {}

// DrawList is a validated immutable sequence of drawing operations. Its zero
// value is a valid empty list.
type DrawList struct{ ops []DrawOp }

// Clear creates an operation that fills the complete surface with color.
func Clear(color Color) DrawOp { return clearOp{Color: color} }

// FillRect creates a rectangle fill operation.
func FillRect(rect Rect, fill FillStyle) DrawOp { return fillRectOp{Rect: rect, Fill: fill} }

// StrokeRect creates a rectangle stroke operation.
func StrokeRect(rect Rect, stroke StrokeStyle) DrawOp {
	return strokeRectOp{Rect: rect, Stroke: stroke}
}

// FillRoundRect creates a rounded rectangle fill operation.
func FillRoundRect(rect Rect, radius int, fill FillStyle) DrawOp {
	return fillRoundRectOp{Rect: rect, Radius: radius, Fill: fill}
}

// StrokeRoundRect creates a rounded rectangle stroke operation.
func StrokeRoundRect(rect Rect, radius int, stroke StrokeStyle) DrawOp {
	return strokeRoundRectOp{Rect: rect, Radius: radius, Stroke: stroke}
}

// DrawLine creates a line operation.
func DrawLine(from, to Point, stroke StrokeStyle) DrawOp {
	return lineOp{From: from, To: to, Stroke: stroke}
}

// FillEllipse creates an ellipse fill operation.
func FillEllipse(rect Rect, fill FillStyle) DrawOp { return fillEllipseOp{Rect: rect, Fill: fill} }

// StrokeEllipse creates an ellipse stroke operation.
func StrokeEllipse(rect Rect, stroke StrokeStyle) DrawOp {
	return strokeEllipseOp{Rect: rect, Stroke: stroke}
}

// DrawText creates a text operation bounded by rect.
func DrawText(text string, rect Rect, paint TextPaint) DrawOp {
	return textOp{Text: text, Rect: rect, Paint: paint}
}

// PushClip begins a rectangular clip scope.
func PushClip(rect Rect) DrawOp { return pushClipOp{Rect: rect} }

// PopClip closes the most recent clip scope.
func PopClip() DrawOp { return popClipOp{} }

// NewDrawList validates and defensively copies ops. On error it returns the
// zero DrawList and the original structured validation error.
func NewDrawList(ops ...DrawOp) (DrawList, error) {
	if len(ops) > drawMaxOps {
		return DrawList{}, &DrawValidationError{ID: drawIDTooManyOps, OpIndex: -1}
	}
	if len(ops) == 0 {
		return DrawList{}, nil
	}
	canonical := make([]DrawOp, len(ops))
	clipDepth := 0
	for i, op := range ops {
		if op == nil {
			return DrawList{}, drawError(drawIDEnumUnknown, i, "op")
		}
		copied, err := canonicalizeDrawOp(op)
		if err != nil {
			if validation, ok := err.(*DrawValidationError); ok {
				validation.OpIndex = i
			}
			return DrawList{}, err
		}
		if _, ok := copied.(pushClipOp); ok {
			clipDepth++
			if clipDepth > drawMaxClipDepth {
				return DrawList{}, drawError(drawIDClipUnbalanced, i, "clip_depth")
			}
		}
		if _, ok := copied.(popClipOp); ok {
			if clipDepth == 0 {
				return DrawList{}, drawError(drawIDClipUnderflow, i, "clip")
			}
			clipDepth--
		}
		canonical[i] = copied
	}
	if clipDepth != 0 {
		return DrawList{}, &DrawValidationError{ID: drawIDClipUnbalanced, OpIndex: -1}
	}
	return DrawList{ops: canonical}, nil
}

// ValidateDrawList defensively revalidates a previously constructed list.
func ValidateDrawList(list DrawList) error {
	if len(list.ops) == 0 {
		return nil
	}
	_, err := NewDrawList(list.ops...)
	return err
}

// MustDrawList is NewDrawList with an error-valued panic on invalid input.
func MustDrawList(ops ...DrawOp) DrawList {
	list, err := NewDrawList(ops...)
	if err != nil {
		panic(err)
	}
	return list
}

// Clone returns an independent immutable snapshot.
func (list DrawList) Clone() DrawList {
	if len(list.ops) == 0 {
		return DrawList{}
	}
	return DrawList{ops: append([]DrawOp(nil), list.ops...)}
}

// Equal compares two validated lists by value.
func (list DrawList) Equal(other DrawList) bool {
	if len(list.ops) != len(other.ops) {
		return false
	}
	for i := range list.ops {
		if !drawOpEqual(list.ops[i], other.ops[i]) {
			return false
		}
	}
	return true
}

// Len reports the number of operations in the list.
func (list DrawList) Len() int { return len(list.ops) }

// Ops returns a defensive copy of the immutable operation sequence.
func (list DrawList) Ops() []DrawOp {
	if len(list.ops) == 0 {
		return []DrawOp{}
	}
	return append([]DrawOp(nil), list.ops...)
}

func drawOpEqual(a, b DrawOp) bool {
	switch av := a.(type) {
	case clearOp:
		bv, ok := b.(clearOp)
		return ok && av == bv
	case fillRectOp:
		bv, ok := b.(fillRectOp)
		return ok && av == bv
	case strokeRectOp:
		bv, ok := b.(strokeRectOp)
		return ok && av == bv
	case fillRoundRectOp:
		bv, ok := b.(fillRoundRectOp)
		return ok && av == bv
	case strokeRoundRectOp:
		bv, ok := b.(strokeRoundRectOp)
		return ok && av == bv
	case lineOp:
		bv, ok := b.(lineOp)
		return ok && av == bv
	case fillEllipseOp:
		bv, ok := b.(fillEllipseOp)
		return ok && av == bv
	case strokeEllipseOp:
		bv, ok := b.(strokeEllipseOp)
		return ok && av == bv
	case textOp:
		bv, ok := b.(textOp)
		return ok && av == bv
	case pushClipOp:
		bv, ok := b.(pushClipOp)
		return ok && av == bv
	case popClipOp:
		_, ok := b.(popClipOp)
		return ok
	default:
		return false
	}
}

const drawHashOffset64 uint64 = 14695981039346656037

type drawHasher uint64

func (h *drawHasher) addByte(value byte) {
	*h ^= drawHasher(value)
	*h *= 1099511628211
}

func (h *drawHasher) addInt(value int) {
	h.addUint64(uint64(int64(value)))
}

func (h *drawHasher) addUint64(unsigned uint64) {
	for shift := 0; shift < 64; shift += 8 {
		h.addByte(byte(unsigned >> shift))
	}
}

func (h *drawHasher) addString(value string) {
	h.addInt(len(value))
	for index := range value {
		h.addByte(value[index])
	}
}

// drawListHash returns a deterministic cache sample. Equality remains the
// correctness check; callers must never treat this hash as collision-free.
func drawListHash(list DrawList) uint64 {
	hash := drawHasher(drawHashOffset64)
	hash.addInt(len(list.ops))
	for _, op := range list.ops {
		switch value := op.(type) {
		case clearOp:
			hash.addByte(1)
			hash.addUint64(uint64(value.Color))
		case fillRectOp:
			hash.addByte(2)
			hashRect(&hash, value.Rect)
			hash.addUint64(uint64(value.Fill.Color))
		case strokeRectOp:
			hash.addByte(3)
			hashRect(&hash, value.Rect)
			hashStroke(&hash, value.Stroke)
		case fillRoundRectOp:
			hash.addByte(4)
			hashRect(&hash, value.Rect)
			hash.addInt(value.Radius)
			hash.addUint64(uint64(value.Fill.Color))
		case strokeRoundRectOp:
			hash.addByte(5)
			hashRect(&hash, value.Rect)
			hash.addInt(value.Radius)
			hashStroke(&hash, value.Stroke)
		case lineOp:
			hash.addByte(6)
			hash.addInt(value.From.X)
			hash.addInt(value.From.Y)
			hash.addInt(value.To.X)
			hash.addInt(value.To.Y)
			hashStroke(&hash, value.Stroke)
		case fillEllipseOp:
			hash.addByte(7)
			hashRect(&hash, value.Rect)
			hash.addUint64(uint64(value.Fill.Color))
		case strokeEllipseOp:
			hash.addByte(8)
			hashRect(&hash, value.Rect)
			hashStroke(&hash, value.Stroke)
		case textOp:
			hash.addByte(9)
			hash.addString(value.Text)
			hashRect(&hash, value.Rect)
			hashTextPaint(&hash, value.Paint)
		case pushClipOp:
			hash.addByte(10)
			hashRect(&hash, value.Rect)
		case popClipOp:
			hash.addByte(11)
		default:
			hash.addByte(0)
		}
	}
	return uint64(hash)
}

func hashRect(hash *drawHasher, rect Rect) {
	hash.addInt(rect.X)
	hash.addInt(rect.Y)
	hash.addInt(rect.W)
	hash.addInt(rect.H)
}

func hashStroke(hash *drawHasher, stroke StrokeStyle) {
	hash.addUint64(uint64(stroke.Color))
	hash.addInt(stroke.Width)
	hash.addInt(int(stroke.Style))
}

func hashTextPaint(hash *drawHasher, paint TextPaint) {
	hash.addString(paint.Font.Family)
	hash.addInt(paint.Font.Size)
	hash.addInt(int(paint.Font.Weight))
	for _, flag := range []bool{paint.Font.Italic, paint.Font.Underline, paint.Font.Strikeout, paint.Mnemonic} {
		if flag {
			hash.addByte(1)
		} else {
			hash.addByte(0)
		}
	}
	hash.addUint64(uint64(paint.Color))
	hash.addInt(int(paint.Horizontal))
	hash.addInt(int(paint.Vertical))
	hash.addInt(int(paint.Wrap))
	hash.addInt(int(paint.Overflow))
}

func canonicalizeDrawOp(op DrawOp) (DrawOp, error) {
	switch value := op.(type) {
	case clearOp:
		if err := validateColor(value.Color, true); err != nil {
			return nil, drawField(err, "color")
		}
		return value, nil
	case fillRectOp:
		if err := validateRect(value.Rect); err != nil {
			return nil, err
		}
		if err := validateColor(value.Fill.Color, true); err != nil {
			return nil, drawField(err, "fill.color")
		}
		return value, nil
	case strokeRectOp:
		if err := validateRect(value.Rect); err != nil {
			return nil, err
		}
		if err := validateStroke(value.Stroke); err != nil {
			return nil, err
		}
		return value, nil
	case fillRoundRectOp:
		if err := validateRect(value.Rect); err != nil {
			return nil, err
		}
		if err := validateRadius(value.Radius); err != nil {
			return nil, err
		}
		if err := validateColor(value.Fill.Color, true); err != nil {
			return nil, drawField(err, "fill.color")
		}
		return value, nil
	case strokeRoundRectOp:
		if err := validateRect(value.Rect); err != nil {
			return nil, err
		}
		if err := validateRadius(value.Radius); err != nil {
			return nil, err
		}
		if err := validateStroke(value.Stroke); err != nil {
			return nil, err
		}
		return value, nil
	case lineOp:
		if err := validatePoint(value.From); err != nil {
			return nil, err
		}
		if err := validatePoint(value.To); err != nil {
			return nil, err
		}
		if err := validateStroke(value.Stroke); err != nil {
			return nil, err
		}
		return value, nil
	case fillEllipseOp:
		if err := validateRect(value.Rect); err != nil {
			return nil, err
		}
		if err := validateColor(value.Fill.Color, true); err != nil {
			return nil, drawField(err, "fill.color")
		}
		return value, nil
	case strokeEllipseOp:
		if err := validateRect(value.Rect); err != nil {
			return nil, err
		}
		if err := validateStroke(value.Stroke); err != nil {
			return nil, err
		}
		return value, nil
	case textOp:
		if len([]byte(value.Text)) > drawMaxTextBytes {
			return nil, drawError(drawIDTextTooLong, -1, "text")
		}
		if !utf8.ValidString(value.Text) {
			return nil, drawError(drawIDEnumUnknown, -1, "text")
		}
		if err := validateRect(value.Rect); err != nil {
			return nil, err
		}
		if err := validateColor(value.Paint.Color, true); err != nil {
			return nil, drawField(err, "color")
		}
		font, err := canonicalizeFont(value.Paint.Font)
		if err != nil {
			return nil, err
		}
		if err := validateAlignment(value.Paint.Horizontal); err != nil {
			return nil, drawField(err, "horizontal")
		}
		if err := validateAlignment(value.Paint.Vertical); err != nil {
			return nil, drawField(err, "vertical")
		}
		if value.Paint.Wrap != TextNoWrap && value.Paint.Wrap != TextWrapWord {
			return nil, drawError(drawIDEnumUnknown, -1, "wrap")
		}
		if value.Paint.Overflow != TextOverflowClip && value.Paint.Overflow != TextOverflowEllipsis {
			return nil, drawError(drawIDEnumUnknown, -1, "overflow")
		}
		value.Paint.Font = font
		return value, nil
	case pushClipOp:
		if err := validateRect(value.Rect); err != nil {
			return nil, err
		}
		return value, nil
	case popClipOp:
		return value, nil
	default:
		return nil, drawError(drawIDEnumUnknown, -1, "op")
	}
}

func canonicalizeFont(font FontSpec) (FontSpec, error) {
	if font.Size < 0 {
		return FontSpec{}, drawError(drawIDFontSize, -1, "size")
	}
	font = NormalizeFontSpec(font)
	if len([]byte(font.Family)) > drawMaxFamilyByte {
		return FontSpec{}, drawError(drawIDFontSize, -1, "family")
	}
	if !utf8.ValidString(font.Family) {
		return FontSpec{}, drawError(drawIDEnumUnknown, -1, "family")
	}
	switch font.Weight {
	case FontWeightNormal, FontWeightMedium, FontWeightSemibold, FontWeightBold:
	default:
		return FontSpec{}, drawError(drawIDEnumUnknown, -1, "weight")
	}
	return font, nil
}

func validateAlignment(value TextAlignment) error {
	if value > TextAlignEnd {
		return drawError(drawIDEnumUnknown, -1, "alignment")
	}
	return nil
}

func validateStroke(stroke StrokeStyle) error {
	if stroke.Width <= 0 {
		return drawError(drawIDStrokeWidth, -1, "width")
	}
	if stroke.Width > drawMaxCoordinate {
		return drawError(drawIDCoordinateRange, -1, "width")
	}
	if stroke.Style != StrokeSolid {
		return drawError(drawIDEnumUnknown, -1, "style")
	}
	if err := validateColor(stroke.Color, true); err != nil {
		return drawField(err, "color")
	}
	return nil
}

func validateRadius(radius int) error {
	if radius < 0 {
		return drawError(drawIDRadiusNegative, -1, "radius")
	}
	if radius > drawMaxCoordinate {
		return drawError(drawIDCoordinateRange, -1, "radius")
	}
	return nil
}

func validatePoint(point Point) error {
	if !inDrawRange(point.X) || !inDrawRange(point.Y) {
		return drawError(drawIDCoordinateRange, -1, "point")
	}
	return nil
}

func validateRect(rect Rect) error {
	if rect.W < 0 || rect.H < 0 {
		return drawError(drawIDNegativeSize, -1, "rect")
	}
	if !inDrawRange(rect.X) || !inDrawRange(rect.Y) || !inDrawRange(rect.W) || !inDrawRange(rect.H) {
		return drawError(drawIDCoordinateRange, -1, "rect")
	}
	return nil
}

func inDrawRange(value int) bool { return value >= -drawMaxCoordinate && value <= drawMaxCoordinate }

func validateColor(color Color, required bool) error {
	if color == 0 {
		if required {
			return drawError(drawIDColorRequired, -1, "color")
		}
		return nil
	}
	if uint8(uint32(color)>>24) != 0xff {
		return drawError(drawIDAlphaUnsupported, -1, "color")
	}
	return nil
}

func drawError(id MessageID, index int, field string) error {
	return &DrawValidationError{ID: id, OpIndex: index, Field: field}
}

func drawField(err error, field string) error {
	if validation, ok := err.(*DrawValidationError); ok {
		validation.Field = field
	}
	return err
}

// DrawController is the optional headless/native capability for DrawSurface.
type DrawController interface {
	SetDrawList(Handle, DrawList)
	ResetDrawList(Handle)
	InvalidateDraw(Handle)
}

// PaintCommandsToDrawList converts the legacy PaintBox command model into the
// CD1 value model without changing command order or color semantics.
func PaintCommandsToDrawList(commands []PaintCommand) (DrawList, error) {
	if err := ValidatePaintCommands(commands); err != nil {
		if legacy, ok := err.(*PaintValidationError); ok {
			id := drawIDEnumUnknown
			switch legacy.Kind {
			case PaintValidationPartialAlpha:
				id = drawIDAlphaUnsupported
			case PaintValidationClearColor, PaintValidationCirclePaint:
				id = drawIDColorRequired
			case PaintValidationCircleRadius:
				id = drawIDRadiusNegative
			case PaintValidationStrokeWidthNegative, PaintValidationStrokeWidthRequired:
				id = drawIDStrokeWidth
			case PaintValidationStrokeColorRequired:
				id = drawIDColorRequired
			}
			return DrawList{}, &DrawValidationError{ID: id, OpIndex: legacy.Index, Field: "legacy"}
		}
		return DrawList{}, err
	}
	ops := make([]DrawOp, 0, len(commands))
	for index, command := range commands {
		switch command.Kind {
		case PaintClear:
			ops = append(ops, Clear(command.Color))
		case PaintCircle:
			if !inDrawRange(command.X) || !inDrawRange(command.Y) ||
				command.Radius > drawMaxCoordinate/2 ||
				(command.StrokeColor != 0 && command.StrokeWidth > drawMaxCoordinate) {
				return DrawList{}, &DrawValidationError{ID: drawIDCoordinateRange, OpIndex: index, Field: "circle"}
			}
			rect := Rect{
				X: command.X - command.Radius, Y: command.Y - command.Radius,
				W: command.Radius * 2, H: command.Radius * 2,
			}
			if err := validateRect(rect); err != nil {
				if validation, ok := err.(*DrawValidationError); ok {
					validation.OpIndex = index
				}
				return DrawList{}, err
			}
			if command.FillColor != 0 {
				ops = append(ops, FillEllipse(rect, FillStyle{Color: command.FillColor}))
			}
			if command.StrokeColor != 0 {
				ops = append(ops, StrokeEllipse(rect, StrokeStyle{Color: command.StrokeColor, Width: command.StrokeWidth}))
			}
		default:
			return DrawList{}, &DrawValidationError{ID: drawIDEnumUnknown, OpIndex: index, Field: "kind"}
		}
	}
	return NewDrawList(ops...)
}
