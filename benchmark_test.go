package flux_test

import (
	"fmt"
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// benchmarkRenderer 只计数 mutation，不保存日志，避免 Mock 的日志增长主导基准。
// 它实现当前控件使用的全部窄能力，使基准覆盖真实 diff 分发路径。
type benchmarkRenderer struct {
	next      render.Handle
	mutations uint64
}

func (r *benchmarkRenderer) mutation()                                          { r.mutations++ }
func (r *benchmarkRenderer) Create(string) render.Handle                        { r.next++; r.mutation(); return r.next }
func (r *benchmarkRenderer) Destroy(render.Handle)                              { r.mutation() }
func (r *benchmarkRenderer) SetParent(render.Handle, render.Handle)             { r.mutation() }
func (r *benchmarkRenderer) SetBounds(render.Handle, render.Rect)               { r.mutation() }
func (r *benchmarkRenderer) SetVisible(render.Handle, bool)                     { r.mutation() }
func (r *benchmarkRenderer) SetText(render.Handle, string)                      { r.mutation() }
func (r *benchmarkRenderer) SetEnabled(render.Handle, bool)                     { r.mutation() }
func (r *benchmarkRenderer) SetColor(render.Handle, render.Color)               { r.mutation() }
func (r *benchmarkRenderer) SetFontColor(render.Handle, render.Color)           { r.mutation() }
func (r *benchmarkRenderer) SetTitleBarDark(render.Handle, bool)                { r.mutation() }
func (r *benchmarkRenderer) NewTimer(int, func()) func()                        { return func() {} }
func (r *benchmarkRenderer) TextExtent(text string) (int, int)                  { return len(text) * 8, 20 }
func (r *benchmarkRenderer) ClientSize() (int, int)                             { return 1200, 900 }
func (r *benchmarkRenderer) OnResize(func(int, int))                            {}
func (r *benchmarkRenderer) SetEvent(render.Handle, string, any)                { r.mutation() }
func (r *benchmarkRenderer) AttachRef(render.Handle, render.Ref)                { r.mutation() }
func (r *benchmarkRenderer) ApplyNative(render.Handle, func(any))               { r.mutation() }
func (r *benchmarkRenderer) RunOnUI(fn func())                                  { fn() }
func (r *benchmarkRenderer) HandleAllocated(h render.Handle) bool               { return h != 0 }
func (r *benchmarkRenderer) SetChecked(render.Handle, bool)                     { r.mutation() }
func (r *benchmarkRenderer) OnCheckedChange(render.Handle, func(bool))          { r.mutation() }
func (r *benchmarkRenderer) SetItems(render.Handle, []string)                   { r.mutation() }
func (r *benchmarkRenderer) SetSelectedIndex(render.Handle, int)                { r.mutation() }
func (r *benchmarkRenderer) OnSelectionChange(render.Handle, func(int))         { r.mutation() }
func (r *benchmarkRenderer) SetMinimum(render.Handle, int)                      { r.mutation() }
func (r *benchmarkRenderer) SetMaximum(render.Handle, int)                      { r.mutation() }
func (r *benchmarkRenderer) SetValue(render.Handle, int)                        { r.mutation() }
func (r *benchmarkRenderer) SetGroupIndex(render.Handle, int)                   { r.mutation() }
func (r *benchmarkRenderer) SyncPages(render.Handle, []render.Handle)           { r.mutation() }
func (r *benchmarkRenderer) SetPageSelectedIndex(render.Handle, int)            { r.mutation() }
func (r *benchmarkRenderer) OnPageSelectionChange(render.Handle, func(int))     { r.mutation() }
func (r *benchmarkRenderer) SetScrollConfig(render.Handle, render.ScrollConfig) { r.mutation() }
func (r *benchmarkRenderer) SetScrollPos(render.Handle, int)                    { r.mutation() }
func (r *benchmarkRenderer) OnScroll(render.Handle, func(int))                  { r.mutation() }
func (r *benchmarkRenderer) SetSliderStep(render.Handle, int)                   { r.mutation() }
func (r *benchmarkRenderer) OnSliderValueChange(render.Handle, func(int))       { r.mutation() }
func (r *benchmarkRenderer) SetPaintCommands(render.Handle, []render.PaintCommand) {
	r.mutation()
}
func (r *benchmarkRenderer) InvalidatePaint(render.Handle) { r.mutation() }
func (r *benchmarkRenderer) SetGridSize(render.Handle, render.GridSize) {
	r.mutation()
}
func (r *benchmarkRenderer) SetGridHeaders(render.Handle, []string) { r.mutation() }
func (r *benchmarkRenderer) SetGridColumnWidths(render.Handle, []int) {
	r.mutation()
}
func (r *benchmarkRenderer) SetGridCells(render.Handle, [][]string) { r.mutation() }
func (r *benchmarkRenderer) SetGridEditable(render.Handle, bool)    { r.mutation() }
func (r *benchmarkRenderer) SetGridSelection(render.Handle, render.GridSelection) {
	r.mutation()
}
func (r *benchmarkRenderer) OnGridCellSelect(render.Handle, func(render.GridCell)) {
	r.mutation()
}
func (r *benchmarkRenderer) OnGridCellEdit(render.Handle, func(render.GridCell, string)) {
	r.mutation()
}

func benchmarkControlTree() flux.Widget {
	return flux.Window(
		flux.Title("benchmark"),
		flux.Column(
			flux.Text("label"),
			flux.Button("button"),
			flux.Input(),
			flux.Memo("memo"),
			flux.CheckBox("check", flux.Checked(true)),
			flux.RadioButton("radio", flux.Checked(true), flux.GroupIndex(1)),
			flux.ComboBox(flux.Items([]string{"a", "b"}), flux.SelectedIndex(1)),
			flux.ProgressBar(flux.Minimum(0), flux.Maximum(100), flux.Value(50)),
			flux.Slider(flux.Minimum(0), flux.Maximum(100), flux.Value(50), flux.Step(5)),
			flux.StringGrid(2, 2, flux.Headers([]string{"A", "B"}),
				flux.Cells([][]string{{"A1", "B1"}, {"A2", "B2"}})),
			flux.PaintBox([]flux.PaintCommand{{
				Kind: flux.PaintCircle, X: 20, Y: 20, Radius: 10, FillColor: flux.RGB(20, 80, 160),
			}}, flux.Width(120), flux.Height(80)),
			flux.ScrollBox(flux.Text("scroll content")),
			flux.PageControl(
				flux.TabPage("A", flux.Input(), flux.Key("page-a")),
				flux.TabPage("B", flux.Text("page"), flux.Key("page-b")),
				flux.SelectedIndex(0),
			),
			flux.Expanded(flux.ListView(100000, 20, func(index int) flux.Widget {
				return flux.Text(fmt.Sprintf("row %d", index))
			})),
		),
	)
}

// BenchmarkControlMount 衡量现有 native 控件全集的构建、布局和首次挂载。
func BenchmarkControlMount(b *testing.B) {
	b.ReportAllocs()
	var mutations uint64
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r := &benchmarkRenderer{}
		app := flux.NewApp(r)
		if err := app.Render(benchmarkControlTree()); err != nil {
			b.Fatal(err)
		}
		mutations += r.mutations
	}
	b.ReportMetric(float64(mutations)/float64(b.N), "mutations/op")
}

func benchmarkPatchTree(updated bool) flux.Widget {
	text := "before"
	value := 10
	if updated {
		text = "after"
		value = 20
	}
	return flux.Window(flux.Column(
		flux.Text(text, flux.Key("text")),
		flux.ProgressBar(flux.Key("progress"), flux.Value(value)),
	))
}

// BenchmarkControlPurePropertyPatch 衡量同 identity 控件的纯属性 patch。
func BenchmarkControlPurePropertyPatch(b *testing.B) {
	r := &benchmarkRenderer{}
	app := flux.NewApp(r)
	if err := app.Render(benchmarkPatchTree(false)); err != nil {
		b.Fatal(err)
	}
	startMutations := r.mutations
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := app.Render(benchmarkPatchTree(i%2 == 0)); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(r.mutations-startMutations)/float64(b.N), "mutations/op")
}

func benchmarkPageTree(selected int) flux.Widget {
	return flux.Window(flux.PageControl(
		flux.TabPage("A", flux.Input(flux.Key("input-a")), flux.Key("page-a")),
		flux.TabPage("B", flux.Input(flux.Key("input-b")), flux.Key("page-b")),
		flux.Key("pages"), flux.SelectedIndex(selected),
	))
}

// BenchmarkPageSwitch 衡量受控页切换；页面和页内控件均保持原 identity。
func BenchmarkPageSwitch(b *testing.B) {
	r := &benchmarkRenderer{}
	app := flux.NewApp(r)
	if err := app.Render(benchmarkPageTree(0)); err != nil {
		b.Fatal(err)
	}
	startMutations := r.mutations
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := app.Render(benchmarkPageTree((i + 1) % 2)); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(r.mutations-startMutations)/float64(b.N), "mutations/op")
}

func benchmarkListTree(offset *flux.State[int]) flux.Widget {
	return flux.Window(flux.Expanded(flux.ListView(100000, 20, func(index int) flux.Widget {
		return flux.Text(fmt.Sprintf("row %d", index))
	}, flux.Key("list"), flux.ScrollOffset(offset))))
}

// BenchmarkVirtualListScrollPatch 记录十万行列表中段滚动的控件池 patch 成本。
func BenchmarkVirtualListScrollPatch(b *testing.B) {
	r := &benchmarkRenderer{}
	app := flux.NewApp(r)
	offset := flux.NewState(5000)
	if err := app.Mount(func() flux.Widget { return benchmarkListTree(offset) }); err != nil {
		b.Fatal(err)
	}
	startMutations := r.mutations
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		value := 5000
		if i%2 == 0 {
			value = 5200
		}
		offset.Set(value)
	}
	b.ReportMetric(float64(r.mutations-startMutations)/float64(b.N), "mutations/op")
}

func benchmarkGridCells(updated bool) [][]string {
	cells := make([][]string, 100)
	for row := range cells {
		cells[row] = make([]string, 10)
		for column := range cells[row] {
			cells[row][column] = fmt.Sprintf("R%dC%d", row, column)
		}
	}
	if updated {
		cells[50][5] = "updated"
	}
	return cells
}

func benchmarkGridTree(updated bool) flux.Widget {
	return flux.Window(flux.StringGrid(100, 10,
		flux.Key("grid"), flux.Cells(benchmarkGridCells(updated)), flux.SelectedCell(50, 5),
	))
}

// BenchmarkStringGridUpdate 记录 1000 个受控单元格中一个值变化的深比较和原地 patch 成本。
func BenchmarkStringGridUpdate(b *testing.B) {
	r := &benchmarkRenderer{}
	app := flux.NewApp(r)
	if err := app.Render(benchmarkGridTree(false)); err != nil {
		b.Fatal(err)
	}
	startMutations := r.mutations
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := app.Render(benchmarkGridTree(i%2 == 0)); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(r.mutations-startMutations)/float64(b.N), "mutations/op")
}

func benchmarkPaintTree(radius int) flux.Widget {
	return flux.Window(flux.PaintBox([]flux.PaintCommand{{
		Kind: flux.PaintCircle, X: 80, Y: 80, Radius: radius,
		FillColor: flux.RGB(20, 80, 160),
	}}, flux.Key("paint")))
}

// BenchmarkPaintInvalidate 记录稳定绘制命令变化后更新快照并 invalidate 的成本。
func BenchmarkPaintInvalidate(b *testing.B) {
	r := &benchmarkRenderer{}
	app := flux.NewApp(r)
	if err := app.Render(benchmarkPaintTree(20)); err != nil {
		b.Fatal(err)
	}
	startMutations := r.mutations
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		radius := 20
		if i%2 == 0 {
			radius = 30
		}
		if err := app.Render(benchmarkPaintTree(radius)); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(r.mutations-startMutations)/float64(b.N), "mutations/op")
}

func benchmarkDrawList(opCount int) flux.DrawList {
	color := flux.RGB(20, 80, 160)
	ops := make([]flux.DrawOp, 0, opCount)
	for i := 0; i < opCount; i++ {
		ops = append(ops, flux.FillRect(flux.Rect{X: i, Y: i, W: 20, H: 20}, flux.FillStyle{Color: color}))
	}
	return flux.MustDrawList(ops...)
}

// BenchmarkDrawListBuildAndEqual records the CD1 value construction and
// equality cost independently of a native executor.
func BenchmarkDrawListBuildAndEqual(b *testing.B) {
	b.ReportAllocs()
	base := benchmarkDrawList(64)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		copy := base.Clone()
		if !base.Equal(copy) {
			b.Fatal("cloned DrawList is not equal")
		}
	}
}
