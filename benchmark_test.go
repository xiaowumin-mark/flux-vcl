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
