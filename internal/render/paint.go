package render

import "fmt"

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
				return fmt.Errorf("command %d: clear color must be non-zero", i)
			}
		case PaintCircle:
			if command.Radius <= 0 {
				return fmt.Errorf("command %d: circle radius must be > 0", i)
			}
			if command.StrokeWidth < 0 {
				return fmt.Errorf("command %d: circle stroke width must be >= 0", i)
			}
			if command.FillColor == 0 && command.StrokeColor == 0 {
				return fmt.Errorf("command %d: circle needs a fill or stroke color", i)
			}
			if command.StrokeColor != 0 && command.StrokeWidth <= 0 {
				return fmt.Errorf("command %d: circle stroke width must be > 0 when stroke is set", i)
			}
			if command.StrokeColor == 0 && command.StrokeWidth != 0 {
				return fmt.Errorf("command %d: circle stroke width requires a stroke color", i)
			}
		default:
			return fmt.Errorf("command %d: unknown paint command kind %d", i, command.Kind)
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
