package render

import "math"

// DIP 换算（Phase 3.5 DPI，design.md D5 / research.md §5.4）。
//
// 布局引擎与 Renderer 接口全用 DIP（96 DPI 基准的逻辑单位，显示器无关）；
// 只有 native 适配层在边界做 DIP↔物理像素换算：SetBounds 收 DIP、ClientSize
// 返 DIP、TextExtent 返 DIP，全部经此处换算。Mock 恒为 96 DPI，px==DIP。
//
// 换算公式与 Win32 MulDiv 同语义：scaleInt(v, num, den) = round(v*num/den)，
// 四舍五入保证往返稳定（DIPToPX(PXToDIP(x)) 尽可能还原）。

// DIPToPX 把 DIP 值按 dpi 换算为物理像素。dpi 为当前显示器 PPI。
func DIPToPX(dip, dpi int) int {
	return scaleInt(dip, dpi, 96)
}

// PXToDIP 把物理像素按 dpi 换算为 DIP。dpi<=0（未知）时按 96 兜底（恒等）。
func PXToDIP(px, dpi int) int {
	return scaleInt(px, 96, safeDPI(dpi))
}

// safeDPI 把非法（<=0）的 DPI 归一为 96，避免除零/负换算。
func safeDPI(dpi int) int {
	if dpi <= 0 {
		return 96
	}
	return dpi
}

// scaleInt 返回 round(v*num/den)（den<=0 视为无缩放恒等返回 v）。
// 四舍五入取"远离零"语义（math.Round），与 Win32 MulDiv 一致，负值不会向零截断。
func scaleInt(v, num, den int) int {
	if den <= 0 {
		return v
	}
	// float64 对 GUI 量级的整数足够精确（值域远小于 2^52，无精度损失）。
	return int(math.Round(float64(v) * float64(num) / float64(den)))
}
