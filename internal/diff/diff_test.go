package diff_test

import (
	"fmt"
	"testing"

	"github.com/xiaowumin-mark/flux-vcl/internal/diff"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
	"github.com/xiaowumin-mark/flux-vcl/internal/widget"
)

// —— 树构造 helpers（测试手写 Bounds，布局 pass 属 flux 层职责）——

func windowTree(col *widget.Node) *widget.Node {
	w := widget.NewNode("Window")
	w.Props.Set("Visible", true)
	w.Add(col)
	return w
}

func colWith(children ...*widget.Node) *widget.Node {
	c := widget.NewNode("Column")
	for _, n := range children {
		c.Add(n)
	}
	return c
}

func textNode(s, key string) *widget.Node {
	n := widget.NewNode("Text")
	n.Props.Set("Text", s)
	n.Props.Set("Bounds", render.Rect{X: 0, Y: 0, W: 80, H: 20})
	n.Key = key
	return n
}

func btnNode(s, key string) *widget.Node {
	n := widget.NewNode("Button")
	n.Props.Set("Text", s)
	n.Props.Set("Bounds", render.Rect{X: 0, Y: 0, W: 120, H: 40})
	n.Key = key
	return n
}

// —— 断言 helpers ——

func countOps(ops []render.Op, t render.OpType) int {
	n := 0
	for _, op := range ops {
		if op.Type == t {
			n++
		}
	}
	return n
}

func firstOpOf(ops []render.Op, t render.OpType) *render.Op {
	for i := range ops {
		if ops[i].Type == t {
			return &ops[i]
		}
	}
	return nil
}

func findByKey(e *diff.Element, key string) *diff.Element {
	if e.Key == key {
		return e
	}
	for _, c := range e.Children {
		if found := findByKey(c, key); found != nil {
			return found
		}
	}
	return nil
}

// —— 测试 ——

// TestMount 首次挂载产生正确的 op 序列：Window/Text/Button 各 Create 一次，
// Column 为透明容器不产生任何原生 op；子控件挂到祖父句柄。
func TestMount(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	ops := rc.Render(windowTree(colWith(textNode("hi", ""), btnNode("go", ""))))

	if n := countOps(ops, render.OpCreate); n != 3 { // Window + Text + Button
		t.Errorf("Create 次数 = %d，期望 3：%+v", n, ops)
	}
	if n := countOps(ops, render.OpSetText); n != 2 {
		t.Errorf("SetText 次数 = %d，期望 2（Text+Button 各一）", n)
	}
	if n := countOps(ops, render.OpAppendChild); n != 2 {
		t.Errorf("AppendChild 次数 = %d，期望 2（Text+Button 挂 Window，Column 透明）", n)
	}
}

// TestD7aPurePropertyChangeNoRebuild 纯属性变化绝不重建控件：
// 文本更新只走 SetText patch，零 Create / 零 Destroy。
func TestD7aPurePropertyChangeNoRebuild(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	rc.Render(windowTree(colWith(textNode("Hello", "t"), btnNode("Click me", ""))))
	rc.Render(windowTree(colWith(textNode("Clicked 1", "t"), btnNode("Clicked 1", ""))))

	ops := rc.Render(windowTree(colWith(textNode("Clicked 2", "t"), btnNode("Clicked 2", ""))))
	if n := countOps(ops, render.OpCreate); n != 0 {
		t.Errorf("Create 次数 = %d，期望 0（纯属性变化绝不重建）：%+v", n, ops)
	}
	if n := countOps(ops, render.OpDestroy); n != 0 {
		t.Errorf("Destroy 次数 = %d，期望 0", n)
	}
	if n := countOps(ops, render.OpSetText); n != 2 {
		t.Errorf("SetText 次数 = %d，期望 2（Text 与 Button 各 patch 一次）", n)
	}
}

// TestD7bStableKeyReorderNoRebuild 稳定 key 列表重排不迁移焦点/IME：
// 重排只调整顺序，零 Create / 零 Destroy，且每个 key 对应的句柄保持不变
// （控件实例未重建，焦点/IME/caret 不会漂移 —— D3/D7b 的工程动机）。
func TestD7bStableKeyReorderNoRebuild(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	rc.Render(windowTree(colWith(
		btnNode("A", "a"), btnNode("B", "b"), btnNode("C", "c"),
	)))
	// 记录首挂载的句柄
	root := rc.Root()
	handles := map[string]render.Handle{}
	for _, k := range []string{"a", "b", "c"} {
		handles[k] = findByKey(root, k).Handle
	}

	// 重排：C 移到最前
	ops := rc.Render(windowTree(colWith(
		btnNode("C", "c"), btnNode("A", "a"), btnNode("B", "b"),
	)))

	if n := countOps(ops, render.OpCreate); n != 0 {
		t.Errorf("重排后 Create 次数 = %d，期望 0（控件池复用，不迁移焦点）", n)
	}
	if n := countOps(ops, render.OpDestroy); n != 0 {
		t.Errorf("重排后 Destroy 次数 = %d，期望 0", n)
	}
	root = rc.Root()
	for _, k := range []string{"a", "b", "c"} {
		if got := findByKey(root, k).Handle; got != handles[k] {
			t.Errorf("重排后 key %q 句柄 %d ≠ 首挂载 %d —— 控件被重建", k, got, handles[k])
		}
	}
}

// TestD7cSameTreeZeroMutation 相同树 diff 零 mutation：
// 完全相同（含无事件回调）的树二次 diff 不产生任何 op。
func TestD7cSameTreeZeroMutation(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	rc.Render(windowTree(colWith(textNode("A", "a"), btnNode("B", "b"))))
	ops := rc.Render(windowTree(colWith(textNode("A", "a"), btnNode("B", "b"))))

	if len(ops) != 0 {
		t.Fatalf("相同树二次 diff 应零 mutation，实际 %d 条：%+v", len(ops), ops)
	}
}

// TestTypeChangeRebuildsOnlyThatNode 类型变化只重建该节点（D1）：
// Text 换成 Button，只销毁/重建该节点，Column 与 Window 不重建，兄弟不受影响。
func TestTypeChangeRebuildsOnlyThatNode(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	rc.Render(windowTree(colWith(textNode("A", "a"), btnNode("B", "b"))))
	ops := rc.Render(windowTree(colWith(btnNode("A", "a"), btnNode("B", "b"))))

	if n := countOps(ops, render.OpDestroy); n != 1 {
		t.Errorf("Destroy 次数 = %d，期望 1（只销毁被换类型的旧 Text）", n)
	}
	if n := countOps(ops, render.OpCreate); n != 1 {
		t.Errorf("Create 次数 = %d，期望 1（只新建新 Button），其余节点应原地复用", n)
	}
	if firstOpOf(ops, render.OpDestroy).Key != "" {
		t.Errorf("Destroy op 不应带 Key")
	}
}

// TestEventReboundEachRender 事件回调每次 render 重新绑定（函数值无法比较），
// 但不重建控件（D2 逃逸口行为）。
func TestEventReboundEachRender(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	btn := btnNode("go", "")
	btn.Props.Set("OnClick", func(render.Event) {})
	rc.Render(windowTree(colWith(btn)))

	btn2 := btnNode("go", "")
	btn2.Props.Set("OnClick", func(render.Event) {})
	ops := rc.Render(windowTree(colWith(btn2)))

	if n := countOps(ops, render.OpSetEvent); n != 1 {
		t.Errorf("SetEvent 次数 = %d，期望 1（每次重新绑定）", n)
	}
	if n := countOps(ops, render.OpCreate); n != 0 {
		t.Errorf("Create 次数 = %d，期望 0（事件重绑不应重建控件）", n)
	}
}

// TestRemoveSubtreeDestroys 子树被删除时整棵销毁（后序：先子后父）。
func TestRemoveSubtreeDestroys(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	rc.Render(windowTree(colWith(btnNode("A", "a"), btnNode("B", "b"), btnNode("C", "c"))))
	ops := rc.Render(windowTree(colWith(btnNode("A", "a"), btnNode("C", "c"))))

	if n := countOps(ops, render.OpDestroy); n != 1 {
		t.Errorf("Destroy 次数 = %d，期望 1（仅被删除的 B）", n)
	}
	if n := countOps(ops, render.OpCreate); n != 0 {
		t.Errorf("Create 次数 = %d，期望 0", n)
	}
}

// TestD3KeyedChildNotConsumedPositionally 回归（Phase 6 后发现的 reconcile 缺陷）：
// 同一容器混合 keyed 与非 keyed 兄弟时，keyed 旧元素绝不能被无 key 子节点的"位置
// 匹配"抢占。被抢占的路径会销毁 keyed 元素、但其句柄仍在 oldByKey 索引里 —— 同一
// render 内后续 keyed 新节点按 key 匹配到"已销毁"Element → 死句柄复活（对已 Free
// 原生控件二次 patch/二次 Destroy → native nil panic）。修复后：
//   - render2 插入无 key 元素不再销毁 keyed 兄弟（零 Destroy 该句柄）；
//   - keyed 元素保持 D3 身份（句柄不变、跨 render 原地 patch）。
func TestD3KeyedChildNotConsumedPositionally(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	colWith := func(kids ...*widget.Node) *widget.Node {
		c := widget.NewNode("Column")
		for _, k := range kids {
			c.Add(k)
		}
		return c
	}
	text := func(s, key string) *widget.Node {
		n := textNode(s, key)
		n.Props.Set("Text", s)
		return n
	}

	// render1：仅挂载 keyed 头部
	rc.Render(colWith(text("hdr", "hdr")))
	firstHdr := findByKey(rc.Root(), "hdr").Handle

	// render2：在头部前插入无 key 元素 → 位置匹配不得抢占 keyed hdr
	ops := rc.Render(colWith(text("body", ""), text("hdr", "hdr")))
	destroyedHdr := false
	for _, op := range ops {
		if op.Type == render.OpDestroy && op.Handle == firstHdr {
			destroyedHdr = true
		}
	}
	if destroyedHdr {
		t.Fatalf("keyed hdr（handle=%d）被无 key 兄弟的位置匹配抢占销毁 —— D3 破坏", firstHdr)
	}
	if got := findByKey(rc.Root(), "hdr").Handle; got != firstHdr {
		t.Fatalf("keyed hdr 句柄变化：%d → %d（应原地复用）", firstHdr, got)
	}

	// render3：修改 keyed 头部文本 → 原地 patch 到存活句柄。若 render2 误销毁
	// 了 hdr，此处 SetText 就命中已删除句柄（native nil panic）—— 死句柄复活信号。
	ops = rc.Render(colWith(text("body", ""), text("hdr-updated", "hdr")))
	for _, op := range ops {
		if op.Type == render.OpSetText && op.Handle == firstHdr && destroyedHdr {
			t.Fatalf("已销毁的 keyed hdr（handle=%d）又被 SetText 命中 —— 死句柄复活", firstHdr)
		}
	}
	if n := countOps(ops, render.OpCreate); n != 0 {
		t.Fatalf("render3 不应新建控件（keyed 原地 patch），Create=%d", n)
	}
	if got := findByKey(rc.Root(), "hdr").Props.String("Text"); got != "hdr-updated" {
		t.Fatalf("keyed hdr 文本应原地 patch 为 hdr-updated，实际 %q", got)
	}
}

// TestTransparentContainerPropsNoNativeOp 透明容器的属性 patch 绝不命中继承的
// 父句柄（审查修复：reconcile 对 Column 的 Visible patch 会 SetVisible(Window
// 句柄) → 整窗被隐藏/改色；统一守卫后透明容器不应用任何原生属性，零 mutation）。
func TestTransparentContainerPropsNoNativeOp(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	col := colWith(textNode("hi", ""))
	col.Props.Set("Visible", false) // 透明容器上的属性（无 UI 语义）
	rc.Render(windowTree(col))

	col2 := colWith(textNode("hi", ""))
	col2.Props.Set("Visible", true) // patch：Visible 变化 → 应用路径
	ops := rc.Render(windowTree(col2))

	if len(ops) != 0 {
		t.Fatalf("透明容器属性 patch 应零 mutation（不命中继承的父句柄），实际 %d 条：%+v", len(ops), ops)
	}
	if n := countOps(ops, render.OpSetProperty); n != 0 {
		t.Errorf("SetProperty 次数 = %d，期望 0（透明容器不应用原生属性）", n)
	}
}

// TestRemovedPropsReset 属性移除回落到挂载默认值（D2 对称）：二次 diff 删掉
// Visible/Enabled/Color/FontColor/OnClick → 各自重置 + 事件解绑，且不重建控件。
func TestRemovedPropsReset(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	btn := btnNode("go", "b")
	btn.Props.Set("Visible", false)
	btn.Props.Set("Enabled", false)
	btn.Props.Set("Color", render.RGB(1, 2, 3))
	btn.Props.Set("FontColor", render.RGB(4, 5, 6))
	btn.Props.Set("OnClick", func(render.Event) {})
	rc.Render(windowTree(colWith(btn)))
	h := findByKey(rc.Root(), "b").Handle
	if m.EventHandler(h, "OnClick") == nil {
		t.Fatalf("render1 应绑定 OnClick")
	}

	// 移除全部非 Text/Bounds 属性
	btn2 := btnNode("go", "b")
	ops := rc.Render(windowTree(colWith(btn2)))

	if n := countOps(ops, render.OpCreate); n != 0 {
		t.Errorf("Create 次数 = %d，期望 0（移除属性不重建控件）", n)
	}
	if n := countOps(ops, render.OpDestroy); n != 0 {
		t.Errorf("Destroy 次数 = %d，期望 0", n)
	}
	findProp := func(key string, want any) bool {
		for _, op := range ops {
			if op.Type == render.OpSetProperty && op.Handle == h && op.Key == key &&
				fmt.Sprint(op.Value) == fmt.Sprint(want) {
				return true
			}
		}
		return false
	}
	if !findProp("Visible", true) {
		t.Errorf("Visible 应重置为挂载默认 true：%+v", ops)
	}
	if !findProp("Enabled", true) {
		t.Errorf("Enabled 应重置为挂载默认 true")
	}
	if !findProp("Color", render.Color(0)) {
		t.Errorf("Color 应重置为 0")
	}
	if !findProp("FontColor", render.Color(0)) {
		t.Errorf("FontColor 应重置为 0")
	}
	if m.EventHandler(h, "OnClick") != nil {
		t.Errorf("OnClick 应已解绑（EventHandler 返回 nil）")
	}
}

// TestRemovedPropOnTransparentContainerNoOp 透明容器的属性移除不产生重置 op
// （applyRemoved 对透明容器直接跳过 —— 重置会命中继承的父句柄）。
func TestRemovedPropOnTransparentContainerNoOp(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	col := colWith(textNode("hi", ""))
	col.Props.Set("Visible", false)
	rc.Render(windowTree(col))

	// 移除透明容器的 Visible（新树不再声明）
	ops := rc.Render(windowTree(colWith(textNode("hi", ""))))

	if n := countOps(ops, render.OpSetProperty); n != 0 {
		t.Errorf("SetProperty 次数 = %d，期望 0（透明容器移除属性不重置到父句柄）", n)
	}
}

// TestPanicInEventCallbackIsCaught D4 错误边界：用户事件回调 panic 不崩进程
// （diff 包装 render.Guard，mock 直接触发回调的测试路径同样受保护）。若未捕获，
// 本测试会直接崩溃 —— 回调内 panic 是验收信号。
func TestPanicInEventCallbackIsCaught(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	btn := btnNode("go", "b")
	btn.Props.Set("OnClick", func(render.Event) { panic("boom") })
	rc.Render(windowTree(colWith(btn)))

	fn, ok := m.EventHandler(findByKey(rc.Root(), "b").Handle, "OnClick").(func(render.Event))
	if !ok {
		t.Fatalf("OnClick 回调应可触发")
	}
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("事件回调 panic 应被 D4 错误边界捕获，实际逃逸：%v", rec)
			}
		}()
		fn(render.Event{Type: render.EventClick})
	}()
}

// TestPanicInLifecycleIsCaught 生命周期钩子 panic 同被 D4 错误边界捕获（OnMount）。
func TestPanicInLifecycleIsCaught(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	btn := btnNode("go", "b")
	btn.Props.Set("OnMount", func() { panic("boom") })
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				t.Fatalf("OnMount panic 应被捕获，实际逃逸：%v", rec)
			}
		}()
		rc.Render(windowTree(colWith(btn)))
	}()
}
