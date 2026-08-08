package flux

import "github.com/xiaowumin-mark/flux-vcl/internal/render"

// Theme 系统（design.md §14，Phase 5.2）。
//
// 设计取舍：主题是"数据"，不是运行时对象。构建函数按当前主题显式传颜色
// （Color/FontColor Opt），主题切换 = 换一个 Theme 值 → State 触发 re-render
// → diff 引擎按 D2 属性级 patch 只改变化的颜色属性（"主题切换=全量 re-diff"
// 的声明式落地：颜色全变就全 patch，未变子树零 mutation，见 D7c）。
//
// FontSize/Radius 本轮为文档字段（native 未接入字体大小/圆角）—— 诚实标注，
// 颜色是可视主题的落地部分。

// ColorValue 是 ARGB 颜色（0xAARRGGBB；别名 internal/render.Color，D6 接口与
// 后端无关，native 边界换算为 LCL TColor）。命名带 Value 后缀以让出 Color 标识符
// 给 Opt 构造器（type 与 func 包级同名冲突）：用户几乎不直接写类型名 ——
// 用 flux.RGB(...) 或主题字段（Theme.Primary 等）即可，类型自动推断。
type ColorValue = render.Color

// RGB 构造不透明颜色（alpha=0xFF）。例：RGB(0x1E, 0x90, 0xFF) = 道奇蓝。
func RGB(r, g, b uint8) ColorValue { return render.RGB(r, g, b) }

// Theme 是主题调色板。Primary/Surface/Text/Accent 供构建函数取用，
// Background 通常传给 Window 的 Color Opt，Primary 给按钮，Text 给文字。
type Theme struct {
	Primary    ColorValue // 主色（按钮/强调）
	Background ColorValue // 窗体背景
	Surface    ColorValue // 卡片/面板表面
	Text       ColorValue // 主要文字
	Accent     ColorValue // 点缀色（hover/选中）
	FontSize   int        // 文档字段：字体大小（Phase 5 未接入 native 字体缩放）
	Radius     int        // 文档字段：圆角（原生控件无统一圆角 API）
}

// LightTheme 浅色主题（类 Flutter Material Light 调性）。
var LightTheme = Theme{
	Primary:    RGB(0x1E, 0x90, 0xFF),
	Background: RGB(0xF5, 0xF5, 0xF5),
	Surface:    RGB(0xFF, 0xFF, 0xFF),
	Text:       RGB(0x21, 0x21, 0x21),
	Accent:     RGB(0xFF, 0x6F, 0x00),
	FontSize:   14,
	Radius:     4,
}

// DarkTheme 深色主题。
var DarkTheme = Theme{
	Primary:    RGB(0x4F, 0xA8, 0xFF),
	Background: RGB(0x12, 0x12, 0x12),
	Surface:    RGB(0x1E, 0x1E, 0x1E),
	Text:       RGB(0xE6, 0xE6, 0xE6),
	Accent:     RGB(0xFF, 0xA7, 0x26),
	FontSize:   14,
	Radius:     4,
}

// Color 设置控件背景色（Phase 5.2）。对应 native IControl.SetColor；
// 典型用法 Button("OK", Color(th.Primary)) 或 Window(Color(th.Background))。
func Color(c ColorValue) Opt {
	return optFn(func(n *Node) { n.Props.Set("Color", c) })
}

// FontColor 设置控件文字颜色。对应 native 字体对象 SetColor；
// 典型用法 Text("hi", FontColor(th.Text))。
func FontColor(c ColorValue) Opt {
	return optFn(func(n *Node) { n.Props.Set("FontColor", c) })
}
