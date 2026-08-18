# FluxVCL 开发规范

> 版本：0.1 ｜ 日期：2026-08-09
> 配套文档：贡献规范（根目录 [CONTRIBUTING.md](../CONTRIBUTING.md)）、
> [命名规范](./naming-conventions.md)、[设计文档](./design.md)、[开发计划](./development-plan.md)。

## 目录

1. [适用与目标](#1-适用与目标)
2. [环境与工具链](#2-环境与工具链)
3. [目录与包职责](#3-目录与包职责)
4. [Go 代码风格](#4-go-代码风格)
5. [架构不变量 D1–D7](#5-架构不变量-d1d7)
6. [测试规范](#6-测试规范)
7. [文档规范](#7-文档规范)
8. [示例规范](#8-示例规范)
9. [提交前检查清单](#9-提交前检查清单)

---

## 1. 适用与目标

本文面向所有在 `github.com/xiaowumin-mark/flux-vcl` 上写代码 / 测试 / 文档的贡献者，目标是让仓库长期保持：

- **架构不变量**（D1–D7）不被无意破坏；
- **无头可测**：核心逻辑不依赖 LCL/DLL，任意平台 `go test ./...` 可跑；
- **声明式一致**：所有用户 API 遵循同一套构造器 / Opt / State 惯例；
- **文档同步**：行为变化必须反映到 design.md 与 README。

---

## 2. 环境与工具链

| 项 | 要求 | 说明 |
|---|---|---|
| Go | 1.22+ | `go.mod` 锁 `go 1.22`；CI 覆盖 1.22–1.26 与当前 1.27 RC |
| 绑定依赖 | `github.com/energye/lcl v1.0.3` | **版本必须与 `libenergy-amd64.dll` 严格一致**（见 `docs/phase0-e2-libenergy-mapping.md`），升级需同步替换 DLL 并记录 |
| 运行时 DLL | `libenergy-amd64.dll` | 构建脚本复制到 exe 旁；可用 `FVCL_LIBENERGY_DLL` 指定路径 |
| 资源生成 | go-winres | 生成 `rsrc_windows_amd64.syso`（manifest/icon/version） |
| 构建 / 冒烟 | `scripts/build.ps1` / `scripts/smoke.ps1` | 见 README「快速开始」 |

> 单元测试一律**不依赖** DLL 与显示；只有 `internal/native` 的适配层测试与冒烟脚本才碰真实后端。

---

## 3. 目录与包职责

| 路径 | 职责 | 依赖方向 |
|---|---|---|
| 根包 `flux` | 用户 API：构造器 / Opt / `App` / `State` / `Bind` / 动画 / 主题 / 逃逸口 | → internal |
| `internal/widget` | `Widget` 接口 + `Node` + `Props`（有序属性集，D2 diff 的输入） | 纯数据，无外部依赖 |
| `internal/diff` | Element 树 + diff/reconciliation 引擎（D1/D2/D3/D7） | → widget |
| `internal/render` | `Renderer` 窄接口 + mutation op 集 + DIP 换算 + Mock（D6） | 面向接口，无 LCL 依赖 |
| `internal/native` | 默认 LCL 后端适配 + 事件映射（D4/D6 落点） | → render |
| `inspector/` | P7.1 独立只读 Inspector 工具窗 | → 根包 |
| `examples/` | 可运行演示（见 [命名规范 §2](naming-conventions.md)） | → 根包 / inspector |
| `scripts/` | 构建 / 冒烟 / 取 DLL 脚本 | — |
| `docs/` | 设计 / 计划 / 调研 / 规范文档 | — |

- **依赖只进不出**：`internal/*` 不得反向 import 根包；用户代码只 import 根包，不得 import `internal/*`（Go internal 规则保证）。
- 新增模块（如多后端适配）须先在 development-plan 立项再落代码。

---

## 4. Go 代码风格

- **格式化与静态检查**：`gofmt` 必过；`go vet ./...` 无警告。命名遵循 Go 惯例（导出 CamelCase / 私有 camelCase），细节见 [命名规范](naming-conventions.md)。
- **文档注释**：所有导出标识符必须有**中文** doc comment。包注释范式见 `flux.go`：用途 → 编程模型 → 关键约束/链接（design.md / development-plan.md）。
- **泛型**：Go 方法不支持类型参数 → 需要泛型的顶层函数用**包级泛型函数**（参考 `flux.Async[T]`，`app.go`），并说明为什么不是方法。
- **错误处理**：库代码返回 error；示例的启动失败用 `fmt.Fprintln(os.Stderr, ...)` + `os.Exit(2)`（见各 `examples/*/main.go`）。
- **Panic 边界**：构造器对非法参数 `panic` 并给出明确信息（如 `flux.Window: 参数必须是 Widget 或 Opt`）。
- **注释语言**：代码内注释与提交、文档一致用中文；标识符用英文。
- **高内聚文件**：根包按特性拆小文件（`state.go`/`event.go`/`event_opts.go`/`animation.go`/`theme.go`/`box.go`/`layout.go`/`controls.go`/`plugin.go`），不要堆成一个大 `flux.go`。
- **插件边界**：第三方插件只能 import 公开 `flux` 包；builder 组合公开 Widget，不得 import `internal/*`、energye/lcl 或持有 Renderer/原生句柄。插件专属后端信息通过具名可选 capability 读取，缺失时必须安全退化（design §19）。

---

## 5. 架构不变量 D1–D7

> 摘自 [development-plan.md §0](./development-plan.md)。**任何代码不得违背**；偏离需单独评审并记录到 design.md。新功能落代码前先对照下表。

| # | 不变量 | 关键约束 | 违规信号（评审时查） |
|---|---|---|---|
| **D1** | 三棵树模型 + `canUpdate` | `同类型 && 同 key` → 原地 patch；否则只重建该节点，**绝不上溯重建祖先** | 事件里手动重建整树；容器在子变更时重建 |
| **D2** | 属性级 patch + 批量提交 | diff 只产生 mutation op 集，按"先删后建、先上后下"应用；高频热路径用逃逸口（`App.SetBounds`/`App.Animate`）**绕过整树 diff** | 高频属性（动画帧）走 re-diff；未变属性重复 Set |
| **D3** | 列表身份：稳定 key | key 必须来自模型 / 创建时生成一次；**绝不用数组 index、绝不每次 render 随机** | `Key(strconv.Itoa(i))`；`Key(fmt.Sprint(time.Now()))` |
| **D4** | 单一 UI 线程 + marshalling | 控件访问只在主线程；跨线程 State 变更落地经 `runOnUI`；**销毁入队延后**，绝不在事件回调内同步 Free；事件分发统一 `recover` 错误边界 | 事件回调内 `Free`；goroutine 直接碰控件；无 recover 的 handler |
| **D5** | 自定义布局，禁用原生 Align | constraints 下传 / size 上抛 / 父定 offset；框架控件 `Align=alNone`，几何只走 `SetBounds`；全坐标 DIP（`MulDiv(dip,dpi,96)`） | 新控件设了原生 Align；像素/DIP 混用 |
| **D6** | 绑定隔离 | 控件访问收敛到窄接口（`Create/SetBounds/...`），绑定库藏在适配层后；**事件显式注册回调，禁止反射方法名绑定** | 上层代码直接 import lcl 控件类型 |
| **D7** | 三条测试不变量 | (a) 纯属性变化绝不重建控件；(b) 稳定 key 重排不迁移焦点/IME；(c) 相同树 diff 零 mutation | 对应测试被删 / 不再断言 |

---

## 6. 测试规范

- **无头优先**：核心逻辑（widget/diff/render/flux 声明式层）用 `internal/render.Mock` 测试，不接触 energye/lcl/DLL，任意平台 `go test` 可跑（见 `renderer_test.go` 的 `TestMockHasNoDisplayDependency`）。
- **命名**：`Test<特征><场景>`，英文 PascalCase；架构不变量用 `TestD<N><...>` 前缀（如 `TestD7aPurePropertyChangeNoRebuild`）。
- **文件**：`<被测文件>_test.go`（如 `state_test.go`、`inspector_test.go`），跨特性收尾测试用功能域名（如 `phase5_test.go`、`scroll_inspect_test.go`）。
- **并发**：涉及 goroutine / 跨线程 marshalling 的测试必须用 `go test -race` 通过（如 Phase 2 的 5 goroutine 并发 Set）。
- **覆盖**：新功能必须带测试；修 bug 先加复现测试再修。
- **CI 红线**：Go 1.22–1.26 与当前 1.27rc3 的 `go test ./...`、1.27rc3 工具链
  `go vet ./...` 与
  `go test -race ./...` 全绿；DLL 到位且非 race 时 native probe 不得跳过；全部公开
  examples 由 Windows CI 独立构建/冒烟并上传经像素检查的非空截图。
- **性能基线**：基准必须 `-benchmem` 并记录环境、mutation 数和样本结果；耗时用于
  固定环境的趋势比较，不给共享 runner 设置绝对阈值。当前记录见
  [performance-baseline.md](performance-baseline.md)。

---

## 7. 文档规范

- `docs/` 与 README、代码注释**全中文**。
- **引用格式**：设计文档按章节引用，统一 `design.md §N.M` / `development-plan §N`（示例：`design §4.1`、`§5`）。引用时不写绝对章节标题以免文档演进后失配。
- **已知限制必须记录**：实测到的后端限制（如 win32 下 `TButton` 由 OS 主题绘制、`Color`/`FontColor` 不渲染）须记入 design.md 相关章节 + 提交信息，并在 README「已知限制」区同步。禁止"发现不说、下个阶段才想起来"。
- **README 同步**：新 API / 新示例 / 阶段状态表 / 目录结构变化 → 同步更新 README。
- **示例包注释**：`examples/*/main.go` 顶部用包注释写清：演示什么 → 引用的 design/plan 章节 → 工程约束（尤其 smoke）→ 构建命令（见 [命名规范 §2.3](naming-conventions.md)）。

---

## 8. 示例规范

示例是用户的第一眼，也是冒烟的载体。要点（完整规则见 [命名规范 §2](naming-conventions.md)）：

- 目录名用**语义英文小写**，表达"演示的能力"，优先领域语义而非阶段号。
- 每个示例**自包含**：`main.go` + `winres/winres.json`（+ 生成的 `.syso`）。
- **smoke 约束**：纳入 CI 冒烟的示例，窗口内 `Button` 必须**唯一**，点击信号可枚举断言（按钮文本变化）；非按钮交互用可点击 `Text`（`TLabel` 无 HWND，不干扰冒烟）。
- 触发新特性的示例要引用 design 章节，方便读者对照设计。

---

## 9. 提交前检查清单

- [ ] `gofmt` 通过，`go vet ./...` 无警告
- [ ] `go test ./...` 全绿（并发相关已 `-race`）
- [ ] 未违背 D1–D7（对照 §5 表自审）
- [ ] 导出 API 有中文 doc comment
- [ ] 行为变化已更新 design.md / README / 开发计划状态
- [ ] 新示例符合命名规范 §2（目录 / winres / smoke）
- [ ] 提交信息符合 [CONTRIBUTING §4](../CONTRIBUTING.md#4-提交信息规范)
