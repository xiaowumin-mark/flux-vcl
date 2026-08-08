# FluxVCL

基于 Go 的现代声明式 UI 框架，使用原生 Windows 控件。

提供类似 Flutter / Vue / SwiftUI 的开发体验，同时保留桌面原生控件能力与底层可访问性。

```go
Window(
    Column(
        Text("Hello"),

        Button(
            "OK",
            OnClick(func() {
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

## 快速开始

**前提**：Go 1.22+；`libenergy-amd64.dll`（获取方式见 [E2 文档](docs/phase0-e2-libenergy-mapping.md)，
可通过环境变量 `FVCL_LIBENERGY_DLL` 指定路径）。

```go
// 声明式 UI：UI 即结构，状态变化重建树，diff 引擎只 patch 变化的属性
// （r 为绑定层 renderer，由 internal/native 适配，见 examples/basic/main.go）
app := flux.NewApp(r)

var count int
build := func() flux.Widget {
    return flux.Window(
        flux.Column(
            flux.Text("Count: "+strconv.Itoa(count)),
            flux.Button("+1", flux.OnClick(func() {
                count++
                app.Render(build()) // 零控件重建，只更新文本
            })),
        ),
    )
}
app.Render(build())
```

```powershell
# 构建（生成资源 -> windowsgui exe -> 复制 DLL）
.\scripts\build.ps1

# 无头冒烟（验证窗口出现、按钮点击生效、干净退出）
.\scripts\smoke.ps1

# 或手动运行
.\bin\basic.exe
```

完整可运行示例见 `examples/basic`。

## 快速开始

**前提**：Go 1.22+；`libenergy-amd64.dll`（获取方式见 [E2 文档](docs/phase0-e2-libenergy-mapping.md)，
可通过环境变量 `FVCL_LIBENERGY_DLL` 指定路径）。

```powershell
# 构建（生成资源 -> windowsgui exe -> 复制 DLL）
.\scripts\build.ps1

# 无头冒烟（验证窗口出现、按钮点击生效、干净退出）
.\scripts\smoke.ps1

# 或手动运行
.\bin\basic.exe
```

## 目录结构

```
flux-vcl/
├── flux.go                # 框架主包：声明式 API（构造器/Opt/App/逃逸口）
├── internal/
│   ├── widget/            # Widget 接口 + Node + Props（有序属性集，D2 diff）
│   ├── diff/              # Element 树 + diff/reconciliation 引擎（Phase 1.4）
│   ├── render/            # Renderer 窄接口 + Mutation op 集 + Mock
│   └── native/            # 默认 LCL 后端适配（energye/lcl + libenergy DLL）
├── examples/
│   └── basic/             # 声明式冒烟应用（窗体+文本+按钮+输入框+点击）
│       └── winres/        # go-winres 资源配置（manifest/icon/version）
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
