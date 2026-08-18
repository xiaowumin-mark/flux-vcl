// FluxVCL 7GUIs Cells：可编辑 StringGrid 与示例层公式依赖图。
//
// 设计边界见 docs/design.md §21.2、§21.4 与 docs/7guis.md。A1 引用、
// 加法、依赖传播及错误/循环检测均属于本示例，未进入 StringGrid API。
//
// 构建：scripts/build.ps1 -Target 7guis-cells
// 冒烟：scripts/smoke.ps1 -Target 7guis-cells
package main

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/native"
)

const (
	sheetRows    = 8
	sheetColumns = 5

	formulaError = "#ERROR!"
	valueError   = "#VALUE!"
	referenceErr = "#REF!"
	cycleError   = "#CYCLE!"
	numberError  = "#NUM!"
)

var cellReferencePattern = regexp.MustCompile(`^([A-Za-z]+)([1-9][0-9]*)$`)

type cellCoord struct {
	row    int
	column int
}

type formulaTerm struct {
	reference *cellCoord
	number    float64
}

type cellResult struct {
	display string
	number  float64
	numeric bool
	errCode string
}

type sheetModel struct {
	Rows         int
	Columns      int
	Raw          [][]string
	Display      [][]string
	Selected     cellCoord
	Dependencies map[cellCoord]map[cellCoord]struct{}
	Dependents   map[cellCoord]map[cellCoord]struct{}
	Status       string
}

func (m *sheetModel) String() string {
	if m == nil || !m.valid(m.Selected) {
		return "No cell selected"
	}
	name := cellName(m.Selected)
	raw := m.Raw[m.Selected.row][m.Selected.column]
	value := m.Display[m.Selected.row][m.Selected.column]
	return fmt.Sprintf("%s   Source: %q   Value: %s   Dependents: %d   %s",
		name, raw, value, len(m.Dependents[m.Selected]), m.Status)
}

func stringMatrix(rows, columns int) [][]string {
	values := make([][]string, rows)
	for row := range values {
		values[row] = make([]string, columns)
	}
	return values
}

func cloneStringMatrix(values [][]string) [][]string {
	copy := make([][]string, len(values))
	for row := range values {
		copy[row] = append([]string(nil), values[row]...)
	}
	return copy
}

func cloneCoordMap(values map[cellCoord]map[cellCoord]struct{}) map[cellCoord]map[cellCoord]struct{} {
	copy := make(map[cellCoord]map[cellCoord]struct{}, len(values))
	for key, set := range values {
		setCopy := make(map[cellCoord]struct{}, len(set))
		for item := range set {
			setCopy[item] = struct{}{}
		}
		copy[key] = setCopy
	}
	return copy
}

func newBlankSheet(rows, columns int) *sheetModel {
	m := &sheetModel{
		Rows:         rows,
		Columns:      columns,
		Raw:          stringMatrix(rows, columns),
		Display:      stringMatrix(rows, columns),
		Selected:     cellCoord{},
		Dependencies: make(map[cellCoord]map[cellCoord]struct{}),
		Dependents:   make(map[cellCoord]map[cellCoord]struct{}),
		Status:       "Ready",
	}
	m.recalculate(m.allCells())
	return m
}

func newDemoSheet() *sheetModel {
	m := newBlankSheet(sheetRows, sheetColumns)
	demo := map[cellCoord]string{
		{row: 0, column: 0}: "1",
		{row: 0, column: 1}: "=A1+2",
		{row: 0, column: 2}: "=B1+3",
		{row: 1, column: 0}: "10",
		{row: 1, column: 1}: "=A2+A1",
		{row: 1, column: 2}: "=B2+C1",
		{row: 2, column: 0}: "text",
		{row: 2, column: 1}: "=A3+1",
		{row: 3, column: 3}: "=E4+1",
		{row: 3, column: 4}: "=D4+1",
		{row: 4, column: 4}: "=Z99",
	}
	for cell, raw := range demo {
		m.setCell(cell, raw)
	}
	m.Selected = cellCoord{}
	m.Status = "Ready"
	return m
}

func (m *sheetModel) clone() *sheetModel {
	if m == nil {
		return nil
	}
	return &sheetModel{
		Rows:         m.Rows,
		Columns:      m.Columns,
		Raw:          cloneStringMatrix(m.Raw),
		Display:      cloneStringMatrix(m.Display),
		Selected:     m.Selected,
		Dependencies: cloneCoordMap(m.Dependencies),
		Dependents:   cloneCoordMap(m.Dependents),
		Status:       m.Status,
	}
}

func (m *sheetModel) valid(cell cellCoord) bool {
	return cell.row >= 0 && cell.row < m.Rows && cell.column >= 0 && cell.column < m.Columns
}

func (m *sheetModel) allCells() map[cellCoord]struct{} {
	result := make(map[cellCoord]struct{}, m.Rows*m.Columns)
	for row := 0; row < m.Rows; row++ {
		for column := 0; column < m.Columns; column++ {
			result[cellCoord{row: row, column: column}] = struct{}{}
		}
	}
	return result
}

func (m *sheetModel) dependentClosure(start cellCoord) map[cellCoord]struct{} {
	result := map[cellCoord]struct{}{start: {}}
	queue := []cellCoord{start}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for dependent := range m.Dependents[current] {
			if _, seen := result[dependent]; seen {
				continue
			}
			result[dependent] = struct{}{}
			queue = append(queue, dependent)
		}
	}
	return result
}

func (m *sheetModel) setCell(cell cellCoord, raw string) {
	if !m.valid(cell) {
		return
	}
	m.Raw[cell.row][cell.column] = raw
	m.replaceDependencies(cell)
	m.recalculate(m.dependentClosure(cell))
	m.Selected = cell
	m.Status = cellName(cell) + " recalculated"
}

func (m *sheetModel) replaceDependencies(cell cellCoord) {
	for reference := range m.Dependencies[cell] {
		delete(m.Dependents[reference], cell)
		if len(m.Dependents[reference]) == 0 {
			delete(m.Dependents, reference)
		}
	}

	_, references, _ := parseFormula(m.Raw[cell.row][cell.column], m.Rows, m.Columns)
	m.Dependencies[cell] = references
	for reference := range references {
		if m.Dependents[reference] == nil {
			m.Dependents[reference] = make(map[cellCoord]struct{})
		}
		m.Dependents[reference][cell] = struct{}{}
	}
}

func (m *sheetModel) recalculate(cells map[cellCoord]struct{}) {
	memo := make(map[cellCoord]cellResult)
	visiting := make(map[cellCoord]bool)
	for cell := range cells {
		result := m.evaluate(cell, memo, visiting)
		m.Display[cell.row][cell.column] = result.display
	}
}

func (m *sheetModel) evaluate(cell cellCoord, memo map[cellCoord]cellResult, visiting map[cellCoord]bool) cellResult {
	if cached, ok := memo[cell]; ok {
		return cached
	}
	if !m.valid(cell) {
		return cellResult{display: referenceErr, errCode: referenceErr}
	}
	if visiting[cell] {
		return cellResult{display: cycleError, errCode: cycleError}
	}

	raw := m.Raw[cell.row][cell.column]
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "=") {
		result := cellResult{display: raw}
		if trimmed == "" {
			result.numeric = true
		} else if number, err := strconv.ParseFloat(trimmed, 64); err == nil {
			result.number = number
			result.numeric = true
		}
		memo[cell] = result
		return result
	}

	visiting[cell] = true
	defer delete(visiting, cell)
	terms, _, parseError := parseFormula(raw, m.Rows, m.Columns)
	if parseError != "" {
		result := cellResult{display: parseError, errCode: parseError}
		memo[cell] = result
		return result
	}

	total := 0.0
	for _, term := range terms {
		if term.reference == nil {
			total += term.number
			continue
		}
		value := m.evaluate(*term.reference, memo, visiting)
		if value.errCode != "" {
			result := cellResult{display: value.errCode, errCode: value.errCode}
			memo[cell] = result
			return result
		}
		if !value.numeric {
			result := cellResult{display: valueError, errCode: valueError}
			memo[cell] = result
			return result
		}
		total += value.number
	}
	if math.IsInf(total, 0) || math.IsNaN(total) {
		result := cellResult{display: numberError, errCode: numberError}
		memo[cell] = result
		return result
	}
	result := cellResult{display: formatNumber(total), number: total, numeric: true}
	memo[cell] = result
	return result
}

func parseFormula(raw string, rows, columns int) ([]formulaTerm, map[cellCoord]struct{}, string) {
	references := make(map[cellCoord]struct{})
	trimmed := strings.TrimSpace(raw)
	if !strings.HasPrefix(trimmed, "=") {
		return nil, references, ""
	}
	expression := strings.TrimSpace(strings.TrimPrefix(trimmed, "="))
	if expression == "" {
		return nil, references, formulaError
	}

	parts := strings.Split(expression, "+")
	terms := make([]formulaTerm, 0, len(parts))
	for _, part := range parts {
		text := strings.TrimSpace(part)
		if text == "" {
			return terms, references, formulaError
		}
		if match := cellReferencePattern.FindStringSubmatch(text); match != nil {
			column, ok := columnNumber(match[1])
			if !ok {
				return terms, references, referenceErr
			}
			row, err := strconv.Atoi(match[2])
			cell := cellCoord{row: row - 1, column: column}
			if err != nil || cell.row < 0 || cell.row >= rows || cell.column < 0 || cell.column >= columns {
				return terms, references, referenceErr
			}
			reference := cell
			terms = append(terms, formulaTerm{reference: &reference})
			references[cell] = struct{}{}
			continue
		}
		number, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return terms, references, formulaError
		}
		terms = append(terms, formulaTerm{number: number})
	}
	return terms, references, ""
}

func columnNumber(label string) (int, bool) {
	column := 0
	for _, r := range strings.ToUpper(label) {
		if r < 'A' || r > 'Z' {
			return 0, false
		}
		if column > (math.MaxInt-int(r-'A'+1))/26 {
			return 0, false
		}
		column = column*26 + int(r-'A'+1)
	}
	return column - 1, column > 0
}

func columnName(column int) string {
	column++
	var result string
	for column > 0 {
		column--
		result = string(rune('A'+column%26)) + result
		column /= 26
	}
	return result
}

func cellName(cell cellCoord) string {
	return columnName(cell.column) + strconv.Itoa(cell.row+1)
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法获取 exe 路径:", err)
		os.Exit(2)
	}
	dllPath := filepath.Join(filepath.Dir(exe), "libenergy-amd64.dll")
	if _, err := os.Stat(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll。请用 scripts/build.ps1 -Target 7guis-cells 构建。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}

	r := native.NewRenderer()
	app := flux.NewApp(r)
	initial := newDemoSheet()
	sheet := flux.NewState(initial)
	formula := flux.NewState(initial.Raw[0][0])

	if err := app.Mount(func() flux.Widget {
		snapshot := sheet.Get()
		headers := make([]string, snapshot.Columns)
		widths := make([]int, snapshot.Columns)
		for column := range headers {
			headers[column] = columnName(column)
			widths[column] = 68
		}

		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - 7GUIs Cells"),
			flux.Column(
				flux.CrossAxis(flux.CrossAxisStretch),
				flux.Row(
					flux.Text(cellName(snapshot.Selected), flux.Width(44)),
					flux.Expanded(flux.Input(
						flux.Bind(formula),
						flux.Key("formula"),
						flux.OnChange(func(value string) {
							formula.Set(value)
							next := sheet.Get().clone()
							next.setCell(next.Selected, value)
							sheet.Set(next)
						}),
					)),
					flux.Button("Clear cell", flux.OnClick(func(_ flux.Event) {
						next := sheet.Get().clone()
						next.setCell(next.Selected, "")
						sheet.Set(next)
						formula.Set("")
					})),
				),
				flux.Expanded(flux.StringGrid(snapshot.Rows, snapshot.Columns,
					flux.Key("sheet-grid"),
					flux.Headers(headers),
					flux.ColumnWidths(widths),
					flux.Cells(snapshot.Display),
					flux.SelectedCell(snapshot.Selected.row, snapshot.Selected.column),
					flux.Editable(true),
					flux.OnCellSelect(func(cell flux.GridCell) {
						selected := cellCoord{row: cell.Row, column: cell.Column}
						current := sheet.Get()
						if !current.valid(selected) {
							return
						}
						next := current.clone()
						next.Selected = selected
						next.Status = cellName(selected) + " selected"
						sheet.Set(next)
						formula.Set(next.Raw[selected.row][selected.column])
					}),
					flux.OnCellEdit(func(cell flux.GridCell, value string) {
						edited := cellCoord{row: cell.Row, column: cell.Column}
						next := sheet.Get().clone()
						if !next.valid(edited) {
							return
						}
						next.setCell(edited, value)
						sheet.Set(next)
						formula.Set(value)
					}),
				)),
				flux.Text(flux.Bind(sheet)),
			),
		)
	}); err != nil {
		fmt.Fprintln(os.Stderr, "挂载失败:", err)
		os.Exit(2)
	}

	native.Run()
}
