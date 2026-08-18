package diff_test

import (
	"testing"

	"github.com/xiaowumin-mark/flux-vcl/internal/diff"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
	"github.com/xiaowumin-mark/flux-vcl/internal/widget"
)

func sliderNode(configured bool, calls *int) *widget.Node {
	n := widget.NewNode("Slider")
	n.Key = "slider"
	if configured {
		n.Props.Set("Minimum", 10)
		n.Props.Set("Maximum", 90)
		n.Props.Set("Step", 5)
		n.Props.Set("Value", 40)
		n.Props.Set("OnValueChange", func(int) { *calls++ })
	}
	return n
}

func TestSliderPropertyAndEventRemovalResetsDefaults(t *testing.T) {
	mock := render.NewMock()
	rc := diff.New(mock)
	calls := 0
	rc.Render(widget.NewNode("Window").Add(sliderNode(true, &calls)))
	h := findByKey(rc.Root(), "slider").Handle
	mock.FireValueChange(h, 50)
	if calls != 1 {
		t.Fatalf("mount event calls=%d，期望 1", calls)
	}

	ops := rc.Render(widget.NewNode("Window").Add(sliderNode(false, &calls)))
	for _, op := range ops {
		if op.Type == render.OpCreate || op.Type == render.OpDestroy {
			t.Fatalf("Slider 属性移除重建控件: %+v", ops)
		}
	}
	minimum, maximum, value := mock.Progress(h)
	if minimum != 0 || maximum != 100 || value != 0 || mock.SliderStep(h) != 1 {
		t.Fatalf("Slider 移除后=%d/%d/%d step=%d", minimum, maximum, value, mock.SliderStep(h))
	}
	mock.FireValueChange(h, 60)
	if calls != 1 {
		t.Fatalf("OnValueChange 移除后仍执行: %d", calls)
	}
}

func gridNode(configured bool, selectCalls, editCalls *int) *widget.Node {
	n := widget.NewNode("StringGrid")
	n.Key = "grid"
	n.Props.Set("GridSize", render.GridSize{Rows: 2, Columns: 2})
	if configured {
		n.Props.Set("Headers", []string{"A", "B"})
		n.Props.Set("ColumnWidths", []int{80, 100})
		n.Props.Set("Cells", [][]string{{"A1", "B1"}, {"A2", "B2"}})
		n.Props.Set("Editable", true)
		n.Props.Set("GridSelection", render.GridSelection{
			Cell: render.GridCell{Row: 1, Column: 1}, RowOnly: true,
		})
		n.Props.Set("OnCellSelect", func(render.GridCell) { *selectCalls++ })
		n.Props.Set("OnCellEdit", func(render.GridCell, string) { *editCalls++ })
	}
	return n
}

func TestStringGridPropertyAndEventRemovalResetsDefaults(t *testing.T) {
	mock := render.NewMock()
	rc := diff.New(mock)
	selectCalls, editCalls := 0, 0
	rc.Render(widget.NewNode("Window").Add(gridNode(true, &selectCalls, &editCalls)))
	h := findByKey(rc.Root(), "grid").Handle
	mock.FireGridCellSelect(h, render.GridCell{Row: 0, Column: 1})
	mock.FireGridCellEdit(h, render.GridCell{Row: 0, Column: 0}, "edit")
	if selectCalls != 1 || editCalls != 1 {
		t.Fatalf("mount events select=%d edit=%d", selectCalls, editCalls)
	}

	ops := rc.Render(widget.NewNode("Window").Add(gridNode(false, &selectCalls, &editCalls)))
	for _, op := range ops {
		if op.Type == render.OpCreate || op.Type == render.OpDestroy {
			t.Fatalf("StringGrid 属性移除重建控件: %+v", ops)
		}
	}
	size, headers, widths, cells, editable, selection := mock.GridSnapshot(h)
	if size != (render.GridSize{Rows: 2, Columns: 2}) || len(headers) != 0 || len(widths) != 0 || editable ||
		selection != (render.GridSelection{Cell: render.GridCell{Row: 0, Column: 0}}) {
		t.Fatalf("Grid 移除后状态不符: size=%+v headers=%v widths=%v editable=%v selection=%+v",
			size, headers, widths, editable, selection)
	}
	for row := range cells {
		for column := range cells[row] {
			if cells[row][column] != "" {
				t.Fatalf("Cells 移除后未清空: %v", cells)
			}
		}
	}
	mock.FireGridCellSelect(h, render.GridCell{Row: 1, Column: 1})
	mock.FireGridCellEdit(h, render.GridCell{Row: 0, Column: 0}, "ignored")
	if selectCalls != 1 || editCalls != 1 {
		t.Fatalf("Grid 事件移除后仍执行: select=%d edit=%d", selectCalls, editCalls)
	}
}

func TestStringGridSizeRemovalUsesSafeEmptyDefault(t *testing.T) {
	mock := render.NewMock()
	rc := diff.New(mock)
	configured := widget.NewNode("StringGrid")
	configured.Key = "grid"
	configured.Props.Set("GridSize", render.GridSize{Rows: 3, Columns: 2})
	configured.Props.Set("Headers", []string{"A", "B"})
	configured.Props.Set("ColumnWidths", []int{80, 100})
	configured.Props.Set("Cells", [][]string{{"A1", "B1"}, {"A2", "B2"}, {"A3", "B3"}})
	configured.Props.Set("GridSelection", render.GridSelection{Cell: render.GridCell{Row: 2, Column: 1}})
	rc.Render(widget.NewNode("Window").Add(configured))
	h := findByKey(rc.Root(), "grid").Handle

	empty := widget.NewNode("StringGrid")
	empty.Key = "grid"
	rc.Render(widget.NewNode("Window").Add(empty))
	size, _, _, cells, _, selection := mock.GridSnapshot(h)
	if size != (render.GridSize{Columns: 1}) || len(cells) != 0 ||
		selection.Cell != (render.GridCell{Row: -1, Column: -1}) {
		t.Fatalf("GridSize 移除后 size=%+v cells=%v selection=%+v", size, cells, selection)
	}
}

func TestStringGridSizeChangeBeforeDependentRemoval(t *testing.T) {
	mock := render.NewMock()
	rc := diff.New(mock)
	configured := widget.NewNode("StringGrid")
	configured.Key = "grid"
	configured.Props.Set("GridSize", render.GridSize{Rows: 2, Columns: 2})
	configured.Props.Set("Cells", [][]string{{"A1", "B1"}, {"A2", "B2"}})
	configured.Props.Set("GridSelection", render.GridSelection{Cell: render.GridCell{Row: 1, Column: 1}})
	rc.Render(widget.NewNode("Window").Add(configured))
	h := findByKey(rc.Root(), "grid").Handle

	resized := widget.NewNode("StringGrid")
	resized.Key = "grid"
	resized.Props.Set("GridSize", render.GridSize{Rows: 3, Columns: 3})
	rc.Render(widget.NewNode("Window").Add(resized))

	size, _, _, cells, _, selection := mock.GridSnapshot(h)
	if size != (render.GridSize{Rows: 3, Columns: 3}) ||
		selection != (render.GridSelection{Cell: render.GridCell{Row: 0, Column: 0}}) {
		t.Fatalf("组合更新后 size=%+v selection=%+v", size, selection)
	}
	if len(cells) != 3 || len(cells[0]) != 3 {
		t.Fatalf("组合更新后 Cells 维度=%v", cells)
	}
	for row := range cells {
		for column := range cells[row] {
			if cells[row][column] != "" {
				t.Fatalf("组合更新后 Cells 未清空: %v", cells)
			}
		}
	}
}
