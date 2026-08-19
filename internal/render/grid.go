package render

import "fmt"

// GridValidationKind identifies one stable category of grid validation failure.
// The root package maps these categories to localized framework diagnostics.
type GridValidationKind uint8

const (
	GridValidationInvalidSize GridValidationKind = iota + 1
	GridValidationRowCount
	GridValidationColumnCount
)

// GridValidationError carries validation details without making rendered error
// text part of the cross-package contract.
type GridValidationError struct {
	Kind GridValidationKind
	Row  int
	Got  int
	Want int
}

func (e *GridValidationError) Error() string {
	switch e.Kind {
	case GridValidationInvalidSize:
		return fmt.Sprintf("invalid grid size %dx%d", e.Got, e.Want)
	case GridValidationRowCount:
		return fmt.Sprintf("Cells row count=%d, want %d", e.Got, e.Want)
	case GridValidationColumnCount:
		return fmt.Sprintf("Cells row %d column count=%d, want %d", e.Row, e.Got, e.Want)
	default:
		return "invalid grid cells"
	}
}

// GridCell 是 StringGrid 的逻辑数据坐标，不包含可选的原生表头行。
type GridCell struct {
	Row    int
	Column int
}

// GridSize 是 StringGrid 的有界逻辑尺寸。Columns 始终大于 0，Rows 可为 0。
type GridSize struct {
	Rows    int
	Columns int
}

// GridSelection 保存受控焦点单元格和是否采用整行选择模式。
type GridSelection struct {
	Cell    GridCell
	RowOnly bool
}

// GridController 是 StringGrid 的 D6 窄能力。所有 slice 实现都必须取得所有权
// 或深复制；事件坐标使用不含表头的逻辑行号。
type GridController interface {
	SetGridSize(h Handle, size GridSize)
	SetGridHeaders(h Handle, headers []string)
	SetGridColumnWidths(h Handle, widths []int)
	SetGridCells(h Handle, cells [][]string)
	SetGridEditable(h Handle, editable bool)
	SetGridSelection(h Handle, selection GridSelection)
	OnGridCellSelect(h Handle, fn func(GridCell))
	OnGridCellEdit(h Handle, fn func(GridCell, string))
}

// CloneGridCells 深复制二维字符串矩阵，并把 nil 统一为非 nil 空 slice。
func CloneGridCells(cells [][]string) [][]string {
	if len(cells) == 0 {
		return [][]string{}
	}
	out := make([][]string, len(cells))
	for row := range cells {
		out[row] = append([]string(nil), cells[row]...)
		if len(out[row]) == 0 {
			out[row] = []string{}
		}
	}
	return out
}

// ValidateGridCells 验证矩阵与有界逻辑尺寸严格匹配。
func ValidateGridCells(size GridSize, cells [][]string) error {
	if size.Rows < 0 || size.Columns <= 0 {
		return &GridValidationError{Kind: GridValidationInvalidSize, Got: size.Rows, Want: size.Columns}
	}
	if len(cells) != size.Rows {
		return &GridValidationError{Kind: GridValidationRowCount, Got: len(cells), Want: size.Rows}
	}
	for row := range cells {
		if len(cells[row]) != size.Columns {
			return &GridValidationError{
				Kind: GridValidationColumnCount,
				Row:  row,
				Got:  len(cells[row]),
				Want: size.Columns,
			}
		}
	}
	return nil
}

// ValidGridSelection 报告受控选择是否适用于给定逻辑尺寸。
func ValidGridSelection(size GridSize, selection GridSelection) bool {
	if size.Rows == 0 {
		return selection.Cell.Row == -1 && selection.Cell.Column == -1
	}
	return selection.Cell.Row >= 0 && selection.Cell.Row < size.Rows &&
		selection.Cell.Column >= 0 && selection.Cell.Column < size.Columns
}
