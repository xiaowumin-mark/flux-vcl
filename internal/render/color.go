package render

// Color 是 ARGB 颜色（0xAARRGGBB，与 Go/Win32 惯例一致，高位 alpha）。
//
// 采用 ARGB 而非 LCL 的 TColor（$00BBGGRR，BGR 布局）：对用户更直觉（写
// 0xRRGGBB 即红），换算收在 native 边界（colorToTColor，见 internal/native），
// D6 保持接口与后端无关。alpha 在原生控件（无透明合成）中忽略。
type Color uint32

// RGB 构造不透明颜色（alpha=0xFF）。
func RGB(r, g, b uint8) Color {
	return 0xFF000000 | Color(r)<<16 | Color(g)<<8 | Color(b)
}
