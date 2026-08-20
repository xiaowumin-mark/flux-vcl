package flux

import (
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
	"github.com/xiaowumin-mark/flux-vcl/internal/widget"
)

// PaintCommandKind 是 PaintBox 的绘制命令种类。
type PaintCommandKind = render.PaintCommandKind

// v0.1.0 PaintBox 命令模型支持的绘制操作。
const (
	// PaintClear 表示使用 PaintCommand.Color 填满整个 PaintBox 绘制表面。
	PaintClear = render.PaintClear
	// PaintCircle 表示使用圆心、半径及可选填充/描边绘制圆形。
	PaintCircle = render.PaintCircle
)

// PaintCommand 是一条不可变 PaintBox 绘制操作。所有几何量均以 PaintBox 客户区
// 左上角为原点、使用 DIP 表示。
//
// PaintClear 使用 Color 填满 surface；PaintCircle 以 X/Y 为圆心、Radius 为半径。
// FillColor 或 StrokeColor 为零表示禁用对应部分；非零颜色必须为 A=0xFF 的
// 不透明 ARGB（首版不支持 partial alpha），非零 StrokeColor 要求 StrokeWidth > 0。
// PaintBox 按 slice 顺序执行命令。
type PaintCommand = render.PaintCommand

// PaintBox 创建由稳定值命令驱动的原生 TPaintBox。命令会被校验并防御性复制，
// 只有值变化时才请求重绘。OnMouseDown 等鼠标选项报告 DIP 坐标，命中测试由
// 应用层自行决定。
func PaintBox(commands []PaintCommand, opts ...Opt) Widget {
	if err := render.ValidatePaintCommands(commands); err != nil {
		panic(paintCommandsDiagnostic(err))
	}
	n := widget.NewNode("PaintBox")
	n.Props.Set("PaintCommands", render.ClonePaintCommands(commands))
	applyOpts(n, opts)
	return widgetNode{n}
}

func paintCommandsDiagnostic(err error) string {
	if validation, ok := err.(*render.PaintValidationError); ok {
		switch validation.Kind {
		case render.PaintValidationClearColor:
			return DiagnosticText(DiagnosticPaintClearColor, validation.Index)
		case render.PaintValidationCircleRadius:
			return DiagnosticText(DiagnosticPaintCircleRadius, validation.Index)
		case render.PaintValidationStrokeWidthNegative:
			return DiagnosticText(DiagnosticPaintStrokeWidthNegative, validation.Index)
		case render.PaintValidationCirclePaint:
			return DiagnosticText(DiagnosticPaintCirclePaint, validation.Index)
		case render.PaintValidationStrokeWidthRequired:
			return DiagnosticText(DiagnosticPaintStrokeWidthRequired, validation.Index)
		case render.PaintValidationStrokeColorRequired:
			return DiagnosticText(DiagnosticPaintStrokeColorRequired, validation.Index)
		case render.PaintValidationPartialAlpha:
			return DiagnosticText(DiagnosticPaintPartialAlpha, validation.Index)
		case render.PaintValidationUnknownKind:
			return DiagnosticText(DiagnosticPaintUnknownKind, validation.Index, validation.Command)
		}
	}
	return DiagnosticText(DiagnosticPaintCommands, err)
}
