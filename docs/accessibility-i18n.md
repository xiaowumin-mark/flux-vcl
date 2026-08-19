# Accessibility / i18n 能力表

本文记录 FluxVCL v0.1.0 候选版在 Windows 默认 `energye/lcl v1.0.3`
后端上的可验证能力。它区分 Windows 原生继承、FluxVCL 框架补充和绑定层限制；
存在限制的控件不会仅因公开了 `AccessibleName` 就被描述成完整 UIA Provider。

## 可访问性分层

| 对象 | 原生键盘/窗口行为 | FluxVCL 补充 | UIA / energye 限制 |
|---|---|---|---|
| Button | 原生焦点框、Space、Enter、Esc；桌面 UIA Win32 代理为 Button/Invoke | `AccessibleName/Description`、`DefaultButton`、`CancelButton` | UIA Name 使用可见 caption，LCL AccessibleName/Description 未投射。 |
| Input | 原生单行编辑、caret、选择、IME；Win32 代理提供 Edit/Value/Text | 名称/说明/值、声明树 Tab 顺序、`TabStop` | UIA Name 可能是当前输入内容，LCL AccessibleName/Description 未投射。 |
| Memo | 原生多行编辑、caret、选择、IME 和滚动；Win32 代理提供 Document/Scroll/Text | 名称/说明/值、声明树 Tab 顺序、`TabStop` | 不是 Edit/Value；LCL AccessibleName/Description 未投射。 |
| CheckBox | 原生焦点与 Space 切换；Win32 代理继承 CheckBox/Toggle | 名称/说明、Tab 顺序 | LCL Accessible 元数据未投射。 |
| RadioButton | 原生焦点和选中绘制；Win32 代理继承 RadioButton/SelectionItem | 同一逻辑父级和 `GroupIndex` 的互斥；Left/Up/Right/Down 循环选择并移动焦点 | 逐项内部 Panel 不形成标准 UIA 分组容器；LCL Accessible 元数据未投射。 |
| ComboBox | 原生下拉、选择和方向键；桌面 UIA 代理提供 ComboBox/Selection/ExpandCollapse | 名称/说明/当前值、受控选择 | 不可编辑的 `CsDropDownList` 没有 Value Pattern；UIA Name 来自原生显示值，LCL Accessible 元数据未投射。 |
| Slider | 原生拖动和方向键；桌面 UIA 代理提供 Slider/RangeValue | 名称/说明/文本值、受控数值 | LCL AccessibleValue 未投射；Step 是 FluxVCL 键盘步长契约。 |
| ProgressBar | 原生只读进度显示；Win32 代理提供 ProgressBar/RangeValue | 受控最小值、最大值和当前值 | 不进入 Tab 环；LCL AccessibleValue 未投射。 |
| PageControl | 原生页签选择；Win32 代理提供 Tab/Selection/Scroll 和 TabItem 子项 | 稳定页面 Key、声明树顺序 | 页面结构、子项名称和附加 Pattern 仍由 LCL/Win32 代理决定。 |
| StringGrid | 可聚焦原生窗口；键盘选择和内嵌编辑器 | 可访问名称/说明/文本值 | 桌面 UIA 代理仍为 Pane，无 Grid/Table Pattern 或可枚举 cell，元数据也未投射；屏幕阅读器不能把它当完整数据表读取。 |
| Text | 原生标签绘制 | 可设置元数据供支持它的后端使用 | `TLabel` 无独立 HWND，默认 Win32 UIA 树通常不含该节点；不能作为输入控件唯一的可访问标签。 |
| PaintBox | 无独立 HWND | 可设置名称/说明/文本值；示例提供邻接状态和等价 Button 操作 | `TPaintBox` 不在 UIA 树，图元不是可访问子对象；当前后端没有虚拟 UIA Provider。 |
| ListView | ScrollBox/滚动条的原生窗口行为 | 可见行控件池保持有界；示例中的行操作使用 Button | 这是布局虚拟化容器，不是原生 List UIA Pattern；纯文本虚拟行没有列表项语义，离屏行也不会进入 UIA 树。 |

专用 Windows smoke 持续锁定 Button、Input、ComboBox、Slider 和 StringGrid 的 UIA
合同；Memo、CheckBox、RadioButton、ProgressBar 与 PageControl 行来自同一锁定运行时的
人工 UIA 能力审计，尚未作为逐控件 CI 回归。表格因此是能力清单，不是所有屏幕阅读器
或 Windows 版本的兼容认证。

等待进程 input-idle 且 UIA provider 就绪后，从主窗体
`AutomationElement.FromHandle(...).FindAll` 枚举，或从桌面根节点按 ProcessId +
NativeWindowHandle 查询同一 HWND，Windows client proxy 都能为标准
Button/Input/Memo/ComboBox/Slider/ProgressBar/PageControl 提供上表的原生类型和
Pattern。启动早期的不完整树不是
稳定能力契约。两条查询路径都确认 LCL `AccessibleName/Description/Value` 没有投射
到 Provider：Name 来自 caption/当前值，HelpText 为空。StringGrid 仍只有
Pane/Window 且无 Grid Pattern。
这不是对 NVDA、JAWS 或所有 Windows 版本的兼容承诺。`energye/lcl` 没有公开
自定义 MSAA/UIA Provider、`NotifyWinEvent` 或虚拟子节点桥接，因此这些限制不能
由 Props 单独消除。

## 框架补充

- `AccessibleName`、`AccessibleDescription`、`AccessibleValue` 通过可选 Renderer
  capability 下发；默认后端写入 LCL 对象但当前 UIA bridge 不读取它们，不实现该
  capability 的第三方后端则安全退化。
- `TabStop` 可覆盖原生默认值，移除 Opt 后恢复控件类型的创建默认值。
- 同一原生父级内的 `TabOrder` 由声明树自动生成。透明布局/组件不另开顺序；
  keyed 重排只 patch 顺序，不重建有状态控件。
- `DefaultButton(true)` 对应 Enter，`CancelButton(true)` 对应 Esc；一个窗体应各自
  只声明一个。
- 标准控件保留系统焦点指示。高对比度模式下，应用声明的背景色和文字色回落为
  `ClDefault`，暗色标题栏关闭；PaintBox 使用 `ClWindow`、`ClHighlight` 和
  `ClWindowText`。系统设置、主题和系统颜色消息会触发重新应用。
- 自动化可在启动进程前设置 `FLUXVCL_FORCE_HIGH_CONTRAST=1`；这只用于测试，
  未设置时以 Windows `SPI_GETHIGHCONTRAST` 为准。

## 国际化资源

```go
catalog := flux.MustCatalog("en", flux.Resources{
    "en":    {"save": "Save"},
    "zh-CN": {"save": "保存"},
})
locale := flux.NewState[flux.Locale]("en")

flux.Button(catalog.Bind(locale, "save"))
```

- `Locale` 和 `MessageID` 是不透明、区分大小写的字符串；项目通常使用 BCP 47 tag。
- `NewCatalog` 校验并深复制两层 map；Lookup 先查请求 locale，再查 fallback。
  缺失消息由 `Format` 返回 Message ID，使问题可见且结果确定。
- `Catalog.Bind` 订阅 `State[Locale]`。切换 locale 会重新 build，但 diff 只 patch
  变化的文字和可访问属性；稳定 Key 对应的 Input、ComboBox、Slider、Grid 句柄和
  受控状态保持不变。
- 框架公开校验诊断使用稳定 `Diagnostic...` Message ID。`SetDiagnosticLocale`
  选择内建中/英文资源；`SetDiagnosticCatalog` 可替换进程级资源，缺失项回落到
  FluxVCL 内建目录。哨兵错误仍用于 `errors.Is`，不应通过匹配显示文本判定错误。
- 框架不规定 JSON、YAML 或远端资源格式。`examples/accessibility-i18n` 演示把
  嵌入式 `en.json`、`zh-CN.json` 解析为 `Resources`。

## 验证矩阵

| 层级 | 自动化证据 |
|---|---|
| 无头 | 可访问属性添加/移除、默认 TabStop 恢复、keyed 重排零重建、Catalog fallback/防御复制、诊断替换、locale 切换保持状态和句柄。 |
| Native probe | LCL Accessible 属性、TabOrder/TabStop、默认/取消按钮、Radio 方向键逻辑、高对比度颜色回落和 PaintBox/Grid 元数据。 |
| Windows smoke | 8 个代表性控件上的真实 `SendInput` Tab/Shift+Tab、方向键、Space、Enter、Esc；`GetGUIThreadInfo` 焦点；provider 就绪后的桌面根/`FromHandle` UIA 原生类型与 Pattern、未投射 AccessibleName/HelpText 和 Grid Pattern 缺失；中英切换前后 HWND/焦点/状态；强制高对比度像素有效截图。 |

## 应用要求

- 每个无可见原生标签的 Input、ComboBox、Slider、Grid 和自绘区域都应声明稳定、
  本地化的 `AccessibleName`。
- 不要用带 `OnClick` 的 Text 代替 Button。PaintBox 鼠标操作必须提供可聚焦的
  等价命令，或在产品文档中明确该操作不可由键盘完成。
- locale 切换前后应测试最长支持语言、窗口缩放、焦点、caret、选择和布局诊断；
  仅验证字符串变化不足以证明国际化完成。

## English summary

FluxVCL forwards accessible name, description, and value metadata into LCL,
derives per-parent tab order from the declarative tree, preserves that
order across keyed reorders, supports explicit tab stops and default/cancel
buttons, and restores arrow-key navigation for logical radio groups. In high
contrast it defers standard-control colors to Windows and maps PaintBox drawing
to system window, highlight, and text colors.

The locked backend does not project LCL accessible metadata into a Windows UIA
provider. After input-idle and provider readiness, both a `FromHandle` subtree
lookup and a desktop-root lookup by process and HWND let the Windows client proxy
expose standard contracts including Button/Invoke, Input Edit/Value,
Memo Document/Scroll/Text, ComboBox/Selection, Slider and ProgressBar RangeValue,
and PageControl Tab/Selection/Scroll with TabItem children. In both paths, an
`AccessibleName` override is not used and HelpText stays empty. StringGrid remains
a Pane without Grid/Table patterns; PaintBox and
TLabel have no child HWND; virtual ListView rows do not form a UIA List. The
metadata remains useful to a future backend/provider, but must not be presented
as current custom-name or custom-provider support.

`Catalog`, `Resources`, and `Catalog.Bind` provide immutable fallback resources
and reactive locale switching. Framework validation diagnostics use stable
message IDs and replaceable built-in Chinese/English catalogs. See
`examples/accessibility-i18n` for embedded JSON resources and the Windows
keyboard/UIA/high-contrast verification target.
