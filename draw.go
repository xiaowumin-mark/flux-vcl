package flux

import (
	"errors"
	"fmt"
	"strings"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// DrawList is a validated immutable sequence of headless drawing operations.
// Its zero value represents an empty list.
type DrawList = render.DrawList

// DrawOp is the sealed value operation interface used by DrawList.
type DrawOp = render.DrawOp

// FillStyle describes an opaque fill.
type FillStyle = render.FillStyle

// StrokeKind identifies a supported stroke pattern.
type StrokeKind = render.StrokeKind

// StrokeStyle describes an opaque DIP-width stroke.
type StrokeStyle = render.StrokeStyle

// FontWeight is a supported CSS/Win32-compatible font weight.
type FontWeight = render.FontWeight

// FontSpec describes a system or explicit UI font in DIP units.
type FontSpec = render.FontSpec

// TextAlignment controls horizontal or vertical text alignment.
type TextAlignment = render.TextAlignment

// TextWrap controls line wrapping.
type TextWrap = render.TextWrap

// TextOverflow controls clipped or ellipsized overflow.
type TextOverflow = render.TextOverflow

// TextPaint combines font, color, alignment, wrapping, and mnemonic policy.
type TextPaint = render.TextPaint

// TextMeasureRequest and TextMeasureConstraints expose the same immutable
// request vocabulary used by optional renderer capabilities. They are aliases
// so a FontSpec passed to DrawText and to measurement has one concrete type.
type TextMeasureRequest = render.TextMeasureRequest
type TextMeasureConstraints = render.TextMeasureConstraints
type TextMeasureConstraint = render.TextMeasureConstraint
type TextMeasureCacheKey = render.TextMeasureCacheKey
type TextMeasureSize = render.Size
type StyledTextMeasurer = render.StyledTextMeasurer
type FontController = render.FontController

const (
	StrokeSolid = render.StrokeSolid

	FontWeightNormal   = render.FontWeightNormal
	FontWeightMedium   = render.FontWeightMedium
	FontWeightSemibold = render.FontWeightSemibold
	FontWeightBold     = render.FontWeightBold

	TextAlignStart  = render.TextAlignStart
	TextAlignCenter = render.TextAlignCenter
	TextAlignEnd    = render.TextAlignEnd

	TextNoWrap           = render.TextNoWrap
	TextWrapWord         = render.TextWrapWord
	TextOverflowClip     = render.TextOverflowClip
	TextOverflowEllipsis = render.TextOverflowEllipsis
)

// ErrInvalidDrawList identifies every DrawList validation failure.
var ErrInvalidDrawList = render.ErrInvalidDrawList

// DrawValidationError carries the stable ID, operation index, and field for a
// DrawList validation failure.
type DrawValidationError struct {
	ID      MessageID
	OpIndex int
	Field   string
}

// Error returns the active localized diagnostic text.
func (e *DrawValidationError) Error() string {
	if e == nil {
		return "<nil>"
	}
	text := DiagnosticText(e.ID)
	if e.OpIndex >= 0 {
		if strings.Contains(text, "%") {
			return DiagnosticText(e.ID, e.OpIndex, e.Field)
		}
		return fmt.Sprintf("%s (op %d, field %s)", text, e.OpIndex, e.Field)
	}
	return text
}

// Unwrap lets errors.Is classify the error as ErrInvalidDrawList.
func (e *DrawValidationError) Unwrap() error { return ErrInvalidDrawList }

// NewDrawList validates, canonicalizes, and snapshots ops.
func NewDrawList(ops ...DrawOp) (DrawList, error) {
	list, err := render.NewDrawList(ops...)
	return list, publicDrawError(err)
}

// MustDrawList is NewDrawList with an error-valued panic on invalid input.
func MustDrawList(ops ...DrawOp) DrawList {
	list, err := NewDrawList(ops...)
	if err != nil {
		panic(err)
	}
	return list
}

// Clear fills the complete drawing surface with color.
func Clear(color ColorValue) DrawOp { return render.Clear(color) }

// FillRect creates a rectangle fill operation.
func FillRect(rect Rect, fill FillStyle) DrawOp { return render.FillRect(rect, fill) }

// StrokeRect creates a rectangle stroke operation.
func StrokeRect(rect Rect, stroke StrokeStyle) DrawOp { return render.StrokeRect(rect, stroke) }

// FillRoundRect creates a rounded-rectangle fill operation.
func FillRoundRect(rect Rect, radius int, fill FillStyle) DrawOp {
	return render.FillRoundRect(rect, radius, fill)
}

// StrokeRoundRect creates a rounded-rectangle stroke operation.
func StrokeRoundRect(rect Rect, radius int, stroke StrokeStyle) DrawOp {
	return render.StrokeRoundRect(rect, radius, stroke)
}

// DrawLine creates a line operation.
func DrawLine(from, to Point, stroke StrokeStyle) DrawOp {
	return render.DrawLine(
		render.Point{X: from.X, Y: from.Y},
		render.Point{X: to.X, Y: to.Y},
		stroke,
	)
}

// FillEllipse creates an ellipse fill operation.
func FillEllipse(rect Rect, fill FillStyle) DrawOp { return render.FillEllipse(rect, fill) }

// StrokeEllipse creates an ellipse stroke operation.
func StrokeEllipse(rect Rect, stroke StrokeStyle) DrawOp { return render.StrokeEllipse(rect, stroke) }

// DrawText creates a text operation bounded by rect.
func DrawText(text string, rect Rect, paint TextPaint) DrawOp {
	return render.DrawText(text, rect, paint)
}

// PushClip intersects drawing with rect until the matching PopClip.
func PushClip(rect Rect) DrawOp { return render.PushClip(rect) }

// PopClip closes the most recent rectangular clip.
func PopClip() DrawOp { return render.PopClip() }

func publicDrawError(err error) error {
	if err == nil {
		return nil
	}
	var internal *render.DrawValidationError
	if errors.As(err, &internal) {
		return &DrawValidationError{
			ID: MessageID(internal.ID), OpIndex: internal.OpIndex, Field: internal.Field,
		}
	}
	if errors.Is(err, render.ErrInvalidDrawList) {
		return fmt.Errorf("%w: %v", ErrInvalidDrawList, err)
	}
	return err
}
