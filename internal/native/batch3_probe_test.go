//go:build windows && !race

package native

import (
	"testing"

	"github.com/energye/lcl/lcl"
	"github.com/energye/lcl/types"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// TestBatch3NativeProbe 验证锁定 LCL v1.0.3 的 Slider、StringGrid 和 PaintBox
// setter 能力，并确认程序化受控写入不会被当作用户事件回传。
func TestBatch3NativeProbe(t *testing.T) {
	if err := Init(radioProbeDLL(t)); err != nil {
		t.Fatal(err)
	}
	r := NewRenderer()
	parent := r.Create("Window")

	inputHandle := r.Create("Input")
	r.SetParent(inputHandle, parent)
	textEvents := 0
	r.SetEvent(inputHandle, "OnChange", func(string) { textEvents++ })
	r.SetText(inputHandle, "controlled")
	input, ok := r.controls[inputHandle].(lcl.ICustomEdit)
	if !ok {
		t.Fatalf("Input 未映射到 ICustomEdit: %T", r.controls[inputHandle])
	}
	if input.Text() != "controlled" {
		t.Fatalf("Input native 文本=%q，期望 controlled", input.Text())
	}
	if textEvents != 0 {
		t.Fatalf("Input 程序化 SetText 触发了 %d 次用户事件", textEvents)
	}
	input.SetText("user")
	if textEvents != 1 {
		t.Fatalf("Input 原生文本变化事件=%d，期望 1", textEvents)
	}

	sliderHandle := r.Create("Slider")
	r.SetParent(sliderHandle, parent)
	sliderEvents := 0
	r.OnSliderValueChange(sliderHandle, func(int) { sliderEvents++ })
	r.SetMinimum(sliderHandle, 10)
	r.SetMaximum(sliderHandle, 90)
	r.SetSliderStep(sliderHandle, 5)
	r.SetValue(sliderHandle, 40)
	slider, ok := r.controls[sliderHandle].(lcl.ITrackBar)
	if !ok {
		t.Fatal("Slider 未映射到 ITrackBar")
	}
	if slider.Min() != 10 || slider.Max() != 90 || slider.LineSize() != 5 || slider.Position() != 40 {
		t.Fatalf("Slider native 状态不符: min=%d max=%d step=%d value=%d",
			slider.Min(), slider.Max(), slider.LineSize(), slider.Position())
	}
	if sliderEvents != 0 {
		t.Fatalf("Slider 程序化 patch 触发了 %d 次用户事件", sliderEvents)
	}
	r.SetMinimum(sliderHandle, 200)
	r.SetMaximum(sliderHandle, 200)
	r.SetValue(sliderHandle, 200)
	if slider.Min() != 200 || slider.Max() != 200 || slider.Position() != 200 {
		t.Fatalf("Slider 高于原生默认范围的收敛值不符: min=%d max=%d value=%d",
			slider.Min(), slider.Max(), slider.Position())
	}
	if sliderEvents != 0 {
		t.Fatalf("Slider 高范围程序化 patch 触发了 %d 次用户事件", sliderEvents)
	}

	gridHandle := r.Create("StringGrid")
	r.SetParent(gridHandle, parent)
	selectEvents, editEvents := 0, 0
	selectedCell := render.GridCell{Row: -1, Column: -1}
	r.OnGridCellSelect(gridHandle, func(cell render.GridCell) {
		selectEvents++
		selectedCell = cell
	})
	if r.gridPollStop == nil {
		t.Fatal("Grid 选择回调未启动 Renderer 级轮询器")
	}
	r.OnGridCellEdit(gridHandle, func(render.GridCell, string) { editEvents++ })
	if r.grids[gridHandle].onEdit == nil {
		t.Fatal("Grid 原生编辑提交桥未注册")
	}
	r.SetGridSize(gridHandle, render.GridSize{Rows: 2, Columns: 2})
	r.SetGridHeaders(gridHandle, []string{"A", "B"})
	r.SetGridColumnWidths(gridHandle, []int{80, 100})
	r.SetGridCells(gridHandle, [][]string{{"A1", "B1"}, {"A2", "B2"}})
	r.SetGridEditable(gridHandle, true)
	r.SetGridSelection(gridHandle, render.GridSelection{
		Cell: render.GridCell{Row: 1, Column: 1}, RowOnly: true,
	})
	grid, ok := r.controls[gridHandle].(lcl.IStringGrid)
	if !ok {
		t.Fatal("StringGrid 未映射到 IStringGrid")
	}
	if grid.ColCount() != 2 || grid.RowCount() != 3 || grid.FixedCols() != 0 || grid.FixedRows() != 1 {
		t.Fatalf("Grid native shape=%dx%d fixed=%d/%d，期望 2x3 fixed=0/1",
			grid.ColCount(), grid.RowCount(), grid.FixedCols(), grid.FixedRows())
	}
	if grid.Cells(0, 0) != "A" || grid.Cells(1, 2) != "B2" || grid.Col() != 1 || grid.Row() != 2 {
		t.Fatalf("Grid native 内容/选择不符: header=%q cell=%q col=%d row=%d",
			grid.Cells(0, 0), grid.Cells(1, 2), grid.Col(), grid.Row())
	}
	if !grid.Options().In(int32(types.GoEditing)) {
		t.Fatal("Grid Editable 未启用 GoEditing")
	}
	if !grid.Options().In(int32(types.GoRowSelect)) {
		t.Fatal("Grid RowOnly 未启用 GoRowSelect")
	}
	if selectEvents != 0 || editEvents != 0 {
		t.Fatalf("Grid 程序化 patch 触发用户事件: select=%d edit=%d", selectEvents, editEvents)
	}
	r.grids[gridHandle].pending = &gridEdit{cell: render.GridCell{}, value: "stale"}
	r.SetGridSelection(gridHandle, render.GridSelection{
		Cell: render.GridCell{Row: 1, Column: 1}, RowOnly: true,
	})
	if r.grids[gridHandle].pending != nil || editEvents != 0 {
		t.Fatal("Grid 程序化 patch 未丢弃原生编辑器的陈旧待提交值")
	}
	grid.SetColRow(types.Point(1, 1))
	if selectEvents != 1 || selectedCell != (render.GridCell{Row: 0, Column: 1}) {
		t.Fatalf("Grid 原生选择未映射到逻辑坐标: calls=%d cell=%+v", selectEvents, selectedCell)
	}
	r.OnGridCellSelect(gridHandle, nil)
	if r.gridPollStop != nil {
		t.Fatal("Grid 最后一个选择回调解绑后未停止 Renderer 级轮询器")
	}
	firstPollTimer := r.gridPollTimer
	if firstPollTimer == nil || firstPollTimer.Enabled() {
		t.Fatal("Grid 空闲轮询器应保留单个禁用实例供后续复用")
	}
	grid.SetColRow(types.Point(1, 1))
	if selectEvents != 1 {
		t.Fatalf("Grid 选择事件解除后仍执行: %d", selectEvents)
	}
	r.OnGridCellEdit(gridHandle, nil)
	if r.grids[gridHandle].onEdit != nil {
		t.Fatal("Grid 原生编辑提交桥未解除")
	}
	if editEvents != 0 {
		t.Fatalf("Grid 编辑事件解绑过程中触发回调: %d", editEvents)
	}

	r.OnGridCellSelect(gridHandle, func(render.GridCell) {})
	if r.gridPollTimer != firstPollTimer || !r.gridPollTimer.Enabled() {
		t.Fatal("Grid 重新绑定选择事件时未复用并启用 Renderer 级轮询器")
	}
	secondGrid := r.Create("StringGrid")
	r.SetParent(secondGrid, parent)
	r.OnGridCellSelect(secondGrid, func(render.GridCell) {})

	paintHandle := r.Create("PaintBox")
	r.SetParent(paintHandle, parent)
	commands := []render.PaintCommand{{
		Kind: render.PaintCircle, X: 20, Y: 20, Radius: 10,
		FillColor: render.Color(0xFF336699),
	}}
	r.SetPaintCommands(paintHandle, commands)
	commands[0].Radius = 99
	r.InvalidatePaint(paintHandle)
	paint := r.paints[paintHandle]
	if paint == nil || len(paint.commands) != 1 || paint.commands[0].Radius != 10 {
		t.Fatalf("PaintBox native 命令快照未防御复制: %+v", paint)
	}

	// 布局对外保持 DIP；默认后端只在 SetBounds 边界按窗口 DPI 转换为物理像素。
	r.dpi = 144
	r.refreshDPISensitiveControls()
	if grid.ColWidths(0) != 120 || grid.ColWidths(1) != 150 {
		t.Fatalf("Grid 144 DPI 列宽=%d/%d，期望 120/150", grid.ColWidths(0), grid.ColWidths(1))
	}
	for _, handle := range []render.Handle{sliderHandle, gridHandle, paintHandle} {
		control := r.controls[handle]
		r.SetBounds(handle, render.Rect{X: 10, Y: 20, W: 120, H: 80})
		if control.Left() != 15 || control.Top() != 30 || control.Width() != 180 || control.Height() != 120 {
			t.Fatalf("控件 %d 的 144 DPI bounds=(%d,%d %dx%d)，期望 (15,30 180x120)",
				handle, control.Left(), control.Top(), control.Width(), control.Height())
		}
		if r.controls[handle] != control {
			t.Fatalf("控件 %d 在 SetBounds/invalidate 后被重建", handle)
		}
	}

	r.Destroy(gridHandle)
	if r.gridPollStop == nil {
		t.Fatal("销毁一个 Grid 时误停了仍有订阅者的轮询器")
	}
	r.Destroy(secondGrid)
	if r.gridPollStop != nil {
		t.Fatal("销毁最后一个 Grid 后未停止轮询器")
	}
	if r.gridPollTimer == nil || r.gridPollTimer.Enabled() {
		t.Fatal("销毁最后一个 Grid 后轮询器未禁用")
	}
	r.releaseGridSelectionPoller()
	if r.gridPollStop != nil || r.gridPollTimer != nil {
		t.Fatal("Renderer 关闭路径未释放 Grid 轮询器引用")
	}
}
