// Package diff 实现 FluxVCL 的 diff / reconciliation 引擎
// （design.md §5，开发计划 Phase 1.4）。
//
// 全项目最高优先级代码。输入是每次 render 重建的 Widget 树（*widget.Node），
// 输出是面向 Renderer 的 mutation op 集，直接应用到 renderer（Mock 记录供断言）。
//
// 核心规则（D1/D2/D3）：
//   - canUpdate：旧控件类型==新类型 && 旧key==新key → 原地 patch；否则只重建该节点。
//   - 属性级 patch：逐属性比较（widget.Props.Diff），只对变化者调 Set*。
//   - 稳定 key：带 key 的列表按 key 匹配复用（重排不重建）；无 key 按位置匹配。
//   - 隐式寻址（D3 补充）：每个 Element 维护树路径 Path（"Window/0/Column/1/Text"），
//     供 FindByPath 定位与事件 Source 回落 —— 寻址与身份解耦：静态树零 Key 也可寻址，
//     身份敏感的控件（列表行/动画目标/需 Source 区分的同型控件）仍用稳定 Key。
//
// 事件回调 / 逃逸口（函数值）无法比较相等性，每次 diff 重新绑定（D2 逃逸口行为）。
package diff

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
	"github.com/xiaowumin-mark/flux-vcl/internal/widget"
)

// transparentType 返回类型是否为透明容器：Column/Row/Expanded/Flexible 是纯
// 逻辑分组，不创建原生控件，Element 的 handle 继承父容器，children 直接挂到祖父。
// Component（Phase 5.4）同样是透明分组：子树 = build() 结果，身份靠外部 Key
// （D3），不产生原生控件。ListViewRow（Phase 6）是虚拟列表行：也是透明包装
// （不建原生控件、子挂祖父），身份靠 slot key（控件池槽位，跨 render 复用）。
// 插件透明性由框架写入的 PluginLifecycle marker 证明，不能只信任 Type 字符串。
func transparentType(t string) bool {
	switch t {
	case "Column", "Row", "Expanded", "Flexible", "Component", "ListViewRow":
		return true
	}
	return false
}

func transparentNode(node *widget.Node) bool {
	if node == nil {
		return false
	}
	if transparentType(node.Type) {
		return true
	}
	return widget.IsPlugin(node)
}

func transparentElement(e *Element) bool {
	if e == nil {
		return false
	}
	if transparentType(e.Type) {
		return true
	}
	return e.plugin
}

func copyItems(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	return append([]string(nil), items...)
}

// Element 是 Element 树的节点（design.md §4.3 / D1）：持久 identity。
//
// 在控件存活期间复用同一 Element（Type+Key 匹配时），持有当前原生句柄
// 与上一次的 Props（prevConfig，用于属性级 diff）。widget 树每次重建，
// Element 树跨 render 存活 —— 二者通过 reconcile 对齐。
//
// Path 是该元素每次 render 时的树路径（"Window/0/Column/1/Text"，含自身类型），
// 由 mount/reconcile 自顶向下维护 —— 隐式寻址（FindByPath）与无 Key 控件的事件
// Source 回落的数据源。Path 是位置身份：结构重排后随之漂移（这正是静态树适用、
// 身份敏感控件需用稳定 Key 的原因）。
type Element struct {
	Type               string
	Key                string
	Path               string
	plugin             bool          // 由 internal/widget 的不可导出标记复制，不能由 Type 前缀伪造
	pageSelectionDirty bool          // 用户切页后，下一次 render 必须重施受控索引
	Props              *widget.Props // prevConfig：上一次成功 reconcile 的属性集
	Handle             render.Handle // 原生句柄（透明容器 = 父容器句柄）
	Parent             *Element
	Children           []*Element
}

// Reconciler 是 diff 引擎：持有 Element 树根与 Renderer，逐次 Render 对齐。
type Reconciler struct {
	r         render.Renderer
	root      *Element
	lastOps   []render.Op
	rebuilds  []Rebuild
	eventSink func(EventDispatch)
}

// Rebuild 描述一次 canUpdate 失败或 keyed replacement 导致的原生子树重建。
type Rebuild struct {
	Path                    string
	OldPath                 string
	OldType, OldKey         string
	NewType, NewKey         string
	Reason                  string
	TypeChanged, KeyChanged bool
}

// EventDispatch 是 diff 注入 Source 后、调用用户 handler 前产生的只读事件记录。
type EventDispatch struct {
	Path     string
	Name     string
	Source   string
	Event    render.Event
	HasEvent bool
	Value    any
}

// PluginLifecycle 是根包插件运行时接入 Element 生命周期的最小接口。
// diff 不依赖公开 flux 包，从而保持依赖方向；实现负责自身 panic/error 边界。
type PluginLifecycle interface {
	PluginMount(key string, props *widget.Props)
	PluginUpdate(key string, props *widget.Props)
	PluginUnmount(key string, props *widget.Props)
}

// New 创建空 Reconciler。
func New(r render.Renderer) *Reconciler { return &Reconciler{r: r} }

// SetEventSink 设置事件观测回调。回调只能读取事件，不参与 diff 状态修改。
func (rc *Reconciler) SetEventSink(fn func(EventDispatch)) { rc.eventSink = fn }

// Root 返回当前 Element 树根（首次 Render 前为 nil）。
func (rc *Reconciler) Root() *Element { return rc.root }

// IsTransparent 报告类型是否为透明容器（无原生控件，Element 句柄继承父）。
// 供 flux 根包的逃逸口（App.SetBounds）判断目标是否有独立原生句柄可直改。
func IsTransparent(e *Element) bool { return transparentElement(e) }

// Lookup 按 key 在 Element 树中查找节点（深度优先，先父后子）。
//
// Phase 5.1 动画的逃逸口支撑：App.SetBounds 用稳定 key 定位 Element 句柄，
// 直接应用几何而不重跑 diff（D2 逃逸口）。未找到返回 nil。
//
// key 是稳定身份（D3）—— 动画目标、跨 render 需要保持同一控件的场景必须用 key
// 定位（路径会随结构变动漂移）。静态树/一次性寻址用 FindByPath。
func (rc *Reconciler) Lookup(key string) *Element {
	if rc.root == nil || key == "" {
		return nil
	}
	var find func(e *Element) *Element
	find = func(e *Element) *Element {
		if e.Key == key {
			return e
		}
		for _, c := range e.Children {
			if f := find(c); f != nil {
				return f
			}
		}
		return nil
	}
	return find(rc.root)
}

// FindByPath 沿控件树定位 Element（隐式寻址，D3 补充：寻址与身份解耦）。
//
// 静态树（结构固定、不重排）可不写 Key，用路径定位测试/排查目标；身份敏感的
// 控件（列表行/动画目标/需 Source 区分的同型控件）仍用 Key（D3）+ Lookup。
//
// path 格式为每次 render 维护的 Element.Path："Window/0/Column/1/Text"。
// 首段为当前节点（通常为根）类型，必须匹配；其后交替 数字段=取子节点下标、
// 类型段=校验该子节点类型。未命中（含空路径/nil 接收者）返回 nil。
func (e *Element) FindByPath(path string) *Element {
	if e == nil || path == "" {
		return nil
	}
	segs := strings.Split(path, "/")
	if segs[0] != e.Type {
		return nil
	}
	cur := e
	i := 1
	for i < len(segs) {
		idx, err := strconv.Atoi(segs[i])
		if err != nil || idx < 0 || idx >= len(cur.Children) {
			return nil
		}
		i++
		cur = cur.Children[idx]
		if i < len(segs) {
			if segs[i] != cur.Type {
				return nil
			}
			i++
		}
	}
	return cur
}

// Render 对整棵树做一次 diff：首次调用挂载整棵树，后续按 D1/D2 增量 patch。
//
// 返回本次产生的 mutation op 列表（已直接应用到 renderer；副本返回供断言/日志）。
// 相同树 diff 零 mutation（D7c）：未变属性不产生 op，未匹配节点不重建。
func (rc *Reconciler) Render(root *widget.Node) []render.Op {
	rc.lastOps = nil
	rc.rebuilds = nil
	if rc.root == nil {
		rc.root = rc.mount(root, nil, root.Type)
	} else {
		rc.root = rc.reconcile(rc.root, root, root.Type)
	}
	return append([]render.Op(nil), rc.lastOps...)
}

// Unmount 卸载当前 Element 树并返回已应用的 mutation 副本。
// 生命周期按子后父触发，Renderer.Destroy 仍遵循其延后释放策略（D4）。
func (rc *Reconciler) Unmount() []render.Op {
	rc.lastOps = nil
	rc.rebuilds = nil
	if rc.root != nil {
		rc.destroySubtree(rc.root)
		rc.root = nil
	}
	return append([]render.Op(nil), rc.lastOps...)
}

// Rebuilds 返回最近一次 Render 的重建记录副本。
func (rc *Reconciler) Rebuilds() []Rebuild {
	return append([]Rebuild(nil), rc.rebuilds...)
}

// mount 挂载新子树：创建原生控件（透明容器除外）、应用全部属性、递归子节点。
// path 为该节点的树路径（隐式寻址，自顶向下拼接），子节点路径 = 父路径+下标+类型。
// 子树全部挂载完成后触发 OnMount（Phase 4.3：父钩子在子钩子之后，可访问完整子树）。
func (rc *Reconciler) mount(node *widget.Node, parent *Element, path string) *Element {
	e := &Element{
		Type: node.Type, Key: node.Key, Path: path, plugin: widget.IsPlugin(node),
		Props: node.Props, Parent: parent,
	}
	parentHandle := render.Handle(0)
	if parent != nil {
		parentHandle = parent.Handle
	}
	earlyPageAttach := node.Type == "TabPage" && parent != nil && parent.Type == "PageControl"

	if transparentNode(node) {
		e.Handle = parentHandle // 透明容器：无原生控件，继承父句柄
	} else {
		e.Handle = rc.r.Create(node.Type)
		rc.record(e, render.Op{Type: render.OpCreate, Handle: e.Handle, Key: node.Type})
		// TTabSheet 必须先归属 TPageControl，再把页内控件挂到它的客户区。
		// 其他原生控件继续沿用原有的“子树完成后挂父级”顺序。
		if earlyPageAttach {
			rc.r.SetParent(e.Handle, parentHandle)
			rc.record(e, render.Op{
				Type: render.OpAppendChild, Handle: e.Handle,
				Parent: parentHandle, ParentPath: parent.Path,
			})
		}
		rc.applyProps(e, node.Props)
	}

	for i, c := range node.Children {
		ce := rc.mount(c, e, path+"/"+strconv.Itoa(i)+"/"+c.Type)
		e.Children = append(e.Children, ce)
	}
	if e.Type == "PageControl" {
		rc.syncPages(e)
		rc.applyPageSelectedIndex(e, node.Props)
	}
	rc.firePluginLifecycle(widget.IsPlugin(node), node.Key, node.Props, "mount")
	rc.fireLifecycle(node.Props, "OnMount")
	if !transparentNode(node) && parent != nil && !earlyPageAttach {
		rc.r.SetParent(e.Handle, parentHandle)
		rc.record(e, render.Op{
			Type: render.OpAppendChild, Handle: e.Handle,
			Parent: parentHandle, ParentPath: parent.Path,
		})
	}
	return e
}

// reconcile 对齐一个已匹配节点（canUpdate 通过）：属性 patch + 递归 children。
// canUpdate 未通过时：销毁旧子树、挂载新子树（只重建该节点，不上溯祖先 —— D1）。
// 子树对齐完成后，若节点存在"真实属性变化"（非事件重绑/生命周期钩子），触发
// OnUpdate（Phase 4.3：Flutter didUpdateWidget 语义 —— 配置变化才回调，避免
// 每次 render 都触发导致钩子里 Set State → 无限 re-render）。
func (rc *Reconciler) reconcile(old *Element, node *widget.Node, path string) *Element {
	if !canUpdate(old, node) {
		oldPath := old.Path
		reason := "type-mismatch"
		if old.Type == node.Type {
			reason = "key-mismatch"
		} else if old.Key != node.Key {
			reason = "type-and-key-mismatch"
		}
		rc.rebuilds = append(rc.rebuilds, Rebuild{
			Path: path, OldPath: oldPath, OldType: old.Type, OldKey: old.Key, NewType: node.Type, NewKey: node.Key,
			Reason: reason, TypeChanged: old.Type != node.Type, KeyChanged: old.Key != node.Key,
		})
		rc.destroySubtree(old)
		for i := len(rc.lastOps) - 1; i >= 0; i-- {
			if rc.lastOps[i].Type == render.OpDestroy && rc.lastOps[i].Path == oldPath {
				rc.lastOps[i].Path = path
				break
			}
		}
		return rc.mount(node, old.Parent, path)
	}

	old.Path = path // 位置变更（重排/跨容器移动）时更新；事件 Source 回落据此取最新路径
	changed := false
	if old.Type == "PageControl" {
		// 页面顺序和数量必须先稳定，再把受控索引应用到最终页面列表。
		if rc.reconcileChildren(old, node) {
			rc.syncPages(old)
		}
		changed = rc.patchProps(old, node)
	} else {
		changed = rc.patchProps(old, node)
		rc.reconcileChildren(old, node)
	}
	if changed {
		rc.firePluginLifecycle(widget.IsPlugin(node), node.Key, node.Props, "update")
		rc.fireLifecycle(node.Props, "OnUpdate")
	}
	old.Props = node.Props
	return old
}

// canUpdate 判定两个节点是否同一 identity（D1）：类型、key 与可信插件标记均相同。
func canUpdate(old *Element, node *widget.Node) bool {
	return old.Type == node.Type && old.Key == node.Key && old.plugin == widget.IsPlugin(node)
}

// patchProps 对变化属性逐一应用（D2 属性级 patch）。完全相等直接跳过。
// 返回是否存在"真实属性变化"（值非函数且非生命周期/_bind 隐藏键）——
// OnUpdate 触发判定（Phase 4.3）。事件回调/生命周期钩子恒判变化（函数值），
// 但它们不是配置更新，不计入。
var orderedProps = []string{
	"Items", "SelectedIndex",
	"Minimum", "Maximum", "Step", "Value",
	"GridSize", "Headers", "ColumnWidths", "Cells", "Editable", "GridSelection",
}

func orderedProp(key string) bool {
	for _, candidate := range orderedProps {
		if key == candidate {
			return true
		}
	}
	return false
}

func (rc *Reconciler) patchProps(e *Element, node *widget.Node) bool {
	selectionDirty := e.Type == "PageControl" && e.pageSelectionDirty
	if e.Props.Equal(node.Props) {
		if selectionDirty {
			rc.applyPageSelectedIndex(e, node.Props)
			e.pageSelectionDirty = false
		}
		return false
	}
	changed := false
	// 移除的属性（新树不再声明）→ 回落到挂载默认值（D2 对称：删除的配置不残留
	// 在原生控件上）。框架管理键/透明容器在 applyRemoved 内天然跳过。
	// 受控组合属性的移除也必须按其依赖顺序处理，不能依赖 Props 的 map 遍历顺序。
	removedKeys := e.Props.Removed(node.Props)
	for _, key := range removedKeys {
		if orderedProp(key) {
			continue
		}
		if rc.applyRemoved(e, key, node.Props) {
			changed = true
		}
	}
	diffKeys := node.Props.Diff(e.Props)
	for _, key := range diffKeys {
		if orderedProp(key) {
			continue
		}
		v, _ := node.Props.Get(key)
		// "_bind" 是 flux 隐藏的绑定依赖键（state.go，diff 不 import flux 防循环）。
		// 同 State 的 Binding 指针判等、不在 Diff 内；这里兜底排除。
		if !widget.IsFunc(v) && key != "_bind" && key != "_pluginRuntime" && key != "OnMount" && key != "OnUpdate" && key != "OnUnmount" {
			changed = true
		}
		rc.applyProp(e, key, v)
	}
	// 受控组合属性按固定顺序逐项完成移除或更新，不能先处理全部移除再处理
	// 全部更新。例如同一轮改变 GridSize 并移除 Cells 时，必须先下发新尺寸，
	// 再按新尺寸生成空矩阵。
	selectionPatched := false
	for _, key := range orderedProps {
		removedKey := false
		for _, candidate := range removedKeys {
			if candidate == key {
				removedKey = true
				break
			}
		}
		if removedKey {
			if rc.applyRemoved(e, key, node.Props) {
				changed = true
			}
			if e.Type == "PageControl" && key == "SelectedIndex" {
				selectionPatched = true
			}
			continue
		}
		changedKey := false
		for _, diffKey := range diffKeys {
			if diffKey == key {
				changedKey = true
				break
			}
		}
		if !changedKey {
			continue
		}
		v, _ := node.Props.Get(key)
		changed = true
		rc.applyProp(e, key, v)
		if e.Type == "PageControl" && key == "SelectedIndex" {
			selectionPatched = true
		}
	}
	if selectionDirty {
		if !selectionPatched {
			rc.applyPageSelectedIndex(e, node.Props)
		}
		e.pageSelectionDirty = false
	}
	return changed
}

// applyProps 挂载时全量应用属性（无旧值可比）。ComboBox 的 Items 必须先于
// SelectedIndex 应用，避免调用方的 Opt 声明顺序使有效的受控索引先被空列表钳制。
func (rc *Reconciler) applyProps(e *Element, props *widget.Props) {
	for _, key := range props.Keys() {
		if orderedProp(key) {
			continue
		}
		v, _ := props.Get(key)
		rc.applyProp(e, key, v)
	}
	for _, key := range orderedProps {
		if e.Type == "PageControl" && key == "SelectedIndex" {
			continue
		}
		if v, ok := props.Get(key); ok {
			rc.applyProp(e, key, v)
		}
	}
}

// applyPageSelectedIndex 在页面全部挂载并完成顺序同步后应用最终受控索引。
// 手写 Node 未声明 SelectedIndex 时，非空页面默认第 0 页；后端会把空页面规范为 -1。
func (rc *Reconciler) applyPageSelectedIndex(e *Element, props *widget.Props) {
	index := 0
	if value, ok := props.Get("SelectedIndex"); ok {
		configured, valid := value.(int)
		if !valid {
			return
		}
		index = configured
	}
	rc.applyProp(e, "SelectedIndex", index)
}

// applyRemoved 处理被移除的属性（新树不再声明）→ 回落到挂载默认值（D2 对称）。
//
// 只重置"用户声明、框架不写"的属性；框架写入/管理的键（Bounds/Scroll*、
// Flex/对齐/绑定/生命周期/逃逸口）绝不在此重置 —— 布局与滚动引擎每次 render
// 重新写入，生命周期与绑定语义天然以"新 props 为准"。透明容器无独立原生句柄，
// 任何重置都会命中继承的父句柄 → 直接跳过。
//
// 返回是否构成真实配置变化（OnUpdate 触发判定）：事件/框架键的移除不算
// （与 patchProps 的 func 排除一致）。
func (rc *Reconciler) applyRemoved(e *Element, key string, next *widget.Props) bool {
	if transparentElement(e) {
		return false
	}
	v, _ := e.Props.Get(key)
	switch key {
	case "Visible":
		rc.applyProp(e, key, true) // 挂载默认：可见
		return true
	case "Enabled":
		rc.applyProp(e, key, true) // 挂载默认：启用
		return true
	case "AccessibleName", "AccessibleDescription", "AccessibleValue":
		rc.applyProp(e, key, "")
		return true
	case "TabStop":
		if accessibility, ok := rc.r.(render.AccessibilityController); ok {
			accessibility.ResetTabStop(e.Handle)
			rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: key, Value: "default"})
		}
		return true
	case "DefaultButton", "CancelButton":
		rc.applyProp(e, key, false)
		return true
	case "Color":
		rc.applyProp(e, key, render.Color(0)) // 挂载默认：无背景
		return true
	case "FontColor":
		rc.applyProp(e, key, render.Color(0)) // 挂载默认：无文字色
		return true
	case "TitleBarDark":
		rc.applyProp(e, key, false)
		return true
	case "Checked":
		rc.applyProp(e, key, false) // 挂载默认：未选中
		return true
	case "Items":
		rc.applyProp(e, key, []string{}) // ComboBox 挂载默认：空选项
		return true
	case "SelectedIndex":
		if e.Type == "PageControl" {
			rc.applyProp(e, key, 0) // PageControl 默认：首个页面；空页面由后端规范为 -1
		} else {
			rc.applyProp(e, key, -1) // ComboBox 默认：未选择
		}
		return true
	case "Minimum":
		rc.applyProp(e, key, 0) // ProgressBar 挂载默认：最小值 0
		return true
	case "Maximum":
		rc.applyProp(e, key, 100) // ProgressBar 挂载默认：最大值 100
		return true
	case "Step":
		rc.applyProp(e, key, 1) // Slider 挂载默认：键盘步长 1
		return true
	case "Value":
		rc.applyProp(e, key, 0) // ProgressBar 挂载默认：当前值 0
		return true
	case "GroupIndex":
		rc.applyProp(e, key, 0) // RadioButton 挂载默认：组 0
		return true
	case "PaintCommands":
		rc.applyProp(e, key, []render.PaintCommand{}) // PaintBox 默认：空命令并重绘
		return true
	case "GridSize":
		rc.applyProp(e, key, render.GridSize{Columns: 1})
		return true
	case "Headers":
		rc.applyProp(e, key, []string{})
		return true
	case "ColumnWidths":
		rc.applyProp(e, key, []int{})
		return true
	case "Cells":
		size := resetGridSize(next)
		rc.applyProp(e, key, emptyGridCells(size))
		return true
	case "Editable":
		rc.applyProp(e, key, false)
		return true
	case "GridSelection":
		size := resetGridSize(next)
		selection := render.GridSelection{Cell: render.GridCell{Row: -1, Column: -1}}
		if size.Rows > 0 {
			selection.Cell = render.GridCell{}
		}
		rc.applyProp(e, key, selection)
		return true
	case "Text":
		rc.applyProp(e, key, "") // 挂载默认：空文本
		return true
	case "_bind":
		// 绑定依赖只供 App 收集 State 订阅；真实 native 解绑由对应公开
		// 属性（例如 Scroll）处理，不能按依赖值类型重复分派事件。
		return false
	default:
		if key == "OnValueChange" {
			if s, ok := rc.r.(render.SliderController); ok {
				s.OnSliderValueChange(e.Handle, nil)
				rc.record(e, render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: nil})
			}
			return false
		}
		if key == "OnCellSelect" || key == "OnCellEdit" {
			if grid, ok := rc.r.(render.GridController); ok {
				if key == "OnCellSelect" {
					grid.OnGridCellSelect(e.Handle, nil)
				} else {
					grid.OnGridCellEdit(e.Handle, nil)
				}
				rc.record(e, render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: nil})
			}
			return false
		}
		switch v.(type) {
		case func(render.Event), func(string): // 事件移除：解绑（native SetEvent nil 分支）
			rc.r.SetEvent(e.Handle, key, nil)
			rc.record(e, render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: nil})
		case render.ScrollTarget:
			if s, ok := rc.r.(render.Scrollable); ok {
				s.OnScroll(e.Handle, nil)
				rc.record(e, render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: nil})
			}
		case func(bool):
			if c, ok := rc.r.(render.Checkable); ok {
				c.OnCheckedChange(e.Handle, nil)
				rc.record(e, render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: nil})
			}
		case func(int):
			if e.Type == "PageControl" {
				if p, ok := rc.r.(render.PageController); ok {
					p.OnPageSelectionChange(e.Handle, nil)
					rc.record(e, render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: nil})
				}
			} else if s, ok := rc.r.(render.Selectable); ok {
				s.OnSelectionChange(e.Handle, nil)
				rc.record(e, render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: nil})
			}
		}
		return false
	}
}

func resetGridSize(next *widget.Props) render.GridSize {
	if next != nil {
		if value, ok := next.Get("GridSize"); ok {
			if size, valid := value.(render.GridSize); valid && size.Rows >= 0 && size.Columns > 0 {
				return size
			}
		}
	}
	return render.GridSize{Columns: 1}
}

func emptyGridCells(size render.GridSize) [][]string {
	cells := make([][]string, size.Rows)
	for row := range cells {
		cells[row] = make([]string, size.Columns)
	}
	return cells
}

// applyProp 按属性名分发到 Renderer 具体方法并记录 op。
// "Native"/"Ref" 为逃逸口；函数值为事件（SetEvent）。
func (rc *Reconciler) applyProp(e *Element, key string, v any) {
	// 透明容器（Column/Row/Expanded/Flexible/Component/ListViewRow）没有独立原生
	// 控件：Element.Handle 继承父容器句柄。挂载路径从不走进这里（mount 对透明
	// 容器跳过 applyProps），但 reconcile 路径会 —— 属性 patch 落到继承的父句柄
	// （如整个 Window）上：Visible(false) 会藏掉整窗、Color 会改父控件底色、事件
	// 会注册到父控件。统一守卫：透明容器不应用任何原生属性（Bounds 分支原有的
	// 透明判断因此冗余，保留作防御纵深）。
	if transparentElement(e) {
		return
	}
	switch key {
	case "Text":
		if s, ok := v.(string); ok {
			rc.r.SetText(e.Handle, s)
			rc.record(e, render.Op{Type: render.OpSetText, Handle: e.Handle, Value: s})
		}
	case "Visible":
		if b, ok := v.(bool); ok {
			rc.r.SetVisible(e.Handle, b)
			rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Visible", Value: b})
		}
	case "Enabled":
		if b, ok := v.(bool); ok {
			rc.r.SetEnabled(e.Handle, b)
			rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Enabled", Value: b})
		}
	case "AccessibleName":
		if value, ok := v.(string); ok {
			if accessibility, ok := rc.r.(render.AccessibilityController); ok {
				accessibility.SetAccessibleName(e.Handle, value)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: key, Value: value})
			}
		}
	case "AccessibleDescription":
		if value, ok := v.(string); ok {
			if accessibility, ok := rc.r.(render.AccessibilityController); ok {
				accessibility.SetAccessibleDescription(e.Handle, value)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: key, Value: value})
			}
		}
	case "AccessibleValue":
		if value, ok := v.(string); ok {
			if accessibility, ok := rc.r.(render.AccessibilityController); ok {
				accessibility.SetAccessibleValue(e.Handle, value)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: key, Value: value})
			}
		}
	case "TabStop":
		if enabled, ok := v.(bool); ok {
			if accessibility, ok := rc.r.(render.AccessibilityController); ok {
				accessibility.SetTabStop(e.Handle, enabled)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: key, Value: enabled})
			}
		}
	case "DefaultButton":
		if enabled, ok := v.(bool); ok {
			if accessibility, ok := rc.r.(render.AccessibilityController); ok {
				accessibility.SetDefaultButton(e.Handle, enabled)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: key, Value: enabled})
			}
		}
	case "CancelButton":
		if enabled, ok := v.(bool); ok {
			if accessibility, ok := rc.r.(render.AccessibilityController); ok {
				accessibility.SetCancelButton(e.Handle, enabled)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: key, Value: enabled})
			}
		}
	case "_tabOrder":
		if order, ok := v.(int); ok {
			if tabOrder, ok := rc.r.(render.TabOrderController); ok {
				tabOrder.SetTabOrder(e.Handle, order)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "TabOrder", Value: order})
			}
		}
	case "Checked":
		if b, ok := v.(bool); ok {
			if c, ok := rc.r.(render.Checkable); ok {
				c.SetChecked(e.Handle, b)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Checked", Value: b})
			}
		}
	case "Items":
		if items, ok := v.([]string); ok {
			if s, ok := rc.r.(render.Selectable); ok {
				items = copyItems(items)
				s.SetItems(e.Handle, items)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Items", Value: items})
			}
		}
	case "SelectedIndex":
		if index, ok := v.(int); ok {
			if e.Type == "PageControl" {
				if p, ok := rc.r.(render.PageController); ok {
					p.SetPageSelectedIndex(e.Handle, index)
					rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "SelectedIndex", Value: index})
				}
			} else if s, ok := rc.r.(render.Selectable); ok {
				s.SetSelectedIndex(e.Handle, index)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "SelectedIndex", Value: index})
			}
		}
	case "Minimum":
		if minimum, ok := v.(int); ok {
			if p, ok := rc.r.(render.Progressable); ok {
				p.SetMinimum(e.Handle, minimum)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Minimum", Value: minimum})
			}
		}
	case "Maximum":
		if maximum, ok := v.(int); ok {
			if p, ok := rc.r.(render.Progressable); ok {
				p.SetMaximum(e.Handle, maximum)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Maximum", Value: maximum})
			}
		}
	case "Step":
		if step, ok := v.(int); ok {
			if s, ok := rc.r.(render.SliderController); ok {
				s.SetSliderStep(e.Handle, step)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Step", Value: step})
			}
		}
	case "Value":
		if value, ok := v.(int); ok {
			if p, ok := rc.r.(render.Progressable); ok {
				p.SetValue(e.Handle, value)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Value", Value: value})
			}
		}
	case "GroupIndex":
		if groupIndex, ok := v.(int); ok {
			if g, ok := rc.r.(render.RadioGroupable); ok {
				g.SetGroupIndex(e.Handle, groupIndex)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "GroupIndex", Value: groupIndex})
			}
		}
	case "GridSize":
		if size, ok := v.(render.GridSize); ok {
			if grid, ok := rc.r.(render.GridController); ok {
				grid.SetGridSize(e.Handle, size)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: key, Value: size})
			}
		}
	case "Headers":
		if headers, ok := v.([]string); ok {
			if grid, ok := rc.r.(render.GridController); ok {
				headers = copyItems(headers)
				grid.SetGridHeaders(e.Handle, headers)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: key, Value: headers})
			}
		}
	case "ColumnWidths":
		if widths, ok := v.([]int); ok {
			if grid, ok := rc.r.(render.GridController); ok {
				widths = append([]int(nil), widths...)
				if len(widths) == 0 {
					widths = []int{}
				}
				grid.SetGridColumnWidths(e.Handle, widths)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: key, Value: widths})
			}
		}
	case "Cells":
		if cells, ok := v.([][]string); ok {
			if grid, ok := rc.r.(render.GridController); ok {
				cells = render.CloneGridCells(cells)
				grid.SetGridCells(e.Handle, cells)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: key, Value: cells})
			}
		}
	case "Editable":
		if editable, ok := v.(bool); ok {
			if grid, ok := rc.r.(render.GridController); ok {
				grid.SetGridEditable(e.Handle, editable)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: key, Value: editable})
			}
		}
	case "GridSelection":
		if selection, ok := v.(render.GridSelection); ok {
			if grid, ok := rc.r.(render.GridController); ok {
				grid.SetGridSelection(e.Handle, selection)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: key, Value: selection})
			}
		}
	case "PaintCommands":
		if e.Type != "PaintBox" {
			break
		}
		if commands, ok := v.([]render.PaintCommand); ok {
			if err := render.ValidatePaintCommands(commands); err != nil {
				panic(fmt.Sprintf("diff: invalid PaintCommands for %s: %v", eventSource(e), err))
			}
			if surface, ok := rc.r.(render.PaintController); ok {
				commands = render.ClonePaintCommands(commands)
				surface.SetPaintCommands(e.Handle, commands)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "PaintCommands", Value: commands})
				surface.InvalidatePaint(e.Handle)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "InvalidatePaint"})
			}
		}
	case "Bounds":
		// Window 是窗体本身：Bounds 只用于布局定位/诊断，应用到原生控件会把窗体外框收缩。
		// 透明容器已由 applyProp 顶部统一守卫截断（此判断冗余，防御纵深）。
		if transparentElement(e) || e.Type == "Window" || e.Type == "TabPage" {
			break
		}
		if r, ok := v.(render.Rect); ok {
			rc.r.SetBounds(e.Handle, r)
			rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Bounds", Value: r})
		}
	case "Color": // Phase 5.2 Theme 背景色
		if c, ok := v.(render.Color); ok {
			rc.r.SetColor(e.Handle, c)
			rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Color", Value: c})
		}
	case "FontColor": // Phase 5.2 Theme 文字色
		if c, ok := v.(render.Color); ok {
			rc.r.SetFontColor(e.Handle, c)
			rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "FontColor", Value: c})
		}
	case "TitleBarDark": // Phase 5.2 Theme 标题栏沉浸式暗色（win32 DWM；仅 Window 生效）
		if b, ok := v.(bool); ok {
			rc.r.SetTitleBarDark(e.Handle, b)
			rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "TitleBarDark", Value: b})
		}
	case "Native": // 逃逸口：控件创建后调用，注入绑定层原生对象
		if fn, ok := v.(func(any)); ok {
			rc.r.ApplyNative(e.Handle, fn)
			rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Native", Value: "fn"})
		}
	case "ItemCount", "ItemHeight", "Builder", "PluginProperties", "_pluginRuntime":
		// ListView 虚拟列表配置（Phase 6）：布局引擎据此重建可见区 slot 子树，
		// 无对应原生属性 —— 静默忽略（Builder 是 func，漏过 default 会误走 SetEvent
		// 触发 native panic）。
	case "ScrollConfig": // ListView 滚动配置（内容总高/滚轮步长，DIP）→ Scrollable
		if cfg, ok := v.(render.ScrollConfig); ok {
			if s, ok := rc.r.(render.Scrollable); ok {
				s.SetScrollConfig(e.Handle, cfg)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "ScrollConfig", Value: cfg})
			}
		}
	case "ScrollPos": // ListView 滚动位置（DIP）→ Scrollable
		if v, ok := v.(int); ok {
			if s, ok := rc.r.(render.Scrollable); ok {
				s.SetScrollPos(e.Handle, v)
				rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "ScrollPos", Value: v})
			}
		}
	case "Scroll": // ListView 滚动目标（ScrollTarget，可比值类型）→ 绑 OnScroll
		if st, ok := v.(render.ScrollTarget); ok {
			if s, ok := rc.r.(render.Scrollable); ok {
				s.OnScroll(e.Handle, func(pos int) {
					render.Guard("event.OnScroll", func() {
						if rc.eventSink != nil {
							rc.eventSink(EventDispatch{
								Name: "Scroll", Path: e.Path, Source: eventSource(e), Value: pos,
							})
						}
						st.Apply(pos)
					})
				})
				rc.record(e, render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: "Scroll", Value: st})
			}
		}
	case "OnCheckedChange":
		if fn, ok := v.(func(bool)); ok {
			if c, ok := rc.r.(render.Checkable); ok {
				c.OnCheckedChange(e.Handle, func(checked bool) {
					render.Guard("event.OnCheckedChange", func() {
						if rc.eventSink != nil {
							rc.eventSink(EventDispatch{
								Name: key, Path: e.Path, Source: eventSource(e), Value: checked,
							})
						}
						fn(checked)
					})
				})
				rc.record(e, render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: v})
			}
		}
	case "OnSelectionChange":
		if fn, ok := v.(func(int)); ok {
			wrap := func(index int) {
				render.Guard("event.OnSelectionChange", func() {
					if e.Type == "PageControl" {
						e.pageSelectionDirty = true
					}
					if rc.eventSink != nil {
						rc.eventSink(EventDispatch{
							Name: key, Path: e.Path, Source: eventSource(e), Value: index,
						})
					}
					fn(index)
				})
			}
			if e.Type == "PageControl" {
				if p, ok := rc.r.(render.PageController); ok {
					p.OnPageSelectionChange(e.Handle, wrap)
					rc.record(e, render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: v})
				}
			} else if s, ok := rc.r.(render.Selectable); ok {
				s.OnSelectionChange(e.Handle, wrap)
				rc.record(e, render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: v})
			}
		}
	case "OnValueChange":
		if fn, ok := v.(func(int)); ok {
			if s, ok := rc.r.(render.SliderController); ok {
				s.OnSliderValueChange(e.Handle, func(value int) {
					render.Guard("event.OnValueChange", func() {
						if rc.eventSink != nil {
							rc.eventSink(EventDispatch{
								Name: key, Path: e.Path, Source: eventSource(e), Value: value,
							})
						}
						fn(value)
					})
				})
				rc.record(e, render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: v})
			}
		}
	case "OnCellSelect":
		if fn, ok := v.(func(render.GridCell)); ok {
			if grid, ok := rc.r.(render.GridController); ok {
				grid.OnGridCellSelect(e.Handle, func(cell render.GridCell) {
					render.Guard("event.OnCellSelect", func() {
						if rc.eventSink != nil {
							rc.eventSink(EventDispatch{
								Name: key, Path: e.Path, Source: eventSource(e), Value: cell,
							})
						}
						fn(cell)
					})
				})
				rc.record(e, render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: v})
			}
		}
	case "OnCellEdit":
		if fn, ok := v.(func(render.GridCell, string)); ok {
			if grid, ok := rc.r.(render.GridController); ok {
				grid.OnGridCellEdit(e.Handle, func(cell render.GridCell, value string) {
					render.Guard("event.OnCellEdit", func() {
						if rc.eventSink != nil {
							rc.eventSink(EventDispatch{
								Name: key, Path: e.Path, Source: eventSource(e), Value: struct {
									Cell  render.GridCell
									Value string
								}{Cell: cell, Value: value},
							})
						}
						fn(cell, value)
					})
				})
				rc.record(e, render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: v})
			}
		}
	case "OnMount", "OnUpdate", "OnUnmount":
		// 生命周期钩子（Phase 4.3）：由 mount/reconcile/destroySubtree 显式触发，
		// 此处跳过 —— 不落 SetEvent（否则 native 按未知事件 panic），零 mutation。
	default:
		if ref, ok := v.(render.Ref); ok { // 逃逸口：引用绑定
			rc.r.AttachRef(e.Handle, ref)
			rc.record(e, render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Ref", Value: ref})
		} else if widget.IsFunc(v) {
			// 事件回调：每次重新绑定（无法比较相等性，D2 逃逸口行为）。
			// 统一事件（func(Event)）：注入稳定 Source（Type#Key，D3）后转发，
			// 供共享 handler 区分事件来源。
			if ef, ok := v.(func(render.Event)); ok {
				src := eventSource(e)
				rc.r.SetEvent(e.Handle, key, func(ev render.Event) {
					render.Guard("event."+key, func() {
						ev.Source = src
						if rc.eventSink != nil {
							rc.eventSink(EventDispatch{Name: key, Path: e.Path, Source: src, Event: ev, HasEvent: true})
						}
						ef(ev)
					})
				})
			} else if cf, ok := v.(func(string)); ok {
				// func(string)（如 Input 双向绑定 OnChange）：同样包错误边界。
				rc.r.SetEvent(e.Handle, key, func(s string) {
					render.Guard("event."+key, func() {
						if rc.eventSink != nil {
							rc.eventSink(EventDispatch{
								Name: key, Path: e.Path, Source: eventSource(e), Value: s,
							})
						}
						cf(s)
					})
				})
			} else {
				rc.r.SetEvent(e.Handle, key, v)
			}
			rc.record(e, render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: v})
		}
		// 其余未识别属性 Phase 1 静默忽略
	}
}

func eventSource(e *Element) string {
	source := e.Type
	if e.Key != "" {
		return source + "#" + e.Key
	}
	if e.Path != "" {
		return source + "@" + e.Path
	}
	return source
}

// fireLifecycle 触发生命周期钩子（OnMount/OnUpdate/OnUnmount，Phase 4.3）。
// props 来源随语义不同：挂载/更新用本次新节点 props，销毁用 Element 最后
// 成功的 props（prevConfig，保存用户最后一次设置的钩子）。
func (rc *Reconciler) fireLifecycle(props *widget.Props, name string) {
	if v, ok := props.Get(name); ok {
		if fn, ok := v.(func()); ok {
			render.Guard("lifecycle."+name, fn)
		}
	}
}

func (rc *Reconciler) firePluginLifecycle(isPlugin bool, key string, props *widget.Props, stage string) {
	if !isPlugin {
		return
	}
	lifecycle, ok := pluginLifecycle(props)
	if !ok {
		return
	}
	switch stage {
	case "mount":
		lifecycle.PluginMount(key, props)
	case "update":
		lifecycle.PluginUpdate(key, props)
	case "unmount":
		lifecycle.PluginUnmount(key, props)
	}
}

func pluginLifecycle(props *widget.Props) (PluginLifecycle, bool) {
	if props == nil {
		return nil, false
	}
	value, ok := props.Get("_pluginRuntime")
	if !ok {
		return nil, false
	}
	lifecycle, ok := value.(PluginLifecycle)
	return lifecycle, ok && lifecycle != nil
}

// reconcileChildren 对齐子节点列表（D3 稳定 key / 无 key 按位置）。
//
// 匹配策略：带 key 的子节点按 key 精确匹配旧 element（复用，重排不重建）；
// 无 key 按位置匹配，且位置匹配只在"无 key 的旧子节点"之间进行 —— keyed 旧
// 元素是 key 匹配的专属资源（D3），绝不参与位置匹配，否则会被无 key 子抢占：
// canUpdate 因 Key 不同而失败 → keyed 旧元素被销毁、其句柄进入延后销毁，但
// oldByKey 索引仍持有它 → 同一 render 内后续 keyed 新节点按 key 匹配到"已销毁"
// 的 Element → 死句柄复活（对已 Free 控件二次 patch/二次 Destroy，native 崩溃）。
//
// 匹配者 reconcile（原地 patch），未匹配者挂载；未复用的旧 element 整棵销毁。
// 透明容器子节点直接挂到祖父句柄。
func (rc *Reconciler) reconcileChildren(oldP *Element, newP *widget.Node) bool {
	oldKids := oldP.Children
	oldOrder := childHandles(oldKids)

	oldByKey := make(map[string]*Element) // D3：带 key 的旧子节点索引
	var nonKeyedOlds []*Element           // 无 key 旧子节点（按顺序，位置匹配池）
	for _, e := range oldKids {
		if e.Key != "" {
			oldByKey[e.Key] = e
		} else {
			nonKeyedOlds = append(nonKeyedOlds, e)
		}
	}
	newKeys := make(map[string]struct{})
	for _, node := range newP.Children {
		if node.Key != "" {
			newKeys[node.Key] = struct{}{}
		}
	}

	used := make(map[*Element]bool)
	newElems := make([]*Element, 0, len(newP.Children))
	posIdx := 0
	for i, nc := range newP.Children {
		childPath := oldP.Path + "/" + strconv.Itoa(i) + "/" + nc.Type // 隐式寻址：子路径 = 父路径+下标+类型
		var oe *Element
		if nc.Key != "" {
			if cand := oldByKey[nc.Key]; cand != nil && !used[cand] {
				oe = cand
			}
		} else if posIdx < len(nonKeyedOlds) {
			oe = nonKeyedOlds[posIdx]
		}

		var ne *Element
		if oe != nil {
			used[oe] = true
			if nc.Key == "" {
				posIdx++ // 消耗一个无 key 旧槽位
			}
			ne = rc.reconcile(oe, nc, childPath)
		} else {
			// keyed replacement 不会进入 canUpdate：新 key 无候选，旧 key 在尾部销毁。
			// 仅当同槽旧 key 也从新列表消失时配对，避免把插入或重排误报为重建。
			if nc.Key != "" && i < len(oldKids) {
				candidate := oldKids[i]
				_, oldKeyStillPresent := newKeys[candidate.Key]
				if candidate.Key != "" && !used[candidate] && !oldKeyStillPresent {
					reason := "key-mismatch"
					if candidate.Type != nc.Type {
						reason = "type-and-key-mismatch"
					}
					rc.rebuilds = append(rc.rebuilds, Rebuild{
						Path: childPath, OldPath: candidate.Path, OldType: candidate.Type, OldKey: candidate.Key,
						NewType: nc.Type, NewKey: nc.Key, Reason: reason,
						TypeChanged: candidate.Type != nc.Type, KeyChanged: true,
					})
				}
			}
			ne = rc.mount(nc, oldP, childPath)
		}

		// 仅当父关系实际变化（新建或跨容器移动）才发挂载 op；
		// 原地复用（D7c：相同树零 mutation）与列表内重排（D7b：不迁移焦点）不发。
		if !transparentNode(nc) && ne.Parent != oldP {
			rc.r.SetParent(ne.Handle, oldP.Handle)
			rc.record(ne, render.Op{Type: render.OpAppendChild, Handle: ne.Handle, Parent: oldP.Handle, ParentPath: oldP.Path})
		}
		ne.Parent = oldP
		newElems = append(newElems, ne)
	}

	for _, oe := range oldKids { // 销毁未复用的旧子树
		if !used[oe] {
			rc.destroySubtree(oe)
		}
	}
	oldP.Children = newElems
	return !equalHandles(oldOrder, childHandles(newElems))
}

func childHandles(children []*Element) []render.Handle {
	handles := make([]render.Handle, len(children))
	for i, child := range children {
		handles[i] = child.Handle
	}
	return handles
}

func equalHandles(a, b []render.Handle) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (rc *Reconciler) syncPages(parent *Element) {
	pager, ok := rc.r.(render.PageController)
	if !ok {
		return
	}
	pages := childHandles(parent.Children)
	pager.SyncPages(parent.Handle, pages)
	rc.record(parent, render.Op{Type: render.OpSetProperty, Handle: parent.Handle, Key: "Pages", Value: pages})
}

// destroySubtree 销毁整棵子树（后序：先子后父）。透明容器不销毁句柄（无原生控件），
// 只递归销毁真实子控件。卸载前触发 OnUnmount（Phase 4.3）；物理释放由
// Renderer.Destroy 入队延后（D4，App 在 render 后 DrainDestroy）。
func (rc *Reconciler) destroySubtree(e *Element) {
	for _, c := range e.Children {
		rc.destroySubtree(c)
	}
	rc.fireLifecycle(e.Props, "OnUnmount")
	rc.firePluginLifecycle(e.plugin, e.Key, e.Props, "unmount")
	if !transparentElement(e) {
		rc.r.Destroy(e.Handle)
		rc.record(e, render.Op{Type: render.OpDestroy, Handle: e.Handle})
	}
}

func (rc *Reconciler) record(e *Element, op render.Op) {
	if e != nil {
		op.Path = e.Path
		if op.ParentPath == "" && e.Parent != nil {
			op.ParentPath = e.Parent.Path
		}
	}
	rc.lastOps = append(rc.lastOps, op)
}
