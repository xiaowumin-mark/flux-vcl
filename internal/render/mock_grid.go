package render

import "fmt"

type mockGrid struct {
	size       GridSize
	headers    []string
	widths     []int
	cells      [][]string
	editable   bool
	selection  GridSelection
	onSelect   func(GridCell)
	onCellEdit func(GridCell, string)
}

func (m *Mock) ensureGridLocked(h Handle) *mockGrid {
	if m.grids == nil {
		m.grids = make(map[Handle]*mockGrid)
	}
	if m.grids[h] == nil {
		m.grids[h] = &mockGrid{
			size: GridSize{Columns: 1}, cells: [][]string{},
			selection: GridSelection{Cell: GridCell{Row: -1, Column: -1}},
		}
	}
	return m.grids[h]
}

func emptyGridCells(size GridSize) [][]string {
	out := make([][]string, size.Rows)
	for row := range out {
		out[row] = make([]string, size.Columns)
	}
	return out
}

// SetGridSize 设置 StringGrid 的逻辑行列数，并重置不再有效的单元格与选择。
func (m *Mock) SetGridSize(h Handle, size GridSize) {
	if size.Rows < 0 || size.Columns <= 0 {
		panic(fmt.Sprintf("render.Mock: invalid grid size %+v", size))
	}
	m.mu.Lock()
	g := m.ensureGridLocked(h)
	g.size = size
	g.cells = emptyGridCells(size)
	if size.Rows == 0 {
		g.selection = GridSelection{Cell: GridCell{Row: -1, Column: -1}}
	} else if !ValidGridSelection(size, g.selection) {
		g.selection = GridSelection{Cell: GridCell{}}
	}
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "GridSize", Value: size})
	m.mu.Unlock()
}

// SetGridHeaders 设置 StringGrid 表头，并防御性复制输入。
func (m *Mock) SetGridHeaders(h Handle, headers []string) {
	m.mu.Lock()
	g := m.ensureGridLocked(h)
	if len(headers) != 0 && len(headers) != g.size.Columns {
		m.mu.Unlock()
		panic("render.Mock: Headers 长度与 GridSize 不一致")
	}
	g.headers = append([]string(nil), headers...)
	if len(g.headers) == 0 {
		g.headers = []string{}
	}
	value := append([]string(nil), g.headers...)
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Headers", Value: value})
	m.mu.Unlock()
}

// SetGridColumnWidths 设置 StringGrid 列宽，并防御性复制输入。
func (m *Mock) SetGridColumnWidths(h Handle, widths []int) {
	m.mu.Lock()
	g := m.ensureGridLocked(h)
	if len(widths) != 0 && len(widths) != g.size.Columns {
		m.mu.Unlock()
		panic("render.Mock: ColumnWidths 长度与 GridSize 不一致")
	}
	for _, width := range widths {
		if width <= 0 {
			m.mu.Unlock()
			panic("render.Mock: ColumnWidths 必须 > 0")
		}
	}
	g.widths = append([]int(nil), widths...)
	if len(g.widths) == 0 {
		g.widths = []int{}
	}
	value := append([]int(nil), g.widths...)
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "ColumnWidths", Value: value})
	m.mu.Unlock()
}

// SetGridCells 设置 StringGrid 单元格，并深复制二维输入。
func (m *Mock) SetGridCells(h Handle, cells [][]string) {
	m.mu.Lock()
	g := m.ensureGridLocked(h)
	if err := ValidateGridCells(g.size, cells); err != nil {
		m.mu.Unlock()
		panic("render.Mock: " + err.Error())
	}
	g.cells = CloneGridCells(cells)
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Cells", Value: CloneGridCells(cells)})
	m.mu.Unlock()
}

// SetGridEditable 设置 StringGrid 是否允许用户编辑。
func (m *Mock) SetGridEditable(h Handle, editable bool) {
	m.mu.Lock()
	g := m.ensureGridLocked(h)
	g.editable = editable
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Editable", Value: editable})
	m.mu.Unlock()
}

// SetGridSelection 设置 StringGrid 的受控选择状态。
func (m *Mock) SetGridSelection(h Handle, selection GridSelection) {
	m.mu.Lock()
	g := m.ensureGridLocked(h)
	if !ValidGridSelection(g.size, selection) {
		m.mu.Unlock()
		panic("render.Mock: GridSelection 越界")
	}
	g.selection = selection
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "GridSelection", Value: selection})
	m.mu.Unlock()
}

// OnGridCellSelect 绑定 StringGrid 的用户选择回调。
func (m *Mock) OnGridCellSelect(h Handle, fn func(GridCell)) {
	m.mu.Lock()
	g := m.ensureGridLocked(h)
	g.onSelect = fn
	m.ops = append(m.ops, Op{Type: OpSetEvent, Handle: h, Key: "OnCellSelect", Value: fn})
	m.mu.Unlock()
}

// OnGridCellEdit 绑定 StringGrid 的用户编辑提交回调。
func (m *Mock) OnGridCellEdit(h Handle, fn func(GridCell, string)) {
	m.mu.Lock()
	g := m.ensureGridLocked(h)
	g.onCellEdit = fn
	m.ops = append(m.ops, Op{Type: OpSetEvent, Handle: h, Key: "OnCellEdit", Value: fn})
	m.mu.Unlock()
}

// GridSnapshot 返回 StringGrid Mock 状态的防御性副本。
func (m *Mock) GridSnapshot(h Handle) (GridSize, []string, []int, [][]string, bool, GridSelection) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g := m.ensureGridLocked(h)
	return g.size, append([]string(nil), g.headers...), append([]int(nil), g.widths...),
		CloneGridCells(g.cells), g.editable, g.selection
}

// FireGridCellSelect 模拟用户选择，回调在锁外执行。
func (m *Mock) FireGridCellSelect(h Handle, cell GridCell) {
	m.mu.Lock()
	g := m.ensureGridLocked(h)
	selection := GridSelection{Cell: cell, RowOnly: g.selection.RowOnly}
	if !ValidGridSelection(g.size, selection) {
		m.mu.Unlock()
		return
	}
	g.selection = selection
	fn := g.onSelect
	m.mu.Unlock()
	if fn != nil {
		fn(cell)
	}
}

// FireGridCellEdit 模拟已提交的用户编辑，回调在锁外执行。
func (m *Mock) FireGridCellEdit(h Handle, cell GridCell, value string) {
	m.mu.Lock()
	g := m.ensureGridLocked(h)
	if !ValidGridSelection(g.size, GridSelection{Cell: cell}) || !g.editable {
		m.mu.Unlock()
		return
	}
	fn := g.onCellEdit
	m.mu.Unlock()
	if fn != nil {
		fn(cell, value)
	}
}
