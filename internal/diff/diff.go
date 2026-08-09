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
//
// 事件回调 / 逃逸口（函数值）无法比较相等性，每次 diff 重新绑定（D2 逃逸口行为）。
package diff

import (
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
	"github.com/xiaowumin-mark/flux-vcl/internal/widget"
)

// transparentType 返回类型是否为透明容器：Column/Row/Expanded/Flexible 是纯
// 逻辑分组，不创建原生控件，Element 的 handle 继承父容器，children 直接挂到祖父。
// Component（Phase 5.4）同样是透明分组：子树 = build() 结果，身份靠外部 Key
// （D3），不产生原生控件。ListViewRow（Phase 6）是虚拟列表行：也是透明包装
// （不建原生控件、子挂祖父），身份靠 slot key（控件池槽位，跨 render 复用）。
func transparentType(t string) bool {
	switch t {
	case "Column", "Row", "Expanded", "Flexible", "Component", "ListViewRow":
		return true
	}
	return false
}

// Element 是 Element 树的节点（design.md §4.3 / D1）：持久 identity。
//
// 在控件存活期间复用同一 Element（Type+Key 匹配时），持有当前原生句柄
// 与上一次的 Props（prevConfig，用于属性级 diff）。widget 树每次重建，
// Element 树跨 render 存活 —— 二者通过 reconcile 对齐。
type Element struct {
	Type     string
	Key      string
	Props    *widget.Props // prevConfig：上一次成功 reconcile 的属性集
	Handle   render.Handle // 原生句柄（透明容器 = 父容器句柄）
	Parent   *Element
	Children []*Element
}

// parentHandle 返回父容器的原生句柄（root 无父时为零值）。
func (e *Element) parentHandle() render.Handle {
	if e.Parent != nil {
		return e.Parent.Handle
	}
	return 0
}

// Reconciler 是 diff 引擎：持有 Element 树根与 Renderer，逐次 Render 对齐。
type Reconciler struct {
	r       render.Renderer
	root    *Element
	lastOps []render.Op
}

// New 创建空 Reconciler。
func New(r render.Renderer) *Reconciler { return &Reconciler{r: r} }

// Root 返回当前 Element 树根（首次 Render 前为 nil）。
func (rc *Reconciler) Root() *Element { return rc.root }

// IsTransparent 报告类型是否为透明容器（无原生控件，Element 句柄继承父）。
// 供 flux 根包的逃逸口（App.SetBounds）判断目标是否有独立原生句柄可直改。
func IsTransparent(t string) bool { return transparentType(t) }

// Lookup 按 key 在 Element 树中查找节点（深度优先，先父后子）。
//
// Phase 5.1 动画的逃逸口支撑：App.SetBounds 用稳定 key 定位 Element 句柄，
// 直接应用几何而不重跑 diff（D2 逃逸口）。未找到返回 nil。
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

// Render 对整棵树做一次 diff：首次调用挂载整棵树，后续按 D1/D2 增量 patch。
//
// 返回本次产生的 mutation op 列表（已直接应用到 renderer；副本返回供断言/日志）。
// 相同树 diff 零 mutation（D7c）：未变属性不产生 op，未匹配节点不重建。
func (rc *Reconciler) Render(root *widget.Node) []render.Op {
	rc.lastOps = nil
	if rc.root == nil {
		rc.root = rc.mount(root, 0)
	} else {
		rc.root = rc.reconcile(rc.root, root)
	}
	return append([]render.Op(nil), rc.lastOps...)
}

// mount 挂载新子树：创建原生控件（透明容器除外）、应用全部属性、递归子节点。
// 子树全部挂载完成后触发 OnMount（Phase 4.3：父钩子在子钩子之后，可访问完整子树）。
func (rc *Reconciler) mount(node *widget.Node, parentHandle render.Handle) *Element {
	e := &Element{Type: node.Type, Key: node.Key, Props: node.Props}

	if transparentType(node.Type) {
		e.Handle = parentHandle // 透明容器：无原生控件，继承父句柄
	} else {
		e.Handle = rc.r.Create(node.Type)
		rc.record(render.Op{Type: render.OpCreate, Handle: e.Handle, Key: node.Type})
		rc.applyProps(e, node.Props)
	}

	for _, c := range node.Children {
		ce := rc.mount(c, e.Handle)
		ce.Parent = e
		e.Children = append(e.Children, ce)
		if !transparentType(c.Type) {
			rc.r.SetParent(ce.Handle, e.Handle)
			rc.record(render.Op{Type: render.OpAppendChild, Handle: ce.Handle, Parent: e.Handle})
		}
	}
	rc.fireLifecycle(node.Props, "OnMount")
	return e
}

// reconcile 对齐一个已匹配节点（canUpdate 通过）：属性 patch + 递归 children。
// canUpdate 未通过时：销毁旧子树、挂载新子树（只重建该节点，不上溯祖先 —— D1）。
// 子树对齐完成后，若节点存在"真实属性变化"（非事件重绑/生命周期钩子），触发
// OnUpdate（Phase 4.3：Flutter didUpdateWidget 语义 —— 配置变化才回调，避免
// 每次 render 都触发导致钩子里 Set State → 无限 re-render）。
func (rc *Reconciler) reconcile(old *Element, node *widget.Node) *Element {
	if !canUpdate(old, node) {
		rc.destroySubtree(old)
		return rc.mount(node, old.parentHandle())
	}

	changed := rc.patchProps(old, node)
	rc.reconcileChildren(old, node)
	if changed {
		rc.fireLifecycle(node.Props, "OnUpdate")
	}
	old.Props = node.Props
	return old
}

// canUpdate 判定两个节点是否同一 identity（D1）：类型相同 && key 相同。
func canUpdate(old *Element, node *widget.Node) bool {
	return old.Type == node.Type && old.Key == node.Key
}

// patchProps 对变化属性逐一应用（D2 属性级 patch）。完全相等直接跳过。
// 返回是否存在"真实属性变化"（值非函数且非生命周期/_bind 隐藏键）——
// OnUpdate 触发判定（Phase 4.3）。事件回调/生命周期钩子恒判变化（函数值），
// 但它们不是配置更新，不计入。
func (rc *Reconciler) patchProps(e *Element, node *widget.Node) bool {
	if e.Props.Equal(node.Props) {
		return false
	}
	changed := false
	// 移除的属性（新树不再声明）→ 回落到挂载默认值（D2 对称：删除的配置不残留
	// 在原生控件上）。框架管理键/透明容器在 applyRemoved 内天然跳过。
	for _, key := range e.Props.Removed(node.Props) {
		if rc.applyRemoved(e, key) {
			changed = true
		}
	}
	for _, key := range node.Props.Diff(e.Props) {
		v, _ := node.Props.Get(key)
		// "_bind" 是 flux 隐藏的绑定依赖键（state.go，diff 不 import flux 防循环）。
		// 同 State 的 Binding 指针判等、不在 Diff 内；这里兜底排除。
		if !widget.IsFunc(v) && key != "_bind" && key != "OnMount" && key != "OnUpdate" && key != "OnUnmount" {
			changed = true
		}
		rc.applyProp(e, key, v)
	}
	return changed
}

// applyProps 挂载时全量应用属性（无旧值可比）。
func (rc *Reconciler) applyProps(e *Element, props *widget.Props) {
	for _, key := range props.Keys() {
		v, _ := props.Get(key)
		rc.applyProp(e, key, v)
	}
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
func (rc *Reconciler) applyRemoved(e *Element, key string) bool {
	if transparentType(e.Type) {
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
	case "Color":
		rc.applyProp(e, key, render.Color(0)) // 挂载默认：无背景
		return true
	case "FontColor":
		rc.applyProp(e, key, render.Color(0)) // 挂载默认：无文字色
		return true
	case "TitleBarDark":
		rc.applyProp(e, key, false)
		return true
	case "Text":
		rc.applyProp(e, key, "") // 挂载默认：空文本
		return true
	default:
		switch v.(type) {
		case func(render.Event), func(string): // 事件移除：解绑（native SetEvent nil 分支）
			rc.r.SetEvent(e.Handle, key, nil)
			rc.record(render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: nil})
		}
		return false
	}
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
	if transparentType(e.Type) {
		return
	}
	switch key {
	case "Text":
		if s, ok := v.(string); ok {
			rc.r.SetText(e.Handle, s)
			rc.record(render.Op{Type: render.OpSetText, Handle: e.Handle, Value: s})
		}
	case "Visible":
		if b, ok := v.(bool); ok {
			rc.r.SetVisible(e.Handle, b)
			rc.record(render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Visible", Value: b})
		}
	case "Enabled":
		if b, ok := v.(bool); ok {
			rc.r.SetEnabled(e.Handle, b)
			rc.record(render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Enabled", Value: b})
		}
	case "Bounds":
		// Window 是窗体本身：Bounds 只用于布局定位/诊断，应用到原生控件会把窗体外框收缩。
		// 透明容器已由 applyProp 顶部统一守卫截断（此判断冗余，防御纵深）。
		if transparentType(e.Type) || e.Type == "Window" {
			break
		}
		if r, ok := v.(render.Rect); ok {
			rc.r.SetBounds(e.Handle, r)
			rc.record(render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Bounds", Value: r})
		}
	case "Color": // Phase 5.2 Theme 背景色
		if c, ok := v.(render.Color); ok {
			rc.r.SetColor(e.Handle, c)
			rc.record(render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Color", Value: c})
		}
	case "FontColor": // Phase 5.2 Theme 文字色
		if c, ok := v.(render.Color); ok {
			rc.r.SetFontColor(e.Handle, c)
			rc.record(render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "FontColor", Value: c})
		}
	case "TitleBarDark": // Phase 5.2 Theme 标题栏沉浸式暗色（win32 DWM；仅 Window 生效）
		if b, ok := v.(bool); ok {
			rc.r.SetTitleBarDark(e.Handle, b)
			rc.record(render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "TitleBarDark", Value: b})
		}
	case "Native": // 逃逸口：控件创建后调用，注入绑定层原生对象
		if fn, ok := v.(func(any)); ok {
			rc.r.ApplyNative(e.Handle, fn)
			rc.record(render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Native", Value: "fn"})
		}
	case "ItemCount", "ItemHeight", "Builder":
		// ListView 虚拟列表配置（Phase 6）：布局引擎据此重建可见区 slot 子树，
		// 无对应原生属性 —— 静默忽略（Builder 是 func，漏过 default 会误走 SetEvent
		// 触发 native panic）。
	case "ScrollConfig": // ListView 滚动配置（内容总高/滚轮步长，DIP）→ Scrollable
		if cfg, ok := v.(render.ScrollConfig); ok {
			if s, ok := rc.r.(render.Scrollable); ok {
				s.SetScrollConfig(e.Handle, cfg)
				rc.record(render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "ScrollConfig", Value: cfg})
			}
		}
	case "ScrollPos": // ListView 滚动位置（DIP）→ Scrollable
		if v, ok := v.(int); ok {
			if s, ok := rc.r.(render.Scrollable); ok {
				s.SetScrollPos(e.Handle, v)
				rc.record(render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "ScrollPos", Value: v})
			}
		}
	case "Scroll": // ListView 滚动目标（ScrollTarget，可比值类型）→ 绑 OnScroll
		if st, ok := v.(render.ScrollTarget); ok {
			if s, ok := rc.r.(render.Scrollable); ok {
				s.OnScroll(e.Handle, func(pos int) {
					render.Guard("event.OnScroll", func() { st.Apply(pos) })
				})
				rc.record(render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: "Scroll", Value: st})
			}
		}
	case "OnMount", "OnUpdate", "OnUnmount":
		// 生命周期钩子（Phase 4.3）：由 mount/reconcile/destroySubtree 显式触发，
		// 此处跳过 —— 不落 SetEvent（否则 native 按未知事件 panic），零 mutation。
	default:
		if ref, ok := v.(render.Ref); ok { // 逃逸口：引用绑定
			rc.r.AttachRef(e.Handle, ref)
			rc.record(render.Op{Type: render.OpSetProperty, Handle: e.Handle, Key: "Ref", Value: ref})
		} else if widget.IsFunc(v) {
			// 事件回调：每次重新绑定（无法比较相等性，D2 逃逸口行为）。
			// 统一事件（func(Event)）：注入稳定 Source（Type#Key，D3）后转发，
			// 供共享 handler 区分事件来源。
			if ef, ok := v.(func(render.Event)); ok {
				src := e.Type
				if e.Key != "" {
					src += "#" + e.Key
				}
				rc.r.SetEvent(e.Handle, key, func(ev render.Event) {
					render.Guard("event."+key, func() {
						ev.Source = src
						ef(ev)
					})
				})
			} else if cf, ok := v.(func(string)); ok {
				// func(string)（如 Input 双向绑定 OnChange）：同样包错误边界。
				rc.r.SetEvent(e.Handle, key, func(s string) {
					render.Guard("event."+key, func() { cf(s) })
				})
			} else {
				rc.r.SetEvent(e.Handle, key, v)
			}
			rc.record(render.Op{Type: render.OpSetEvent, Handle: e.Handle, Key: key, Value: v})
		}
		// 其余未识别属性 Phase 1 静默忽略
	}
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
func (rc *Reconciler) reconcileChildren(oldP *Element, newP *widget.Node) {
	oldKids := oldP.Children

	oldByKey := make(map[string]*Element) // D3：带 key 的旧子节点索引
	var nonKeyedOlds []*Element           // 无 key 旧子节点（按顺序，位置匹配池）
	for _, e := range oldKids {
		if e.Key != "" {
			oldByKey[e.Key] = e
		} else {
			nonKeyedOlds = append(nonKeyedOlds, e)
		}
	}

	used := make(map[*Element]bool)
	newElems := make([]*Element, 0, len(newP.Children))
	posIdx := 0
	for _, nc := range newP.Children {
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
			ne = rc.reconcile(oe, nc)
		} else {
			ne = rc.mount(nc, oldP.Handle)
		}

		// 仅当父关系实际变化（新建或跨容器移动）才发挂载 op；
		// 原地复用（D7c：相同树零 mutation）与列表内重排（D7b：不迁移焦点）不发。
		if !transparentType(nc.Type) && ne.Parent != oldP {
			rc.r.SetParent(ne.Handle, oldP.Handle)
			rc.record(render.Op{Type: render.OpAppendChild, Handle: ne.Handle, Parent: oldP.Handle})
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
}

// destroySubtree 销毁整棵子树（后序：先子后父）。透明容器不销毁句柄（无原生控件），
// 只递归销毁真实子控件。卸载前触发 OnUnmount（Phase 4.3）；物理释放由
// Renderer.Destroy 入队延后（D4，App 在 render 后 DrainDestroy）。
func (rc *Reconciler) destroySubtree(e *Element) {
	for _, c := range e.Children {
		rc.destroySubtree(c)
	}
	rc.fireLifecycle(e.Props, "OnUnmount")
	if !transparentType(e.Type) {
		rc.r.Destroy(e.Handle)
		rc.record(render.Op{Type: render.OpDestroy, Handle: e.Handle})
	}
}

func (rc *Reconciler) record(op render.Op) { rc.lastOps = append(rc.lastOps, op) }
