package flux_test

import (
	"fmt"
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

func batch3Control(kind string, opts ...flux.Opt) flux.Widget {
	switch kind {
	case "Slider":
		return flux.Slider(opts...)
	case "StringGrid":
		return flux.StringGrid(2, 2, append(opts,
			flux.Cells([][]string{{"A1", "B1"}, {"A2", "B2"}}),
			flux.Headers([]string{"A", "B"}),
		)...)
	case "PaintBox":
		return flux.PaintBox([]flux.PaintCommand{{
			Kind: flux.PaintClear, Color: flux.RGB(255, 255, 255),
		}}, opts...)
	default:
		panic("unknown batch 3 control: " + kind)
	}
}

func TestBatch3ExplicitSizeAndResizeMatrix(t *testing.T) {
	for _, kind := range []string{"Slider", "StringGrid", "PaintBox"} {
		t.Run(kind, func(t *testing.T) {
			explicit := render.NewMock()
			explicit.SetClientSize(640, 480)
			app := flux.NewApp(explicit)
			if err := app.Render(flux.Window(batch3Control(kind,
				flux.Key("subject"), flux.Width(240), flux.Height(90),
			))); err != nil {
				t.Fatal(err)
			}
			if got := boundsOf(findByKey(t, app.Root(), "subject")); got != (render.Rect{W: 240, H: 90}) {
				t.Fatalf("显式尺寸=%+v，期望 240x90 DIP", got)
			}

			resized := render.NewMock()
			app = flux.NewApp(resized)
			if err := app.Mount(func() flux.Widget {
				return flux.Window(flux.Column(
					flux.CrossAxis(flux.CrossAxisStretch),
					flux.Expanded(batch3Control(kind, flux.Key("subject"))),
				))
			}); err != nil {
				t.Fatal(err)
			}
			target := findByKey(t, app.Root(), "subject")
			handle := target.Handle
			if got := boundsOf(target); got != (render.Rect{W: 400, H: 300}) {
				t.Fatalf("初始紧约束=%+v，期望 400x300 DIP", got)
			}
			base := len(resized.Ops())
			resized.TriggerResize(640, 360)
			if got := boundsOf(findByKey(t, app.Root(), "subject")); got != (render.Rect{W: 640, H: 360}) {
				t.Fatalf("resize 后 bounds=%+v，期望 640x360 DIP", got)
			}
			if findByKey(t, app.Root(), "subject").Handle != handle {
				t.Fatal("resize 重建了原生控件")
			}
			ops := resized.Ops()[base:]
			if countOps(ops, render.OpCreate)+countOps(ops, render.OpDestroy) != 0 {
				t.Fatalf("resize 产生 create/destroy: %+v", ops)
			}
		})
	}
}

func TestBatch3ContainerClientAreaAndOverflowMatrix(t *testing.T) {
	for _, kind := range []string{"Slider", "StringGrid", "PaintBox"} {
		t.Run(kind+"/client-area", func(t *testing.T) {
			mock := render.NewMock()
			app := flux.NewApp(mock)
			if err := app.Render(flux.Window(flux.PageControl(
				flux.TabPage("Page", flux.Column(
					flux.CrossAxis(flux.CrossAxisStretch),
					flux.Expanded(batch3Control(kind, flux.Key("subject"))),
				), flux.Key("page")),
			))); err != nil {
				t.Fatal(err)
			}
			target := findByKey(t, app.Root(), "subject")
			if got := boundsOf(target); got != (render.Rect{W: 312, H: 188}) {
				t.Fatalf("PageControl 客户区 bounds=%+v，期望 312x188 DIP", got)
			}
			page := findByKey(t, app.Root(), "page")
			if parent := mock.InspectNative()[target.Handle].Parent; parent != page.Handle {
				t.Fatalf("native parent=%d，期望 TabPage %d", parent, page.Handle)
			}
		})

		t.Run(kind+"/overflow", func(t *testing.T) {
			mock := render.NewMock()
			mock.SetClientSize(200, 100)
			app := flux.NewApp(mock)
			count := 2
			if kind == "Slider" {
				count = 4
			}
			children := make([]any, 0, count)
			for i := 0; i < count; i++ {
				children = append(children, batch3Control(kind, flux.Key(fmt.Sprintf("subject-%d", i))))
			}
			if err := app.Render(flux.Window(flux.Column(children...))); err != nil {
				t.Fatal(err)
			}
			found := false
			for _, diag := range app.LastLayoutDiags() {
				if diag.Type == "Column" && diag.OverflowH > 0 {
					found = true
				}
			}
			if !found {
				t.Fatalf("缺少 %s 的纵向溢出诊断: %+v", kind, app.LastLayoutDiags())
			}
			last := boundsOf(findByKey(t, app.Root(), fmt.Sprintf("subject-%d", count-1)))
			if last.W < 0 || last.H < 0 || last.Y+last.H <= 100 {
				t.Fatalf("溢出节点 bounds=%+v，期望保持非负 intrinsic 并越过客户区", last)
			}
		})
	}
}
