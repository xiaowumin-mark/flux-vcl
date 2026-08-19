package flux

import (
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
	"github.com/xiaowumin-mark/flux-vcl/internal/widget"
)

// GridCell 是 StringGrid 的逻辑数据单元格坐标。Row 不包含可选的原生表头行，
// Row/Column 均从 0 开始；{-1,-1} 表示没有可选数据行。
type GridCell = render.GridCell

// StringGrid 创建有界原生 TStringGrid。rows 是逻辑数据行数（可为 0），columns
// 必须大于 0。Cells、Headers 与 ColumnWidths 使用严格矩形模型：非空值的维度
// 必须与 rows/columns 一致，非法参数会确定性 panic。
//
// 数据、选择和 Editable 都是受控值。表头不计入 rows；OnCellSelect/OnCellEdit
// 返回逻辑坐标。二维 Cells 在公开、diff、Mock 和 native 边界均深复制。
func StringGrid(rows, columns int, opts ...Opt) Widget {
	if rows < 0 {
		panic(DiagnosticText(DiagnosticGridRows))
	}
	if columns <= 0 {
		panic(DiagnosticText(DiagnosticGridColumns))
	}
	n := widget.NewNode("StringGrid")
	applyOpts(n, opts)

	size := render.GridSize{Rows: rows, Columns: columns}
	cells := makeGridCells(rows, columns)
	if value, ok := n.Props.Get("Cells"); ok {
		configured, valid := value.([][]string)
		if !valid {
			panic(DiagnosticText(DiagnosticGridCellsType))
		}
		if err := render.ValidateGridCells(size, configured); err != nil {
			panic(gridCellsDiagnostic(err))
		}
		cells = render.CloneGridCells(configured)
	}

	headers := []string{}
	if value, ok := n.Props.Get("Headers"); ok {
		configured, valid := value.([]string)
		if !valid || (len(configured) != 0 && len(configured) != columns) {
			panic(DiagnosticText(DiagnosticGridHeadersLength, columns))
		}
		headers = append([]string(nil), configured...)
	}

	widths := []int{}
	if value, ok := n.Props.Get("ColumnWidths"); ok {
		configured, valid := value.([]int)
		if !valid || (len(configured) != 0 && len(configured) != columns) {
			panic(DiagnosticText(DiagnosticGridWidthsLength, columns))
		}
		for _, width := range configured {
			if width <= 0 {
				panic(DiagnosticText(DiagnosticGridWidthsPositive))
			}
		}
		widths = append([]int(nil), configured...)
	}

	selection := render.GridSelection{Cell: render.GridCell{Row: -1, Column: -1}}
	if rows > 0 {
		selection.Cell = render.GridCell{Row: 0, Column: 0}
	}
	if value, ok := n.Props.Get("GridSelection"); ok {
		configured, valid := value.(render.GridSelection)
		if !valid || !render.ValidGridSelection(size, configured) {
			panic(DiagnosticText(DiagnosticGridSelection))
		}
		selection = configured
	}

	n.Props.Set("GridSize", size)
	n.Props.Set("Headers", headers)
	n.Props.Set("ColumnWidths", widths)
	n.Props.Set("Cells", cells)
	n.Props.Set("Editable", n.Props.Bool("Editable"))
	n.Props.Set("GridSelection", selection)
	return widgetNode{n}
}

func gridCellsDiagnostic(err error) string {
	if validation, ok := err.(*render.GridValidationError); ok {
		switch validation.Kind {
		case render.GridValidationRowCount:
			return DiagnosticText(DiagnosticGridCellsRowCount, validation.Got, validation.Want)
		case render.GridValidationColumnCount:
			return DiagnosticText(DiagnosticGridCellsColumnCount, validation.Row, validation.Got, validation.Want)
		}
	}
	return DiagnosticText(DiagnosticGridCellsInvalid, err)
}

func makeGridCells(rows, columns int) [][]string {
	values := make([][]string, rows)
	for row := range values {
		values[row] = make([]string, columns)
	}
	return values
}

// Cells 设置 StringGrid 的受控字符串矩阵并立即深复制。矩阵必须严格矩形；
// StringGrid 构造时还会校验其尺寸与声明的 rows/columns 完全一致。
func Cells(values [][]string) Opt {
	copy := render.CloneGridCells(values)
	if len(copy) > 0 {
		columns := len(copy[0])
		for _, row := range copy {
			if len(row) != columns {
				panic(DiagnosticText(DiagnosticCellsRectangular))
			}
		}
	}
	return optFn(func(n *Node) { n.Props.Set("Cells", render.CloneGridCells(copy)) })
}

// Headers 设置 StringGrid 的可选表头。空 slice 表示没有表头；非空时长度必须
// 等于 StringGrid 的 columns。输入会被防御性复制。
func Headers(values []string) Opt {
	copy := append([]string(nil), values...)
	if len(copy) == 0 {
		copy = []string{}
	}
	return optFn(func(n *Node) { n.Props.Set("Headers", append([]string(nil), copy...)) })
}

// ColumnWidths 设置每列的 DIP 宽度。空 slice 使用 96 DIP 默认列宽；非空时长度
// 必须等于 columns，且每项必须大于 0。输入会被防御性复制。
func ColumnWidths(values []int) Opt {
	copy := append([]int(nil), values...)
	for _, width := range copy {
		if width <= 0 {
			panic(DiagnosticText(DiagnosticColumnWidthsPositive))
		}
	}
	if len(copy) == 0 {
		copy = []int{}
	}
	return optFn(func(n *Node) { n.Props.Set("ColumnWidths", append([]int(nil), copy...)) })
}

// SelectedCell 设置受控单元格选择。坐标在 StringGrid 构造时按逻辑行列校验。
func SelectedCell(row, column int) Opt {
	selection := render.GridSelection{Cell: render.GridCell{Row: row, Column: column}}
	return optFn(func(n *Node) { n.Props.Set("GridSelection", selection) })
}

// SelectedRow 设置受控整行选择。回调仍返回实际焦点列；行号不包含表头。
func SelectedRow(row int) Opt {
	selection := render.GridSelection{
		Cell: render.GridCell{Row: row, Column: 0}, RowOnly: true,
	}
	return optFn(func(n *Node) { n.Props.Set("GridSelection", selection) })
}

// Editable 设置 StringGrid 是否允许原生单元格编辑，缺省为 false。
func Editable(value bool) Opt {
	return optFn(func(n *Node) { n.Props.Set("Editable", value) })
}

// OnCellSelect 绑定 StringGrid 的用户选择事件，坐标为不含表头的逻辑 GridCell。
func OnCellSelect(fn func(cell GridCell)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnCellSelect", fn) })
}

// OnCellEdit 绑定 StringGrid 的编辑提交事件。回调应更新受控 Cells，下一次
// render 会把业务值镜像回 native；程序化 Cells patch 不触发本事件。
func OnCellEdit(fn func(cell GridCell, value string)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnCellEdit", fn) })
}
