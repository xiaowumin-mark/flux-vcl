package render

import "testing"

// TestDIPToPX DIP→物理像素换算（Phase 3.5 DPI）。
// 基准 96 DPI，四舍五入：dip*dpi/96。
func TestDIPToPX(t *testing.T) {
	cases := []struct {
		name string
		dip  int
		dpi  int
		want int
	}{
		{"96 DPI 恒等", 42, 96, 42},
		{"144 DPI 1.5x 整数", 10, 144, 15},
		{"150 DPI 四舍五入进位", 5, 150, 8}, // round(5*150/96)=round(7.8125)
		{"150 DPI 四舍五入舍去", 3, 150, 5}, // round(4.6875)
		{"零 DIP", 0, 144, 0},
		{"一像素大屏", 1, 384, 4},
		{"负 DIP 保留符号", -10, 144, -15},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := DIPToPX(c.dip, c.dpi); got != c.want {
				t.Errorf("DIPToPX(%d, %d) = %d，期望 %d", c.dip, c.dpi, got, c.want)
			}
		})
	}
}

// TestPXToDIP 物理像素→DIP 反向换算：round(px*96/dpi)。
func TestPXToDIP(t *testing.T) {
	cases := []struct {
		name string
		px   int
		dpi  int
		want int
	}{
		{"96 DPI 恒等", 42, 96, 42},
		{"144 DPI 反向", 15, 144, 10}, // round(15*96/144)
		{"150 DPI 反向", 8, 150, 5},   // round(5.12)
		{"零 px", 0, 144, 0},
		{"dpi 未知兜底 96", 100, 0, 100},
		{"dpi 负值兜底 96", 100, -1, 100},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := PXToDIP(c.px, c.dpi); got != c.want {
				t.Errorf("PXToDIP(%d, %d) = %d，期望 %d", c.px, c.dpi, got, c.want)
			}
		})
	}
}

// TestScaleIntRoundTrip 往返稳定：整数倍 DPI 下 DIPToPX 后 PXToDIP 应还原。
// 这是 native 边界（SetBounds→ClientSize）在同一显示器上不漂移的前提。
func TestScaleIntRoundTrip(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 192, 288} { // 96 的整数倍
		for dip := 0; dip <= 2000; dip += 7 {
			px := DIPToPX(dip, dpi)
			back := PXToDIP(px, dpi)
			if back != dip {
				t.Fatalf("往返漂移 dpi=%d dip=%d → px=%d → %d", dpi, dip, px, back)
			}
		}
	}
}

// TestScaleIntBadDenominator scaleInt 对非法分母返回原值（恒等兜底）。
func TestScaleIntBadDenominator(t *testing.T) {
	if got := scaleInt(42, 10, 0); got != 42 {
		t.Errorf("scaleInt(42,10,0) = %d，期望 42", got)
	}
	if got := scaleInt(42, 10, -3); got != 42 {
		t.Errorf("scaleInt(42,10,-3) = %d，期望 42", got)
	}
}

// TestSafeDPI safeDPI 把非法值归一为 96。
func TestSafeDPI(t *testing.T) {
	for _, dpi := range []int{0, -1, -192} {
		if got := safeDPI(dpi); got != 96 {
			t.Errorf("safeDPI(%d) = %d，期望 96", dpi, got)
		}
	}
	if got := safeDPI(144); got != 144 {
		t.Errorf("safeDPI(144) = %d，期望 144", got)
	}
}
