# FluxVCL 命名规范

> 版本：0.1 ｜ 日期：2026-08-09
> 配套文档：[贡献规范](../CONTRIBUTING.md)（提交信息 / 分支）、[开发规范](./development-guide.md)（风格 / 测试 / 文档）。

## 目录

1. [通用原则](#1-通用原则)
2. [Example 命名](#2-example-命名)
3. [包与文件命名](#3-包与文件命名)
4. [标识符命名](#4-标识符命名)
5. [资源命名](#5-资源命名)
6. [提交信息词汇表](#6-提交信息词汇表)
7. [分支命名](#7-分支命名)

---

## 1. 通用原则

- **标识符英文，注释 / 文档 / 提交中文**。
- 遵循 Go 惯例：导出 CamelCase（`Window`、`NewState`），私有 camelCase（`renderWidget`）。
- 命名**语义化、简短**，与既有 API 保持一致；不另起风格、不缩写到不可读。
- 面向用户的概念（Widget / Opt / State）命名要在 `flux` 根包统一暴露，内部实现藏于 `internal/`。

---

## 2. Example 命名

### 2.1 目录命名

规则：`examples/<语义英文小写名>`，表达"**演示了哪种能力 / 主题**"，优先领域语义而非阶段号。

| 现有示例 | 命名依据 |
|---|---|
| `examples/basic` | State 驱动最小用例（counter + 双向绑定） |
| `examples/layout` | 布局引擎 demo（flex 分栏、滚动、DPI 读数） |
| `examples/events` | 事件与生命周期 demo |
| `examples/phase5` | **历史遗留**：阶段推进期的多特性合屏（动画/主题/Async/组件） |
| `examples/form-controls` | 常用表单控件基线（Memo/CheckBox/ComboBox/ProgressBar/RadioButton） |
| `examples/virtual-list` | 大数据：10 万行虚拟列表（控件池虚拟化 + 滚动双向绑定 + 多窗口） |
| `examples/inspector` | P7.1 三层树、mutation、事件、布局与重建风险查看。 |

- **新增示例不再用 `phaseN`**；若需多特性合屏，按主特性命名。
- 推荐名（按能力）：`animation` / `theme` / `async` / `component` / `scroll` / `dpi` / `virtual-list` / `multi-backend`…（`inspector` 已使用）
- `virtual-list` 为 `list` 子系统的主示例（控件池虚拟化、滚动双向绑定、多窗口三合一）。
- **不随意重命名 / 删除既有目录**：`scripts/*.ps1 -Target` 与 CI 的引用会失效；确需改名须同步改脚本与文档。

### 2.2 目录结构（自包含）

```
examples/<name>/
├── main.go                # package main；头注释见 2.3
├── winres/winres.json     # go-winres 资源配置（见 2.4）
└── rsrc_windows_amd64.syso  # 生成的资源（*.syso 已被 gitignore，不提交）
```

### 2.3 main.go 头注释（包注释）

每个示例 `main.go` 顶部写包注释，按此顺序：

1. **演示什么**：一句话 + 特性列表。
2. **设计引用**：`design.md §N.M` / `development-plan §N`，让读者对照设计。
3. **工程约束**：尤其 **smoke 交互约束**（哪个控件是冒烟的可观测信号、为什么）。
4. **构建命令**：`scripts/build.ps1 -Target <name>` / `scripts/smoke.ps1 -Target <name>`。

范式见 `examples/phase5/main.go`（约束写得很细：唯一按钮、可点击 Text 替代、冒烟断言逻辑）。

### 2.4 winres.json

| 字段 | 规则 | 示例 |
|---|---|---|
| `identity.name` | `FluxVCL.<PascalCase示例名>` | `FluxVCL.Phase5` |
| `file_version` / `product_version` | 四段，与 `flux.Version` 对齐（`0.x.y` → `0.x.y.0`） | `0.1.0.0` |
| `ProductVersion` | 三段 = `flux.Version` | `0.1.0` |
| `OriginalFilename` | `<target>.exe` | `phase5.exe` |
| `dpi-awareness` | `perMonitorV2`（D5 要求，勿改） | `perMonitorV2` |
| `use-common-controls-v6` | `true` | `true` |

### 2.5 smoke 约束

- 纳入 CI 冒烟的示例：窗口内 `Button` 必须**唯一**，点击信号必须**可枚举断言**（smoke 按 class 枚举子控件读按钮文本，按钮文本变化 = 点击生效信号）。
- 非按钮交互（如主题切换）用**可点击 `Text`**（`TLabel` 无独立 HWND，非 Button 类，不干扰冒烟）。
- 冒烟截图统一命名：`bin/<target>-smoke.png`（CI 按此收集 artifact）。

---

## 3. 包与文件命名

| 项 | 规则 | 现有 |
|---|---|---|
| 用户包 | 根包 `flux`（对外即框架名） | `flux.go` 等 |
| 内部包 | 小写语义名，按层：widget / diff / render / native | `internal/widget` `internal/diff` `internal/render` `internal/native` |
| 根包文件 | **全小写**语义短名，同类加后缀细分 | `event.go` / `event_opts.go`；`flux.go` `state.go` `theme.go` `box.go` `layout.go` `controls.go` |
| 测试文件 | `<被测文件>_test.go`，跨特性收尾用功能域名 | `state_test.go`；`phase5_test.go` `scroll_inspect_test.go` |

---

## 4. 标识符命名

### 4.1 控件构造器（导出，PascalCase，名词）

`Window` / `Column` / `Row` / `ScrollBox` / `Text` / `Button` / `Input` / `Component`

- 容器类（Column/Row/ScrollBox）可带布局子参数（`Expanded`/`Flexible`）；构造器接受 `...any`，混排子节点与 Opt。
- **Node type 字符串与构造器同名**（PascalCase）：`"Window"` / `"Column"` / `"Row"`…（见 `widget.NewNode("Window")`）。

### 4.2 Opt（导出，PascalCase，动词 / 名词）

- 事件 / 生命周期：`On<Event>` —— `OnClick` / `OnMouseDown` / `OnMouseMove` / `OnKeyDown` / `OnMount` / `OnUpdate` / `OnUnmount`。
- 身份 / 几何 / 样式：`Key` / `Width` / `Height` / `Color` / `FontColor` / `Title` / `Expanded` / `Flexible`。
- 布局对齐：`MainAxis(MainAxisCenter)` / `CrossAxis(...)` —— 对齐值也导出。
- 绑定类 Opt：`Bind(state)` 返回绑定值（非 Opt 名后缀，但用法同参数）；`BindRef(ref)`。

### 4.3 State / 绑定 / App（导出）

- `State[T]`（泛型类型）、`NewState[T](initial)`（构造函数）、`Bind(s)`（绑定）。
- `NewApp(r render.Renderer)`、`App.Mount` / `App.Render` / `App.Root` / `App.Animate` / `App.SetBounds` / `App.Inspect` / `App.LastLayoutDiags`。
- 顶层泛型函数：`Async[T](app, load, onSuccess, onError...)`（Go 方法不支持泛型，故为包级函数）。

### 4.4 常量（导出）

| 类 | 命名 | 现有 |
|---|---|---|
| 版本 | `Version`（semver，`-dev` 后缀表未发布） | `Version = "0.1.0-dev"` |
| 主题 | `Light<X>` / `Dark<X>` | `LightTheme` / `DarkTheme` |
| 事件类型 | `Event<类型>` | `EventClick` `EventMouseDown` `EventMouseMove` `EventKeyDown`… |
| 对齐 / 弹性 | 语义名 | `MainAxisStart` `MainAxisCenter` `CrossAxisStart`…；`Expanded` / `Flexible` |
| 曲线 | 动词短语 | `EaseIn` `EaseInOut` `ElasticOut`…（类型为 `Curve`） |

### 4.5 统一事件结构体

`Event{ Type, X, Y, Key, Text, Button, Mods, Source }`

- 字段：短名，几何用 DIP 的 `X/Y`，键盘 `Key`，IME 结果 `Text`，鼠标 `Button`，修饰键 `Mods`，控件标识 `Source`。
- `Source` 格式：带 Key 时 `"<Type>#<Key>"`（如 `"Button#btn"`，稳定身份 D3）；未设 Key 时回落为 `"<Type>@<树路径>"`（隐式寻址，如 `"Button@Window/0/Column/1/Button"`，结构重排后漂移）。

### 4.6 内部类型（internal/）

- `widget.Node` / `widget.Props` / `widget.Widget`；`diff.Element`；`render.Rect` / `render.Renderer` / `render.Mutation`；`native.NewRenderer`。
- 与绑定库的互操作类型（如 `EventType`、对齐枚举）在 `flux` 层 type alias 后暴露，外部不直接 import internal。

---

## 5. 资源命名

| 资源 | 命名 | 说明 |
|---|---|---|
| 图标 | `assets/icon.png` | 全项目共用，winres 通过相对路径引用 |
| 运行时 DLL | `libenergy-amd64.dll` | 架构后缀；构建脚本复制到 exe 旁；可用 `FVCL_LIBENERGY_DLL` 覆盖路径 |
| 资源对象 | `rsrc_windows_<arch>.syso` | go-winres 生成（`amd64`），已 gitignore |
| 冒烟截图 | `bin/<target>-smoke.png` | CI artifact 收集路径 |
| 版本四段 | 与 `flux.Version` 对齐 | `0.x.y` → `0.x.y.0` |

---

## 6. 提交信息词汇表

> 完整格式与示例见 [CONTRIBUTING §4](../CONTRIBUTING.md#4-提交信息规范)。这里只列 **type / scope 的白名单**，新增 type/scope 须先补进两处文档。

**type**：`feat` / `fix` / `docs` / `test` / `refactor` / `perf` / `build` / `ci` / `chore` / `revert`

**scope**：`phaseN`（阶段主推进）、`example`、以及子系统名 —— `theme` / `animation` / `state` / `layout` / `list`（Phase 6 虚拟列表） / `inspector` / `event` / `diff` / `native` / `render` / `widget` / `docs` / `ci` / `scripts`

---

## 7. 分支命名

`<type>/<kebab-case 描述>`，type 用 §6 白名单：

```
feat/animation-controller
fix/example-phase5-chip
docs/naming-conventions
```

描述用英文 kebab-case、语义化、能对应到提交 subject。
