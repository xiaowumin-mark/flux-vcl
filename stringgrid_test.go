package flux_test

import (
	"reflect"
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

func gridValues(first string) [][]string {
	return [][]string{{first, "B1"}, {"A2", "B2"}}
}

func TestStringGridValidationAndDefensiveCopy(t *testing.T) {
	cells := gridValues("A1")
	headers := []string{"A", "B"}
	widths := []int{100, 120}
	node := flux.StringGrid(2, 2,
		flux.Cells(cells), flux.Headers(headers), flux.ColumnWidths(widths),
		flux.SelectedCell(1, 1), flux.Editable(true),
	).Create()
	cells[0][0], headers[0], widths[0] = "changed", "changed", 999

	storedCells, _ := node.Props.Get("Cells")
	storedHeaders, _ := node.Props.Get("Headers")
	storedWidths, _ := node.Props.Get("ColumnWidths")
	if got := storedCells.([][]string)[0][0]; got != "A1" {
		t.Fatalf("Cells 未防御性复制: %q", got)
	}
	if got := storedHeaders.([]string)[0]; got != "A" {
		t.Fatalf("Headers 未防御性复制: %q", got)
	}
	if got := storedWidths.([]int)[0]; got != 100 {
		t.Fatalf("ColumnWidths 未防御性复制: %d", got)
	}

	invalid := []struct {
		name string
		fn   func()
	}{
		{"negative rows", func() { _ = flux.StringGrid(-1, 1) }},
		{"zero columns", func() { _ = flux.StringGrid(1, 0) }},
		{"ragged cells", func() { _ = flux.StringGrid(2, 2, flux.Cells([][]string{{"a"}, {"b", "c"}})) }},
		{"wrong rows", func() { _ = flux.StringGrid(2, 1, flux.Cells([][]string{{"a"}})) }},
		{"wrong headers", func() { _ = flux.StringGrid(1, 2, flux.Headers([]string{"only"})) }},
		{"wrong widths", func() { _ = flux.StringGrid(1, 2, flux.ColumnWidths([]int{80})) }},
		{"non-positive width", func() { _ = flux.ColumnWidths([]int{0}) }},
		{"invalid selection", func() { _ = flux.StringGrid(1, 1, flux.SelectedCell(2, 0)) }},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("非法参数未 panic")
				}
			}()
			test.fn()
		})
	}
}

func TestStringGridMountPatchD7cAndPropertyOrder(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	build := func(first string, editable, rowOnly bool) flux.Widget {
		selection := flux.SelectedCell(0, 1)
		if rowOnly {
			selection = flux.SelectedRow(1)
		}
		return flux.Window(flux.StringGrid(2, 2,
			flux.Key("grid"), flux.Cells(gridValues(first)), flux.Headers([]string{"A", "B"}),
			flux.ColumnWidths([]int{90, 110}), selection, flux.Editable(editable),
		))
	}
	if err := app.Render(build("A1", false, false)); err != nil {
		t.Fatal(err)
	}
	target := findByKey(t, app.Root(), "grid")
	h := target.Handle
	size, headers, widths, cells, editable, selection := mock.GridSnapshot(h)
	if size != (render.GridSize{Rows: 2, Columns: 2}) ||
		!reflect.DeepEqual(headers, []string{"A", "B"}) ||
		!reflect.DeepEqual(widths, []int{90, 110}) ||
		!reflect.DeepEqual(cells, gridValues("A1")) || editable ||
		selection != (render.GridSelection{Cell: render.GridCell{Row: 0, Column: 1}}) {
		t.Fatalf("mount Grid 状态不符: size=%+v headers=%v widths=%v cells=%v editable=%v selection=%+v",
			size, headers, widths, cells, editable, selection)
	}
	if bounds, ok := target.Props.Get("Bounds"); !ok || bounds != (render.Rect{W: 360, H: 220}) {
		t.Fatalf("StringGrid intrinsic Bounds=%v", bounds)
	}

	wantOrder := []string{"GridSize", "Headers", "ColumnWidths", "Cells", "Editable", "GridSelection"}
	position := -1
	for _, key := range wantOrder {
		found := -1
		for index, op := range mock.Ops() {
			if op.Handle == h && op.Type == render.OpSetProperty && op.Key == key {
				found = index
				break
			}
		}
		if found <= position {
			t.Fatalf("Grid 属性应用顺序错误，%s index=%d after=%d", key, found, position)
		}
		position = found
	}

	base := len(mock.Ops())
	if err := app.Render(build("A1", false, false)); err != nil {
		t.Fatal(err)
	}
	if ops := mock.Ops()[base:]; len(ops) != 0 {
		t.Fatalf("D7c 相同 Grid 树产生 mutation: %+v", ops)
	}

	base = len(mock.Ops())
	if err := app.Render(build("changed", true, true)); err != nil {
		t.Fatal(err)
	}
	ops := mock.Ops()[base:]
	if countOps(ops, render.OpCreate)+countOps(ops, render.OpDestroy) != 0 {
		t.Fatalf("StringGrid patch 重建控件: %+v", ops)
	}
	if got := findByKey(t, app.Root(), "grid").Handle; got != h {
		t.Fatalf("StringGrid handle=%d，期望保持 %d", got, h)
	}
	_, _, _, cells, editable, selection = mock.GridSnapshot(h)
	if cells[0][0] != "changed" || !editable || !selection.RowOnly || selection.Cell.Row != 1 {
		t.Fatalf("patch 后状态不符: cells=%v editable=%v selection=%+v", cells, editable, selection)
	}
}

func TestStringGridStateWritebackAndEventUnbind(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	selected := flux.NewState(0)
	value := flux.NewState("A1")
	bound := true
	build := func() flux.Widget {
		opts := []flux.Opt{
			flux.Key("grid"), flux.Cells(gridValues(value.Get())), flux.Headers([]string{"A", "B"}),
			flux.SelectedRow(selected.Get()), flux.Editable(true),
		}
		if bound {
			opts = append(opts,
				flux.OnCellSelect(func(cell flux.GridCell) { selected.Set(cell.Row) }),
				flux.OnCellEdit(func(cell flux.GridCell, text string) {
					if cell == (flux.GridCell{Row: 0, Column: 0}) {
						value.Set(text)
					}
				}),
			)
		}
		return flux.Window(flux.Column(
			flux.Text(flux.Bind(selected)), flux.Text(flux.Bind(value)),
			flux.StringGrid(2, 2, opts...),
		))
	}
	if err := app.Mount(build); err != nil {
		t.Fatal(err)
	}
	h := findByKey(t, app.Root(), "grid").Handle
	mock.FireGridCellSelect(h, render.GridCell{Row: 1, Column: 1})
	if selected.Get() != 1 || findByKey(t, app.Root(), "grid").Handle != h {
		t.Fatalf("选择 State 未原地回写: selected=%d", selected.Get())
	}
	mock.FireGridCellEdit(h, render.GridCell{Row: 0, Column: 0}, "edited")
	if value.Get() != "edited" {
		t.Fatalf("编辑 State=%q，期望 edited", value.Get())
	}
	_, _, _, cells, _, _ := mock.GridSnapshot(h)
	if cells[0][0] != "edited" {
		t.Fatalf("受控 Cells 未收敛: %v", cells)
	}

	bound = false
	base := len(mock.Ops())
	if err := app.Render(build()); err != nil {
		t.Fatal(err)
	}
	ops := mock.Ops()[base:]
	if !hasOp(ops, render.OpSetEvent, h, "OnCellSelect", nil) ||
		!hasOp(ops, render.OpSetEvent, h, "OnCellEdit", nil) {
		t.Fatalf("Grid 事件未 nil 解绑: %+v", ops)
	}
	mock.FireGridCellSelect(h, render.GridCell{Row: 0, Column: 0})
	mock.FireGridCellEdit(h, render.GridCell{Row: 0, Column: 0}, "ignored")
	if selected.Get() != 1 || value.Get() != "edited" {
		t.Fatalf("解绑后旧回调仍执行: selected=%d value=%q", selected.Get(), value.Get())
	}
}

func TestStringGridMockBoundariesRemainDefensive(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	if err := app.Render(flux.Window(flux.StringGrid(1, 1,
		flux.Key("grid"), flux.Cells([][]string{{"owned"}}), flux.Headers([]string{"H"}),
	))); err != nil {
		t.Fatal(err)
	}
	h := findByKey(t, app.Root(), "grid").Handle
	_, headers, _, cells, _, _ := mock.GridSnapshot(h)
	headers[0], cells[0][0] = "mutated", "mutated"
	_, headers, _, cells, _, _ = mock.GridSnapshot(h)
	if headers[0] != "H" || cells[0][0] != "owned" {
		t.Fatalf("GridSnapshot 泄漏内部 slice: headers=%v cells=%v", headers, cells)
	}
	for _, op := range mock.Ops() {
		if op.Key == "Cells" {
			op.Value.([][]string)[0][0] = "mutated-op"
		}
	}
	_, _, _, cells, _, _ = mock.GridSnapshot(h)
	if cells[0][0] != "owned" {
		t.Fatalf("Ops 泄漏 Grid 内部 slice: %v", cells)
	}
}

type rendererWithoutGrid struct{ render.Renderer }

func TestStringGridCapabilityMissingSafelyDegrades(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(rendererWithoutGrid{Renderer: mock})
	if err := app.Render(flux.Window(flux.StringGrid(1, 1,
		flux.Cells([][]string{{"A"}}), flux.OnCellSelect(func(flux.GridCell) {}),
		flux.OnCellEdit(func(flux.GridCell, string) {}),
	))); err != nil {
		t.Fatal(err)
	}
}
