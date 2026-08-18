package diff_test

import (
	"testing"

	"github.com/xiaowumin-mark/flux-vcl/internal/diff"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
	"github.com/xiaowumin-mark/flux-vcl/internal/widget"
)

func paintNode(withCommands bool) *widget.Node {
	n := widget.NewNode("PaintBox")
	n.Key = "paint"
	if withCommands {
		n.Props.Set("PaintCommands", []render.PaintCommand{{
			Kind: render.PaintClear, Color: render.RGB(250, 250, 250),
		}})
	}
	return n
}

// TestPaintCommandsRemovalClearsAndInvalidates 验证 D2 移除对称性：移除值属性会
// 清空保留的命令快照并请求重绘，同时保持 PaintBox identity。
func TestPaintCommandsRemovalClearsAndInvalidates(t *testing.T) {
	mock := render.NewMock()
	rc := diff.New(mock)
	first := widget.NewNode("Window").Add(paintNode(true))
	rc.Render(first)
	h := findByKey(rc.Root(), "paint").Handle
	if got := mock.PaintInvalidations(h); got != 1 {
		t.Fatalf("mount invalidations=%d, want 1", got)
	}

	second := widget.NewNode("Window").Add(paintNode(false))
	ops := rc.Render(second)
	for _, op := range ops {
		if op.Type == render.OpCreate || op.Type == render.OpDestroy {
			t.Fatalf("PaintCommands removal rebuilt the control: %+v", ops)
		}
	}
	if got := mock.PaintCommands(h); len(got) != 0 {
		t.Fatalf("commands after removal=%+v, want empty", got)
	}
	if got := mock.PaintInvalidations(h); got != 2 {
		t.Fatalf("removal invalidations=%d, want 2", got)
	}
}
