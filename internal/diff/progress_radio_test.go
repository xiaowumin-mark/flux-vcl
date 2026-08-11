package diff_test

import (
	"fmt"
	"testing"

	"github.com/xiaowumin-mark/flux-vcl/internal/diff"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
	"github.com/xiaowumin-mark/flux-vcl/internal/widget"
)

// TestProgressBarOrderedPatch 覆盖范围优先、值在后的 mount/patch 语义，及同树
// 零 mutation。公共构造器已规范化，本测试直接覆盖 diff 的确定性应用顺序。
func TestProgressBarOrderedPatch(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	ops := rc.Render(windowTree(colWith(progressBarNode(20, 80, 60, "progress"))))
	h := findByKey(rc.Root(), "progress").Handle
	var keys []string
	for _, op := range ops {
		if op.Handle == h && op.Type == render.OpSetProperty {
			keys = append(keys, op.Key)
		}
	}
	if fmt.Sprint(keys) != "[Bounds Minimum Maximum Value]" {
		t.Fatalf("ProgressBar mount 属性顺序 = %v，期望 Bounds 后 Minimum→Maximum→Value", keys)
	}
	if minimum, maximum, value := m.Progress(h); minimum != 20 || maximum != 80 || value != 60 {
		t.Fatalf("首次 Progress = %d/%d/%d，期望 20/80/60", minimum, maximum, value)
	}

	ops = rc.Render(windowTree(colWith(progressBarNode(40, 50, 50, "progress"))))
	keys = keys[:0]
	for _, op := range ops {
		if op.Handle == h && op.Type == render.OpSetProperty {
			keys = append(keys, op.Key)
		}
	}
	if fmt.Sprint(keys) != "[Minimum Maximum Value]" {
		t.Fatalf("ProgressBar patch 属性顺序 = %v，期望 Minimum→Maximum→Value", keys)
	}
	if minimum, maximum, value := m.Progress(h); minimum != 40 || maximum != 50 || value != 50 {
		t.Errorf("patch 后 Progress = %d/%d/%d，期望 40/50/50", minimum, maximum, value)
	}
	if ops := rc.Render(windowTree(colWith(progressBarNode(40, 50, 50, "progress")))); len(ops) != 0 {
		t.Fatalf("相同 ProgressBar 树应零 mutation，实际 %+v", ops)
	}
}

// TestProgressBarRemovalAndRadioButtonPatch 覆盖属性移除的默认值回落，以及
// RadioButton 对 Checkable/RadioGroupable 的复用和事件解绑。
func TestProgressBarRemovalAndRadioButtonPatch(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)
	called := false

	radio := radioButtonNode("one", "radio", true, 2)
	radio.Props.Set("OnCheckedChange", func(checked bool) { called = checked })
	rc.Render(windowTree(colWith(progressBarNode(10, 90, 55, "progress"), radio)))
	progress := findByKey(rc.Root(), "progress").Handle
	radioHandle := findByKey(rc.Root(), "radio").Handle
	m.FireCheckedChange(radioHandle, true)
	if !called || !m.Checked(radioHandle) || m.GroupIndex(radioHandle) != 2 {
		t.Fatalf("RadioButton 状态/事件未接通：called=%v checked=%v group=%d", called, m.Checked(radioHandle), m.GroupIndex(radioHandle))
	}

	bareProgress := widget.NewNode("ProgressBar")
	bareProgress.Props.Set("Bounds", render.Rect{X: 0, Y: 0, W: 180, H: 20})
	bareProgress.Key = "progress"
	bareRadio := radioButtonNode("one", "radio", false, 0)
	ops := rc.Render(windowTree(colWith(bareProgress, bareRadio)))
	if minimum, maximum, value := m.Progress(progress); minimum != 0 || maximum != 100 || value != 0 {
		t.Errorf("移除 ProgressBar 属性后 = %d/%d/%d，期望 0/100/0", minimum, maximum, value)
	}
	var removedKeys []string
	for _, op := range ops {
		if op.Handle == progress && op.Type == render.OpSetProperty {
			removedKeys = append(removedKeys, op.Key)
		}
	}
	if fmt.Sprint(removedKeys) != "[Minimum Maximum Value]" {
		t.Errorf("ProgressBar 移除属性顺序 = %v，期望 Minimum→Maximum→Value", removedKeys)
	}
	if m.GroupIndex(radioHandle) != 0 || m.Checked(radioHandle) {
		t.Errorf("RadioButton 移除/更新后 group=%d checked=%v，期望 0/false", m.GroupIndex(radioHandle), m.Checked(radioHandle))
	}
	if !hasSetEvent(ops, radioHandle, "OnCheckedChange", nil) {
		t.Errorf("移除 RadioButton OnCheckedChange 应解绑：%+v", ops)
	}
	called = false
	m.FireCheckedChange(radioHandle, true)
	if called {
		t.Error("移除 RadioButton 回调后不应继续触发旧回调")
	}
}

// TestRadioButtonLogicalGroups 覆盖 Flux 逻辑 GroupIndex 的异组独立、同组互斥、
// 用户变更和属性移除回落默认组。Mock 模拟 Renderer 的公开契约，原生细节由 native probe 覆盖。
func TestRadioButtonLogicalGroups(t *testing.T) {
	m := render.NewMock()
	rc := diff.New(m)

	left := radioButtonNode("left", "left", true, 1)
	right := radioButtonNode("right", "right", true, 2)
	rc.Render(windowTree(colWith(left, right)))
	leftHandle := findByKey(rc.Root(), "left").Handle
	rightHandle := findByKey(rc.Root(), "right").Handle
	if !m.Checked(leftHandle) || !m.Checked(rightHandle) {
		t.Fatalf("不同 GroupIndex 应独立选中：left=%v right=%v", m.Checked(leftHandle), m.Checked(rightHandle))
	}

	right = radioButtonNode("right", "right", true, 1)
	rc.Render(windowTree(colWith(left, right)))
	if m.Checked(leftHandle) || !m.Checked(rightHandle) {
		t.Fatalf("同组声明 true 时后下发控件应成为唯一选中项：left=%v right=%v", m.Checked(leftHandle), m.Checked(rightHandle))
	}

	m.FireCheckedChange(leftHandle, true)
	if !m.Checked(leftHandle) || m.Checked(rightHandle) {
		t.Fatalf("用户选择同组 RadioButton 后应清除 peer：left=%v right=%v", m.Checked(leftHandle), m.Checked(rightHandle))
	}

	bareLeft := widget.NewNode("RadioButton")
	bareLeft.Props.Set("Text", "left")
	bareLeft.Props.Set("Checked", true)
	bareLeft.Props.Set("Bounds", render.Rect{X: 0, Y: 0, W: 120, H: 24})
	bareLeft.Key = "left"
	rc.Render(windowTree(colWith(bareLeft, right)))
	if m.GroupIndex(leftHandle) != 0 || !m.Checked(leftHandle) || m.Checked(rightHandle) {
		t.Fatalf("移除 GroupIndex 应回落默认组，且不得改变另一 group 的现有状态：left group=%d checked=%v right=%v", m.GroupIndex(leftHandle), m.Checked(leftHandle), m.Checked(rightHandle))
	}
}
