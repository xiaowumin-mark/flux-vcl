# FluxVCL 自绘与样式系统设计

> 状态：CD1 Draw Core 已实现（无头）；native executor/主题 API 仍按后续阶段推进
> 日期：2026-08-20
> 适用范围：`flux` 根包、`internal/widget`、`internal/diff`、`internal/render`、
> `internal/native` 以及第三方主题包
> 前置事实：当前公开兼容入口仍是 `PaintBox([]PaintCommand, ...)`；CD1 已新增纯值
> `DrawList` API，公开 `DrawSurface` 与真实 native 像素绘制仍按 CD4 阶段门实现。

本文定义 FluxVCL 下一阶段的底层自绘协议、样式解析模型和主题包边界，并把实施工作拆成
可独立验收的 `CD0-CD8` 阶段。目标不是先堆若干皮肤属性，而是先建立一个稳定、可测试、
不泄漏 LCL/Win32 的绘制底座，使后续 Fluent、经典 Windows、紧凑型、无边框等主题包只需
提供数据，不需要修改 diff 或默认后端的核心流程。

---

## 1. 结论摘要

本设计固定以下决策：

1. **公开自绘输入是不可变 `DrawList`，不是 `func(Canvas)`。** 用户与主题包不能持有
   LCL Canvas、HDC、GDI 对象或 paint 回调期资源。
2. **主题包是普通 Go 数据包。** 它返回强类型 `DesignTheme` 值，不使用进程全局注册、
   `init` 副作用或后端私有 API。
3. **样式在布局前解析。** 字体、padding、border 和最小尺寸必须同时影响 Measure/Layout
   与最终绘制，不能出现“画出来是 40 DIP，布局仍按 32 DIP”的两套事实。
4. **交互视觉状态由后端拥有。** hover、pressed、focused 等状态只使对应控件 invalidate，
   不通过应用 `State` 触发整棵 Widget 树重建。
5. **原生模式和自绘模式双轨并存。** 默认原生控件继续可用；需要主题完整控制时显式选择
   styled rendering。渲染模式是创建身份的一部分，不能作为普通热更新属性偷偷替换 HWND。
6. **基础 `render.Renderer` 不继续膨胀。** 创建、样式、绘制、字体测量和资源分别使用可选窄
   capability；Mock 与默认 native 后端同步实现。
7. **交互控件不能伪装成 PaintBox。** Button、Input 等必须保留 HWND、焦点、键盘、IME、
   Default/Cancel 和可访问语义；纯图形 surface 与交互控件是两条实现路径。
8. **默认后端先实现确定性的 2D CPU 绘制。** GPU、Direct2D、任意 path、滤镜和动画过渡
   不作为底座首批门槛。

---

## 2. 目标与非目标

### 2.1 目标

- 为控件自绘、`Surface`、图表/画布和 item/cell custom draw 提供同一套后端无关图元。
- 让第三方主题包只 import `github.com/xiaowumin-mark/flux-vcl`。
- 支持 light/dark、高对比度、DPI 切换和运行时主题切换。
- 相同 `DrawList`/样式值重复 render 时保持 D7c 零 mutation、零重复 invalidate。
- 样式变化原位 patch；只有创建身份变化时才局部重建控件。
- 所有核心规则可由无头 Mock 验证，真实 Windows 只承担绑定和像素验收。
- 为未来图像、图标、渐变、alpha 和动画保留资源与能力版本边界。

### 2.2 首批非目标

- CSS 解析器、selector、class name、任意级联和运行时样式字符串。
- 向公开 API 暴露 LCL Canvas、Win32 HDC、GDI handle 或 `OnPaint` 回调。
- GPU renderer、Direct2D/Skia 后端、任意矢量 path、shader、blur、acrylic。
- 完全自绘文本编辑器、caret、selection、IME 或富文本排版。
- 运行时 DLL/Go plugin 主题热加载。
- 主题包执行任意 paint 代码；首版主题包保持数据驱动。
- 为圆角视觉改变 HWND 命中区域；首版命中区域仍是可预测的矩形控件框。

---

## 3. 与 D1-D7 的关系

| 不变量 | 自绘/样式系统约束 |
|---|---|
| D1 三棵树 | `RenderingMode`/内部 `NativeKind` 参与 Element identity；颜色、边框等只 patch。 |
| D2 属性 patch | resolved style 与 DrawList 是稳定值；只在值变化时 Set + invalidate。 |
| D3 稳定 key | 主题切换不改变 key；同一 rendering mode 下保留 HWND、焦点和输入状态。 |
| D4 UI 线程 | native 资源创建、paint、invalidate 和销毁仅在 UI 线程；OnPaint 不调用用户代码。 |
| D5 自定义布局 | 全部几何使用 DIP；字体、padding、border 在布局前解析，native 边界才转 px。 |
| D6 绑定隔离 | Draw API 与样式类型不引用 LCL/Win32；绑定细节只在 `internal/native`。 |
| D7 测试门 | 纯样式 patch 不重建、keyed 重排不迁移状态、同值树零 mutation/零 invalidate。 |

任何绕过这些约束的“快捷实现”，例如在 `Native()` 中替换 OnPaint、把 hover 写回应用
State，或让 theme package 保存 HDC，都不进入正式 API。

---

## 4. 总体架构

```text
第三方主题包
  DesignTheme（纯值）
        |
Widget tree + ThemeScope + Variant + Style override
        |
        v
style resolution pass（plugin 展开之后、layout 之前）
        |
        +--> FontSpec / Padding / MinSize --> Measure + Layout
        |
        +--> ResolvedAppearance / DrawList --> Props diff
                                                 |
                                                 v
              optional render capabilities + Mock recording
                                                 |
                                                 v
native control state + visual state + resource cache
                                                 |
                                                 v
                              Draw executor（DIP -> px）
                                                 |
                                                 v
                              LCL Canvas / Win32 owner-draw
```

### 4.1 数据所有权

- Widget 树持有用户声明值和样式覆盖值。
- style resolution pass 生成规范化的 resolved 值；不修改主题包共享数据。
- Element 持有上一次已提交 resolved Props，用于 diff。
- native 后端对 DrawList、Appearance 做防御性快照，并拥有 GDI/LCL 资源。
- paint 回调只读取最后一次成功提交的快照和 native visual state。
- Theme/DrawList/Appearance 不持有 Renderer、Handle、Canvas 或释放函数。

### 4.2 提交流程

```text
build
  -> plugin expand
  -> validate tree
  -> resolve theme/style
  -> measure/layout
  -> collect bindings
  -> reconcile/diff
  -> set native snapshots
  -> request invalidate
  -> OS paint message later executes snapshot
```

`SetDrawList`/`SetAppearance` 与 `Invalidate` 分开建模，便于 Mock 分别断言“数据已更新”和
“只请求了一次重绘”。同一次 diff 中多个样式字段变化必须聚合为一次 appearance 提交，避免
中间帧和重复 invalidate。

---

## 5. 底层 Draw API

### 5.1 API 形状

以下代码是 CD0 冻结的签名方向；它不表示这些 API 已经实现或发布：

```go
// DrawList 是经过校验、可防御性复制的不可变绘制命令序列。
// 零值表示空列表。
type DrawList struct { /* unexported immutable storage */ }

// DrawOp 是框架封闭的值操作集合；第三方不能注入后端代码。
type DrawOp interface { /* sealed */ }

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

拟议的新 surface 构造器：

```go
func DrawSurface(list DrawList, opts ...Opt) Widget
```

现有 `PaintBox([]PaintCommand, ...)` 在兼容期保留，并在内部转换到 DrawList。新图元不继续
向当前“所有字段挤在一个 `PaintCommand` struct”模型无限加字段。

### 5.2 基础值类型

```go
type FillStyle struct {
    Color ColorValue
}

type StrokeStyle struct {
    Color ColorValue
    Width int // DIP；必须 > 0
    Style StrokeKind
}

type FontSpec struct {
    Family    string // 空字符串 = 系统 UI 字体
    Size      int    // DIP，不使用 point
    Weight    FontWeight
    Italic    bool
    Underline bool
    Strikeout bool
}

type TextPaint struct {
    Font       FontSpec
    Color      ColorValue
    Horizontal TextAlignment
    Vertical   TextAlignment
    Wrap       TextWrap
    Overflow   TextOverflow
    Mnemonic   bool
}

type Insets struct {
    Left, Top, Right, Bottom int
}

type BorderSpec struct {
    Color ColorValue
    Width int
}
```

首版所有公开几何继续使用整数 DIP，与现有 `Rect`、布局和事件坐标保持一致。后端负责 pixel
snapping，不能把物理像素反向写回 DrawList。若未来确需亚 DIP 几何，应新增版本化类型，不能
悄悄改变现有整数语义。

### 5.3 操作语义

| 操作 | 首版语义 |
|---|---|
| Clear | 填满当前 surface，不受前序 brush 状态影响。 |
| Rect/RoundRect/Ellipse | fill 与 stroke 分离；右/下边界采用统一 half-open 规则。 |
| Line | 使用 StrokeStyle；后端统一端点与 pixel snapping。 |
| Text | 在给定 Rect 内执行对齐、裁剪、换行/ellipsis 和 mnemonic。 |
| PushClip/PopClip | 只支持矩形 clip；必须平衡，不能越过 DrawList 边界。 |
| Image | 延后到资源阶段；首版 DrawList 不携带 `[]byte`、bitmap 或 native handle。 |

首版不提供任意 Save/Restore、transform stack 或 path。每个 DrawList 执行前后，后端必须保存并
恢复 Canvas/HDC 状态，避免一条列表污染下一控件。

### 5.4 值语义与校验

- `DrawList` 构造时验证，提交到 diff/native 时再次做防御性验证。
- 所有 slice 由框架复制；字符串按 Go 不可变值处理；不得保存调用方可变 map/pointer。
- 拒绝负尺寸、负 radius、非正 stroke width、不平衡 clip、未知枚举和溢出坐标。
- 零值 DrawList 合法，表示“不绘制”；零色继续表示“无 paint/使用默认”的语义，不与透明黑混用。
- `Props` 对同值 DrawList 必须稳定相等；后续可加缓存 hash 优化，但 hash 不能成为唯一正确性依据。
- 单个 DrawList 的命令数、文本长度等防御上限在 CD0 probe 后确定；超限返回结构化诊断。

### 5.5 Alpha 约束

`ColorValue` 仍采用 ARGB，但首个 native executor 只承诺 `A=0xFF` 的不透明色和零值无 paint。
`0 < A < 0xFF` 必须明确返回“不支持 alpha”的校验错误，不能像当前 TColor 转换一样静默丢弃
alpha。CD7 在离屏 32-bit surface 和合成语义验证完成后再开放半透明色。

---

## 6. 样式模型与主题包

### 6.1 样式基础值

```go
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

type ButtonAppearance struct {
    Normal   ControlStyle
    Hovered  ControlStyle
    Pressed  ControlStyle
    Disabled ControlStyle
    Focus    FocusStyle
}
```

`ControlStyle` 是跨控件可复用的视觉原语，不强迫所有控件共享同一种状态模型。CheckBox、
Input、Slider、Grid 等使用自己的 Appearance 类型组合基础值，避免一个巨型 struct 塞入大量
无效字段。

`ControlStyle` 表示**完全解析后的样式**，其中零 padding、零 radius 都是合法且明确的值。
用户局部覆盖不能靠“字段为零表示未设置”，否则无法表达从主题值覆盖回 0。覆盖值使用独立
presence mask：

```go
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

公开构造器优先通过 `StyleOption` 生成 mask，不要求用户手写位运算。控件专属状态覆盖同样使用
`ButtonAppearancePatch` 等明确 presence 的值类型，不使用 pointer/map 表示 optional。resolver
完成后不再保留“缺失”状态，交给 layout/diff/native 的始终是完整 `ControlStyle`/Appearance。

### 6.2 Theme 数据

为避免直接破坏现有 `Theme`，新增版本化的 `DesignTheme`：

```go
type DesignTheme struct {
    Name       string
    Colors     ColorScheme
    Typography Typography
    Metrics    MetricTokens
    Components ComponentThemes
    // 包内保留版本/规范化字段，阻止第三方依赖无键名复合字面量。
}

type ComponentThemes struct {
    Surface  SurfaceTheme
    Text     TextTheme
    Button   ButtonTheme
    Input    InputTheme
    CheckBox CheckBoxTheme
    Radio    RadioTheme
    ComboBox ComboBoxTheme
    Progress ProgressTheme
    Slider   SliderTheme
    Grid     GridTheme
    Tabs     TabTheme
}
```

`FromLegacyTheme(Theme) DesignTheme` 将现有 Light/Dark 调色板映射为完整默认值。旧的 `Theme`、
`LightTheme`、`DarkTheme`、`Color`、`FontColor` 不在本计划中直接删除。

### 6.3 主题包约定

主题包是普通 Go package：

```go
package fluent

func Light() flux.DesignTheme { /* 返回新的纯值 */ }
func Dark() flux.DesignTheme  { /* 返回新的纯值 */ }
```

约束：

- 只 import 公开 `flux` 根包，不 import `internal/*`、energye/lcl 或 Win32 wrapper。
- 不在 `init` 中注册全局 painter，不保存 App/Renderer/Handle。
- 每次构造返回独立值，不导出可被调用方修改的共享 slice/map。
- CD3 主题包只配置 token、component appearance 和 variant，不执行 paint 代码；CD7 后才可
  增加稳定 ResourceRef。
- 主题包可声明最低 Draw API 版本/feature requirements；加载不满足时给出明确诊断。

数据驱动主题覆盖常见的 Fluent、Classic、Minimal、High-density 等风格。真正需要任意绘制算法
的第三方 painter registry 不属于首版；若未来引入，也必须通过稳定 Painter ID + 声明式
DrawList 输出，不能把 Canvas 回调塞进 Props。

如果多个正式主题证明固定控件 painter 的表达力不足，后续可设计声明式 `ControlRecipe`：由
状态条件、background/content/focus 等语义 slot 和 DrawOp layer 组成，在 paint 前编译为
DrawList。它仍然是可校验数据，不是 Go 回调；在没有真实主题需求前不进入 CD0-CD6。

### 6.4 ThemeScope 与解析时机

拟议用法：

```go
flux.ThemeScope(theme,
    flux.Column(
        flux.Button("保存",
            flux.Rendering(flux.StyledRendering),
            flux.Variant(flux.ButtonPrimary),
        ),
    ),
)
```

`ThemeScope` 是透明语义节点。解析顺序必须是：

1. 框架控件默认值；
2. 最近祖先 `DesignTheme` 的 component 默认值；
3. Variant / density / size；
4. `Style(...)` 或控件专属 Appearance override；
5. 现有显式 `Color`、`FontColor`、未来 `Font`/`Padding` 等原子 Opt；
6. native 高对比度和系统策略覆盖。

同层同属性遵循 Opt 后写覆盖前写的现有习惯。解析发生在 plugin builder 展开之后，因此主题可
进入插件组合出来的公开 Widget 子树；发生在 layout 之前，因此 resolved font/padding/min-size
可以参与测量。

解析产物必须是规范化纯值，例如 `_ResolvedFont`、`_ResolvedAppearance`、内部 `NativeKind`。
原始 theme/variant 信息保留给 Inspector，但不能误下发到透明节点继承的父句柄。

### 6.5 运行时主题切换

- 同一 `RenderingMode` 内切换 Light/Dark：保留控件 identity，只 patch resolved style。
- Native 与 Styled 模式之间切换：允许该控件局部重建，并由 Inspector 标记原因。
- Theme 值完全相同时：全树 build/resolve 可以发生，但 native mutation 和 invalidate 必须为零。
- 不设置 ThemeScope 时：使用兼容当前外观的默认 DesignTheme，不改变现有应用视觉。

---

## 7. VisualState 与控件 painter

### 7.1 状态位

内部统一状态集合至少包含：

```text
Disabled | Hovered | Pressed | Focused | Default
Checked | Indeterminate | Selected | ReadOnly
```

不是每个控件都使用全部状态。状态位来自 native 消息、受控 Props 和焦点系统，不进入 Widget
业务 State。后端只在状态实际变化时 invalidate。

### 7.2 决定性解析

Button 基础层优先级固定为：

```text
Disabled > Pressed > Hovered > Normal
```

`Focused` 和 `Default` 是覆盖层，不替代基础层。CheckBox/Radio 的 Checked/Indeterminate、
Grid/Combo 的 Selected 由各自 painter 明确组合。不得依赖 map 遍历或主题字段声明顺序。

### 7.3 Painter 边界

- 内建 painter 是无副作用纯逻辑：`Appearance + VisualState + bounds/content -> DrawList`。
- painter 不读取 App State，不调用 Renderer，不分配 native 资源。
- native paint 期间只执行已经生成或可确定生成的 DrawList，不调用用户 handler。
- hover/focus 内部监听必须与公开 OnMouse/OnKey handler multiplex，不能覆盖用户回调槽。
- 高频状态变化可缓存按 `(appearance, state, size, dpi)` 生成的列表；缓存只影响性能。

---

## 8. internal/render 与 diff 协议

### 8.1 创建身份

当前 `Renderer.Create(widgetType)` 看不到 Props，而自绘模式可能对应不同 native 类或窗口样式。
因此引入内部 `NativeKind`，并让它与 Type/Key 一起参与 `canUpdate`：

```text
semantic Type: Button
native kind:   Button.Native | Button.OwnerDraw
identity:      Type + Key + NativeKind
```

拟议可选 capability：

```go
type ControlCreateSpec struct {
    WidgetType string
    NativeKind string
}

type StyledControlCreator interface {
    CreateStyled(spec ControlCreateSpec) Handle
}
```

普通 NativeKind 继续回落现有 `Renderer.Create`。请求 styled 但后端不支持时：Auto 模式可回落原生，
Required 模式在 prepare 阶段返回结构化 unsupported error，不在 commit 中途 panic。

### 8.2 可选窄能力

```go
type DrawController interface {
    SetDrawList(Handle, DrawList)
    ResetDrawList(Handle)
    InvalidateDraw(Handle)
}

type AppearanceController interface {
    SetAppearance(Handle, ResolvedAppearance)
    ResetAppearance(Handle)
}

type StyledTextMeasurer interface {
    MeasureText(TextMeasureRequest) Size
}

type DrawResourceController interface {
    InstallResource(ResourceDescriptor) error
    ReleaseResource(ResourceRef)
}
```

这些接口位于 `internal/render`；公开类型在根包构造时转换为 internal 值。基础 Renderer 不增加
控件专属 setter。Grid item、Combo item 等特殊协议可继续使用各自窄 capability。

### 8.3 diff 规则

- DrawList 变化：clone -> validate -> SetDrawList -> 单次 InvalidateDraw。
- Appearance 变化：原子 SetAppearance -> 单次 invalidate；不能逐字段出现中间帧。
- 值相同：无 Set、无 invalidate。
- 属性移除：Reset 到当前 theme/default；清除旧 native 快照并 invalidate。
- NativeKind 变化：记录 rebuild 原因，只重建当前节点，不上溯祖先。
- capability 缺失：按 RenderingMode 的 fallback/required 规则确定，行为必须可测试。
- Inspector 展示 semantic style、resolved style、native kind、visual state 和 invalidate 原因。

### 8.4 字体测量

当前文本缓存只以 text 为 key。新缓存 key 至少包含：

```text
text + FontSpec + DPI/font revision + wrap/constraint mode
```

Measure 和 paint 必须使用同一 FontSpec 解析与 fallback 链。DPI、系统字体、主题字体或高对比度
相关设置变化时，测量缓存和 native font cache 同时失效。

---

## 9. 默认 LCL/Win32 后端

### 9.1 Draw executor

默认 executor 负责：

- 将整数 DIP 按当前目标窗口 DPI 转为物理像素；
- 为 pen/brush/font 建立有界缓存，并在 DPI/主题/销毁时释放；
- 每次列表执行 Save/Restore Canvas/HDC 状态；
- 应用 clip、文本对齐、mnemonic 和 focus ring；
- 使用离屏 bitmap/double buffering 避免 owner-draw 闪烁；
- paint 失败进入 `render.Guard`/诊断，不让异常跨越 native callback；
- 高对比度时把主题色映射到系统色，而不是执行声明色。

CD1-CD6 不为每帧创建/销毁 GDI 对象。资源缓存必须可观测命中数和存活数，Windows probe 使用
`GetGuiResources` 检查重复 repaint 后 GDI handle 不持续增长。

### 9.2 Owner-draw 消息路由

Button 的首选实现是保留 `TButton` 并设置 `BS_OWNERDRAW`，以保住 HWND、Tab、Space、
Enter/Esc、Default/Cancel 和 UIA Button Pattern。

`WM_DRAWITEM` 发给直接 native parent，不保证发给主窗体。Button 可能位于 `ScrollBox`、
`TabPage`、ListView viewport 或内部 Panel，因此不能只扩展现有 Form WndProc hook。默认后端需要：

```text
parent HWND subclass registry
  parent HWND -> ref count + child HWND map
  child HWND  -> render.Handle
```

- 使用 `SetWindowSubclass/DefSubclassProc`，不直接替换 LCL 的 WindowProc。
- `WM_DRAWITEM/WM_MEASUREITEM/WM_NOTIFY` 在 inherited/default 之前按协议 pre-dispatch。
- 未消费的消息必须交回 `DefSubclassProc`。
- DPI/SETTINGCHANGE/THEMECHANGE 等继续在默认处理后 post-dispatch。
- SetParent 完成时注册直接 parent；reparent/destroy 时引用计数解绑。
- `DRAWITEMSTRUCT`/`NMCUSTOMDRAW` 按 32/64 位对齐拆平台文件并做静态尺寸测试。

若 probe 证明锁定 LCL runtime 无法稳定 owner-draw，才启用 `TCustomControl` 备选；该备选必须先
补齐 Default/Cancel、键盘与 UIA provider，不能以“能画出来”作为完成标准。

### 9.3 各控件策略

| 控件 | 首选策略 | 不能退化的语义 |
|---|---|---|
| Surface | `TCustomControl`/Panel host 自绘背景与边框，子控件仍为真实 children | 本地坐标、padding、子焦点 |
| Button | `TButton + BS_OWNERDRAW + WM_DRAWITEM` | Tab、Space、Enter/Esc、Default/Cancel、UIA Invoke |
| Text | 原生 Label 或轻量 DrawSurface；按主题需求选择 | 文本测量、mnemonic、可访问名称 |
| Input/Memo | 保留真实 Edit/Memo；外层 host 画 chrome，内层编辑器无边框 | caret、selection、clipboard、IME、password |
| CheckBox/Radio | 先验证 owner-draw 标准按钮；必要时组合 host + 原生语义 | Checked、组导航、Space、UIA Toggle/Selection |
| ComboBox | 使用 LCL OwnerDrawFixed/Variable 与 DrawItem/MeasureItem | 下拉键盘、选择、UIA Selection |
| StringGrid | 使用 LCL DrawCell/PrepareCanvas | 编辑器、选择、表头坐标、IME |
| Slider/Progress | 逐控件验证 NM_CUSTOMDRAW；不假设共用一套通知 | 键盘步进、范围和值、UIA RangeValue |
| PageControl | 后期验证 tab custom draw；先保持原生 | 页面 parent、键盘切换、selected identity |
| DrawSurface | PaintBox/TCustomControl，按是否需要 HWND 选择 | DPI、鼠标坐标；无虚假 UIA 子树 |

### 9.4 资源模型

图像和图标不直接放入 DrawList：

```go
type ResourceRef struct {
    Key     string
    Version uint64
}
```

CD7 引入 App/Renderer 作用域的显式资源安装。Theme bundle 可携带 immutable asset descriptor，
Theme 本身只保存 ResourceRef。解码可在后台完成，native bitmap 创建/销毁仍在 UI 线程。资源 key
冲突、版本替换、引用释放和 App.Close 清理必须有确定规则。

---

## 10. Accessibility、高对比度与系统设置

- 交互自绘控件优先保留标准 HWND 和原生 role/pattern。
- Focus ring 是样式的一部分，但高对比度下由系统颜色和最小可见宽度覆盖。
- Disabled、Selected、Checked 不能只靠颜色区分；控件原生状态和辅助技术状态必须同步。
- 自绘 Input/Memo chrome 不得拦截 IME、caret、selection 或 UIA Value provider。
- 无 HWND 的 DrawSurface 继续要求邻接文本和可聚焦等价操作；不虚构图元级 UIA。
- 响应 `WM_SETTINGCHANGE`、`WM_SYSCOLORCHANGE`、`WM_THEMECHANGED`、DPI 和系统字体变化。
- 后续加入动画时响应 reduced-motion/system animation 设置；CD0-CD6 不要求视觉过渡。

高对比度策略仍位于 native 边界。主题包不能通过“更高优先级 Style”覆盖系统可读性策略。

---

## 11. 错误、性能与可观测性

### 11.1 错误边界

- DrawList/Style 构造错误使用稳定诊断 ID，可被 i18n catalog 替换。
- prepare 阶段发现 unsupported required feature 时不提交半棵树。
- native paint 错误记录到 App/Inspector；不能在 OS paint callback 中把 panic 传播出去。
- 资源缺失绘制确定性的占位图或跳过，并记录一次去重诊断；不能每帧刷日志。
- Native escape 不得替换框架管理的 OnPaint/owner-draw handler；需要明确检测或文档保留事件。

### 11.2 性能原则

- 同值 resolved style/DrawList 不重复 Set/invalidate。
- hover/pressed/focus 只重绘一个控件，不执行应用 build。
- DrawList、font、pen、brush、bitmap cache 均有上限和明确失效规则。
- 初期整控件重绘，不提前做脏矩形；证据表明需要后再增加 partial invalidation。
- 基准记录构建、diff、DrawList 生成、native paint、分配和 GDI handle，不设共享 CI 绝对耗时阈值。

### 11.3 Inspector

Inspector 至少增加：

- raw theme/variant/style override；
- resolved font、padding、appearance 和 NativeKind；
- 最近 VisualState、DrawList op 数、invalidate 原因；
- cache hit/miss 与资源缺失诊断；
- Native/Styled 切换导致的重建记录。

---

## 12. 测试矩阵

### 12.1 无头测试

- DrawList 校验、规范化、防御性复制、相等性和 clip 平衡。
- 样式优先级、嵌套 ThemeScope、Variant、显式 Opt 覆盖和 legacy adapter。
- FontSpec 测量 request、cache key、padding/min-size 布局。
- Draw/Appearance mount、patch、remove/reset、capability 缺失。
- D7a：样式变化不重建相同 NativeKind 控件。
- D7b：主题切换/keyed reorder 不迁移 identity。
- D7c：同值 Theme/DrawList/Appearance 零 mutation、零 invalidate。
- VisualState 解析组合表和 painter 的确定性 DrawList 输出。

### 12.2 Native probe

- 每个基础 op 的几何、颜色、clip、文本和 pixel snapping。
- Button Normal/Hover/Pressed/Disabled/Focused/Default。
- Button 嵌套于 Window、ScrollBox、TabPage、ListView viewport 的 parent 消息路由。
- 96/144/192 DPI，切换显示器后字体缓存失效和边框尺寸。
- Light/Dark/高对比度/系统字体变化。
- Button Tab、Space、Enter/Esc、Default/Cancel、UIA Invoke。
- Input/Memo caret、selection、clipboard、中文 IME 和 UIA Value 不退化。
- 重复 repaint/reparent/destroy 后 GDI handle、subclass ref count 和资源归零。

### 12.3 Screenshot 与像素检查

- 新增 `examples/theme-gallery`，同屏覆盖各控件和全部 VisualState。
- 截图先做窗口非空、目标控件区域非背景色、关键色块/边框采样，再做容差比较。
- 不对整张抗锯齿文本做逐像素完全相等断言。
- 每个正式主题至少有 light/dark 两张基准；高对比度使用系统色断言而非品牌色快照。

---

## 13. 兼容与版本策略

1. 当前 `PaintBox`/`PaintCommand` 继续工作；内部适配到 DrawList。
2. 当前 `Theme` 与 Light/Dark 变量继续工作；通过 `FromLegacyTheme` 进入新解析器。
3. `Color`/`FontColor` 保持最高用户显式覆盖层之一。
4. 新 `DesignTheme` 使用构造器和带包内字段的 keyed struct，避免第三方无键名字面量锁死字段布局。
5. Draw API、Theme schema 和 feature 使用独立版本常量；主题包可声明最低版本。
6. 第三方 Renderer 不实现 styled capability 时，现有 Native 模式保持可用。
7. `RenderingMode`/NativeKind 改变是有意的局部重建，不伪装成属性 patch。
8. 公开 API 冻结前，所有拟议名字需通过 CD0 命名审查并同步 `api-vNext` 文档。

---

## 14. 分阶段实施计划

### 14.1 阶段总览

| 阶段 | 目标 | 主要交付 | 进入下一阶段的门 |
|---|---|---|---|
| CD0 | 决策与绑定探针 | API decision record、LCL/Win32 probe、能力矩阵 | 决策冻结，未验证能力明确 deferred |
| CD1 | 纯值 Draw Core | DrawList、FontSpec/TextPaint、基础 ops、validation、Mock | D7c 与防御性复制全绿 |
| CD2 | 字体与布局样式 | Insets/ControlStyle、styled measure/cache | Measure 与 DrawText 共用字体契约 |
| CD3 | Theme SDK 与解析 | DesignTheme、ThemeScope、Variant、resolver | 嵌套 scope 正确；同 NativeKind 切换不重建 |
| CD4 | Native executor 与 Surface | LCL executor、DrawSurface、Surface、DPI/HC | 基础 op 真实像素通过 |
| CD5 | Owner-draw Button 纵切 | NativeKind、消息路由、Button painter | 键盘/UIA/嵌套 parent 全过 |
| CD6 | 控件家族扩展 | Combo/Grid/Check/Radio/Slider/Progress/Input chrome | 每控件语义矩阵全绿 |
| CD7 | 资源与高级图元 | Image/Icon、alpha、gradient/path 取舍 | 资源与 GDI 生命周期可证 |
| CD8 | 主题生态与发布门 | theme-gallery、官方主题包、文档/基准/CI | vNext API 与发布矩阵冻结 |

### 14.2 CD0：决策冻结与 Spike

| ID | 任务 | 交付/验收 | 状态 |
|---|---|---|---|
| CD0.1 | 冻结 `DrawList/DrawOp/DrawSurface`、FontSpec 单位、颜色零值与错误模型 | ADR + API 草图；无名称冲突 | 完成 |
| CD0.2 | 验证 LCL Canvas 的 Rect/RoundRect/Ellipse/Text/Clip、字体和 DPI 行为 | 独立 native probe + 像素截图 | 完成 |
| CD0.3 | 验证 `BS_OWNERDRAW + WM_DRAWITEM` 的 32/64 位结构和状态位 | Button 各状态 probe | 完成，未观察状态 deferred |
| CD0.4 | 验证嵌套 parent 下 `SetWindowSubclass` 路由与销毁解绑 | Window/ScrollBox/TabPage 三种 parent | 完成 |
| CD0.5 | 验证 Combo DrawItem、Grid DrawCell、TrackBar/Progress custom draw 能力 | 控件能力矩阵，不虚构统一协议 | 完成，未观察能力 deferred |
| CD0.6 | 确定 partial alpha 首版拒绝策略和未来离屏合成方案 | 校验用例，禁止静默丢 alpha | 完成 |
| CD0.7 | 确定 Theme 兼容方案与公开命名 | `FromLegacyTheme` 迁移样例 | 完成 |

**完成定义**：所有后续阶段依赖的 native 路径至少有一个最小 probe；未验证能力明确标为 deferred，
不能把猜测写进公开承诺。实际命令、版本、截图散列和能力矩阵见
[`cd0-native-probes.md`](./cd0-native-probes.md)。

### 14.3 CD1：Draw Core（无头）

| ID | 任务 | 交付/验收 | 状态 |
|---|---|---|---|
| CD1.1 | 新增 draw 几何、Fill/Stroke、FontSpec/TextPaint 枚举和值类型 | 导出注释、validation tests | 完成 |
| CD1.2 | 实现 immutable DrawList、sealed DrawOp、clone/canonicalize/equality | 调用方修改输入不影响快照 | 完成 |
| CD1.3 | 实现 Clear/Rect/RoundRect/Line/Ellipse/Text/Clip ops | 每种合法/非法矩阵 | 完成 |
| CD1.4 | 新增 internal DrawController 与 Mock | set/reset/invalidate 可分别断言 | 完成 |
| CD1.5 | diff 增加 DrawList mount/patch/remove/D7c | 相同列表零 mutation/invalidate | 完成 |
| CD1.6 | 当前 PaintCommand -> DrawList adapter | 现有 PaintBox 测试不回归 | 完成 |
| CD1.7 | 增加 DrawList 基准 | op 数、分配、DeepEqual/hash 样本 | 完成 |

**完成定义（已满足）**：不依赖 DLL 即可证明 DrawList 的值语义、diff 对称性和 D7c；新图元
真实显示仍属于 CD4，不在本阶段承诺。

### 14.4 CD2：字体、文本测量与布局原语

| ID | 任务 | 交付/验收 |
|---|---|---|
| CD2.1 | 实现 Insets、BorderSpec、ControlStyle/ControlStylePatch | 纯值 + presence mask，无 map/pointer |
| CD2.2 | 新增 StyledTextMeasurer 和 fallback | 第三方 Renderer 不实现仍可运行 |
| CD2.3 | 升级 native 字体/测量缓存 key 与失效规则 | text+font+DPI 命中正确 |
| CD2.4 | Button/Text/Input intrinsic 使用 effective font/padding/min-size；CD3 再接 theme resolver | 不裁字、不出现绘制/布局尺寸分叉 |
| CD2.5 | Row/Column 增加 Gap；新增显式 Padding 包装或等价布局原语 | 不把 margin/padding 偷塞进 Bounds |
| CD2.6 | DPI/系统字体变化测试 | layout 与 paint 同帧更新 |

**完成定义**：Measure 与 DrawText request 已使用同一 FontSpec 和 fallback 契约，Mock 可证明请求一致；
Button 不再依赖固定 `+32/32`。真实 native DrawText 的一致性在 CD4 像素门验收。

### 14.5 CD3：Style Resolver 与主题包 SDK

| ID | 任务 | 交付/验收 |
|---|---|---|
| CD3.1 | 实现 DesignTheme、ColorScheme、Typography、MetricTokens | keyed/constructor API，版本字段 |
| CD3.2 | 实现 component theme 与 ButtonAppearance 等首批类型 | 状态样式纯值、确定性解析 |
| CD3.3 | 实现 ThemeScope、Variant、Style、Rendering 声明 | 不创建 native handle |
| CD3.4 | 在 plugin expand 后、layout 前增加 resolver pass | 插件组合子树能继承主题 |
| CD3.5 | 固定六层覆盖优先级与 nested scope | 表驱动测试全覆盖 |
| CD3.6 | 实现 FromLegacyTheme 和默认兼容主题 | 旧示例视觉/行为不回归 |
| CD3.7 | 主题切换 D7/Inspector 测试 | 同 NativeKind 零重建，同值零 mutation |
| CD3.8 | 新建最小第三方测试主题包 | 只 import 根包，无注册副作用 |

**完成定义**：一个外部 Go package 能定义完整 Button/Surface 样式并由 resolver 生成稳定 Props，
即使 native 仍未全部绘制，也可由 Mock/Inspector 证明结果正确。

### 14.6 CD4：Native Draw Executor、DrawSurface 与 Surface

| ID | 任务 | 交付/验收 |
|---|---|---|
| CD4.1 | 实现 LCL Canvas Draw executor | CD1 全部 op 的 native probe |
| CD4.2 | 统一 DIP->px、pixel snapping、clip 和 TextPaint | 96/144/192 DPI 截图 |
| CD4.3 | 实现 font/pen/brush cache 与 Save/Restore | 重复 paint 无 GDI 增长 |
| CD4.4 | 实现 DrawSurface 新控件 | DrawList patch 不重建，只 invalidate |
| CD4.5 | 实现真实 Surface 容器与 content padding | 子控件坐标、焦点、滚动正确 |
| CD4.6 | Double buffering 与背景擦除策略 | resize/hover 无明显闪烁 |
| CD4.7 | 高对比度/system change 刷新 | 使用系统色并清相关缓存 |
| CD4.8 | PaintBox 迁移到共享 executor | 现有 Circle Drawer 不回归 |

**完成定义**：Draw API 有真实 Windows 落点，Surface 可承载主题容器；尚不替换交互控件。

### 14.7 CD5：Owner-draw Button 纵向闭环

| ID | 任务 | 交付/验收 |
|---|---|---|
| CD5.1 | 引入 NativeKind/StyledControlCreator/canUpdate 规则 | mode 切换只重建目标 Button |
| CD5.2 | 实现 parent subclass registry 与 O(1) HWND 路由 | 嵌套 parent、reparent、destroy 全覆盖 |
| CD5.3 | 实现 TButton BS_OWNERDRAW 和 DRAWITEM state 映射 | 状态表正确，无重复用户事件 |
| CD5.4 | 实现纯 Button painter 与 appearance cache | 状态->DrawList 确定性测试 |
| CD5.5 | 实现 Style/Variant/Rendering 到 Button 的完整链路 | theme package 无 native 依赖 |
| CD5.6 | 保留 Default/Cancel、Tab、Space、Enter/Esc | 真实输入 smoke |
| CD5.7 | 验证 UIA Invoke、Name、Enabled、Focus | accessibility smoke 不退化 |
| CD5.8 | Light/Dark/HC/DPI/disabled screenshot | 像素与系统语义门全绿 |

**完成定义**：Button 是第一条完整纵切：主题数据 -> resolver -> layout -> diff -> owner-draw ->
键盘/UIA/像素，任何一层缺失都不算完成。

### 14.8 CD6：控件家族扩展

按风险分批，不做一次性“大换肤”：

| ID | 批次 | 任务与验收重点 |
|---|---|---|
| CD6.1 | Text/Surface | typography、背景、边框、padding；文本测量一致 |
| CD6.2 | ComboBox | LCL item draw/measure；键盘选择、下拉、UIA 不退化 |
| CD6.3 | StringGrid | header/cell/selection/focus draw；编辑器、逻辑坐标不退化 |
| CD6.4 | ProgressBar | range/value 与 HC；不以动画替代正确值 |
| CD6.5 | Slider | track/thumb/tick；鼠标、方向键、Step、UIA RangeValue |
| CD6.6 | CheckBox | unchecked/checked/indeterminate/hover/focus；Space/UIA Toggle |
| CD6.7 | RadioButton | checked/focus/disabled；逻辑组与方向键导航保持 |
| CD6.8 | Input/Memo chrome | host border/focus/error，内部原生 editor 保留 IME/caret/UIA |
| CD6.9 | PageControl | probe 通过后再决定 tab custom draw；页面 identity 不变 |

每个控件独立进入契约矩阵：mount/patch/remove/D7c、状态回写、capability 缺失、DPI、HC、
键盘、UIA、截图。前一控件通过不代表后一控件可以复用未经验证的消息协议。

### 14.9 CD7：资源、Alpha 与高级图元

| ID | 任务 | 交付/验收 |
|---|---|---|
| CD7.1 | ResourceRef、ThemeBundle、App 作用域安装/释放 | 冲突/版本/Close 清理确定 |
| CD7.2 | PNG/BMP/Icon 解码与 DPI variant | 后台解码 + UI native 创建边界 |
| CD7.3 | DrawImage fit/alignment/tint | 缺失资源诊断与占位行为 |
| CD7.4 | 32-bit 离屏 surface 与 alpha blend | partial alpha 不再静默丢失 |
| CD7.5 | 线性 gradient | capability/version + fallback |
| CD7.6 | 评估 path/阴影/非对称圆角 | 有真实主题需求和 probe 才立项 |
| CD7.7 | cache budget 与资源压力基准 | 内存/GDI 上限、淘汰和无泄漏 |

**完成定义**：主题可以携带稳定图标/图像资源并正确释放；高级效果不破坏无头值语义和
高对比度 fallback。

### 14.10 CD8：主题生态与发布门

| ID | 任务 | 交付/验收 |
|---|---|---|
| CD8.1 | `examples/theme-gallery` | 全控件、状态、light/dark、density 展示 |
| CD8.2 | 官方参考主题包 | 至少 System-compatible + 一个完整 styled 主题 |
| CD8.3 | 第三方主题包模板与开发指南 | 只依赖根包、版本/feature 声明、测试模板 |
| CD8.4 | Inspector 样式/绘制视图 | resolved style、state、ops、cache、rebuild 原因 |
| CD8.5 | 性能基线 | build/resolve/diff/paint/资源/GDI 趋势 |
| CD8.6 | 全量 CI | race/vet/native probe/smoke/screenshots/HC/UIA |
| CD8.7 | API 与迁移文档 | api-vNext、legacy Theme/PaintBox 迁移 |
| CD8.8 | 发布门审计 | 无虚构能力、所有限制和 fallback 有文档 |

**完成定义**：外部作者可仅依赖公开 SDK 发布主题包；官方示例和 CI 能证明样式变化不牺牲
FluxVCL 已有的原生交互、DPI、Accessibility 和 D1-D7。

---

## 15. 依赖关系与并行边界

```text
CD0
 |
CD1 Draw Core ----+
 |                |
CD2 Text/Layout   |
 |                |
CD3 Theme SDK     |
 |                |
+-------> CD4 Native Executor/Surface
              |
              +----> CD5 Button
              |          |
              |          +----> CD6 Control families
              |
              +----> CD7 Resources/advanced ops
                              |
CD5 + CD6 + CD7 --------------+----> CD8 Ecosystem/release
```

- CD1 的纯值/Mock 与 CD0 后半段 native probe 可并行。
- CD3 resolver 可在 CD4 native executor 前完成并由 Mock 验证。
- CD5 Button 和 CD7 资源在 CD4 后可并行，但 CD8 需要二者稳定。
- CD6 每个控件可独立分支，但共享 API 变更必须先回到主设计评审。

---

## 16. 风险登记

| 风险 | 等级 | 控制措施 |
|---|---|---|
| Owner-draw 破坏键盘/UIA/DefaultButton | 高 | 保留 TButton；真实输入和 UIA 是 CD5 硬门 |
| WM_DRAWITEM 只钩 Form，嵌套控件不绘制 | 高 | direct-parent subclass registry + 嵌套 probe |
| 字体/padding 只改 paint 导致裁剪 | 高 | resolver 在 layout 前；Measure/Paint 共用 FontSpec |
| GDI 对象泄漏或 HDC 越过回调期 | 高 | native 独占资源；Save/Restore；GetGuiResources 压测 |
| 函数 painter 每 render 重绑/paint 重入 | 高 | 首版封闭 DrawOp，禁止公开 Canvas callback |
| 主题切换重建输入控件丢 IME/caret | 高 | RenderingMode 进入 identity；同 mode 只 patch；输入保持原生 editor |
| Alpha 被 TColor 静默丢弃 | 中 | 首版明确拒绝 partial alpha；CD7 后才开放 |
| Theme struct 扩张破坏第三方源码 | 中 | 新 DesignTheme、keyed/constructor API、schema version |
| 不同控件 custom-draw 协议被错误统一 | 中 | 每控件 probe/capability，按家族分批 |
| DrawList 过大导致 diff/分配压力 | 中 | 防御上限、基准、可选 hash/cache，不牺牲正确性 |
| 高对比度仍使用品牌色 | 高 | native 系统色最终覆盖；HC screenshot/UIA 门 |

---

## 17. 首个可交付切片

如果需要在完整路线中选一个最小但架构真实的交付，应选择：

```text
CD0 必要 probe
  + CD1 DrawList/Mock/diff
  + CD2 FontSpec/Insets/measure
  + CD3 最小 DesignTheme resolver
  + CD4 基础 executor
  + CD5 一个 Owner-draw Button
```

只实现“Button OnPaint 能改背景色”不算有效切片，因为它没有验证布局、主题包、identity、嵌套
parent、键盘、UIA、DPI、高对比度和 D7。完成上述纵切后，再增加控件主要是按能力矩阵扩展，
不会再次改写底层协议。

---

## 18. 设计完成检查表

- [ ] 公开 Draw API 无 LCL/Win32 类型和函数回调。
- [ ] DrawList/Style/Theme 都有不可变所有权和结构化校验。
- [ ] style resolution 位于 plugin expand 后、layout 前。
- [ ] FontSpec 同时驱动 Measure 与 paint。
- [ ] VisualState 不写回业务 State，内部 handler 与用户 handler 可组合。
- [ ] RenderingMode/NativeKind 参与 identity，切换原因可观测。
- [ ] 基础 Renderer 未因样式继续膨胀。
- [ ] PaintBox 与 legacy Theme 有兼容适配。
- [ ] 默认后端处理 direct-parent owner-draw 路由和完整销毁。
- [ ] DPI、高对比度、键盘、IME、UIA、GDI 生命周期都有硬验收项。
- [ ] 第三方主题包只 import 根包且无全局注册副作用。
- [ ] 每阶段均有无头门和必要的 Windows 真实门。
