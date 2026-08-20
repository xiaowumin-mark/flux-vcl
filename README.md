# FluxVCL

基于 Go 的现代声明式 UI 框架，使用原生 Windows 控件。

> 中文主文档 ｜ [English README](README.en.md)

提供类似 Flutter / Vue / SwiftUI 的开发体验，同时保留桌面原生控件与键盘行为；
锁定 LCL runtime 的 UIA 能力边界见下方 P7.6 和能力表。

```go
Window(
    Column(
        Text("Hello"),

        Button(
            "OK",
            OnClick(func(e Event) {
                // e.Type == EventClick；e.Source == "Button#..."
                // ...
            }),
        ),
    ),
)
```

## 设计要点

- **声明式 + 状态驱动**：UI 即结构，状态自动同步
- **原生控件**：默认 LCL 后端（`energye/lcl` + libenergy 运行时），零 CGO
- **后端隔离**：当前实现为 LCL（`energye/lcl`）；VCL（`govcl`）仅保留为尚未立项的 B 计划
- **现代布局**：自定义 Measure/Layout（Flutter 风格 constraints），禁用原生 Align
- **线程纪律**：单一 UI 线程 + marshalling 调度器

> 架构不变量（D1–D7）见 [docs/design.md](docs/design.md) 与 [docs/development-plan.md](docs/development-plan.md)。

产品化文档入口：

- [7GUIs 任务映射（中英双语）](docs/7guis.md)
- [v0.1.0 候选 API 冻结清单](docs/api-v0.1.0.md)
- [迁移指南](docs/migration.md) 与 [维护政策](docs/maintenance.md)
- [可验证能力对比](docs/capability-comparison.md)
- [Accessibility / i18n 能力表](docs/accessibility-i18n.md)
- [自绘与样式系统设计](docs/custom-draw-style-system.md)
- [CHANGELOG](CHANGELOG.md) 与 [发布检查表](RELEASE_CHECKLIST.md)

## 项目状态

**Phase 0（地基与选型验证）✅ 完成**：

| 子任务 | 状态 |
|---|---|
| 0.1 绑定选型实验（energye/lcl 构建冒烟） | ✅ [结论](docs/phase0-e2-libenergy-mapping.md) |
| 0.2 DLL 交付与许可方案 | ✅ |
| 0.3 构建脚手架（manifest/icon/`.syso`/构建脚本） | ✅ `scripts/build.ps1` |
| 0.4 仓库与模块 | ✅ |
| 0.5 CI 骨架 | ✅ `.github/workflows/ci.yml` + `scripts/fetch-libenergy.ps1` |
| 0.6 无头测试驱动雏形 | ✅ `internal/render`（接口 + Mock + 无显示测试） |

**Phase 1（声明式核心）✅ 完成**：Widget/Node/Element → diff 引擎 → 基础控件集 → LCL 适配。

| 子任务 | 状态 |
|---|---|
| 1.1 Widget/Node 数据结构 | ✅ `internal/widget` |
| 1.2 Element 树与 identity（D1 canUpdate） | ✅ `internal/diff` |
| 1.3 Renderer 接口 + op 集 + LCL 适配 | ✅ `internal/render` + `internal/native` |
| 1.4 diff/reconciliation 引擎（D2/D3） | ✅ `internal/diff` |
| 1.5 基础控件集（Window/Column/Row/Text/Button/Input） | ✅ flux 根包 |
| 1.6 原生逃逸口 Native/Ref | ✅ |
| 1.7 D7 三不变量测试 | ✅ `internal/diff` + flux 端到端 |

State 系统（Phase 2）、布局引擎（Phase 3）、事件/生命周期（Phase 4）见 [开发计划](docs/development-plan.md)。

**Phase 2（State 系统与数据绑定）✅ 完成**：`State[T]` / `Bind` / 单向 / 双向绑定 / 线程 marshalling / 作用域失效。

| 子任务 | 状态 |
|---|---|
| 2.1 `State[T]` 原语（mutex + 订阅，跨 goroutine 安全） | ✅ `flux/state.go` |
| 2.2 单向绑定（`Text(Bind(state))` → 属性 patch） | ✅ |
| 2.3 双向绑定（`Input(Bind(state))` → OnChange 回写） | ✅ |
| 2.4 作用域失效（未变子树跳过，等价 D7c） | ✅ `TestStateScopeInvalidation` |
| 2.5 线程 marshalling（`RunOnUI` + pending 合并） | ✅ 5 goroutine 并发 Set `-race` 通过 |
| 2.6 Key 系统（D3 稳定 key，Phase 1 已落地） | ✅ |

**Phase 3（布局引擎，核心）✅ 完成**：
BoxConstraints 协议 + 单遍 RenderFlex（Expanded/Flexible、对齐、溢出诊断）+ GDI 文本测量 + resize 即时更新 + DPI 感知（DIP↔像素换算、WM_DPICHANGED 全量重排）+ 滚动容器（TScrollBox 原生滚动）+ inspector 数据源（节点 constraints/size/frame/flex）。

| 子任务 | 状态 |
|---|---|
| 3.1 布局协议（`BoxConstraints`/`Size`/对齐枚举，全 DIP） | ✅ `flux/box.go` |
| 3.2 文本测量（共享 bitmap canvas + `TextExtentWithStr` + 缓存） | ✅ 替换占位 `TextWidth` |
| 3.3 Flex 算法（freeSpace 分配、Expanded=Tight/Flexible=Loose、只增不缩+溢出诊断） | ✅ `flux/layout.go` + 11 项测试 |
| 3.4 定位应用 + Window resize 即时更新（零控件重建） | ✅ `App.LastLayoutDiags` 诊断钩子就绪 |
| 3.5 DPI（DIP↔像素换算、WM_DPICHANGED 钩子、测量归一化） | ✅ `internal/render/dip.go` + native 边界换算；demo 底部 DPI 读数 |
| 3.6 滚动容器（SingleChildScroll 语义、滚动轴 unbounded 测量、原生滚动条） | ✅ `flux.ScrollBox` + `layoutScrollBox`；demo 左面板滚动列表 |
| 3.7 布局调试（全节点 constraints/size/frame/flex 因子） | ✅ `App.Inspect()` + `NodeDiag` |

**Phase 4（事件系统与生命周期）✅ 完成**：统一事件（`Event{Type,X,Y,Key,Text,Button,Mods,Source}`）+ 鼠标/键盘映射（DIP 坐标归一）+ 生命周期（`OnMount/OnUpdate/OnUnmount`，卸载延后销毁）+ 中文输入（`OnUTF8KeyPress` 路由）。

| 子任务 | 状态 |
|---|---|
| 4.1 统一事件（`render.Event` + flux 别名；显式回调注册，禁反射） | ✅ `flux/event_opts.go` |
| 4.2 鼠标/键盘映射（native 边界归一 DIP；`Source="Type#Key"` 注入） | ✅ `internal/native` + diff 注入 |
| 4.3 生命周期（`OnMount/OnUpdate/OnUnmount`；D4 卸载入队延后销毁） | ✅ `internal/diff` + `App.DrainDestroy` |
| 4.4 IME/中文输入（`OnUTF8KeyPress` 逐字符路由，含 IME 组合结果） | ✅ 控件级 `SetOnUTF8KeyPress` |
| 4.5 无头测试（统一事件/映射 DIP/生命周期/Source 注入） | ✅ `flux/event_test.go` + `internal/native/mapping_test.go` |

**Phase 5（高级特性）✅ 完成**：动画（Curve/Tween/`AnimationController` 状态机 + `App.Animate` 主线程 16ms pump + `App.SetBounds` 逃逸口直接落地，绕开整树 re-diff）+ 主题（`Theme` 调色板 + `Color`/`FontColor` Opt + 标题栏沉浸式暗色 `DarkTitleBar`，切换 = 全量 re-diff 只 patch 变化颜色）+ Async（后台 goroutine + `RunOnUI` marshalling）+ 组件化（`Component(build, Key)` 透明分组，身份靠外部 Key 稳定）。

> **已知限制（win32 后端，探针实测）**：TButton 由 OS 主题绘制，`Color`/`FontColor` 均不渲染；TLabel 背景 `Color` 也不渲染。主题切换的可见信号实际来自窗体背景（`Window(Color(...))`）、文字 `FontColor` 与标题栏（`DarkTitleBar` → win32 DWM 沉浸式暗色，随主题亮/暗）。为让按钮支持主题色，需 owner-draw 改造（见 [design.md](docs/design.md)）。

| 子任务 | 状态 |
|---|---|
| 5.1 动画（Curve/EaseIn/Out/InOut/ElasticOut、Tween、Controller.Step、App.Animate pump、App.SetBounds D2 逃逸口） | ✅ `flux/animation.go` + `App.Animate/SetBounds` |
| 5.2 主题（`Theme{Primary,Background,Surface,Text,Accent,DarkTitleBar,FontSize,Radius}`、Light/Dark、`Color`/`FontColor` Opt + `DarkTitleBar` 标题栏暗色 + diff 属性级 patch） | ✅ `flux/theme.go`；FontSize/Radius 为文档字段（native 未接入） |
| 5.3 Async（`Async[T](app, load, onSuccess, onError…)`：后台 goroutine + RunOnUI marshal，D4） | ✅ 包级泛型函数（Go 方法不支持泛型） |
| 5.4 Component（`Build() Widget` 透明分组；组件身份靠外部 Key（D3），不在 Build 内生成 key/嵌套类型） | ✅ `flux.Component` + diff/layout Component 分支 |
| 5.5 无头测试（曲线端点、Tween、Controller 状态机、Animate pump 驱动、SetBounds 命中/跳过、主题零 mutation、组件 key 复用、Async 成败两径、ARGB→TColor 换算） | ✅ `flux/phase5_test.go` + `internal/native/mapping_test.go` |

**Phase 6（列表与虚拟化）✅ 完成**：`ListView(count, itemHeight, builder, ScrollOffset(scroll))` 虚拟滚动列表 —— **控件池虚拟化**（10 万行只建可见区±overscan 的 ~20 个原生控件，内存有界；滚动 = 行内容属性 patch 不重建，行内控件焦点/IME 不漂移）+ 稳定 slot key（`row-i` 槽位身份，D3）+ 滚动双向绑定（`scrollTarget` 值类型，D7c 零 mutation；滚轮/滚动条拖动 → State → re-render）+ 多窗口（第二个 `NewRenderer`/`NewApp` + `Show()`，独立 State 作用域）。

**控件扩充批次 1（常用表单基线）✅ 完成**：`Memo`、`CheckBox`、`ComboBox`、`ProgressBar`、`RadioButton` 均已完成公开 API、布局、diff 属性对称性、Mock、LCL 适配及聚合示例。`ComboBox` 采用 `[]string` + 显式受控 `SelectedIndex`；`ProgressBar` 规范化 `Minimum ≤ Maximum` 并把 `Value` 钳制到该范围；`RadioButton` 由 native Renderer 按 resolved native parent + `GroupIndex` 维护逻辑互斥，规避 energye/lcl v1.0.3 缺少分组 setter 的限制。

**P7.1 Inspector ✅ 完成**：`App.ObserveInspector` + `InspectorSnapshot` 提供只读提交/事件与 Widget → Element → native 三层快照，覆盖 Props、DIP 布局/溢出、create/destroy/reparent/property/event/bounds 统计；type/key canUpdate 失败会标记重建与焦点风险。`inspector.Open(app)` 使用独立工具窗，刷新/关闭不触发目标 App render；`App.SetBounds` 直接动画也作为 direct bounds commit 可见。

**P7.2 插件系统 ✅ 完成**：`RegisterWidget` / `PluginWidget` 提供进程内组合式插件 SDK；第三方 builder 只返回公开 Widget 子树，不接触 `internal/*` 或 LCL。注册表并发安全，支持类型化属性、DIP `Measure`、App 级 Init/Close、实例 Mount/Update/Unmount、具名可选 Renderer capability，以及重复/未知/在用注销、初始化失败和 panic 的可判定错误边界。`examples/plugin-badge/badge` 是只导入根包的第三方 Badge，未修改 native Create switch。

**P7.2c 分页容器 ✅ 完成**：公开 `PageControl` + `TabPage`（不虚构不存在的 `TabControl`），支持稳定页面 Key、受控 `SelectedIndex`、`OnSelectionChange`、每页独立 native parent、inactive 页面保活和 keyed 重排零重建。布局以 `8×32 DIP` 预算扣除页签边框/表头后填充页面客户区；`examples/page-control` 的 Windows smoke 连续切换并重排页面，校验 PageControl、TabSheet parent 与 Edit HWND 不变，同时保存经像素检查的目标窗口截图。Win32/LCL 的页签实际像素仍由 widgetset 主题/DPI 决定。

**P7.3 测试与 CI 门 🟨 本地矩阵完成，Hosted 通过待证**：21 个内建公开控件进入统一 inventory、mount、原地 patch 与 D7c 基线，可配置 native 控件覆盖属性移除/事件解绑，具交互语义控件覆盖 State 回写；`PluginWidget` 的 D7/生命周期由插件测试独立覆盖。CI 已配置 Go 1.22.x–1.26.x 与 1.27.0-rc.3、vet/race、真实 DLL native probe，以及每个公开示例的 Windows build/smoke 和像素有效截图上传。7.3b 的批次 3、全部 7GUIs 和 Accessibility/i18n 验收也已进入矩阵，但当前工作树尚未获得完整 Hosted CI 全绿证据，不能标为 CI 门完成。控件挂载、纯属性 patch、Page 切换和十万行虚拟列表基准见 [性能基线](docs/performance-baseline.md)。

**P7.4 Windows 打包 🟨 实现完成，发布门待验证**：NSIS 3.11 生成 per-user 安装包，包含示例 EXE、严格匹配的 libenergy DLL、项目/Go/已确认第三方许可证、依赖来源锁、开始菜单入口和卸载器。构建会联检 `energye/lcl` module、designer commit/archive 与 DLL SHA-256，并从最终 EXE 反向验证 PerMonitorV2、Common Controls v6 和完整版本资源；`package-installer` job 已配置在全新 Windows VM 执行安装、交互启动、重装和无残留卸载。该 job 的 Hosted CI 成功证据仍待取得，且 opaque DLL 的完整静态组件许可清单仍待上游构建清单或精确源码审计，因此 7.4 暂不标全绿。v0.1.0 采用 exe + DLL，细节见 [Windows 打包文档](docs/packaging.md)。

**P7.5 产品化与控件扩充批次 3 ✅ 完成**：公开内建控件契约已扩展到 21 个，新增受控水平 `Slider`、native `StringGrid` 与命令值驱动的 `PaintBox`。传入 `StringGrid` 的 `Cells` 必须是严格矩形，且行列数与构造器声明精确匹配，不补齐、不截断。Counter、Temperature Converter、Flight Booker、Timer、CRUD、Circle Drawer、Cells 七个 7GUIs 目标均有独立可运行示例、业务专用 smoke 和真实窗口截图；当前仓库共 17 个公开示例。当前 `Version` 仍为 `0.1.0-dev`，但 v0.1.0 候选公开面已冻结为首发基线；正式 SemVer 承诺从版本标签开始。P7.4 的 clean-VM/DLL 许可门仍独立以 [发布检查表](RELEASE_CHECKLIST.md) 为准。

**P7.6 Accessibility / i18n 🟨 本地候选完成，Hosted CI 待证**：公开 `AccessibleName/Description/Value`、`TabStop`、`DefaultButton`、`CancelButton` 与自动声明树 Tab 顺序；RadioButton 补齐逻辑分组方向键导航。默认后端在高对比度下回落系统色并响应设置/主题/系统颜色消息。`Catalog` + `Resources` + `Catalog.Bind` 支持 fallback、响应式 locale 切换，框架公开校验诊断使用稳定 Message ID 和可替换中英文资源。`examples/accessibility-i18n` 已在本地覆盖真实键盘、焦点、UIA、高对比度以及中英切换不重建验收；完整 Windows Hosted CI 成功前，不把它表述为发布门完成。等待 provider 稳定后，桌面根查询与 `AutomationElement.FromHandle` 都可通过 Win32 代理获得 Button/Edit/Combo/Slider 的标准 Pattern；锁定的 LCL runtime 仍未投射 Accessible 覆盖值，StringGrid 也无 Grid Pattern，完整边界见 [能力表](docs/accessibility-i18n.md)。

**CD1 Draw Core ✅ 完成（无头）**：新增不可变 `DrawList`、封闭 `DrawOp`、基础 2D 图元、字体/文本值、结构化校验、legacy PaintCommand adapter，以及 DrawController Mock/diff 的 mount、patch、remove、D7c 契约。当前阶段只证明纯值与生命周期，不声称基础图元已由 LCL 真实绘制；native executor、DPI 与像素探针属于 CD4。实施边界见 [CD1 记录](docs/cd1-draw-core.md)。

**CD2 字体与布局原语 ✅ 完成（无头 + native cache）**：纯值样式与 presence mask、styled text
measurement/fallback、FontSpec/DPI 缓存失效、Button/Text/Input intrinsic、Row/Column Gap 与
Padding 原语已落地。真实 DrawText 像素一致性仍属于 CD4；实施边界见 [CD2 记录](docs/cd2-font-layout.md)。

| 子任务 | 状态 |
|---|---|
| 6.1 ListView + 稳定 key（`ListView(count, itemH, builder)` + slot key=`row-i` 控件池复用；行内容不带数据 key） | ✅ `flux.ListView` + diff/layout `ListViewRow` 透明分支 |
| 6.2 虚拟化（10 万行：可见区±overscan 控件池；滚动=属性 patch；`render.Scrollable` D6 窄接口 + native TScrollBox 视口 + 内部 TScrollBar） | ✅ `layoutListView` + `internal/native` ListView 分支 + `internal/render/scroll.go` |
| 6.3 多窗口（第二个 Window；独立 State 作用域；`r2.Show()` 次要窗体显式显示） | ✅ `native.Renderer.Show()` + `examples/virtual-list` |
| 6.4 无头测试（虚拟化控件数有界、滚动零重建、滚动事件回写、钳制、D7c 零 mutation、无界约束 panic、行局部坐标） | ✅ `flux/list_test.go` |

> **已知限制**：ListView 必须有界约束（放 `Expanded` 或固定高度容器内；直接放 `Column` 未给高度会 panic，勿静默退化）；行内容不得带数据依赖 key（否则滚动换内容时重建，破坏控件池）；行 builder 里读取的 State（如选中标记）必须 `Bind` 出来才响应 `Set`（State 须订阅才触发 re-render，[design §9](docs/design.md)）；`TLabel` caption 变化在 DoubleBuffered 视口内可能滞留旧文本，native 已加 `Invalidate()` 保险（[design §16](docs/design.md)）。

## 快速开始

**前提**：Go 1.22+；`libenergy-amd64.dll`（获取方式见 [E2 文档](docs/phase0-e2-libenergy-mapping.md)，
可通过环境变量 `FVCL_LIBENERGY_DLL` 指定路径）。

```go
package main

import (
    "os"
    "path/filepath"

    flux "github.com/xiaowumin-mark/flux-vcl"
    "github.com/xiaowumin-mark/flux-vcl/native"
)

func main() {
    exe, err := os.Executable()
    if err != nil { panic(err) }
    if err := native.Init(filepath.Join(filepath.Dir(exe), "libenergy-amd64.dll")); err != nil {
        panic(err)
    }
    app := flux.NewApp(native.NewRenderer())
    count := flux.NewState(0)
    if err := app.Mount(func() flux.Widget {
        return flux.Window(flux.Column(
            flux.Text("Count"),
            flux.Button(flux.Bind(count), flux.OnClick(func(flux.Event) {
                count.Set(count.Get() + 1)
            })),
        ))
    }); err != nil { panic(err) }
    native.Run()
}
```

```powershell
# 构建（生成资源 -> windowsgui exe -> 复制 DLL）
.\scripts\build.ps1

# 无头冒烟（验证窗口出现、按钮点击生效、干净退出）
.\scripts\smoke.ps1

# 生成并验证当前 -dev 候选安装包（需 NSIS 3.11；不可公开分发）
.\scripts\package.ps1 -AllowDevVersion
.\scripts\test-installer.ps1 -InstallerPath .\bin\FluxVCL-0.1.0-basic-setup.exe -AllowDevVersion

# 全量质量门与性能样本（性能只记录趋势，不设脆弱绝对阈值）
go test -race ./...
go vet ./...
go test . -run '^$' -bench Benchmark -benchmem

# 或手动运行
.\bin\basic.exe
```

完整可运行示例：
- `examples/basic` —— State 驱动最小用例（counter + two-way 绑定）
- `examples/layout` —— 布局引擎 demo（flex 分配、1:2 分栏、resize 即时重分割、DPI 读数）
- `examples/events` —— 事件与生命周期 demo（hover 坐标 / click Source / 键盘 / 中文 IME / 生命周期计数）
- `examples/phase5` —— 高级特性 demo（点击按钮：计数 + 方块滑动动画 + 异步加载；点击"主题"切换 Light/Dark）
- `examples/form-controls` —— 常用表单控件 demo（Memo/CheckBox/ComboBox/ProgressBar/RadioButton；唯一数字 Button 供 smoke）
- `examples/virtual-list` —— 大数据 demo（10 万行虚拟滚动列表：控件池 + 稳定 key + 滚动双向绑定 + 第二窗体多窗口）
- `examples/inspector` —— P7.1 Inspector demo（三层树、Props/布局、事件/mutation 时间线、重建风险）
- `examples/plugin-badge` —— P7.2 第三方 Badge 插件（公开 SDK、类型化属性、布局/生命周期、零 native switch 改动）
- `examples/page-control` —— P7.2c 多页容器（稳定 Key、受控切页、页内输入子树与 native parent）
- `examples/accessibility-i18n` —— P7.6 键盘、UIA、高对比度与中英文原地切换验收
- `examples/7guis-counter` —— Counter（State、Text、Button）
- `examples/7guis-temperature-converter` —— Temperature Converter（受控 Input、双向数值转换、非法输入）
- `examples/7guis-flight-booker` —— Flight Booker（ComboBox、日期校验、受控 Enabled）
- `examples/7guis-timer` —— Timer（主线程 Animate pump、Slider、ProgressBar）
- `examples/7guis-crud` —— CRUD（StringGrid 选择/编辑、过滤、稳定业务 ID）
- `examples/7guis-circle-drawer` —— Circle Drawer（PaintBox、DIP 命中、撤销/重做）
- `examples/7guis-cells` —— Cells（StringGrid、公式解析与依赖图）

```powershell
# 构建并冒烟 basic（State）
.\scripts\build.ps1 -Target basic; .\scripts\smoke.ps1 -Target basic

# 构建并冒烟 layout（布局引擎）
.\scripts\build.ps1 -Target layout; .\scripts\smoke.ps1 -Target layout

# 构建并冒烟 events（事件与生命周期）
.\scripts\build.ps1 -Target events; .\scripts\smoke.ps1 -Target events

# 构建并冒烟 phase5（动画/主题/Async/组件）
.\scripts\build.ps1 -Target phase5; .\scripts\smoke.ps1 -Target phase5

# 构建并冒烟 form-controls（Memo / CheckBox / ComboBox / ProgressBar / RadioButton）
.\scripts\build.ps1 -Target form-controls; .\scripts\smoke.ps1 -Target form-controls

# 构建并冒烟 virtual-list（10 万行虚拟列表 + 多窗口）
.\scripts\build.ps1 -Target virtual-list; .\scripts\smoke.ps1 -Target virtual-list

# 构建并冒烟 inspector（三层树 + mutation/event + 重建风险）
.\scripts\build.ps1 -Target inspector; .\scripts\smoke.ps1 -Target inspector

# 构建并冒烟 plugin-badge（第三方组合式插件）
.\scripts\build.ps1 -Target plugin-badge; .\scripts\smoke.ps1 -Target plugin-badge

# 构建并冒烟 page-control（P7.2c 多页容器）
.\scripts\build.ps1 -Target page-control; .\scripts\smoke.ps1 -Target page-control

# 依次构建并冒烟全部 7GUIs
$targets = @(
  "7guis-counter", "7guis-temperature-converter", "7guis-flight-booker",
  "7guis-timer", "7guis-crud", "7guis-circle-drawer", "7guis-cells"
)
foreach ($target in $targets) {
  .\scripts\build.ps1 -Target $target
  .\scripts\smoke.ps1 -Target $target
}
```

插件最小用法：

```go
err := flux.RegisterWidget("example.badge", flux.WidgetPlugin{
    Build: func(ctx flux.PluginBuildContext) (flux.Widget, error) {
        label, _ := ctx.Properties.String("label")
        return flux.Text("[" + label + "]"), nil
    },
})
if err != nil { return err }

badge := flux.PluginWidget("example.badge", flux.NewPluginProperties(
    flux.PluginString("label", "Ready"),
), flux.Key("status"))
```

插件是已链接进进程的 Go 代码注册，不是 DLL/Go `plugin` 动态加载。App 使用插件时必须在窗口关闭前调用 `App.Close()`；不要在 build/render 或插件实例 Mount/Update/Unmount 回调内同步关闭，否则返回 `ErrAppCloseDuringRender`，应在回调返回后的外层关闭流程中调用。活跃 App 存在时 `UnregisterWidget` 返回 `ErrPluginInUse`。

## 目录结构

```
flux-vcl/
├── flux.go                # 框架主包：声明式 API（构造器/Opt/App/逃逸口）
├── state.go               # State[T] / Bind / Binding（响应式状态 + 数据绑定，Phase 2）
├── event.go               # 统一事件 Event{Type,X,Y,Key,Text,Button,Mods,Source}（Phase 4.1）
├── event_opts.go          # 事件/生命周期 Opt：OnClick/OnMouse*/OnKey*/OnMount/OnUpdate/OnUnmount
├── animation.go           # Curve/Tween/AnimationController 动画状态机（Phase 5.1）
├── theme.go               # Theme 调色板 + Color/FontColor Opt（Phase 5.2）
├── list.go                # ListView 虚拟滚动列表 + ScrollOffset（Phase 6）
├── slider.go              # 批次 3 受控水平 Slider
├── stringgrid.go          # 批次 3 TStringGrid 严格字符串矩阵
├── paintbox.go            # 批次 3 PaintBox 稳定绘制命令
├── box.go                 # 布局协议：BoxConstraints/Size/Point/对齐枚举（Phase 3.1）
├── layout.go              # 单遍 RenderFlex 布局 + ScrollBox 滚动 + 虚拟列表布局 + NodeDiag（Phase 3/6）
├── inspector.go           # P7.1 只读 observer、提交/事件、三层树快照与有界历史
├── plugin.go              # P7.2 插件注册表、公开 SDK、builder/布局/生命周期/能力
├── controls.go            # 基础/表单/分页控件构造器
├── native/                # 公开默认后端入口：Init/NewRenderer/Run
├── internal/
│   ├── widget/            # Widget 接口 + Node + Props（有序属性集，D2 diff）
│   ├── diff/              # Element 树 + diff/reconciliation 引擎（Phase 1.4）
│   ├── render/            # Renderer 窄接口 + Mutation op 集 + DIP 换算 + Mock
│   └── native/            # 默认 LCL 后端适配（energye/lcl + libenergy DLL）
├── inspector/             # 独立只读 Inspector LCL 工具窗
├── examples/
│   ├── basic/             # State 驱动冒烟应用（counter + two-way 绑定）
│   │   └── winres/        # go-winres 资源配置（manifest/icon/version）
│   ├── layout/            # 布局引擎 demo（flex 分栏 + resize 重分割 + 滚动列表）
│   │   └── winres/
│   ├── events/            # 事件与生命周期 demo（hover/click/键盘/中文 IME/生命周期计数）
│   │   └── winres/
│   ├── phase5/            # 高级特性 demo（动画/主题/Async/组件）
│   │   └── winres/
│   ├── form-controls/     # 常用表单控件 demo（批次 1）
│   │   └── winres/
│   ├── virtual-list/      # 大数据 demo（10 万行虚拟列表 + 多窗口）
│   │   └── winres/
│   ├── inspector/         # P7.1 三层树/mutation/event/rebuild demo
│   │   └── winres/
│   ├── plugin-badge/      # P7.2 第三方 Badge（badge 子包只依赖公开 flux）
│   ├── page-control/      # P7.2c PageControl/TabPage 多页容器
│   ├── accessibility-i18n/ # P7.6 键盘/UIA/高对比度/i18n 验收
│   ├── 7guis-counter/     # Counter
│   ├── 7guis-temperature-converter/
│   ├── 7guis-flight-booker/
│   ├── 7guis-timer/       # Slider + ProgressBar + 主线程动画 pump
│   ├── 7guis-crud/        # StringGrid CRUD
│   ├── 7guis-circle-drawer/ # PaintBox 圆形绘制
│   └── 7guis-cells/       # StringGrid 公式表格
├── scripts/
│   ├── build.ps1          # 构建脚手架 + module/DLL 来源联检
│   ├── smoke.ps1          # 原生交互冒烟
│   ├── package.ps1        # NSIS 安装包
│   └── test-installer.ps1 # 安装/启动/卸载闭环
├── packaging/             # NSIS、依赖锁与第三方通知
├── docs/                  # 设计/计划/API/7GUIs/迁移/维护/能力证据
├── README.en.md           # 英文产品入口
├── CHANGELOG.md           # 版本变更记录
├── RELEASE_CHECKLIST.md   # 发布门禁清单
└── assets/                # 图标等资源
```

## 文档

- [贡献指南](CONTRIBUTING.md) —— 工作流、提交信息、分支/PR 规范
- [开发规范](docs/development-guide.md) —— 代码风格、架构不变量 D1–D7、测试/文档规范
- [命名规范](docs/naming-conventions.md) —— example/包/标识符/资源命名
- [设计文档](docs/design.md) —— 架构、三棵树模型、布局/State/事件设计
- [开发计划](docs/development-plan.md) —— Phase 0–7 任务与验收标准
- [自绘与样式系统设计](docs/custom-draw-style-system.md) —— Draw API、主题包边界与 CD0–CD8 实施计划
- [CD1 Draw Core 实施记录](docs/cd1-draw-core.md) —— 纯值 API、校验、Mock/diff 与阶段边界
- [CD2 字体与布局原语实施记录](docs/cd2-font-layout.md) —— 样式值、测量 fallback、缓存与布局原语
- [English README](README.en.md) —— 英文产品入口
- [7GUIs 任务映射](docs/7guis.md) —— 七项任务、公开 API 与业务边界
- [v0.1.0 候选 API 冻结清单](docs/api-v0.1.0.md) —— 首发公开标识符与兼容边界
- [迁移指南](docs/migration.md) —— 预 1.0 与批次 3 迁移注意事项
- [维护政策](docs/maintenance.md) —— 支持范围、兼容与安全响应
- [可验证能力对比](docs/capability-comparison.md) —— 十项能力（含 Accessibility/i18n）、仓库证据与限制
- [CHANGELOG](CHANGELOG.md) —— v0.1.0 变更摘要
- [发布检查表](RELEASE_CHECKLIST.md) —— clean-VM、许可与发布前门禁
- [底座选型调研](docs/govcl-vs-lcl.md) —— LCL vs VCL 选型依据
- [libenergy DLL 映射](docs/phase0-e2-libenergy-mapping.md) —— 版本↔DLL 锁定关系
- [Windows 打包](docs/packaging.md) —— NSIS、依赖锁、资源门禁、安装验证与单 EXE 评估

## 许可证

[MIT](LICENSE)
