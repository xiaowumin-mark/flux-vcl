package flux

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/xiaowumin-mark/flux-vcl/internal/diff"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
	"github.com/xiaowumin-mark/flux-vcl/internal/widget"
)

// InspectorObserver 接收被检查 App 的只读提交与事件记录。
// 提交回调发生在 render 结束后，事件回调发生在用户 handler 前；实现不得阻塞 UI 线程。
type InspectorObserver interface {
	OnInspectorCommit(InspectorCommit)
	OnInspectorEvent(InspectorEventRecord)
}

// InspectorObserverFuncs 用函数组合实现 InspectorObserver；nil 函数会被跳过。
type InspectorObserverFuncs struct {
	Commit func(InspectorCommit)
	Event  func(InspectorEventRecord)
}

// OnInspectorCommit 转发一次只读提交记录。
func (f InspectorObserverFuncs) OnInspectorCommit(c InspectorCommit) {
	if f.Commit != nil {
		f.Commit(c)
	}
}

// OnInspectorEvent 转发一次只读事件记录。
func (f InspectorObserverFuncs) OnInspectorEvent(e InspectorEventRecord) {
	if f.Event != nil {
		f.Event(e)
	}
}

// InspectorSnapshot 是当前 Widget/Element/native 三层树与最近提交的只读快照。
type InspectorSnapshot struct {
	RenderID uint64
	Root     *InspectorNode
	Commit   InspectorCommit
}

// InspectorNode 是一个 Widget/Element 节点的深复制 Inspector 视图。
// 透明节点没有独立原生控件，Native.Shared=true 且复用祖先句柄。
type InspectorNode struct {
	WidgetType  string
	ElementType string
	Key         string
	Path        string
	ParentPath  string
	Props       []InspectorProperty
	Layout      InspectorLayout
	Native      InspectorNative
	Overflow    *LayoutDiag
	Rebuilt     bool
	Children    []*InspectorNode
}

// InspectorProperty 是经过清洗的属性名和值；函数和指针不会泄漏进快照。
type InspectorProperty struct {
	Name  string
	Value string
}

// InspectorLayout 保存节点的 DIP 布局数据。
type InspectorLayout struct {
	Constraints BoxConstraints
	Size        Size
	Bounds      Rect
	Flex        int
}

// InspectorNative 保存原生控件的只读身份和后端类型。
type InspectorNative struct {
	ID        uint64
	ParentID  uint64
	Type      string
	Allocated bool
	Shared    bool
}

// InspectorCommit 是一次 render 或直接 mutation 的有界、只读记录。
type InspectorCommit struct {
	RenderID  uint64
	Direct    bool
	Mutations []InspectorMutation
	Rebuilds  []InspectorRebuild
	Stats     InspectorCommitStats
}

// InspectorCommitStats 汇总最近一次提交的 mutation 类别。
type InspectorCommitStats struct {
	Total, Create, Destroy, Reparent, Property, Event, Bounds int
}

// InspectorMutation 是一条已应用 mutation 的可展示副本。
type InspectorMutation struct {
	Index      int
	Kind       string
	Path       string
	ParentPath string
	NativeID   uint64
	ParentID   uint64
	Property   string
	Value      string
}

// InspectorRebuild 描述一次非首次挂载的原生节点 replacement。
type InspectorRebuild struct {
	Path                    string
	OldPath                 string
	OldType, OldKey         string
	NewType, NewKey         string
	Reason                  string
	TypeChanged, KeyChanged bool
}

// InspectorEventRecord 是实际分发给用户 handler 前记录的事件。
type InspectorEventRecord struct {
	Sequence uint64
	RenderID uint64
	Name     string
	Path     string
	Source   string
	Event    Event
	Value    string
}

// ObserveInspector 注册只读 observer，并返回幂等取消函数。
// 注册、取消和读取快照都不会触发被检查 App render。
func (a *App) ObserveInspector(observer InspectorObserver) (unsubscribe func()) {
	if observer == nil {
		return func() {}
	}
	a.mu.Lock()
	a.nextInspectorObserver++
	id := a.nextInspectorObserver
	if a.inspectorObservers == nil {
		a.inspectorObservers = make(map[uint64]InspectorObserver)
	}
	a.inspectorObservers[id] = observer
	a.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			a.mu.Lock()
			delete(a.inspectorObservers, id)
			a.mu.Unlock()
		})
	}
}

// InspectorSnapshot 返回当前只读快照的深复制；修改返回值不影响 App。
func (a *App) InspectorSnapshot() InspectorSnapshot {
	a.mu.Lock()
	snapshot := cloneInspectorSnapshot(a.lastInspectorSnapshot)
	a.mu.Unlock()
	return snapshot
}

func (a *App) snapshotInspectorTree(diags []NodeDiag, overflows []LayoutDiag, rebuilds []InspectorRebuild) *InspectorNode {
	root := a.rc.Root()
	if root == nil {
		return nil
	}
	diagByPath := make(map[string]NodeDiag, len(diags))
	for _, diag := range diags {
		diagByPath[diag.Path] = diag
	}
	rebuilt := make(map[string]bool, len(rebuilds))
	for _, item := range rebuilds {
		rebuilt[item.Path] = true
	}
	var nativeSnapshot render.NativeSnapshot
	if inspectable, ok := a.r.(render.NativeInspectable); ok {
		nativeSnapshot = inspectable.InspectNative()
	}
	var walk func(*diff.Element) *InspectorNode
	walk = func(e *diff.Element) *InspectorNode {
		n := &InspectorNode{
			WidgetType: e.Type, ElementType: e.Type, Key: e.Key, Path: e.Path,
			Props: snapshotProps(e.Props), Rebuilt: rebuilt[e.Path],
		}
		if e.Parent != nil {
			n.ParentPath = e.Parent.Path
		}
		if d, ok := diagByPath[e.Path]; ok {
			n.Layout = InspectorLayout{Constraints: d.Constraints, Size: d.Size, Bounds: d.Frame, Flex: d.Flex}
		}
		for i := range overflows {
			if overflowMatches(overflows[i], e) {
				value := overflows[i]
				n.Overflow = &value
				break
			}
		}
		n.Native.ID = uint64(e.Handle)
		n.Native.Shared = diff.IsTransparent(e)
		if info, ok := nativeSnapshot[e.Handle]; ok && !n.Native.Shared {
			n.Native.Type = info.Type
			n.Native.ParentID = uint64(info.Parent)
			n.Native.Allocated = info.Allocated
		} else {
			n.Native.Allocated = e.Handle != 0 && !n.Native.Shared
		}
		for _, child := range e.Children {
			n.Children = append(n.Children, walk(child))
		}
		return n
	}
	return walk(root)
}

func overflowMatches(diag LayoutDiag, e *diff.Element) bool {
	return diag.Path == e.Path
}

func snapshotProps(props *widget.Props) []InspectorProperty {
	if props == nil {
		return nil
	}
	out := make([]InspectorProperty, 0, props.Len())
	for _, key := range props.Keys() {
		if key == "_bind" || key == "_pluginRuntime" {
			continue
		}
		value, _ := props.Get(key)
		out = append(out, InspectorProperty{Name: key, Value: inspectorValue(value)})
	}
	return out
}

func inspectorValue(value any) string {
	if value == nil {
		return "<nil>"
	}
	rv := reflect.ValueOf(value)
	if rv.Kind() == reflect.Func {
		return "<func>"
	}
	if rv.Kind() == reflect.Ptr || rv.Kind() == reflect.UnsafePointer ||
		rv.Kind() == reflect.Chan || rv.Kind() == reflect.Map || rv.Kind() == reflect.Slice {
		if values, ok := value.([]string); ok {
			return "[" + strings.Join(values, ", ") + "]"
		}
		return "<" + rv.Type().String() + ">"
	}
	switch v := value.(type) {
	case string:
		return v
	case bool:
		return fmt.Sprintf("%t", v)
	case int:
		return fmt.Sprintf("%d", v)
	case int8:
		return fmt.Sprintf("%d", v)
	case int16:
		return fmt.Sprintf("%d", v)
	case int32:
		return fmt.Sprintf("%d", v)
	case int64:
		return fmt.Sprintf("%d", v)
	case uint:
		return fmt.Sprintf("%d", v)
	case uint8:
		return fmt.Sprintf("%d", v)
	case uint16:
		return fmt.Sprintf("%d", v)
	case uint32:
		return fmt.Sprintf("%d", v)
	case uint64:
		return fmt.Sprintf("%d", v)
	case uintptr:
		return fmt.Sprintf("%d", v)
	case float32:
		return fmt.Sprintf("%g", v)
	case float64:
		return fmt.Sprintf("%g", v)
	case render.Color:
		return fmt.Sprintf("#%08X", uint32(v))
	case render.Rect:
		return fmt.Sprintf("{%d,%d %dx%d}", v.X, v.Y, v.W, v.H)
	case render.ScrollConfig:
		return fmt.Sprintf("{content:%d step:%d}", v.Content, v.Step)
	case render.ScrollTarget, render.Ref:
		return "<" + rv.Type().String() + ">"
	case PluginProperties:
		return "[" + strings.Join(v.Keys(), ", ") + "]"
	default:
		return "<" + rv.Type().String() + ">"
	}
}

func inspectorCommit(renderID uint64, direct bool, ops []render.Op, rebuilds []diff.Rebuild) InspectorCommit {
	commit := InspectorCommit{RenderID: renderID, Direct: direct}
	for i, op := range ops {
		kind := op.Type.String()
		property := op.Key
		if op.Type == render.OpSetProperty && op.Key == "Bounds" {
			kind = "bounds"
		}
		mutation := InspectorMutation{
			Index: i, Kind: kind, Path: op.Path, ParentPath: op.ParentPath,
			NativeID: uint64(op.Handle), ParentID: uint64(op.Parent), Property: property,
			Value: inspectorValue(op.Value),
		}
		commit.Mutations = append(commit.Mutations, mutation)
		commit.Stats.Total++
		switch kind {
		case "create":
			commit.Stats.Create++
		case "destroy":
			commit.Stats.Destroy++
		case "reparent", "insert", "remove":
			commit.Stats.Reparent++
		case "event":
			commit.Stats.Event++
		case "bounds":
			commit.Stats.Bounds++
		default:
			commit.Stats.Property++
		}
	}
	for _, rebuild := range rebuilds {
		commit.Rebuilds = append(commit.Rebuilds, InspectorRebuild{
			Path: rebuild.Path, OldPath: rebuild.OldPath, OldType: rebuild.OldType, OldKey: rebuild.OldKey,
			NewType: rebuild.NewType, NewKey: rebuild.NewKey, Reason: rebuild.Reason,
			TypeChanged: rebuild.TypeChanged, KeyChanged: rebuild.KeyChanged,
		})
	}
	return commit
}

func cloneInspectorCommit(commit InspectorCommit) InspectorCommit {
	commit.Mutations = append([]InspectorMutation(nil), commit.Mutations...)
	commit.Rebuilds = append([]InspectorRebuild(nil), commit.Rebuilds...)
	return commit
}

func cloneInspectorNode(n *InspectorNode) *InspectorNode {
	if n == nil {
		return nil
	}
	out := *n
	out.Props = append([]InspectorProperty(nil), n.Props...)
	if n.Overflow != nil {
		value := *n.Overflow
		out.Overflow = &value
	}
	out.Children = make([]*InspectorNode, len(n.Children))
	for i, child := range n.Children {
		out.Children[i] = cloneInspectorNode(child)
	}
	return &out
}

func cloneInspectorSnapshot(snapshot InspectorSnapshot) InspectorSnapshot {
	snapshot.Root = cloneInspectorNode(snapshot.Root)
	snapshot.Commit = cloneInspectorCommit(snapshot.Commit)
	return snapshot
}

func (a *App) publishInspectorCommit(commit InspectorCommit) {
	a.mu.Lock()
	a.lastInspectorSnapshot.Commit = cloneInspectorCommit(commit)
	observers := make([]InspectorObserver, 0, len(a.inspectorObservers))
	for _, observer := range a.inspectorObservers {
		observers = append(observers, observer)
	}
	a.mu.Unlock()
	for _, observer := range observers {
		observer := observer
		render.Guard("inspector.OnInspectorCommit", func() {
			observer.OnInspectorCommit(cloneInspectorCommit(commit))
		})
	}
}

func (a *App) queueInspectorCommit(commit InspectorCommit) {
	a.mu.Lock()
	diags := append([]NodeDiag(nil), a.lastInspect...)
	overflows := append([]LayoutDiag(nil), a.lastDiags...)
	a.mu.Unlock()
	snapshot := InspectorSnapshot{
		RenderID: commit.RenderID,
		Root:     a.snapshotInspectorTree(diags, overflows, commit.Rebuilds),
		Commit:   cloneInspectorCommit(commit),
	}
	a.mu.Lock()
	a.lastInspectorSnapshot = snapshot
	a.inspectorPending = append(a.inspectorPending, cloneInspectorCommit(commit))
	a.mu.Unlock()
}

func (a *App) queueInspectorNotification(commit InspectorCommit) {
	a.mu.Lock()
	a.inspectorPending = append(a.inspectorPending, cloneInspectorCommit(commit))
	a.mu.Unlock()
}

func (a *App) flushInspectorCommits() {
	a.mu.Lock()
	pending := a.inspectorPending
	a.inspectorPending = nil
	a.mu.Unlock()
	for _, commit := range pending {
		a.publishInspectorCommit(commit)
	}
}

func (a *App) recordInspectorEvent(event diff.EventDispatch) {
	a.mu.Lock()
	a.inspectorEventSeq++
	record := InspectorEventRecord{
		Sequence: a.inspectorEventSeq, RenderID: a.inspectorRenderID, Name: event.Name,
		Path: event.Path, Source: event.Source, Event: event.Event,
		Value: inspectorValue(event.Value),
	}
	if event.HasEvent {
		record.Source = event.Event.Source
	}
	observers := make([]InspectorObserver, 0, len(a.inspectorObservers))
	for _, observer := range a.inspectorObservers {
		observers = append(observers, observer)
	}
	a.mu.Unlock()
	for _, observer := range observers {
		observer := observer
		render.Guard("inspector.OnInspectorEvent", func() {
			observer.OnInspectorEvent(record)
		})
	}
}

// InspectorHistory 是 observer 的线程安全有界记录器，适合工具窗和无头测试。
type InspectorHistory struct {
	mu      sync.Mutex
	limit   int
	commits []InspectorCommit
	events  []InspectorEventRecord
}

// NewInspectorHistory 创建最多保留 limit 条提交和事件的记录器；limit<1 使用 100。
func NewInspectorHistory(limit int) *InspectorHistory {
	if limit < 1 {
		limit = 100
	}
	return &InspectorHistory{limit: limit}
}

// OnInspectorCommit 记录一次提交。
func (h *InspectorHistory) OnInspectorCommit(commit InspectorCommit) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.commits = append(h.commits, cloneInspectorCommit(commit))
	if len(h.commits) > h.limit {
		h.commits = append([]InspectorCommit(nil), h.commits[len(h.commits)-h.limit:]...)
	}
}

// OnInspectorEvent 记录一次实际事件。
func (h *InspectorHistory) OnInspectorEvent(event InspectorEventRecord) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.events = append(h.events, event)
	if len(h.events) > h.limit {
		h.events = append([]InspectorEventRecord(nil), h.events[len(h.events)-h.limit:]...)
	}
}

// Commits 返回提交历史副本。
func (h *InspectorHistory) Commits() []InspectorCommit {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]InspectorCommit, len(h.commits))
	for i := range h.commits {
		out[i] = cloneInspectorCommit(h.commits[i])
	}
	return out
}

// Events 返回事件历史副本。
func (h *InspectorHistory) Events() []InspectorEventRecord {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]InspectorEventRecord(nil), h.events...)
}
