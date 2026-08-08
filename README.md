# FluxVCL

基于 Go 的现代声明式 UI 框架，使用原生 Windows 控件。

提供类似 Flutter / Vue / SwiftUI 的开发体验，同时保留桌面原生控件能力与底层可访问性。

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
- **LCL/VCL 双后端**：VCL（`govcl`）为 B 计划，通过绑定隔离可切换
- **现代布局**：自定义 Measure/Layout（Flutter 风格 constraints），禁用原生 Align
- **线程纪律**：单一 UI 线程 + marshalling 调度器

> 架构不变量（D1–D7）见 [docs/design.md](docs/design.md) 与 [docs/development-plan.md](docs/development-plan.md)。

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

**Phase 5（高级特性）✅ 完成**：动画（Curve/Tween/`AnimationController` 状态机 + `App.Animate` 主线程 16ms pump + `App.SetBounds` 逃逸口直接落地，绕开整树 re-diff）+ 主题（`Theme` 调色板 + `Color`/`FontColor` Opt，切换 = 全量 re-diff 只 patch 变化颜色）+ Async（后台 goroutine + `RunOnUI` marshalling）+ 组件化（`Component(build, Key)` 透明分组，身份靠外部 Key 稳定）。

| 子任务 | 状态 |
|---|---|
| 5.1 动画（Curve/EaseIn/Out/InOut/ElasticOut、Tween、Controller.Step、App.Animate pump、App.SetBounds D2 逃逸口） | ✅ `flux/animation.go` + `App.Animate/SetBounds` |
| 5.2 主题（`Theme{Font,Color,Radius,Animation}`、Light/Dark、`Color`/`FontColor` Opt + diff 属性级 patch） | ✅ `flux/theme.go`；FontSize/Radius 为文档字段（native 未接入） |
| 5.3 Async（`Async[T](app, load, onSuccess, onError…)`：后台 goroutine + RunOnUI marshal，D4） | ✅ 包级泛型函数（Go 方法不支持泛型） |
| 5.4 Component（`Build() Widget` 透明分组；组件身份靠外部 Key（D3），不在 Build 内生成 key/嵌套类型） | ✅ `flux.Component` + diff/layout Component 分支 |
| 5.5 无头测试（曲线端点、Tween、Controller 状态机、Animate pump 驱动、SetBounds 命中/跳过、主题零 mutation、组件 key 复用、Async 成败两径、ARGB→TColor 换算） | ✅ `flux/phase5_test.go` + `internal/native/mapping_test.go` |

## 快速开始

**前提**：Go 1.22+；`libenergy-amd64.dll`（获取方式见 [E2 文档](docs/phase0-e2-libenergy-mapping.md)，
可通过环境变量 `FVCL_LIBENERGY_DLL` 指定路径）。

```go
// 状态驱动 UI：State 变化自动 re-render，diff 引擎只 patch 变化的属性
// （r 为绑定层 renderer，由 internal/native 适配，见 examples/basic/main.go）
app := flux.NewApp(r)

count := flux.NewState(0)  // 单向：按钮文本随 State 刷新
name := flux.NewState("")  // 双向：输入框 ↔ State 同步
app.Mount(func() flux.Widget {
    return flux.Window(
        flux.Column(
            flux.Text("Count: "+fmt.Sprint(count.Get())),
            flux.Button(flux.Bind(count), flux.OnClick(func(_ flux.Event) {
                count.Set(count.Get() + 1) // 外部修改 State → 自动 re-render
            })),
            flux.Input(flux.Bind(name)),
            flux.Text(flux.Bind(name)), // 回显
        ),
    )
})
```

```powershell
# 构建（生成资源 -> windowsgui exe -> 复制 DLL）
.\scripts\build.ps1

# 无头冒烟（验证窗口出现、按钮点击生效、干净退出）
.\scripts\smoke.ps1

# 或手动运行
.\bin\basic.exe
```

完整可运行示例：
- `examples/basic` —— State 驱动最小用例（counter + two-way 绑定）
- `examples/layout` —— 布局引擎 demo（flex 分配、1:2 分栏、resize 即时重分割、DPI 读数）
- `examples/events` —— 事件与生命周期 demo（hover 坐标 / click Source / 键盘 / 中文 IME / 生命周期计数）
- `examples/phase5` —— 高级特性 demo（点击按钮：计数 + 方块滑动动画 + 异步加载；点击"主题"切换 Light/Dark）

```powershell
# 构建并冒烟 basic（State）
.\scripts\build.ps1 -Target basic; .\scripts\smoke.ps1 -Target basic

# 构建并冒烟 layout（布局引擎）
.\scripts\build.ps1 -Target layout; .\scripts\smoke.ps1 -Target layout

# 构建并冒烟 events（事件与生命周期）
.\scripts\build.ps1 -Target events; .\scripts\smoke.ps1 -Target events

# 构建并冒烟 phase5（动画/主题/Async/组件）
.\scripts\build.ps1 -Target phase5; .\scripts\smoke.ps1 -Target phase5
```

## 目录结构

```
flux-vcl/
├── flux.go                # 框架主包：声明式 API（构造器/Opt/App/逃逸口）
├── state.go               # State[T] / Bind / Binding（响应式状态 + 数据绑定，Phase 2）
├── event.go               # 统一事件 Event{Type,X,Y,Key,Text,Button,Mods,Source}（Phase 4.1）
├── event_opts.go          # 事件/生命周期 Opt：OnClick/OnMouse*/OnKey*/OnMount/OnUpdate/OnUnmount
├── animation.go           # Curve/Tween/AnimationController 动画状态机（Phase 5.1）
├── theme.go               # Theme 调色板 + Color/FontColor Opt（Phase 5.2）
├── box.go                 # 布局协议：BoxConstraints/Size/Point/对齐枚举（Phase 3.1）
├── layout.go              # 单遍 RenderFlex 布局 + ScrollBox 滚动 + NodeDiag 诊断（Phase 3）
├── controls.go            # 控件构造器：Window/Column/Row/ScrollBox/Component/Text/Button/Input
├── internal/
│   ├── widget/            # Widget 接口 + Node + Props（有序属性集，D2 diff）
│   ├── diff/              # Element 树 + diff/reconciliation 引擎（Phase 1.4）
│   ├── render/            # Renderer 窄接口 + Mutation op 集 + DIP 换算 + Mock
│   └── native/            # 默认 LCL 后端适配（energye/lcl + libenergy DLL）
├── examples/
│   ├── basic/             # State 驱动冒烟应用（counter + two-way 绑定）
│   │   └── winres/        # go-winres 资源配置（manifest/icon/version）
│   ├── layout/            # 布局引擎 demo（flex 分栏 + resize 重分割 + 滚动列表）
│   │   └── winres/
│   ├── events/            # 事件与生命周期 demo（hover/click/键盘/中文 IME/生命周期计数）
│   │   └── winres/
│   └── phase5/            # 高级特性 demo（动画/主题/Async/组件）
│       └── winres/
├── scripts/
│   ├── build.ps1          # 构建脚手架
│   └── smoke.ps1          # 无头冒烟
├── docs/                  # 设计/计划/调研/实验文档
└── assets/                # 图标等资源
```

## 文档

- [设计文档](docs/design.md) —— 架构、三棵树模型、布局/State/事件设计
- [开发计划](docs/development-plan.md) —— Phase 0–7 任务与验收标准
- [底座选型调研](docs/govcl-vs-lcl.md) —— LCL vs VCL 选型依据
- [libenergy DLL 映射](docs/phase0-e2-libenergy-mapping.md) —— 版本↔DLL 锁定关系

## 许可证

[MIT](LICENSE)
