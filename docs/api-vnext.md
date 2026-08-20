# FluxVCL vNext API 冻结草图

> 状态：CD1 Draw Core、CD2 字体/布局原语已实现；CD3+ 符号仍按阶段门推进
> 日期：2026-08-19
> 决策依据：[`cd0-decisions.md`](./cd0-decisions.md)
> 当前已发布候选面：[`api-v0.1.0.md`](./api-v0.1.0.md)

本文是 vNext API 审查基线。CD1 Draw Core 的符号已进入当前包；其余各节仍标注计划实现阶段，
在对应阶段落地前，示例不得出现在当前能力列表中。

## 1. CD1 Draw Core

```go
// 复用现有类型：ColorValue、Point、Rect、Size、Widget、Opt。

type DrawList struct { /* unexported immutable storage */ }

// DrawOp 由包内未导出方法封闭。
type DrawOp interface { /* sealed */ }

type FillStyle struct {
    Color ColorValue
}

type StrokeKind uint8

const (
    StrokeSolid StrokeKind = iota
)

type StrokeStyle struct {
    Color ColorValue
    Width int // DIP；必须 > 0
    Style StrokeKind
}

type FontWeight uint16

const (
    FontWeightNormal   FontWeight = 400
    FontWeightMedium   FontWeight = 500
    FontWeightSemibold FontWeight = 600
    FontWeightBold     FontWeight = 700
)

type FontSpec struct {
    Family    string
    Size      int // DIP；0 = 系统 UI 字号
    Weight    FontWeight
    Italic    bool
    Underline bool
    Strikeout bool
}

type TextAlignment uint8

const (
    TextAlignStart TextAlignment = iota
    TextAlignCenter
    TextAlignEnd
)

type TextWrap uint8

const (
    TextNoWrap TextWrap = iota
    TextWrapWord
)

type TextOverflow uint8

const (
    TextOverflowClip TextOverflow = iota
    TextOverflowEllipsis
)

type TextPaint struct {
    Font       FontSpec
    Color      ColorValue
    Horizontal TextAlignment
    Vertical   TextAlignment
    Wrap       TextWrap
    Overflow   TextOverflow
    Mnemonic   bool
}

func NewDrawList(ops ...DrawOp) (DrawList, error)
func MustDrawList(ops ...DrawOp) DrawList

func Clear(color ColorValue) DrawOp
func FillRect(rect Rect, fill FillStyle) DrawOp
func StrokeRect(rect Rect, stroke StrokeStyle) DrawOp
func FillRoundRect(rect Rect, radius int, fill FillStyle) DrawOp
func StrokeRoundRect(rect Rect, radius int, stroke StrokeStyle) DrawOp
func DrawLine(from, to Point, stroke StrokeStyle) DrawOp
func FillEllipse(rect Rect, fill FillStyle) DrawOp
func StrokeEllipse(rect Rect, stroke StrokeStyle) DrawOp
func DrawText(text string, rect Rect, paint TextPaint) DrawOp
func PushClip(rect Rect) DrawOp
func PopClip() DrawOp
```

`StrokeKind` 首版只承诺 Solid；Dash/Dot 在 native 像素和 DPI 规则验证前不预留“看似可用”的
常量。几何、列表限制、颜色和错误模型见 CD0 ADR。

## 2. CD2 Style 基础值

```go
type Insets struct {
    Left, Top, Right, Bottom int
}

type BorderSpec struct {
    Color ColorValue
    Width int
}

type ControlStyle struct {
    Background ColorValue
    Foreground ColorValue
    Font       FontSpec
    Border     BorderSpec
    Radius     int
    Padding    Insets
    MinSize    Size
}

type FocusStyle struct {
    Border BorderSpec
    Inset  int
}

type StyleFieldMask uint64

type ControlStylePatch struct {
    Set        StyleFieldMask
    Background ColorValue
    Foreground ColorValue
    Font       FontSpec
    Border     BorderSpec
    Radius     int
    Padding    Insets
    MinSize    Size
}
```

`ControlStyle` 是完全解析值；`ControlStylePatch` 才携带 presence。CD2 已冻结并实现
`StyleFieldBackground`、`StyleFieldForeground`、`StyleFieldFont`、`StyleFieldBorder`、
`StyleFieldRadius`、`StyleFieldPadding` 和 `StyleFieldMinSize`，以及 `Set*/With*` 安全构造器。
实现记录见 [`cd2-font-layout.md`](./cd2-font-layout.md)。

布局测量使用 `internal/render.TextMeasureRequest` 的可选
`StyledTextMeasurer` 能力；旧 Renderer 自动回落到 `TextExtent`。根包布局原语包括
`Gap(int)`、`Padding(insets[, child])` 和 `PaddingBox(insets, child)`，全部使用 DIP，
且 padding 在约束传递阶段计算，不改变子控件的语义 Bounds。

## 3. CD3 Theme SDK

以下根包名称冻结：

```go
type DesignTheme struct { /* immutable, package-owned storage */ }
type ColorScheme struct { /* value tokens */ }
type Typography struct { /* FontSpec roles */ }
type MetricTokens struct { /* spacing/radius/density */ }
type ComponentThemes struct { /* per-control themes */ }

type SurfaceTheme struct { /* ... */ }
type TextTheme struct { /* ... */ }
type ButtonTheme struct { /* ... */ }
type InputTheme struct { /* ... */ }
type CheckBoxTheme struct { /* ... */ }
type RadioTheme struct { /* ... */ }
type ComboBoxTheme struct { /* ... */ }
type ProgressTheme struct { /* ... */ }
type SliderTheme struct { /* ... */ }
type GridTheme struct { /* ... */ }
type TabTheme struct { /* ... */ }

func FromLegacyTheme(legacy Theme) DesignTheme
func ThemeScope(theme DesignTheme, child Widget) Widget
```

`DesignTheme` 不公开可被外部无键复合字面量锁死的 struct 布局。CD3 通过构造器/options 和只读
访问器完成第三方主题包需求；具体构造器签名随纯值/防御性复制测试一起冻结。官方主题包使用
普通 `Light() flux.DesignTheme` / `Dark() flux.DesignTheme` 函数，不增加类似 legacy
`LightTheme` 的可变共享变量。

Legacy 映射的逐字段契约已在 CD0 ADR 冻结。CD3 不得把 `FromLegacyTheme` 改名为含糊的
`ConvertTheme`，也不得删除或重命名 legacy `Theme`。

迁移前的 legacy 写法：

```go
legacy := flux.LightTheme
return flux.Window(
    flux.Color(legacy.Background),
    flux.DarkTitleBar(legacy.DarkTitleBar),
    flux.Text("Settings", flux.FontColor(legacy.Text)),
)
```

CD3.6 落地后的增量迁移写法：

```go
legacy := flux.LightTheme // 在边界复制可变包变量
theme := flux.FromLegacyTheme(legacy)

return flux.ThemeScope(theme,
    flux.Window(
        // 旧原子 Opt 可在迁移期保留，并继续拥有更高覆盖优先级。
        flux.Color(legacy.Background),
        flux.DarkTitleBar(legacy.DarkTitleBar),
        flux.Text("Settings", flux.FontColor(legacy.Text)),
    ),
)
```

仅增加 `ThemeScope` 不会自动切到 StyledRendering。应用可先迁移主题数据，再逐控件显式迁移
rendering mode；这避免一次性重建 Input/Memo 等有焦点、caret 或 IME 状态的 HWND。当前仓库
尚未实现第二段中的 vNext 符号，可编译的 CD0 reference contract 见
`../cd0_legacy_theme_test.go`。

## 4. CD4 Surface

```go
func DrawSurface(list DrawList, opts ...Opt) Widget
```

`DrawSurface` 是纯绘制 Widget 构造器。若 CD4 增加承载子控件的容器，名称使用 `Surface`，
两者不能合并成一个根据参数动态改变原生身份的构造器。

## 5. 保留与排除

保留的现有名字：`Theme`、`LightTheme`、`DarkTheme`、`ColorValue`、`Color`、`FontColor`、
`PaintBox`、`PaintCommand`、`Rect`、`Point`、`Size`、`Opt`、`Widget`。

本冻结明确不包含：公开 Canvas、Painter callback、HDC/handle、Image/ResourceRef、Gradient、
Path、Transform、任意 Save/Restore、第三方 painter registry。它们不能在 CD1-CD6 期间以未验证
名称混入根包。

## 6. 名称审计清单

CD0 自动审计保留以下拟议包级标识符：

```text
DrawList DrawOp FillStyle StrokeKind StrokeStyle FontWeight FontSpec
TextAlignment TextWrap TextOverflow TextPaint DrawValidationError
ErrInvalidDrawList NewDrawList MustDrawList Clear FillRect StrokeRect
FillRoundRect StrokeRoundRect DrawLine FillEllipse StrokeEllipse DrawText
PushClip PopClip Insets BorderSpec ControlStyle FocusStyle StyleFieldMask
ControlStylePatch DesignTheme ColorScheme Typography MetricTokens
ComponentThemes SurfaceTheme TextTheme ButtonTheme InputTheme CheckBoxTheme
RadioTheme ComboBoxTheme ProgressTheme SliderTheme GridTheme TabTheme
FromLegacyTheme ThemeScope DrawSurface Surface
```

常量与 MessageID 采用各类型/功能前缀，完整名称由 `cd0_api_names_test.go` 同步审计。
