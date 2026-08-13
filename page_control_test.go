package flux_test

import (
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

type pageRawWidget struct{ node *flux.Node }

func (w pageRawWidget) Create() *flux.Node { return w.node }

func TestPageControlMountOperationOrder(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	if err := app.Render(flux.Window(flux.PageControl(
		flux.TabPage("A", flux.Input(flux.Key("input-a")), flux.Key("a")),
	))); err != nil {
		t.Fatal(err)
	}

	pages := findByType(t, app.Root(), "PageControl")
	page := findByKey(t, app.Root(), "a")
	input := findByKey(t, app.Root(), "input-a")
	ops := mock.Ops()
	pageAttach := pageOpIndex(ops, func(op render.Op) bool {
		return op.Type == render.OpAppendChild && op.Handle == page.Handle && op.Parent == pages.Handle
	})
	inputCreate := pageOpIndex(ops, func(op render.Op) bool {
		return op.Type == render.OpCreate && op.Handle == input.Handle
	})
	pagesSync := pageOpIndex(ops, func(op render.Op) bool {
		return op.Type == render.OpSetProperty && op.Handle == pages.Handle && op.Key == "Pages"
	})
	selection := pageOpIndex(ops, func(op render.Op) bool {
		return op.Type == render.OpSetProperty && op.Handle == pages.Handle && op.Key == "SelectedIndex"
	})
	if pageAttach < 0 || inputCreate < 0 || pageAttach >= inputCreate {
		t.Fatalf("TabPage 必须先 attach 再创建页内控件: attach=%d input-create=%d ops=%+v", pageAttach, inputCreate, ops)
	}
	if pagesSync < 0 || selection < 0 || pagesSync >= selection {
		t.Fatalf("页面顺序必须先同步再应用选择: pages=%d selected=%d ops=%+v", pagesSync, selection, ops)
	}
}

func TestPageControlReorderAppliesSelectionToFinalOrder(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	build := func(index int, keys ...string) flux.Widget {
		args := make([]any, 0, len(keys)+1)
		for _, key := range keys {
			args = append(args, flux.TabPage(key, flux.Input(flux.Key("input-"+key)), flux.Key(key)))
		}
		args = append(args, flux.SelectedIndex(index))
		return flux.Window(flux.PageControl(args...))
	}
	if err := app.Render(build(0, "a", "b", "c")); err != nil {
		t.Fatal(err)
	}
	control := findByType(t, app.Root(), "PageControl")
	pageA := findByKey(t, app.Root(), "a").Handle
	base := len(mock.Ops())
	if err := app.Render(build(2, "c", "b", "a")); err != nil {
		t.Fatal(err)
	}

	ops := mock.Ops()[base:]
	pagesSync := pageOpIndex(ops, func(op render.Op) bool {
		return op.Type == render.OpSetProperty && op.Handle == control.Handle && op.Key == "Pages"
	})
	selection := pageOpIndex(ops, func(op render.Op) bool {
		return op.Type == render.OpSetProperty && op.Handle == control.Handle && op.Key == "SelectedIndex"
	})
	if pagesSync < 0 || selection < 0 || pagesSync >= selection {
		t.Fatalf("重排与索引 patch 顺序错误: pages=%d selected=%d ops=%+v", pagesSync, selection, ops)
	}
	if countOps(ops, render.OpCreate)+countOps(ops, render.OpDestroy) != 0 {
		t.Fatalf("keyed 重排不应重建: %+v", ops)
	}
	pages := mock.Pages(control.Handle)
	if len(pages) != 3 || pages[2] != pageA || mock.PageSelectedIndex(control.Handle) != 2 {
		t.Fatalf("选择未应用到最终页面顺序: pages=%v selected=%d", pages, mock.PageSelectedIndex(control.Handle))
	}
}

func TestPageControlControlledSelectionReappliesAfterUserEvent(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	callbackCount := 0
	build := func() flux.Widget {
		return flux.Window(flux.PageControl(
			flux.TabPage("A", flux.Text("a"), flux.Key("a")),
			flux.TabPage("B", flux.Text("b"), flux.Key("b")),
			flux.SelectedIndex(0),
			flux.OnSelectionChange(func(int) { callbackCount++ }),
		))
	}
	if err := app.Render(build()); err != nil {
		t.Fatal(err)
	}
	control := findByType(t, app.Root(), "PageControl")
	mock.FirePageSelectionChange(control.Handle, 1)
	if callbackCount != 1 || mock.PageSelectedIndex(control.Handle) != 1 {
		t.Fatalf("用户选择未进入受控回调: callbacks=%d selected=%d", callbackCount, mock.PageSelectedIndex(control.Handle))
	}

	base := len(mock.Ops())
	if err := app.Render(build()); err != nil {
		t.Fatal(err)
	}
	ops := mock.Ops()[base:]
	if got := mock.PageSelectedIndex(control.Handle); got != 0 {
		t.Fatalf("同值受控索引未在下一次 render 重施: selected=%d ops=%+v", got, ops)
	}
	if !hasOp(ops, render.OpSetProperty, control.Handle, "SelectedIndex", 0) {
		t.Fatalf("缺少受控 SelectedIndex 重施 mutation: %+v", ops)
	}
	if countOps(ops, render.OpCreate)+countOps(ops, render.OpDestroy) != 0 {
		t.Fatalf("受控选择重施不应重建: %+v", ops)
	}
}

func TestPageControlPrepareRejectsInvalidTreesBeforeCommit(t *testing.T) {
	validTree := func() (*flux.Node, *flux.Node, *flux.Node) {
		root := flux.Window(flux.PageControl(
			flux.TabPage("A", flux.Text("a"), flux.Key("a")),
			flux.TabPage("B", flux.Text("b"), flux.Key("b")),
		)).Create()
		control := root.Children[0]
		return root, control, control.Children[0]
	}
	tests := []struct {
		name string
		node func() *flux.Node
	}{
		{
			name: "TabPage outside PageControl",
			node: func() *flux.Node {
				return flux.Window(flux.TabPage("A", flux.Text("a"), flux.Key("a"))).Create()
			},
		},
		{
			name: "non TabPage child",
			node: func() *flux.Node {
				root, _, page := validTree()
				page.Type = "Text"
				return root
			},
		},
		{
			name: "duplicate page key",
			node: func() *flux.Node {
				root, control, _ := validTree()
				control.Children[1].Key = control.Children[0].Key
				return root
			},
		},
		{
			name: "multiple page subtrees",
			node: func() *flux.Node {
				root, _, page := validTree()
				page.Children = append(page.Children, flux.Text("extra").Create())
				return root
			},
		},
		{
			name: "nil page props",
			node: func() *flux.Node {
				root, _, page := validTree()
				page.Props = nil
				return root
			},
		},
		{
			name: "non string title",
			node: func() *flux.Node {
				root, _, page := validTree()
				page.Props.Set("Text", 42)
				return root
			},
		},
		{
			name: "non int selected index",
			node: func() *flux.Node {
				root, control, _ := validTree()
				control.Props.Set("SelectedIndex", "1")
				return root
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			mock := render.NewMock()
			app := flux.NewApp(mock)
			if err := app.Render(pageRawWidget{node: test.node()}); err == nil {
				t.Fatal("非法分页树 Render 未返回错误")
			}
			if ops := mock.Ops(); len(ops) != 0 {
				t.Fatalf("prepare 失败不得产生 native mutation: %+v", ops)
			}
			if app.Root() != nil {
				t.Fatal("prepare 失败不得提交 Element 树")
			}
		})
	}
}

func TestPageControlRawNodeDefaultIndex(t *testing.T) {
	for _, test := range []struct {
		name      string
		pageCount int
		want      int
	}{
		{name: "non-empty", pageCount: 1, want: 0},
		{name: "empty", pageCount: 0, want: -1},
	} {
		t.Run(test.name, func(t *testing.T) {
			control := &flux.Node{Type: "PageControl", Props: flux.Input().Create().Props}
			if test.pageCount != 0 {
				control.Children = []*flux.Node{
					flux.TabPage("A", flux.Text("a"), flux.Key("a")).Create(),
				}
			}
			root := &flux.Node{
				Type: "Window", Props: flux.Input().Create().Props, Children: []*flux.Node{control},
			}
			mock := render.NewMock()
			app := flux.NewApp(mock)
			if err := app.Render(pageRawWidget{node: root}); err != nil {
				t.Fatal(err)
			}
			element := findByType(t, app.Root(), "PageControl")
			if got := mock.PageSelectedIndex(element.Handle); got != test.want {
				t.Fatalf("raw PageControl 默认索引=%d，期望 %d", got, test.want)
			}
		})
	}
}

func TestPageControlSetBoundsDoesNotOverrideTabPageGeometry(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	if err := app.Render(flux.Window(flux.PageControl(
		flux.TabPage("A", flux.Text("a"), flux.Key("a")),
	))); err != nil {
		t.Fatal(err)
	}
	base := len(mock.Ops())
	app.SetBounds("a", render.Rect{X: 10, Y: 20, W: 30, H: 40})
	if ops := mock.Ops()[base:]; len(ops) != 0 {
		t.Fatalf("TabPage 几何由 widgetset 管理，SetBounds 应为零 mutation: %+v", ops)
	}
}

func TestMockPageControlDefensiveContracts(t *testing.T) {
	assertPanic := func(label string, fn func()) {
		t.Helper()
		defer func() {
			if recover() == nil {
				t.Errorf("%s 未 panic", label)
			}
		}()
		fn()
	}

	mock := render.NewMock()
	control := mock.Create("PageControl")
	page := mock.Create("TabPage")
	text := mock.Create("Text")
	base := len(mock.Ops())
	assertPanic("普通控件直挂 PageControl", func() { mock.SetParent(text, control) })
	assertPanic("TabPage 直挂普通控件", func() { mock.SetParent(page, text) })
	if got := len(mock.Ops()); got != base {
		t.Fatalf("非法 SetParent 产生 mutation: %+v", mock.Ops()[base:])
	}

	mock.SetParent(page, control)
	base = len(mock.Ops())
	assertPanic("SyncPages 包含普通控件", func() { mock.SyncPages(control, []render.Handle{page, text}) })
	assertPanic("SyncPages 包含重复页面", func() { mock.SyncPages(control, []render.Handle{page, page}) })
	if got := len(mock.Ops()); got != base || len(mock.Pages(control)) != 0 {
		t.Fatalf("非法 SyncPages 必须原子失败: ops=%+v pages=%v", mock.Ops()[base:], mock.Pages(control))
	}

	callbackCount := 0
	mock.OnPageSelectionChange(control, func(int) { callbackCount++ })
	mock.SyncPages(control, []render.Handle{page})
	mock.SetPageSelectedIndex(control, 0)
	if callbackCount != 0 {
		t.Fatalf("程序化结构/选择变化触发用户回调 %d 次", callbackCount)
	}
	mock.FirePageSelectionChange(control, 0)
	if callbackCount != 1 {
		t.Fatalf("用户选择回调次数=%d，期望 1", callbackCount)
	}
}

func pageOpIndex(ops []render.Op, match func(render.Op) bool) int {
	for index, op := range ops {
		if match(op) {
			return index
		}
	}
	return -1
}
