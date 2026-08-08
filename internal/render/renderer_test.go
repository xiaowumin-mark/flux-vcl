package render

import "testing"

// TestBasicFlowRecordsOps 驱动一个与 examples/basic 等价的声明式流程，
// 但完全在内存中（无 DLL、无窗口）——证明 0.6 无头测试驱动可用。
// 它也是 Phase 1.4 diff 引擎测试的样板：mock renderer + op 断言。
func TestBasicFlowRecordsOps(t *testing.T) {
	m := NewMock()

	// —— 模拟一次 render：创建窗体 + label + button（Phase 1 的 render 管线）——
	form := m.Create("Form")
	label := m.Create("Label")
	btn := m.Create("Button")
	m.SetParent(label, form)
	m.SetParent(btn, form)
	m.SetBounds(btn, Rect{X: 150, Y: 100, W: 120, H: 40})
	m.SetText(btn, "Click me")
	m.SetVisible(form, true)

	// —— 模拟一次状态更新（点击按钮）：只 patch 文本，不重建控件 ——
	m.SetText(btn, "Clicked 1")

	ops := m.Ops()
	// 3 Create + 2 AppendChild + 1 Bounds + 2 SetText + 1 Visible = 9
	if len(ops) != 9 {
		t.Fatalf("ops 数量 = %d，期望 9：%+v", len(ops), ops)
	}

	// D7(a) 不变量前哨：按钮只创建一次，更新走 SetText 而非重建
	if got := m.Count(OpCreate); got != 3 {
		t.Errorf("Create 次数 = %d，期望 3（纯文本更新不应重建控件）", got)
	}

	// 按钮的两条 SetText 落在同一句柄上，顺序正确
	var texts []string
	for _, op := range ops {
		if op.Type == OpSetText && op.Handle == btn {
			texts = append(texts, op.Value.(string))
		}
	}
	if len(texts) != 2 || texts[0] != "Click me" || texts[1] != "Clicked 1" {
		t.Errorf("按钮文本序列 = %v，期望 [Click me Clicked 1]", texts)
	}
}

// TestMockHasNoDisplayDependency 印证 0.6 的"驱动"本身：render 包的测试
// 不接触 energye/lcl 或 libenergy DLL，任何平台 / 任何 CI 上 `go test` 即可运行。
func TestMockHasNoDisplayDependency(t *testing.T) {
	m := NewMock()
	if m.HandleAllocated(0) {
		t.Error("零句柄不应被视为已分配")
	}
	if !m.HandleAllocated(42) {
		t.Error("非零句柄应被视为已分配")
	}
	// 空 mock 无任何 op
	if n := len(m.Ops()); n != 0 {
		t.Errorf("新建 mock 应无 op，实际 %d", n)
	}
}
