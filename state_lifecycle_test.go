package flux

import (
	"testing"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

type panicTextRenderer struct {
	*render.Mock
	panicText string
}

func (r *panicTextRenderer) SetText(h render.Handle, text string) {
	if text == r.panicText {
		panic("test renderer rejected text")
	}
	r.Mock.SetText(h, text)
}

func stateSubscriberCount[T any](s *State[T]) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.subs)
}

// TestStateConditionalBindingRemoval verifies that subscriptions follow the
// committed tree rather than accumulating every Binding built over App's life.
// The branch deliberately uses the same State twice: removal must unsubscribe
// it once, and restoring the branch must subscribe it again.
func TestStateConditionalBindingRemoval(t *testing.T) {
	mock := render.NewMock()
	app := NewApp(mock)
	shown := NewState(true)
	conditional := NewState("before")
	renders := 0

	if err := app.Mount(func() Widget {
		renders++
		children := []any{Text(Bind(shown), Key("shown"))}
		if shown.Get() {
			children = append(children,
				Text(Bind(conditional), Key("conditional-a")),
				Text(Bind(conditional), Key("conditional-b")),
			)
		}
		return Window(Column(children...))
	}); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if got := stateSubscriberCount(conditional); got != 1 {
		t.Fatalf("conditional subscriber count = %d, want 1", got)
	}

	shown.Set(false)
	if got := stateSubscriberCount(conditional); got != 0 {
		t.Fatalf("conditional subscriber count after removal = %d, want 0", got)
	}
	beforeRenders, beforeOps := renders, len(mock.Ops())
	conditional.Set("stale")
	if renders != beforeRenders {
		t.Fatalf("removed State triggered render: got %d renders, want %d", renders, beforeRenders)
	}
	if got := len(mock.Ops()); got != beforeOps {
		t.Fatalf("removed State produced mutations: got %d ops, want %d", got, beforeOps)
	}

	shown.Set(true)
	if got := stateSubscriberCount(conditional); got != 1 {
		t.Fatalf("conditional subscriber count after restore = %d, want 1", got)
	}
	beforeRenders = renders
	conditional.Set("fresh")
	if renders != beforeRenders+1 {
		t.Fatalf("restored State render count = %d, want %d", renders, beforeRenders+1)
	}
}

// TestStateCloseUnsubscribes verifies that Close removes the State -> App edge
// rather than merely relying on App.closed to ignore future invalidations.
func TestStateCloseUnsubscribes(t *testing.T) {
	mock := render.NewMock()
	app := NewApp(mock)
	state := NewState("mounted")

	if err := app.Mount(func() Widget {
		return Window(Text(Bind(state), Key("value")))
	}); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if got := stateSubscriberCount(state); got != 1 {
		t.Fatalf("subscriber count before Close = %d, want 1", got)
	}

	if err := app.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := stateSubscriberCount(state); got != 0 {
		t.Fatalf("subscriber count after Close = %d, want 0", got)
	}

	beforeOps := len(mock.Ops())
	state.Set("after-close")
	if got := len(mock.Ops()); got != beforeOps {
		t.Fatalf("State.Set after Close produced mutations: got %d ops, want %d", got, beforeOps)
	}
}

// TestStateRenderPanicKeepsCommittedBindings 验证 Renderer panic 不能替换最近
// 一次成功 reconcile 的订阅。候选依赖会暂时订阅以支持生命周期 Set，只有
// Reconciler.Render 正常返回才会提交。
func TestStateRenderPanicKeepsCommittedBindings(t *testing.T) {
	oldState := NewState("old")
	newState := NewState("new")
	renderer := &panicTextRenderer{Mock: render.NewMock(), panicText: "new"}
	app := NewApp(renderer)
	builds := 0

	if err := app.Mount(func() Widget {
		builds++
		return Window(Text(Bind(oldState), Key("value")))
	}); err != nil {
		t.Fatalf("Mount: %v", err)
	}
	if got := stateSubscriberCount(oldState); got != 1 {
		t.Fatalf("old subscriber count before panic = %d, want 1", got)
	}

	func() {
		defer func() {
			if recovered := recover(); recovered == nil {
				t.Fatal("Render should propagate renderer panic")
			}
		}()
		app.Render(Window(Text(Bind(newState), Key("value"))))
	}()

	if got := stateSubscriberCount(oldState); got != 1 {
		t.Fatalf("old subscriber count after panic = %d, want 1", got)
	}
	if got := stateSubscriberCount(newState); got != 0 {
		t.Fatalf("new subscriber count after panic = %d, want 0", got)
	}
	beforeBuilds := builds
	newState.Set("later")
	if builds != beforeBuilds {
		t.Fatalf("discarded State triggered render: got %d builds, want %d", builds, beforeBuilds)
	}
	oldState.Set("recovered")
	if builds != beforeBuilds+1 {
		t.Fatalf("committed State did not trigger recovery render: got %d builds, want %d", builds, beforeBuilds+1)
	}
}
