# CD0 自绘与主题决策记录

> 状态：Accepted（实现分阶段）
> 日期：2026-08-20
> 范围：CD0.1、CD0.6、CD0.7 以及后续 CD1-CD7 的共同输入
> API 基线：[`api-vnext.md`](./api-vnext.md)
> 可执行审计：`cd0_api_names_test.go`、`cd0_legacy_theme_test.go`
> 原生 Spike：[`cd0-native-probes.md`](./cd0-native-probes.md)

本文冻结后续实现必须遵守的契约。当前兼容入口仍是 `PaintBox([]PaintCommand, ...)` 和
legacy `Theme`；CD1 已落地 `DrawList`/Draw Core，`DesignTheme`、`ThemeScope`、
`FromLegacyTheme` 等符号仍按 CD3 阶段门实现。

## 1. 决策依据

本次冻结基于以下仓库事实和 Spike 结果：

- 根包已经导出 `Color(c ColorValue) Opt`，Go 的包级标识符不能同时声明同名类型，因此颜色
  类型继续叫 `ColorValue`，不引入 `type Color`。
- `Rect`、`Point`、`Size` 已使用整数 DIP；Draw API 复用这些类型，不另建一套像素几何。
- 当前 `PaintBox` 已证明稳定值命令、防御性复制、同值零重绘和结构化校验路径可行；它在兼容期
  保留，但不继续扩张为通用命令巨型 struct。
- `ColorValue` 是 `0xAARRGGBB`。当前 LCL `TColor` 路径不能表达 partial alpha；新 Draw
  executor 不得沿用“丢弃 alpha 后继续绘制”的行为。
- `LightTheme`、`DarkTheme` 是可变包变量，`Theme` 的全部字段都是值类型。任何 adapter 必须
  在调用时取值并生成独立快照，不能保存包变量或调用方对象的地址。
- `internal/render.PaintSurface` 是内部 capability 的兼容别名，不占用 `flux` 根包命名空间；
  vNext 的公开 surface 构造器仍叫 `DrawSurface`。

## 2. ADR-CD0-01：Draw API 边界与名称

决定：公开绘制输入采用不可变 `DrawList` 和包内封闭的 `DrawOp`，不公开 Canvas/HDC 或
`func(Canvas)`。冻结的根包名称、签名方向和枚举见 `api-vnext.md`。

关键约束：

1. `DrawList` 零值合法，表示空命令列表。
2. `DrawOp` 是带包内方法的 sealed interface；第三方只能使用 `Clear`、`FillRect`、
   `DrawText` 等公开值构造器，不能把后端代码塞入列表。
3. `NewDrawList` 验证并复制全部输入；`MustDrawList` 只是在错误时 panic 的便捷包装，panic
   值必须是原始 `error`，不能退化成字符串。
4. `DrawSurface` 是 Widget 构造函数，不是后端 surface handle 类型。公开 API 不出现 LCL、
   Win32 或 GDI 类型。
5. `PaintBox`/`PaintCommand` 保持兼容；CD1 通过 adapter 转为 `DrawList`。旧入口的行为变更
   只能是补充确定性校验，不能偷偷改变坐标或重绘语义。

### 2.1 防御上限

CD1 的首版验证器采用以下确定上限。上限是拒绝输入的契约，不允许只截断后继续绘制。

| 项 | 首版上限 | 原因 |
|---|---:|---|
| 单个 `DrawList` 操作数 | 4096 | 控制 clone、比较和 paint callback 的最坏成本 |
| 单个 `DrawText` UTF-8 字节数 | 65536 | 足够 UI 文本，避免单条命令携带无界数据 |
| clip 嵌套深度 | 32 | 匹配首版矩形 clip 用途并限制 native state |
| 坐标、宽高、radius、stroke width 的绝对值 | 1048576 DIP | DPI 换算后仍远离 Win32 `int32` 溢出 |
| 字体 family UTF-8 字节数 | 255 | 拒绝异常缓存 key；不按字节截断字体名 |

负宽高、负 radius、非正 stroke width、不平衡 clip、未知枚举和超限值均返回错误。零宽高
矩形是合法空几何；canonicalization 可移除其无效果操作，但不能改变后续 clip 平衡。

## 3. ADR-CD0-02：FontSpec 单位与零值

决定：全部公开几何和 `FontSpec.Size` 使用整数 DIP，不使用 point，也不接受物理像素。

- `FontSpec{}` 表示系统 UI 字体：空 family 使用当前系统 UI family，`Size == 0` 使用当前系统
  UI 字号，零 weight 规范化为 `FontWeightNormal`。
- `Size > 0` 表示 DIP 中的 em size。native 在边界用同一 DPI 规则转换；Measure 和 DrawText
  必须接收同一个已经解析的 `FontSpec`。
- `Size < 0` 非法。调用方若需要按系统设置继承，必须写 0，不能依赖负 LOGFONT height。
- `Weight` 使用 CSS/Win32 一致的数值（400/500/600/700）；首版只公开 Normal、Medium、
  Semibold、Bold。未知值拒绝，不作最近值猜测。
- DPI、系统字体、Theme typography 或高对比度变化都会使字体测量/资源缓存失效。

## 4. ADR-CD0-03：颜色零值与 Alpha

决定：`ColorValue(0)` 是唯一的“没有 paint / 使用系统默认”哨兵，不表示透明黑。

| 值 | Draw v1 语义 |
|---|---|
| `0x00000000` | 无 paint / 未指定；需要实际颜色的 DrawOp 校验失败 |
| `0x00RRGGBB` 且 RGB 非零 | 非法；不能伪装成零值，也不能静默当作不透明色 |
| alpha 在 `0x01` 到 `0xFE` 之间 | `flux.draw.alpha_unsupported` |
| `0xFFRRGGBB` | 合法不透明色 |

因此首版没有“透明黑”值。`Clear`、Fill、Stroke 和 Text 这类必须产生像素的操作收到零色时
返回字段级校验错误；Style 中的零色仍可表达该层不绘制。CD7 只有在 32-bit 离屏 surface、
预乘 alpha、合成顺序和截图测试全部通过后才能开放 partial alpha。

当前 `PaintBox` 同步采用相同拒绝策略，作为新 validator 的先行验证。legacy native 控件路径
仍属于已有兼容面；`FromLegacyTheme` 必须按位保存 legacy 颜色，不能在 adapter 内丢 alpha。
当这些颜色进入 styled Draw 路径时，由 Draw validator 给出上述结构化错误。

## 5. ADR-CD0-04：错误模型

决定：Draw 构造错误使用可判定 sentinel、结构化 error 和稳定 `MessageID` 三层模型。

```go
var ErrInvalidDrawList error

type DrawValidationError struct {
    ID      MessageID // 稳定诊断 ID
    OpIndex int       // -1 表示列表级错误
    Field   string    // 稳定 ASCII 字段名；列表级为空
}
```

- `errors.Is(err, ErrInvalidDrawList)` 判定大类；`errors.As` 取得 op index、field 和 ID。
- `Error()` 从诊断 catalog 格式化展示文本；业务逻辑不得解析错误字符串。
- `NewDrawList` 失败时返回零值 `DrawList` 和 error，不返回部分列表。
- `MustDrawList` panic `error` 对象。diff/native 边界再次验证失败时写入 App/Inspector 诊断，
  OS paint callback 不向外传播 panic。
- 首版稳定 ID 前缀为 `flux.draw.*`：`too_many_ops`、`text_too_long`、
  `clip_underflow`、`clip_unbalanced`、`coordinate_range`、`negative_size`、
  `radius_negative`、`stroke_width`、`color_required`、`alpha_unsupported`、
  `font_size` 和 `enum_unknown`。

## 6. ADR-CD0-05：Legacy Theme 兼容

决定：旧 `Theme` 与 `LightTheme`/`DarkTheme`、`Color`、`FontColor`、`DarkTitleBar` 在
vNext 兼容期继续可用。新系统增加 `DesignTheme` 和：

```go
func FromLegacyTheme(legacy Theme) DesignTheme
```

该函数是确定性、无副作用的值转换，规则如下：

| legacy 字段 | vNext 含义 |
|---|---|
| `Primary` | `ColorScheme.Primary`，按位复制 |
| `Background` | `ColorScheme.Background`，按位复制 |
| `Surface` | `ColorScheme.Surface`，按位复制 |
| `Text` | `ColorScheme.Foreground`，按位复制 |
| `Accent` | `ColorScheme.Accent`，按位复制 |
| `DarkTitleBar` | Window 的 preferred-dark-title-bar token，原值复制 |
| `FontSize > 0` | 默认 typography size，单位解释为 DIP |
| `FontSize <= 0` | 系统 UI 字号，即解析后的 `FontSpec.Size == 0` |
| `Radius >= 0` | 默认 control radius，原值复制 |
| `Radius < 0` | 规范化为 0；legacy 字段以前未接后端，不新增负 radius 语义 |

额外规则：

1. 参数在函数入口按值快照。返回值不引用 `LightTheme`、`DarkTheme` 或调用方可变
   slice/map；调用后修改任一方都不影响另一方。
2. 五个颜色字段包括零值和 alpha 位在内全部按位保存。adapter 不通过猜测填色，也不做
   对比色或 hover/pressed 色算法。
3. legacy 无法表达的 Hovered、Pressed、Disabled、Focused 等 component state 使用内建
   system-compatible 默认值。转换器不虚构品牌状态色。
4. 未显式请求 `StyledRendering` 时，迁移后的 ThemeScope 继续选择 NativeRendering；旧应用
   因此不会因仅包一层 ThemeScope 而重建 HWND 或改变 IME/UIA 行为。
5. `DarkTitleBar` 只影响顶层 Window policy，不下传为普通子控件颜色。

CD0 的可编译迁移样例位于 `cd0_legacy_theme_test.go`。在 CD3.6 实现生产
`FromLegacyTheme` 后，测试中的 reference adapter 必须替换成公开函数并保持同一表格断言。

## 7. 命名冲突审计

`cd0_api_names_test.go` 扫描根包的非测试 Go 声明，并对 `api-vnext.md` 中 CD0 保留名称执行
以下检查：合法导出标识符、清单内无重复、拟议名称不占用现有包级命名空间、复用名称的声明
种类符合预期。

审计结论：

| 名称 | 结论 |
|---|---|
| `Color` | 已是 Opt 函数；颜色类型固定为 `ColorValue` |
| `Text`、`Button`、`Surface` 相关词 | 控件构造器保留；主题类型使用 `TextTheme`、`ButtonTheme`、`SurfaceTheme` 后缀 |
| `PaintSurface` | 仅为 `internal/render` 别名，不导出到根包；不与 `DrawSurface` 冲突 |
| `Theme` | 保留 legacy 类型；新完整主题叫 `DesignTheme` |
| `LightTheme`、`DarkTheme` | 保留 legacy 可变变量；vNext 不再增加可变 DesignTheme 包变量 |
| `DrawList`、`DrawOp`、`DrawSurface`、`FontSpec`、`FromLegacyTheme` | 当前根包无同名声明 |

## 8. 后续实现门

- CD1 已按本记录实现 Draw 值语义、限制、error 和 alpha 单元测试；调整任何上限需新 ADR。
- CD2 必须证明 Measure/DrawText 共用解析后的 FontSpec 和 cache key。
- CD3 必须让公开 `FromLegacyTheme` 通过 CD0 reference mapping、快照和旧示例回归测试。
- CD4/CD5 的 native probe 结果只能收窄 capability 或标注 deferred，不能让后端限制泄漏到公开
  Draw 类型。
- CD7 开放 alpha 前必须保留旧“不支持”诊断作为 capability/version fallback。
