package render

import "fmt"

// PaintValidationKind identifies one stable category of PaintBox command error.
// The root package maps these categories to localized framework diagnostics.
type PaintValidationKind uint8

const (
	PaintValidationClearColor PaintValidationKind = iota + 1
	PaintValidationCircleRadius
	PaintValidationStrokeWidthNegative
	PaintValidationCirclePaint
	PaintValidationStrokeWidthRequired
	PaintValidationStrokeColorRequired
	PaintValidationUnknownKind
)

// PaintValidationError carries the invalid command position and structured
// details so callers do not need to localize an internal Error string.
type PaintValidationError struct {
	Kind    PaintValidationKind
	Index   int
	Command PaintCommandKind
}

func (e *PaintValidationError) Error() string {
	switch e.Kind {
	case PaintValidationClearColor:
		return fmt.Sprintf("command %d: clear color must be non-zero", e.Index)
	case PaintValidationCircleRadius:
		return fmt.Sprintf("command %d: circle radius must be > 0", e.Index)
	case PaintValidationStrokeWidthNegative:
		return fmt.Sprintf("command %d: circle stroke width must be >= 0", e.Index)
	case PaintValidationCirclePaint:
		return fmt.Sprintf("command %d: circle needs a fill or stroke color", e.Index)
	case PaintValidationStrokeWidthRequired:
		return fmt.Sprintf("command %d: circle stroke width must be > 0 when stroke is set", e.Index)
	case PaintValidationStrokeColorRequired:
		return fmt.Sprintf("command %d: circle stroke width requires a stroke color", e.Index)
	case PaintValidationUnknownKind:
		return fmt.Sprintf("command %d: unknown paint command kind %d", e.Index, e.Command)
	default:
		return "invalid paint command"
	}
}

// PaintCommandKind 是 PaintBox 的不可变绘制命令种类。命令使用值语义，因此
// Widget Props 可通过 DeepEqual 稳定比较。
type PaintCommandKind uint8

const (
	// PaintClear 使用 Color 清空整个绘制表面。
	PaintClear PaintCommandKind = iota + 1
	// PaintCircle 使用圆心、半径及填充/描边颜色绘制圆形。
	PaintCircle
)

// PaintCommand 是与后端无关的绘制命令。坐标、半径和线宽均为 DIP；清屏使用
// Color，圆形使用 X/Y/Radius、FillColor、StrokeColor 和 StrokeWidth。颜色为零
// 表示不绘制对应的填充或描边。
type PaintCommand struct {
	Kind        PaintCommandKind
	X           int
	Y           int
	Radius      int
	Color       Color
	FillColor   Color
	StrokeColor Color
	StrokeWidth int
}

// ClonePaintCommands 返回防御性副本，并把空值规范为非 nil 空 slice。公开 API、
// diff 和后端的所有权边界都会调用它。
func ClonePaintCommands(commands []PaintCommand) []PaintCommand {
	if len(commands) == 0 {
		return []PaintCommand{}
	}
	return append([]PaintCommand(nil), commands...)
}

// ValidatePaintCommands 拒绝无法确定绘制语义的命令。坐标允许为负数，以便裁剪
// 超出 surface 的图形。
func ValidatePaintCommands(commands []PaintCommand) error {
	for i, command := range commands {
		switch command.Kind {
		case PaintClear:
			if command.Color == 0 {
				return &PaintValidationError{Kind: PaintValidationClearColor, Index: i}
			}
		case PaintCircle:
			if command.Radius <= 0 {
				return &PaintValidationError{Kind: PaintValidationCircleRadius, Index: i}
			}
			if command.StrokeWidth < 0 {
				return &PaintValidationError{Kind: PaintValidationStrokeWidthNegative, Index: i}
			}
			if command.FillColor == 0 && command.StrokeColor == 0 {
				return &PaintValidationError{Kind: PaintValidationCirclePaint, Index: i}
			}
			if command.StrokeColor != 0 && command.StrokeWidth <= 0 {
				return &PaintValidationError{Kind: PaintValidationStrokeWidthRequired, Index: i}
			}
			if command.StrokeColor == 0 && command.StrokeWidth != 0 {
				return &PaintValidationError{Kind: PaintValidationStrokeColorRequired, Index: i}
			}
		default:
			return &PaintValidationError{Kind: PaintValidationUnknownKind, Index: i, Command: command.Kind}
		}
	}
	return nil
}

// PaintController 是 PaintBox 的可选窄渲染能力。diff 层显式拥有 invalidate，
// 因此命令 patch 可被观测，并能独立于 native identity 测试。
type PaintController interface {
	SetPaintCommands(h Handle, commands []PaintCommand)
	InvalidatePaint(h Handle)
}

// PaintSurface 是 PaintController 的兼容别名。
type PaintSurface = PaintController
