package diff_test

import (
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
	btn.Props.Set("OnClick", func() {})
	rc.Render(windowTree(colWith(btn)))

	btn2 := btnNode("go", "")
	btn2.Props.Set("OnClick", func() {})
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
