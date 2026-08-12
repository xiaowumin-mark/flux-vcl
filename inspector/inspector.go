// Package inspector 提供 FluxVCL 被检查 App 的独立只读工具窗。
//
// 工具窗直接消费 flux.InspectorSnapshot/InspectorObserver，不持有 Element、Props
// 或原生对象指针；刷新与关闭只影响自身窗体，不触发目标 App render。
package inspector

import (
	"fmt"
	"strings"
	"sync"

	"github.com/energye/lcl/lcl"
	"github.com/energye/lcl/types"
	flux "github.com/xiaowumin-mark/flux-vcl"
)

// Window 是一个独立的 Inspector 原生工具窗。
type Window struct {
	mu            sync.Mutex
	target        *flux.App
	history       *flux.InspectorHistory
	form          lcl.IForm
	view          lcl.IMemo
	unsubscribe   func()
	closing       bool
	closed        bool
	refreshQueued bool
}

// Open 创建并显示 target 的只读 Inspector 工具窗。
// 调用方须已完成 native.Init，并在 UI 线程调用。
func Open(target *flux.App) *Window {
	if target == nil {
		panic("inspector.Open: target 不能为空")
	}
	w := &Window{target: target, history: flux.NewInspectorHistory(80)}
	w.form = lcl.NewForm(lcl.Application)
	w.form.SetCaption("FluxVCL Inspector")
	w.form.SetClientWidth(820)
	w.form.SetClientHeight(640)
	w.form.SetPosition(types.PoScreenCenter)
	w.form.SetBorderStyleToFormBorderStyle(types.BsSingle)

	w.view = lcl.NewMemo(w.form)
	w.view.SetParent(w.form)
	w.view.SetAlign(types.AlClient)
	w.view.SetReadOnly(true)
	w.view.SetWordWrap(false)
	w.view.SetScrollBars(types.SsBoth)
	w.view.Font().SetName("Consolas")
	w.view.Font().SetSize(10)

	w.unsubscribe = target.ObserveInspector(flux.InspectorObserverFuncs{
		Commit: func(commit flux.InspectorCommit) {
			w.history.OnInspectorCommit(commit)
			w.scheduleRefresh()
		},
		Event: func(event flux.InspectorEventRecord) {
			w.history.OnInspectorEvent(event)
			w.scheduleRefresh()
		},
	})
	w.form.SetOnClose(func(_ lcl.IObject, action *types.TCloseAction) {
		if action != nil {
			*action = types.CaFree
		}
		w.cleanup()
	})
	w.refreshOnUI()
	w.form.Show()
	return w
}

// Refresh 重新读取 target 快照并刷新工具窗，不触发 target render。
func (w *Window) Refresh() { w.scheduleRefresh() }

// Close 关闭并释放 Inspector 工具窗，同时取消 observer；幂等。
func (w *Window) Close() {
	w.mu.Lock()
	if w.closed || w.closing || w.form == nil {
		w.mu.Unlock()
		return
	}
	w.closing = true
	form := w.form
	w.mu.Unlock()
	lcl.RunOnMainThreadAsync(func(uint32) { form.Close() })
}

func (w *Window) scheduleRefresh() {
	w.mu.Lock()
	if w.closed || w.view == nil || w.refreshQueued {
		w.mu.Unlock()
		return
	}
	w.refreshQueued = true
	w.mu.Unlock()
	lcl.RunOnMainThreadAsync(func(uint32) { w.refreshOnUI() })
}

func (w *Window) refreshOnUI() {
	w.mu.Lock()
	w.refreshQueued = false
	if w.closed || w.view == nil {
		w.mu.Unlock()
		return
	}
	target := w.target
	history := w.history
	w.mu.Unlock()

	snapshot := target.InspectorSnapshot()
	commits := history.Commits()
	events := history.Events()
	text := format(snapshot, commits, events)

	w.mu.Lock()
	if w.closed || w.view == nil {
		w.mu.Unlock()
		return
	}
	view := w.view
	w.mu.Unlock()
	view.Lines().SetTextToStr(text)
}

func (w *Window) cleanup() {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.closed = true
	w.refreshQueued = false
	w.form = nil
	w.view = nil
	unsubscribe := w.unsubscribe
	w.unsubscribe = nil
	w.mu.Unlock()
	if unsubscribe != nil {
		unsubscribe()
	}
}

func format(snapshot flux.InspectorSnapshot, commits []flux.InspectorCommit, events []flux.InspectorEventRecord) string {
	var out strings.Builder
	c := snapshot.Commit
	fmt.Fprintf(&out, "FLUXVCL INSPECTOR   render #%d\r\n", snapshot.RenderID)
	fmt.Fprintf(&out, "commit  total=%d create=%d destroy=%d reparent=%d props=%d events=%d bounds=%d\r\n",
		c.Stats.Total, c.Stats.Create, c.Stats.Destroy, c.Stats.Reparent,
		c.Stats.Property, c.Stats.Event, c.Stats.Bounds)
	if len(c.Rebuilds) > 0 {
		out.WriteString("REBUILD / FOCUS RISK\r\n")
		for _, rebuild := range c.Rebuilds {
			fmt.Fprintf(&out, "  ! %s  %s#%s -> %s#%s (%s)\r\n", rebuild.Path,
				rebuild.OldType, rebuild.OldKey, rebuild.NewType, rebuild.NewKey, rebuild.Reason)
		}
	}
	out.WriteString("\r\nWIDGET / ELEMENT / NATIVE TREE\r\n")
	formatNode(&out, snapshot.Root, 0)

	out.WriteString("\r\nRECENT MUTATIONS\r\n")
	start := 0
	if len(commits) > 6 {
		start = len(commits) - 6
	}
	for _, commit := range commits[start:] {
		label := fmt.Sprintf("render #%d", commit.RenderID)
		if commit.Direct {
			label = fmt.Sprintf("direct @ render #%d", commit.RenderID)
		}
		fmt.Fprintf(&out, "  [%s] %d ops\r\n", label, commit.Stats.Total)
		for _, rebuild := range commit.Rebuilds {
			fmt.Fprintf(&out, "    REBUILD  %-36s %-14s %s#%s -> %s#%s\r\n",
				rebuild.Path, rebuild.Reason, rebuild.OldType, rebuild.OldKey,
				rebuild.NewType, rebuild.NewKey)
		}
		for _, mutation := range commit.Mutations {
			fmt.Fprintf(&out, "    %-8s %-36s %-14s %s\r\n", mutation.Kind, mutation.Path, mutation.Property, mutation.Value)
		}
	}

	out.WriteString("\r\nRECENT DISPATCHED EVENTS\r\n")
	start = 0
	if len(events) > 12 {
		start = len(events) - 12
	}
	for _, event := range events[start:] {
		fmt.Fprintf(&out, "  #%d r%d %-18s %-36s source=%s value=%s\r\n",
			event.Sequence, event.RenderID, event.Name, event.Path, event.Source, event.Value)
	}
	return out.String()
}

func formatNode(out *strings.Builder, node *flux.InspectorNode, depth int) {
	if node == nil {
		return
	}
	indent := strings.Repeat("  ", depth)
	flag := " "
	if node.Rebuilt {
		flag = "!"
	}
	native := "transparent/shared"
	if !node.Native.Shared {
		native = fmt.Sprintf("%s id=%d parent=%d allocated=%t", node.Native.Type,
			node.Native.ID, node.Native.ParentID, node.Native.Allocated)
	}
	fmt.Fprintf(out, "%s%s %s", indent, flag, node.WidgetType)
	if node.Key != "" {
		fmt.Fprintf(out, "#%s", node.Key)
	}
	fmt.Fprintf(out, "  path=%s\r\n", node.Path)
	fmt.Fprintf(out, "%s    Element=%s  Native=%s\r\n", indent, node.ElementType, native)
	l := node.Layout
	fmt.Fprintf(out, "%s    layout c={%d..%d,%d..%d} size=%dx%d bounds={%d,%d %dx%d} flex=%d\r\n",
		indent, l.Constraints.MinW, l.Constraints.MaxW, l.Constraints.MinH, l.Constraints.MaxH,
		l.Size.W, l.Size.H, l.Bounds.X, l.Bounds.Y, l.Bounds.W, l.Bounds.H, l.Flex)
	if node.Overflow != nil {
		fmt.Fprintf(out, "%s    ! overflow w=%d h=%d\r\n", indent, node.Overflow.OverflowW, node.Overflow.OverflowH)
	}
	if len(node.Props) > 0 {
		out.WriteString(indent + "    props ")
		for i, prop := range node.Props {
			if i > 0 {
				out.WriteString("; ")
			}
			fmt.Fprintf(out, "%s=%s", prop.Name, prop.Value)
		}
		out.WriteString("\r\n")
	}
	for _, child := range node.Children {
		formatNode(out, child, depth+1)
	}
}
