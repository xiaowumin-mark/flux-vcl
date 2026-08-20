# CD2 字体、文本测量与布局原语实施记录

状态：完成（无头范围；真实 DrawText 像素门仍属于 CD4）
日期：2026-08-20

CD2 将 CD0 冻结的字体值语义接入布局测量，并为样式和布局增加可组合的纯值原语。扩展保持
`render.Renderer` 的原有窄接口兼容：旧后端不实现 styled measurement 时自动回落到
`TextExtent`，不会因为能力缺失而 panic 或改变挂载流程。

## 已交付

- 根包新增纯值 `Insets`、`BorderSpec`、`ControlStyle`、`FocusStyle`、`StyleFieldMask` 和
  `ControlStylePatch`。Patch 使用 presence mask，可表达“显式覆盖为零”；值中没有 map、slice、
  pointer 或函数。提供构造器、校验、合并和 `Set*/With*` presence-safe options。
- `internal/render` 新增 `TextMeasureRequest`、`TextMeasureConstraints`、`Size`、
  `StyledTextMeasurer` 与 `MeasureText` fallback。请求统一规范化 FontSpec、DPI、换行/溢出和
  约束，并通过 `TextMeasureCacheKey` 固定缓存身份。Mock 保留请求快照，能够断言布局测量使用的
  字体值。
- LCL 后端的测量缓存 key 已升级为 text + FontSpec + DPI + font revision + wrap/constraint；
  共享 bitmap canvas 使用同一解析字体，系统字体、DPI、主题、高对比度和窗口关闭都会释放
  measurement/native-font cache。DPI 和系统字体变化会重施 live control 字体并触发同帧布局；
  字体更新会恢复独立声明的 `FontColor`，避免属性应用顺序造成文字色丢失。
- Text、Button、Input intrinsic 尺寸统一经过 effective FontSpec、padding 和 min-size；默认值
  仍保持兼容尺寸，但 Button 不再在布局代码中散落固定 `+32/32`。
- `FontSpec.Validate` 与样式/字体 Opt 入口遵循 CD0：负字号、未知字重、非法 UTF-8 或超过
  255 字节的 family 直接拒绝；测量 request 边界仍保留防御性归一化，避免第三方 Renderer
  因不受信输入崩溃。
- Row/Column/Window 支持显式 `Gap`（默认仍为 4 DIP，`Gap(0)` 可关闭）；新增 `PaddingBox` 和
  双用途 `Padding(insets[, child])` 原语。Padding 在约束下传阶段消耗/恢复，不篡改子控件 Bounds。
- CD0 名称审计已将已实现的 CD2 类型移入 intentional-reuse 清单；新增无头样例测试覆盖
  presence mask、零值覆盖、缓存 key、fallback 和 styled request 记录；native probe 覆盖
  96/144/192 DPI 归一、字体重施和 FontColor 保持。

## 边界

本记录不承诺 CD4 的真实 Canvas `DrawText` 像素一致性、主题 resolver 或 owner-draw 控件样式。
FontSpec 的系统 family/size fallback 由 native 后端在测量与绘制边界共同解析；第三方 Renderer
仍可只实现旧 `Renderer` 接口。

## 验证命令

```text
gofmt -w controls.go layout.go layout_style.go opts.go style.go internal/render internal/native
go vet ./...
go test ./... -count=1
go test ./... -race
git diff --check
```
