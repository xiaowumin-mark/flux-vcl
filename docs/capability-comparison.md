# 可验证能力对比

本页只记录 FluxVCL v0.1.0 在十个既定维度上的公开语义、默认 LCL 后端落地、
仓库证据和已知边界。它可作为与其他桌面 UI 工具逐项核对的清单，但不替第三方
项目声明能力，也不比较流行度、性能排名或尚未复现的行为。

| 维度 | 公开语义与默认后端落地 | 可复核证据 | 当前边界 |
|---|---|---|---|
| 原生控件 | Widget 经 diff 创建绑定层句柄；默认后端把 Button、Input、PageControl、Slider、StringGrid、PaintBox 等映射到 LCL 控件。 | [21 控件契约清单](../control_contract_test.go)、[批次 3 native probe](../internal/native/batch3_probe_test.go)、[native 适配](../internal/native/native.go) | 默认后端依赖锁定版本的 `energye/lcl` 与 `libenergy-amd64.dll`；PaintBox 图元不是独立可访问原生子控件。 |
| 声明式 diff | `同 Type + 同 Key` 原地复用，属性变化产生 setter/event mutation；相同树不产生 mutation。 | [diff D7 测试](../internal/diff/diff_test.go)、[全控件 mount/patch/D7c 矩阵](../control_contract_test.go) | State 失效会重新执行 build；“局部”指提交阶段只 patch 变化属性，不表示 build 函数自动细粒度跳过。 |
| IME | Input/Memo 使用原生编辑器；字符事件由 `OnUTF8KeyPress` 映射为 UTF-8 `Event.Text`。StringGrid 使用 LCL 内嵌编辑器，其 IME 行为列为手工验收项。 | [事件测试](../event_test.go)、[native 键盘事件映射](../internal/native/native.go)、[events 示例](../examples/events/main.go) | 输入法候选窗、组合过程和 Grid 内嵌编辑器细节由 Windows/LCL 决定；当前没有自动化 Grid IME 证据，不把继承行为表述为已验证能力。 |
| DPI | 公开布局使用 DIP；native 边界执行 DIP/px 换算，窗口收到 `WM_DPICHANGED` 后清测量缓存并触发布局失效。 | [DIP 换算测试](../internal/render/dip_test.go)、[事件坐标归一测试](../internal/native/mapping_test.go)、[DPI hook](../internal/native/native.go) | PageControl 表头等 widgetset 绘制细节仍可能随主题和 DPI 与固定布局预算有细微差异。 |
| 虚拟化 | `ListView` 只构建视口及 overscan 槽位；十万行数据不会创建十万个原生行控件，滚动复用稳定槽位。 | [ListView 有界控件池测试](../list_test.go)、[virtual-list 示例](../examples/virtual-list/main.go)、[性能样本](performance-baseline.md) | 当前为固定行高、垂直列表；必须获得有界高度，行内容不能用数据索引破坏槽位身份。 |
| 多窗口 | 每个窗口使用独立 Renderer/App/State 作用域；次要 Renderer 显式 `Show()`。 | [virtual-list 双窗口示例](../examples/virtual-list/main.go)、[Renderer.Show](../internal/native/native.go) | 当前模型是一窗一 App/Renderer，不是单个 App 管理任意窗口图的 API。 |
| Inspector | `App.ObserveInspector`/`InspectorSnapshot` 暴露只读 Widget、Element、native 三层快照，以及 mutation、事件和重建风险；工具窗使用独立窗口。 | [Inspector 行为测试](../inspector_test.go)、[Inspector 示例](../examples/inspector/main.go)、[工具窗实现](../inspector/inspector.go) | 它是进程内诊断工具，不是采样 profiler，也不承诺读取任意后端私有对象。 |
| 插件 | `RegisterWidget`/`PluginWidget` 注册进程内 Go builder，支持类型化属性、DIP Measure、生命周期和只读具名 capability。 | [插件契约测试](../plugin_test.go)、[Badge 示例](../examples/plugin-badge/badge/badge.go)、[公开插件 API](../plugin.go) | 插件随应用编译链接；不是 DLL、Go `plugin` 热加载或绕过绑定层创建任意原生控件的接口。 |
| Accessibility | LCL name/description/value 元数据、显式 TabStop、声明树 TabOrder、默认/取消按钮、Radio 方向键逻辑和高对比度系统色；Windows smoke 使用真实输入、焦点和 UIA 查询。 | [能力分层与矩阵](accessibility-i18n.md)、[无头契约](../accessibility_test.go)、[native probe](../internal/native/accessibility_probe_test.go)、[专用示例](../examples/accessibility-i18n/main.go) | provider 稳定后，桌面根与 `FromHandle` 查询均由 Win32 代理提供标准控件 Pattern，但 Accessible 覆盖值未投射；StringGrid 无 Grid Pattern，PaintBox/TLabel 无子 HWND，虚拟行无 List Pattern。 |
| i18n | 不可变 Catalog、locale fallback、响应式 `Catalog.Bind`、稳定诊断 Message ID，以及可替换的内建中英文框架诊断。 | [Catalog 测试](../i18n_test.go)、[嵌入式中英资源](../examples/accessibility-i18n/main.go)、[公开 API](api-v0.1.0.md) | Locale tag 区分大小写；框架不规定资源文件格式、复数规则或翻译平台。locale 切换仍会执行 build，但稳定控件只 patch 属性。 |

发布结论以 [候选 API 冻结清单](api-v0.1.0.md)、[发布检查表](../RELEASE_CHECKLIST.md)
和实际 CI 结果为准；未通过的门禁不因本页列出能力而视为完成。
