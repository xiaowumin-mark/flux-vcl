package widget

import "testing"

// TestPropsDiff 验证 D2 属性级 diff 的判定：
// 未变属性不产生 patch；变化/新增属性产生；函数值恒 patch。
func TestPropsDiff(t *testing.T) {
	oldP := NewProps()
	oldP.Set("Text", "hello")
	oldP.Set("Visible", true)
	oldP.Set("Bounds", struct{ X, Y, W, H int }{1, 2, 3, 4})
	oldP.Set("OnClick", func() {}) // 函数值

	newP := NewProps()
	newP.Set("Text", "hello")              // 未变
	newP.Set("Visible", false)             // 变化
	newP.Set("Bounds", struct{ X, Y, W, H int }{1, 2, 3, 4}) // 未变
	newP.Set("OnClick", func() {})         // 新闭包 → 恒变化
	newP.Set("Enabled", true)              // 新增

	changed := newP.Diff(oldP)
	want := []string{"Visible", "OnClick", "Enabled"}
	if len(changed) != len(want) {
		t.Fatalf("Diff 结果 = %v，期望 %v", changed, want)
	}
	for i, k := range want {
		if changed[i] != k {
			t.Errorf("Diff[%d] = %q，期望 %q（顺序应稳定）", i, changed[i], k)
		}
	}
}

// TestPropsEqual 相同属性集（含顺序）应相等；顺序无关但值不同不等。
func TestPropsEqual(t *testing.T) {
	a := NewProps()
	a.Set("A", 1)
	a.Set("B", "x")
	b := NewProps()
	b.Set("A", 1)
	b.Set("B", "x")
	if !a.Equal(b) {
		t.Error("相同属性集应相等")
	}
	b.Set("B", "y")
	if a.Equal(b) {
		t.Error("值不同的属性集不应相等")
	}
}

// TestValuesEqualFunc 函数值永远不相等（事件每次重新绑定）。
func TestValuesEqualFunc(t *testing.T) {
	f1 := func() {}
	f2 := func() {}
	if ValuesEqual(f1, f2) {
		t.Error("函数值不应被认为相等")
	}
	if ValuesEqual(f1, f1) {
		t.Error("即使同一闭包，函数值也应视为不等（保守策略）")
	}
	if !ValuesEqual(1, 1) || !ValuesEqual("a", "a") || !ValuesEqual(true, true) {
		t.Error("基本类型相等比较错误")
	}
	if ValuesEqual(1, "1") {
		t.Error("跨类型不应相等")
	}
}
