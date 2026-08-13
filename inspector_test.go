package flux_test

import (
	"strings"
	"sync"
	"testing"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

type inspectorRenderer struct {
	*render.Mock
	inspectCalls int
}

func (r *inspectorRenderer) InspectNative() render.NativeSnapshot {
	r.inspectCalls++
	return r.Mock.InspectNative()
}

type inspectorWidget struct{ node *flux.Node }

func (w inspectorWidget) Create() *flux.Node { return w.node }

type inspectorPanicStringer struct{ called *bool }

func (s inspectorPanicStringer) String() string {
	*s.called = true
	panic("Inspector 不得调用未知属性的 String")
}

func inspectorFind(node *flux.InspectorNode, key string) *flux.InspectorNode {
	if node == nil {
		return nil
	}
	if node.Key == key {
		return node
	}
	for _, child := range node.Children {
		if found := inspectorFind(child, key); found != nil {
			return found
		}
	}
	return nil
}

// TestInspectorObserverCommitSnapshotAndUnsubscribe 验证 observer 只读提交、三层树
// 快照、重建原因，以及注册/读取/取消都不触发目标 App render。
func TestInspectorObserverCommitSnapshotAndUnsubscribe(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	history := flux.NewInspectorHistory(20)
	unsubscribe := app.ObserveInspector(history)

	app.Render(flux.Window(flux.Column(
		flux.Text("before", flux.Key("status")),
		flux.Button("go", flux.Key("action")),
	)))
	commits := history.Commits()
	if len(commits) != 1 || commits[0].RenderID != 1 {
		t.Fatalf("首次提交 = %+v，期望 render #1", commits)
	}
	if commits[0].Stats.Create != 3 || commits[0].Stats.Bounds == 0 {
		t.Errorf("首次提交统计 = %+v，期望 3 create 且有 bounds", commits[0].Stats)
	}
	if len(commits[0].Rebuilds) != 0 {
		t.Errorf("首次挂载不应标成重建风险：%+v", commits[0].Rebuilds)
	}

	beforeRead := len(history.Commits())
	snapshot := app.InspectorSnapshot()
	if len(history.Commits()) != beforeRead || snapshot.RenderID != 1 {
		t.Fatal("读取快照不得触发目标 render")
	}
	status := inspectorFind(snapshot.Root, "status")
	if status == nil || status.Path == "" || status.Native.Type != "MockText" || !status.Native.Allocated {
		t.Fatalf("三层节点快照不完整：%+v", status)
	}
	if inspectorFind(snapshot.Root, "") == nil {
		t.Fatal("透明 Widget/Element 层应保留")
	}
	snapshot.Root.Children = nil
	if fresh := app.InspectorSnapshot(); fresh.Root == nil || len(fresh.Root.Children) == 0 {
		t.Fatal("修改快照不得反向修改 Element 树")
	}

	app.Render(flux.Window(flux.Column(
		flux.Input(flux.Key("status")), // 同 key、不同 type -> canUpdate type mismatch
		flux.Button("go", flux.Key("action")),
	)))
	commits = history.Commits()
	last := commits[len(commits)-1]
	if last.RenderID != 2 || len(last.Rebuilds) != 1 || last.Rebuilds[0].Reason != "type-mismatch" {
		t.Fatalf("重建原因 = %+v，期望 render #2 type-mismatch", last.Rebuilds)
	}
	if last.Stats.Create == 0 || last.Stats.Destroy == 0 {
		t.Errorf("重建提交应同时含 create/destroy：%+v", last.Stats)
	}

	unsubscribe()
	unsubscribe()
	app.Render(flux.Window(flux.Column(
		flux.Input(flux.Key("status")),
		flux.Button("changed", flux.Key("action")),
	)))
	if got := len(history.Commits()); got != len(commits) {
		t.Fatalf("取消后仍收到提交：%d -> %d", len(commits), got)
	}
}

// TestInspectorObservesDispatchedEventAndDirectBounds 验证实际事件流和绕过 diff 的
// App.SetBounds direct mutation 均可见，且 direct commit 不伪造 render 序号。
func TestInspectorObservesDispatchedEventAndDirectBounds(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	history := flux.NewInspectorHistory(20)
	app.ObserveInspector(history)

	clicked := false
	observedBeforeHandler := false
	app.ObserveInspector(flux.InspectorObserverFuncs{Event: func(flux.InspectorEventRecord) {
		observedBeforeHandler = !clicked
	}})
	app.Render(flux.Window(flux.Button("go", flux.Key("action"), flux.OnClick(func(flux.Event) {
		clicked = true
	}))))
	node := inspectorFind(app.InspectorSnapshot().Root, "action")
	if node == nil {
		t.Fatal("未找到 action")
	}
	handler, ok := mock.EventHandler(render.Handle(node.Native.ID), "OnClick").(func(render.Event))
	if !ok {
		t.Fatal("OnClick 未绑定")
	}
	handler(render.Event{Type: render.EventClick})
	if !clicked {
		t.Fatal("用户 handler 未执行")
	}
	if !observedBeforeHandler {
		t.Fatal("Inspector 事件必须在用户 handler 前发布")
	}
	events := history.Events()
	if len(events) != 1 || events[0].Name != "OnClick" || events[0].Source != "Button#action" {
		t.Fatalf("事件记录 = %+v", events)
	}

	app.SetBounds("action", flux.Rect{X: 7, Y: 8, W: 90, H: 32})
	commits := history.Commits()
	direct := commits[len(commits)-1]
	if !direct.Direct || direct.RenderID != 1 || direct.Stats.Bounds != 1 {
		t.Fatalf("direct bounds 提交 = %+v", direct)
	}
}

// TestInspectorSnapshotUsesCommitCache 验证 observer 回调内读取快照不会与
// renderMu 自锁，且后续读取只复制提交期缓存，不再次查询 native/live Element。
func TestInspectorSnapshotUsesCommitCache(t *testing.T) {
	r := &inspectorRenderer{Mock: render.NewMock()}
	app := flux.NewApp(r)
	var callbackSnapshot flux.InspectorSnapshot
	app.ObserveInspector(flux.InspectorObserverFuncs{Commit: func(commit flux.InspectorCommit) {
		callbackSnapshot = app.InspectorSnapshot()
		if callbackSnapshot.RenderID != commit.RenderID {
			t.Fatalf("回调内快照 render=%d，提交 render=%d", callbackSnapshot.RenderID, commit.RenderID)
		}
	}})

	app.Render(flux.Window(flux.Text("cached", flux.Key("value"))))
	if callbackSnapshot.Root == nil || r.inspectCalls != 1 {
		t.Fatalf("提交期快照不完整：root=%v native 查询=%d", callbackSnapshot.Root, r.inspectCalls)
	}
	for i := 0; i < 5; i++ {
		_ = app.InspectorSnapshot()
	}
	if r.inspectCalls != 1 {
		t.Fatalf("读取缓存又查询了 native：%d", r.inspectCalls)
	}
}

func TestInspectorSnapshotConcurrentReads(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	app.Render(flux.Window(flux.Text("concurrent", flux.Key("value"))))

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				snapshot := app.InspectorSnapshot()
				if snapshot.Root == nil || snapshot.RenderID != 1 {
					t.Errorf("并发快照不完整：%+v", snapshot)
					return
				}
			}
		}()
	}
	wg.Wait()
}

func TestInspectorObserverPanicIsIsolated(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	history := flux.NewInspectorHistory(10)
	app.ObserveInspector(flux.InspectorObserverFuncs{
		Commit: func(flux.InspectorCommit) { panic("commit observer") },
		Event:  func(flux.InspectorEventRecord) { panic("event observer") },
	})
	app.ObserveInspector(history)

	app.Render(flux.Window(flux.Button("go", flux.Key("action"), flux.OnClick(func(flux.Event) {}))))
	node := inspectorFind(app.InspectorSnapshot().Root, "action")
	handler := mock.EventHandler(render.Handle(node.Native.ID), "OnClick").(func(render.Event))
	handler(render.Event{Type: render.EventClick})
	if len(history.Commits()) != 1 || len(history.Events()) != 1 {
		t.Fatalf("panic observer 中断了后续 observer：commits=%d events=%d",
			len(history.Commits()), len(history.Events()))
	}
}

func TestInspectorUnknownPropertyDoesNotInvokeStringer(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	called := false
	root := flux.Window(flux.Text("safe", flux.Key("value"))).Create()
	root.Children[0].Props.Set("Custom", inspectorPanicStringer{called: &called})

	app.Render(inspectorWidget{node: root})
	if called {
		t.Fatal("Inspector 执行了未知属性的 String 方法")
	}
	node := inspectorFind(app.InspectorSnapshot().Root, "value")
	var custom string
	for _, prop := range node.Props {
		if prop.Name == "Custom" {
			custom = prop.Value
		}
	}
	if !strings.Contains(custom, "inspectorPanicStringer") {
		t.Fatalf("未知属性占位 = %q", custom)
	}
}

func TestInspectorDirectBoundsInsideRenderUsesCurrentRenderID(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	history := flux.NewInspectorHistory(10)
	app.ObserveInspector(history)
	tree := func(text string) flux.Widget {
		return flux.Window(flux.Button(text, flux.Key("target"), flux.OnUpdate(func() {
			app.SetBounds("target", flux.Rect{X: 3, Y: 4, W: 80, H: 30})
		})))
	}

	app.Render(tree("before"))
	app.Render(tree("after"))
	commits := history.Commits()
	if len(commits) != 3 {
		t.Fatalf("提交数 = %d，期望首次 render + direct bounds + 二次 render", len(commits))
	}
	direct, renderCommit := commits[1], commits[2]
	if !direct.Direct || direct.RenderID != 2 || direct.Stats.Bounds != 1 {
		t.Fatalf("生命周期内 direct commit = %+v", direct)
	}
	if renderCommit.Direct || renderCommit.RenderID != 2 {
		t.Fatalf("二次 render commit = %+v", renderCommit)
	}
}

func TestInspectorObservesTypedControlEvents(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	history := flux.NewInspectorHistory(10)
	app.ObserveInspector(history)

	app.Render(flux.Window(flux.Column(
		flux.Input(flux.Key("input"), flux.OnChange(func(string) {})),
		flux.CheckBox("check", flux.Key("check"), flux.OnCheckedChange(func(bool) {})),
		flux.ComboBox(flux.Key("combo"), flux.Items([]string{"a", "b"}),
			flux.OnSelectionChange(func(int) {})),
	)))
	snapshot := app.InspectorSnapshot()
	input := inspectorFind(snapshot.Root, "input")
	check := inspectorFind(snapshot.Root, "check")
	combo := inspectorFind(snapshot.Root, "combo")
	mock.EventHandler(render.Handle(input.Native.ID), "OnChange").(func(string))("typed")
	mock.FireCheckedChange(render.Handle(check.Native.ID), true)
	mock.FireSelectionChange(render.Handle(combo.Native.ID), 1)

	events := history.Events()
	if len(events) != 3 {
		t.Fatalf("typed 事件数 = %d，期望 3：%+v", len(events), events)
	}
	wantNames := []string{"OnChange", "OnCheckedChange", "OnSelectionChange"}
	wantSources := []string{"Input#input", "CheckBox#check", "ComboBox#combo"}
	wantValues := []string{"typed", "true", "1"}
	for i := range events {
		if events[i].Name != wantNames[i] || events[i].Source != wantSources[i] || events[i].Value != wantValues[i] {
			t.Errorf("typed 事件[%d] = %+v", i, events[i])
		}
	}
}

func TestInspectorPageControlHierarchyAndNativeParents(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	if err := app.Render(flux.Window(flux.PageControl(
		flux.TabPage("direct", flux.Input(flux.Key("direct-input")), flux.Key("direct")),
		flux.TabPage("transparent", flux.Column(
			flux.Input(flux.Key("nested-input")),
		), flux.Key("transparent")),
		flux.Key("pages"),
	))); err != nil {
		t.Fatal(err)
	}

	snapshot := app.InspectorSnapshot()
	pages := inspectorFind(snapshot.Root, "pages")
	direct := inspectorFind(snapshot.Root, "direct")
	transparent := inspectorFind(snapshot.Root, "transparent")
	directInput := inspectorFind(snapshot.Root, "direct-input")
	nestedInput := inspectorFind(snapshot.Root, "nested-input")
	if pages == nil || direct == nil || transparent == nil || directInput == nil || nestedInput == nil {
		t.Fatal("Inspector 缺少 PageControl -> TabPage -> 子树层级")
	}
	if pages.Native.Type != "MockPageControl" || direct.Native.Type != "MockTabPage" ||
		transparent.Native.Type != "MockTabPage" {
		t.Fatalf("分页 native 类型错误：pages=%+v direct=%+v transparent=%+v",
			pages.Native, direct.Native, transparent.Native)
	}
	if direct.Native.ParentID != pages.Native.ID || transparent.Native.ParentID != pages.Native.ID {
		t.Fatalf("TabPage native parent 错误：pages=%d direct=%d transparent=%d",
			pages.Native.ID, direct.Native.ParentID, transparent.Native.ParentID)
	}
	if directInput.Native.ParentID != direct.Native.ID || nestedInput.Native.ParentID != transparent.Native.ID {
		t.Fatalf("页内控件 native parent 错误：direct=%d/%d nested=%d/%d",
			directInput.Native.ParentID, direct.Native.ID, nestedInput.Native.ParentID, transparent.Native.ID)
	}
}

func TestInspectorObservesScrollEvent(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	history := flux.NewInspectorHistory(10)
	app.ObserveInspector(history)
	scroll := flux.NewState(0)

	app.Mount(func() flux.Widget {
		return flux.Window(flux.ListView(20, 24, func(index int) flux.Widget {
			return flux.Text("row", flux.Key("cell"))
		}, flux.Key("list"), flux.Width(200), flux.Height(120), flux.ScrollOffset(scroll)))
	})
	list := inspectorFind(app.InspectorSnapshot().Root, "list")
	mock.FireScroll(render.Handle(list.Native.ID), 48)
	events := history.Events()
	if len(events) != 1 || events[0].Name != "Scroll" || events[0].Source != "ListView#list" || events[0].Value != "48" {
		t.Fatalf("scroll 事件 = %+v", events)
	}
}

// TestInspectorKeyMismatchAndZeroMutationCommit 覆盖 keyed replacement 绕过
// canUpdate 的路径，并证明零 mutation render 仍有可观察提交。
func TestInspectorKeyMismatchAndZeroMutationCommit(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	history := flux.NewInspectorHistory(20)
	app.ObserveInspector(history)

	tree := func(key string) flux.Widget {
		return flux.Window(flux.Column(flux.Text("same", flux.Key(key))))
	}
	app.Render(tree("old"))
	app.Render(tree("old"))
	commits := history.Commits()
	zero := commits[len(commits)-1]
	if zero.RenderID != 2 || zero.Stats.Total != 0 {
		t.Fatalf("相同树提交 = %+v，期望 render #2 零 mutation", zero)
	}

	app.Render(tree("new"))
	commits = history.Commits()
	last := commits[len(commits)-1]
	if len(last.Rebuilds) != 1 || last.Rebuilds[0].Reason != "key-mismatch" {
		t.Fatalf("key replacement 未定位：%+v", last.Rebuilds)
	}
}

func TestInspectorHistoryIsBounded(t *testing.T) {
	history := flux.NewInspectorHistory(2)
	for i := 1; i <= 3; i++ {
		history.OnInspectorCommit(flux.InspectorCommit{RenderID: uint64(i)})
		history.OnInspectorEvent(flux.InspectorEventRecord{Sequence: uint64(i)})
	}
	if commits := history.Commits(); len(commits) != 2 || commits[0].RenderID != 2 {
		t.Fatalf("有界提交历史 = %+v", commits)
	}
	if events := history.Events(); len(events) != 2 || events[0].Sequence != 2 {
		t.Fatalf("有界事件历史 = %+v", events)
	}
}

// TestInspectorClosePublishesUnmountedSnapshot verifies that Close publishes
// the destroy commit and invalidates the cached Element/native tree together.
func TestInspectorClosePublishesUnmountedSnapshot(t *testing.T) {
	mock := render.NewMock()
	app := flux.NewApp(mock)
	history := flux.NewInspectorHistory(10)
	app.ObserveInspector(history)

	if err := app.Render(flux.Window(flux.Text("closing", flux.Key("value")))); err != nil {
		t.Fatalf("Render: %v", err)
	}
	before := len(history.Commits())
	if err := app.Close(); err != nil {
		t.Fatalf("App.Close: %v", err)
	}
	if root := app.Root(); root != nil {
		t.Errorf("Close 后 App.Root=%+v，期望 nil", root)
	}
	if root := app.InspectorSnapshot().Root; root != nil {
		t.Errorf("Close 后 InspectorSnapshot.Root=%+v，期望 nil", root)
	}
	commits := history.Commits()
	if len(commits) != before+1 {
		t.Fatalf("Close 后提交数=%d，期望 %d", len(commits), before+1)
	}
	if closeCommit := commits[len(commits)-1]; closeCommit.Stats.Destroy == 0 {
		t.Errorf("Close 提交=%+v，期望包含 destroy mutation", closeCommit)
	}
}
