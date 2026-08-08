package native

import (
	"testing"

	"github.com/energye/lcl/types"

	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// Phase 4.2 事件映射纯函数单测：不触碰 libenergy DLL，任意平台可跑
// （go test ./internal/native/）。验证 LCL 回调负载 → 统一 Event 的映射，
// 特别是坐标 DIP 归一（D5 全坐标 DIP）。

// TestMapButton LCL TMouseButton → 统一 MouseButton。
func TestMapButton(t *testing.T) {
	cases := []struct {
		in   types.TMouseButton
		want render.MouseButton
	}{
		{types.MbLeft, render.ButtonLeft},
		{types.MbRight, render.ButtonRight},
		{types.MbMiddle, render.ButtonMiddle},
		{99, render.ButtonNone},
	}
	for _, c := range cases {
		if got := mapButton(c.in); got != c.want {
			t.Errorf("mapButton(%v) = %v，期望 %v", c.in, got, c.want)
		}
	}
}

// TestMapShift LCL TShiftState（位集合）→ 统一 Modifier 掩码。
func TestMapShift(t *testing.T) {
	// 空集合 → 无修饰键
	if got := mapShift(types.TSet(0)); got != 0 {
		t.Errorf("mapShift(空) = %v，期望 0", got)
	}
	// Shift+Ctrl → ModShift|ModCtrl（不含 Alt）
	s := types.NewSet(int32(types.SsShift), int32(types.SsCtrl))
	got := mapShift(s)
	if got&render.ModShift == 0 || got&render.ModCtrl == 0 {
		t.Errorf("mapShift(Shift+Ctrl) = %v，期望含 ModShift|ModCtrl", got)
	}
	if got&render.ModAlt != 0 {
		t.Errorf("mapShift(Shift+Ctrl) 不应含 ModAlt，实际 %v", got)
	}
	// Meta → ModWin（Win 键映射）
	if got := mapShift(types.NewSet(int32(types.SsMeta))); got&render.ModWin == 0 {
		t.Errorf("mapShift(Meta) = %v，期望含 ModWin", got)
	}
}

// TestMouseEventDIPNormalization 鼠标事件坐标按 DPI 从物理像素归一为 DIP。
// 144 DPI 下物理 144px = 96 DIP；96 DPI 下恒等。
func TestMouseEventDIPNormalization(t *testing.T) {
	ev := mouseEvent(render.EventMouseDown, types.MbRight, types.NewSet(int32(types.SsAlt)), 144, 72, 144)
	if ev.Type != render.EventMouseDown {
		t.Errorf("Type = %v，期望 EventMouseDown", ev.Type)
	}
	if ev.X != 96 || ev.Y != 48 {
		t.Errorf("坐标 = (%d,%d)，期望 DIP (96,48)（144 DPI 归一）", ev.X, ev.Y)
	}
	if ev.Button != render.ButtonRight {
		t.Errorf("Button = %v，期望 ButtonRight", ev.Button)
	}
	if ev.Mods&render.ModAlt == 0 {
		t.Errorf("Mods = %v，期望含 ModAlt", ev.Mods)
	}

	// 96 DPI 恒等（px == DIP）
	ev96 := mouseEvent(render.EventMouseUp, types.MbLeft, types.TSet(0), 12, 34, 96)
	if ev96.X != 12 || ev96.Y != 34 {
		t.Errorf("96 DPI 坐标 = (%d,%d)，期望恒等 (12,34)", ev96.X, ev96.Y)
	}
	if ev96.Type != render.EventMouseUp {
		t.Errorf("Type = %v，期望 EventMouseUp", ev96.Type)
	}
}

// TestColorToTColor ARGB（0xAARRGGBB）→ LCL TColor（低 24 位 BGR $00BBGGRR）换算。
// 覆盖字节序（R/G/B 通道位置互换）与 alpha 忽略。
func TestColorToTColor(t *testing.T) {
	cases := []struct {
		in   render.Color
		want types.TColor
	}{
		{render.Color(0xFF000000), types.TColor(0x000000)}, // 黑：BGR 全 0
		{render.Color(0xFFFFFFFF), types.TColor(0xFFFFFF)}, // 白：BGR 全 FF
		// 红 RGB(FF,0,0) = 0xFFFF0000 → TColor $0000FF（R 落低字节）
		{render.Color(0xFFFF0000), types.TColor(0x0000FF)},
		// 绿 RGB(0,FF,0) = 0xFF00FF00 → TColor $00FF00（G 落中间字节）
		{render.Color(0xFF00FF00), types.TColor(0x00FF00)},
		// 蓝 RGB(0,0,FF) = 0xFF0000FF → TColor $FF0000（B 落高字节）
		{render.Color(0xFF0000FF), types.TColor(0xFF0000)},
		// 混合 0xAABBCC → $CCBBAA（三通道翻转，B 为最高有效）
		{render.Color(0xFFAABBCC), types.TColor(0xCCBBAA)},
		// alpha 0x12 被忽略（原生控件无透明合成）
		{render.Color(0x12345678), types.TColor(0x785634)},
	}
	for _, c := range cases {
		if got := colorToTColor(c.in); got != c.want {
			t.Errorf("colorToTColor(%#x) = %#x，期望 %#x", c.in, got, c.want)
		}
	}
}

// TestEventTypeString 事件类型名（demo/inspector 展示）。
func TestEventTypeString(t *testing.T) {
	cases := map[render.EventType]string{
		render.EventClick:      "click",
		render.EventMouseDown:  "mousedown",
		render.EventKeyPress:   "keypress",
		render.EventType(1234): "EventType(1234)",
	}
	for in, want := range cases {
		if got := in.String(); got != want {
			t.Errorf("%v.String() = %q，期望 %q", int(in), got, want)
		}
	}
}
