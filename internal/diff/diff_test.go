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

func checkBoxNode(s, key string, checked bool) *widget.Node {
	n := widget.NewNode("CheckBox")
	n.Props.Set("Text", s)
	n.Props.Set("Checked", checked)
	n.Props.Set("Bounds", render.Rect{X: 0, Y: 0, W: 120, H: 24})
	n.Key = key
	return n
}

func comboBoxNode(items []string, index int, key string) *widget.Node {
	n := widget.NewNode("ComboBox")
	n.Props.Set("Items", append([]string(nil), items...))
	n.Props.Set("SelectedIndex", index)
	n.Props.Set("Bounds", render.Rect{X: 0, Y: 0, W: 120, H: 28})
	n.Key = key
	return n
}

func progressBarNode(minimum, maximum, value int, key string) *widget.Node {
	n := widget.NewNode("ProgressBar")
	n.Props.Set("Minimum", minimum)
	n.Props.Set("Maximum", maximum)
	n.Props.Set("Value", value)
	n.Props.Set("Bounds", render.Rect{X: 0, Y: 0, W: 180, H: 20})
	n.Key = key
	return n
}

func radioButtonNode(text, key string, checked bool, groupIndex int) *widget.Node {
	n := widget.NewNode("RadioButton")
	n.Props.Set("Text", text)
	n.Props.Set("Checked", checked)
	n.Props.Set("GroupIndex", groupIndex)
	n.Props.Set("Bounds", render.Rect{X: 0, Y: 0, W: 120, H: 24})
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

// TestCheckBoxControlledPatch CheckBox 的受控状态变更只更新 Checked 属性；
// 同树保持零 mutation，既不重建也不重复写状态。
func TestCheckBoxControlledPatch(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	rc.Render(windowTree(colWith(checkBoxNode("accept", "check", false))))
	h := findByKey(rc.Root(), "check").Handle
	if m.Checked(h) {
		t.Fatal("首次挂载 Checked(false) 后 mock 状态应为 false")
	}

	ops := rc.Render(windowTree(colWith(checkBoxNode("accept", "check", true))))
	if n := countOps(ops, render.OpCreate); n != 0 {
		t.Errorf("Checked patch 的 Create = %d，期望 0", n)
	}
	if n := countOps(ops, render.OpDestroy); n != 0 {
		t.Errorf("Checked patch 的 Destroy = %d，期望 0", n)
	}
	if !m.Checked(h) {
		t.Error("Checked(true) 未原地写入 mock")
	}
	if ops := rc.Render(windowTree(colWith(checkBoxNode("accept", "check", true)))); len(ops) != 0 {
		t.Fatalf("相同 CheckBox 树应零 mutation，实际 %+v", ops)
	}
}

// TestCheckBoxRemovalAndEvent CheckBox 的 Checked 移除回落 false，布尔事件可触发、
// 每次 render 重绑，并在属性移除时解绑。
func TestCheckBoxRemovalAndEvent(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)
	called := false

	first := checkBoxNode("accept", "check", true)
	first.Props.Set("OnCheckedChange", func(checked bool) { called = checked })
	rc.Render(windowTree(colWith(first)))
	h := findByKey(rc.Root(), "check").Handle
	m.FireCheckedChange(h, true)
	if !called {
		t.Fatal("OnCheckedChange 未通过 Checkable mock 触发")
	}

	second := checkBoxNode("accept", "check", true)
	second.Props.Set("OnCheckedChange", func(bool) {})
	ops := rc.Render(windowTree(colWith(second)))
	if n := countOps(ops, render.OpSetEvent); n != 1 {
		t.Errorf("OnCheckedChange 重绑 SetEvent = %d，期望 1：%+v", n, ops)
	}

	removed := widget.NewNode("CheckBox")
	removed.Props.Set("Text", "accept")
	removed.Props.Set("Bounds", render.Rect{X: 0, Y: 0, W: 120, H: 24})
	removed.Key = "check"
	ops = rc.Render(windowTree(colWith(removed)))
	if m.Checked(h) {
		t.Error("移除 Checked 后应回落到 false")
	}
	if !hasSetProperty(ops, h, "Checked", false) {
		t.Errorf("移除 Checked 应产生 false 回落：%+v", ops)
	}
	if !hasSetEvent(ops, h, "OnCheckedChange", nil) {
		t.Errorf("移除 OnCheckedChange 应产生 nil 解绑：%+v", ops)
	}
	called = false
	m.FireCheckedChange(h, true)
	if called {
		t.Error("移除 OnCheckedChange 后不应继续触发旧回调")
	}
}

// TestComboBoxControlledPatch 覆盖 Items/SelectedIndex 的受控 patch、选项深相等
// 的 D7c 零 mutation，以及 Mock 选择回调的锁外触发。
func TestComboBoxControlledPatch(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	rc.Render(windowTree(colWith(comboBoxNode([]string{"short", "longest"}, 1, "combo"))))
	h := findByKey(rc.Root(), "combo").Handle
	if got := m.SelectedIndex(h); got != 1 {
		t.Fatalf("首次 SelectedIndex = %d，期望 1", got)
	}
	if got := m.Items(h); fmt.Sprint(got) != "[short longest]" {
		t.Fatalf("首次 Items = %v", got)
	}

	ops := rc.Render(windowTree(colWith(comboBoxNode([]string{"short", "longest"}, 0, "combo"))))
	if countOps(ops, render.OpCreate) != 0 || countOps(ops, render.OpDestroy) != 0 {
		t.Fatalf("SelectedIndex patch 不应重建：%+v", ops)
	}
	if !hasSetProperty(ops, h, "SelectedIndex", 0) || m.SelectedIndex(h) != 0 {
		t.Fatalf("SelectedIndex patch 未写入：%+v", ops)
	}

	ops = rc.Render(windowTree(colWith(comboBoxNode([]string{"one", "two", "three"}, 9, "combo"))))
	if countOps(ops, render.OpCreate) != 0 || countOps(ops, render.OpDestroy) != 0 {
		t.Fatalf("Items patch 不应重建：%+v", ops)
	}
	if got := m.SelectedIndex(h); got != 2 {
		t.Errorf("越界 SelectedIndex 应钳制为末项 2，实际 %d", got)
	}
	if !hasSetProperty(ops, h, "Items", []string{"one", "two", "three"}) {
		t.Errorf("Items patch 缺失：%+v", ops)
	}

	if ops := rc.Render(windowTree(colWith(comboBoxNode([]string{"one", "two", "three"}, 9, "combo")))); len(ops) != 0 {
		t.Fatalf("内容相等的新 Items slice 应零 mutation，实际 %+v", ops)
	}

	called := -2
	node := comboBoxNode([]string{"one", "two", "three"}, 2, "combo")
	node.Props.Set("OnSelectionChange", func(index int) { called = index })
	rc.Render(windowTree(colWith(node)))
	m.FireSelectionChange(h, 1)
	if called != 1 {
		t.Fatalf("OnSelectionChange 未由 Selectable mock 触发，实际 %d", called)
	}
}

// TestComboBoxRemovalAndEvent 确保属性删除回落到规范默认值并解绑旧选择回调。
func TestComboBoxRemovalAndEvent(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)
	called := false
	first := comboBoxNode([]string{"a", "b"}, 1, "combo")
	first.Props.Set("OnSelectionChange", func(int) { called = true })
	rc.Render(windowTree(colWith(first)))
	h := findByKey(rc.Root(), "combo").Handle

	removed := widget.NewNode("ComboBox")
	removed.Props.Set("Bounds", render.Rect{X: 0, Y: 0, W: 120, H: 28})
	removed.Key = "combo"
	ops := rc.Render(windowTree(colWith(removed)))
	if got := m.Items(h); len(got) != 0 {
		t.Errorf("移除 Items 后应为空，实际 %v", got)
	}
	if got := m.SelectedIndex(h); got != -1 {
		t.Errorf("移除 SelectedIndex 后应为 -1，实际 %d", got)
	}
	if !hasSetEvent(ops, h, "OnSelectionChange", nil) {
		t.Errorf("移除 OnSelectionChange 应解绑：%+v", ops)
	}
	m.FireSelectionChange(h, 0)
	if called {
		t.Error("移除 OnSelectionChange 后不应继续触发旧回调")
	}
}

func hasSetProperty(ops []render.Op, h render.Handle, key string, want any) bool {
	for _, op := range ops {
		if op.Type == render.OpSetProperty && op.Handle == h && op.Key == key && fmt.Sprint(op.Value) == fmt.Sprint(want) {
			return true
		}
	}
	return false
}

func hasSetEvent(ops []render.Op, h render.Handle, key string, want any) bool {
	for _, op := range ops {
		if op.Type == render.OpSetEvent && op.Handle == h && op.Key == key && op.Value == want {
			return true
		}
	}
	return false
}

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

// TestFindByPath 隐式寻址：每个 Element 维护树路径（"类型/下标/类型..."），
// 静态树零 Key 也可定位（寻址与身份解耦，D3 补充）。验证类型校验/越界/空路径，
// 以及带 key 控件重排后 Path 跟随新位置（身份复用，位置漂移）。
func TestFindByPath(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	// Window
	// ├─ Column(0)
	// │  ├─ Text "a"(0)
	// │  └─ Button "b"(1)
	// └─ Row(1)
	//    └─ Text "c"(0)
	w := widget.NewNode("Window")
	col := colWith(textNode("a", ""), btnNode("b", ""))
	row := widget.NewNode("Row")
	row.Add(textNode("c", ""))
	w.Add(col)
	w.Add(row)

	rc.Render(w)
	root := rc.Root()

	// 路径值由 mount 自顶向下维护。
	if got := root.Path; got != "Window" {
		t.Errorf("root.Path = %q，期望 Window", got)
	}
	colEl := root.Children[0]
	if got := colEl.Path; got != "Window/0/Column" {
		t.Errorf("col.Path = %q，期望 Window/0/Column", got)
	}
	btnEl := root.Children[0].Children[1]
	if got := btnEl.Path; got != "Window/0/Column/1/Button" {
		t.Errorf("btn.Path = %q，期望 Window/0/Column/1/Button", got)
	}

	// 按路径定位：类型段校验 + 下标段选子。
	cases := []struct {
		path string
		typ  string
	}{
		{"Window", "Window"},
		{"Window/0/Column", "Column"},
		{"Window/0/Column/0/Text", "Text"},
		{"Window/0/Column/1/Button", "Button"},
		{"Window/1/Row/0/Text", "Text"},
		{"Window/0/Column/1/Button/", ""}, // 尾部空段：Atoi 失败 → nil
	}
	for _, c := range cases {
		got := root.FindByPath(c.path)
		if c.typ == "" {
			if got != nil {
				t.Errorf("FindByPath(%q) 应返回 nil，实际命中 %s", c.path, got.Type)
			}
			continue
		}
		if got == nil {
			t.Errorf("FindByPath(%q) = nil，期望命中 %s", c.path, c.typ)
			continue
		}
		if got.Type != c.typ {
			t.Errorf("FindByPath(%q).Type = %s，期望 %s", c.path, got.Type, c.typ)
		}
	}

	// 类型不符 / 越界 / 根类型不符 / 空路径 / nil 接收者 → nil。
	for _, path := range []string{"Window/0/Row", "Window/5/Column", "Column/0/Text", "", "Window/0/Column/0/Button"} {
		if got := root.FindByPath(path); got != nil {
			t.Errorf("FindByPath(%q) 应返回 nil，实际命中 %s", path, got.Type)
		}
	}
	var nilEl *diff.Element
	if got := nilEl.FindByPath("Window"); got != nil {
		t.Errorf("nil 接收者 FindByPath 应返回 nil，实际 %+v", got)
	}

	// 带 key 控件重排：按 key 复用同一 Element（句柄不变），Path 跟随新位置。
	keyed := colWith(textNode("a", ""), btnNode("b", "b"))
	rc.Render(windowTree(keyed))
	bEl := rc.Root().FindByPath("Window/0/Column/1/Button")
	if bEl == nil {
		t.Fatal("keyed 首次 render 未命中 Button")
	}
	bHandle := bEl.Handle

	rc.Render(windowTree(colWith(textNode("a", ""), textNode("x", ""), btnNode("b", "b")))) // b 移到下标 2
	bEl2 := rc.Root().FindByPath("Window/0/Column/2/Button")
	if bEl2 == nil {
		t.Fatalf("重排后按新路径未命中 Button（应原地复用）")
	}
	if bEl2.Handle != bHandle {
		t.Errorf("keyed 重排应复用同一句柄：%v != %v（位置身份需配合稳定 key）", bEl2.Handle, bHandle)
	}
	if got := bEl2.Path; got != "Window/0/Column/2/Button" {
		t.Errorf("重排后 Path = %q，期望 Window/0/Column/2/Button（路径跟随新位置）", got)
	}
}
