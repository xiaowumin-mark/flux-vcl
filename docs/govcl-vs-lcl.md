# FluxVCL 底座选型：ying32/govcl vs energye/lcl 专项调研

> 版本：0.2（调研稿）
> 日期：2026-08-08
> 范围：为 FluxVCL 决定"优先采用哪一个底层绑定库"的专项对比调研。维度：维护活跃度、技术架构、线程/事件模型、Go 工具链兼容性、许可与 DLL 分发、控件覆盖、社区生态、自维护风险，以及对 FluxVCL 的适配难度。
> 方法：GitHub API + 网页元数据核对、仓库源码逐文件核对（含两路并行 Agent 对照源码）、Web 检索、SourceForge 分发实查；全部关键结论附一手来源。今天为 2026-08-08，涉及"最新状态"的结论均以此基准核实。
> 定位：本报告是 [research.md](./research.md) §1 底座选型问题的深化，对应 [development-plan.md](./development-plan.md) Phase 0.1。结论供 Phase 0 选型决议使用，最终仍需实验验证（见 §7）。

---

## 0. 摘要（TL;DR）

1. **结论：优先采用 `energye/lcl` 作为 FluxVCL 底座，`ying32/govcl v1.2.10`（真实 Delphi VCL）降级为 B 计划，仅在"必须严格 VCL 语义"时启用。** 依据：energye/lcl 是 2026 年仍在高频维护的 LCL 绑定，且原生提供 govcl 缺失的两项 FluxVCL 关键原语——**主线程异步队列**（`RunOnMainThreadAsync`）与**异常处理回调钩子**（`EctExceptionHandler`）；而 govcl master 已停更两年（最后实质提交 2024-05-08）、两年无新 release、作者基本不回 issue，其真实 VCL 路线（v1.2.10）存在 libvcl.dll 预览-only 许可与 Delphi 商业授权摩擦。
2. **两个库同源但非 fork**：energye/lcl 由 yanghy 维护，**大量复用 govcl 代码**（41 个文件版权头为 ying32：`pkgs/win` 28 个 + `rtl` 13 个），API 与 govcl 的 `vcl`（LCL 绑定）包高度兼容（347 个导出函数名完全重叠），但构造返回接口（`IButton`）而非具体类型、无反射自动绑事件层。因此 research.md §5.1 的 D6"绑定隔离 + 可切换"设计依然成立，切换成本可控。
3. **代价一（语义）**：energye/lcl 是 **LCL（Lazarus）语义而非 Delphi VCL 语义**，且相对 govcl **缺 TRichEdit/TGauge/TMonthCalendar/TMiniWebview/TPngImage**。若 FluxVCL 产品叙事硬性绑定"Delphi VCL"品牌，需修正 design.md 表述（改为"VCL 风格的原生控件"）。
4. **代价二（运行时分发，最大摩擦点）**：`libenergy` 运行时经 Energy 生态的 **SourceForge `liblcl` 项目**分发，存在**命名混乱**（项目名 liblcl、v2.x 文件仍叫 `liblcl.dll`、v3.0 才改叫 `libenergy-*`、Go 代码默认找 `libenergy-amd64.dll`）与**版本脱节**（Go 模块 v1.0.6 ↔ 运行时 v3.0.1 两套版本号）。Windows 上 v3.0 新命名尚未发布（v3.0.1 仅 Linux 包）。可经 `libname.LibName` 手动指定路径缓解，但必须在 Phase 0 用实验定案。
5. **最大未决风险（必须 Phase 0 实验）**：energye/lcl 在现代 Go 工具链（1.22–1.27）+ Windows 上能否构建运行、`libenergy.dll` 从哪个版本/渠道获取并匹配；govcl v1.2.10 在现行 Go 上能否构建同样待验证。

---

## 1. 调研范围与方法

- 调研对象：`ying32/govcl`（含 v2.x LCL 与 v1.2.10/last-vcl-support 真实 VCL 两条线）、`energye/lcl`（含其所属 Energy 生态：energy/designer/liblcl/cef）。
- 一手来源：GitHub REST API（元数据/提交/releases/tags/issues）、`raw.githubusercontent.com` 源码逐文件核对、GitHub 网页、README/源码版权头、pkg.go.dev、DeepWiki 架构页、**SourceForge 分发目录实查**。
- 调研分工：两个并行 Agent 分别对照 govcl 与 energye/lcl 源码逐条核实；本报告据此综合。
- 已排除：SEO 内容农场（datasea.cn 等）与二手转述，仅作线索不作结论。
- 限制说明：未做本地构建实验（纯文档调研）；GitHub API 匿名配额用尽的部分改用网页抓取补充。

---

## 2. 事实基线（元数据，2026-08-08 核对）

| 指标 | ying32/govcl | energye/lcl |
|---|---|---|
| 仓库 | [ying32/govcl](https://github.com/ying32/govcl) | [energye/lcl](https://github.com/energye/lcl) |
| Stars / Forks / Open Issues | ~2,401 / 241 / 23 | ~34 / 1 / 3 |
| 创建时间 | 2017-09-25 | 2024-05-30 |
| master/main 最后 push | 2025-12-30（**master 实质提交 2024-05-08**；push 来自 dev 分支） | 2026-07-01 |
| 维护状态 | master 冻结 + 无新 release；**dev 分支仍由作者演进**；作者基本不回 issue | 活跃：2025-12 至 2026-07 共 84 条提交；issue #2 作者 3 天回复 |
| 最新 release | v2.2.3（2023-08-12，两年未发新版） | v1.0.6（2026-05-18）；tag 已到 v1.0.9 |
| 语言/许可 | Go / Apache-2.0 | Go / Apache-2.0 |
| go.mod | **master 无 go.mod**（`v2.2.3+incompatible`）；last-vcl-support 为 go 1.11 | `go 1.20`，依赖 `purego v0.10.0` + `go-ole` |
| 底层 DLL | liblcl.dll（LCL）；v1.2.10 线为 libvcl.dll/libvclx64.dll（VCL） | libenergy.dll（默认 dev 为 libenergy-<arch>.dll） |
| DLL 分发 | GitHub releases（liblcl-2.2.3.zip / Librarys-1.2.10.zip） | **SourceForge `liblcl` 项目**（libenergy-*，命名见 §3.5） |
| 与另一库的关系 | 独立 | **复用 govcl 41 个文件**（pkgs/win 28 + rtl 13，版权 ying32），API 高度兼容 |

来源：[govcl repo](https://github.com/ying32/govcl) · [govcl commits/master](https://github.com/ying32/govcl/commits/master) · [govcl branches](https://github.com/ying32/govcl/branches) · [govcl releases](https://github.com/ying32/govcl/releases) · [lcl repo](https://github.com/energye/lcl) · [lcl commits](https://github.com/energye/lcl/commits/main) · [lcl releases](https://github.com/energye/lcl/releases) · [lcl issues](https://github.com/energye/lcl/issues)

---

## 3. 逐维度对比

### 3.1 维护活跃度与生命周期

**govcl（master 冻结、dev 缓慢演进）**
- README 原文（2023-11-20）：*"Because GoVCL has already entered a stable stage and is currently in a state of pure maintenance. Under normal circumstances, no new features or components will be added."* 且 *"in principle, a new version will not be released."*（[README](https://github.com/ying32/govcl/blob/master/README.md)）
- **master 最后实质提交 2024-05-08**（typo 修复 [PR #199](https://github.com/ying32/govcl/pull/199)）。
- **但 dev 分支仍在由作者 ying32 提交**：2024-12-19 加 TNoteBook/TPage（[#216](https://github.com/ying32/govcl/pull/216)）、2025-06-25 加 TCheckBox AutoSize/TStatusBar OnDrawPanel（[#223](https://github.com/ying32/govcl/pull/223)、[#224](https://github.com/ying32/govcl/pull/224)、[#231](https://github.com/ying32/govcl/pull/231)）、**2025-12-30 重排 DLL 导入表并重新生成绑定（71 个 vcl 文件、638 增/230 删）**（[commit de29e00](https://github.com/ying32/govcl/commit/de29e0080585f692604d49750411c0c379307be0)）——即"代码库未完全死，但 master 与 release 已冻结"。
- 作者基本不回 issue：#238/#239/#240（2026-01~06）均无维护者回应；#240（2026-06-10）"这是个好项目，希望别中途掉下了"公开表达弃坑担忧（[issue #240](https://github.com/ying32/govcl/issues/240)）。
- 依赖生态收缩：`ying32/dylib` 已 archived（[dylib](https://github.com/ying32/dylib)）；`ying32/exts` 扩展包仓库已删除（404）。

**energye/lcl（活跃）**
- 2025-12 至 2026-07 共 **84 条提交**（12月21 / 1月10 / 2月4 / 3月10 / 4月9 / 5月17 / 6月6 / 7月1），作者 yanghy 一人高频维护（[commits](https://github.com/energye/lcl/commits/main)）。
- release 节奏：v1.0.0（2024-12-26）→ v1.0.5/v1.0.6（2026-05-17/18）→ tag v1.0.7–v1.0.9。
- issue 响应：open issues 仅 3 个（#1 2024-11、#2 2026-03、#3 2026-07），#2 由维护者 3 天内回复并指引用 Designer（[issue #2](https://github.com/energye/lcl/issues/2)）。
- 分支：main / main.old / 3.0-beta（3.0-beta 为 Energy v3 做准备）。

**结论：energye/lcl 显著占优。** govcl 需修正"完全停更"的表述为"master/release 冻结、dev 由作者零星演进、无社区响应"；无论哪种，其可依赖性与响应性都远低于 energye/lcl。

### 3.2 技术架构与绑定机制

**govcl**
- v2.x：纯 Go + `syscall.NewLazyDLL` 经 `ying32/dylib`（**已 archived**）加载 `liblcl.dll`，零 CGO（Windows）。`vcl/init.go` 在 `init()` 中 `runtime.LockOSThread()` + `SetEventCallback`（[源码](https://github.com/ying32/govcl/blob/master/vcl/init.go)）；`vcl/api/dllimports` 用 `syscall.LoadLibrary/GetProcAddress/SyscallN` 直调（[dll_windows.go](https://github.com/ying32/govcl/blob/master/vcl/api/dllimports/dll_windows.go)）。
- v1.2.10/last-vcl-support：同一架构，但加载 `libvcl.dll`/`libvclx64.dll`（Delphi VCL）；go.mod 为 go 1.11 + dylib 依赖（[go.mod](https://github.com/ying32/govcl/blob/last-vcl-support/go.mod)）。

**energye/lcl**
- **Windows 用 stdlib `syscall`（LoadLibrary/GetProcAddress/SyscallN），Unix 用 `ebitengine/purego`（Dlopen/Dlsym/SyscallN/NewCallback）**，零 CGO 三平台统一（[go.mod](https://github.com/energye/lcl/blob/main/go.mod)、[load_windows.go](https://github.com/energye/lcl/blob/main/api/imports/load_windows.go)、[load_cgo_disable_unix.go](https://github.com/energye/lcl/blob/main/api/imports/load_cgo_disable_unix.go)）。
- **大量复用 govcl 代码**：`pkgs/win/user32dll.go` 等 28 个 + `rtl/` 13 个文件版权头为 `Copyright © ying32`（[user32dll.go](https://github.com/energye/lcl/blob/main/pkgs/win/user32dll.go)、[rtl/init_windows.go](https://github.com/energye/lcl/blob/main/rtl/init_windows.go)）——即沿用 govcl 的 Win32 封装与运行时库，属"继承性实现"而非独立重写。
- DLL 命名演进：2025-11-22 提交"更新 liblcl > libenergy"将原生库改名（[commits](https://github.com/energye/lcl/commits/main)）；`api/libname` 按 dev/prod 解析 `libenergy-<arch>.dll`（默认 dev）/`libenergy.dll`（prod）（[lib.go](https://github.com/energye/lcl/blob/main/api/libname/lib.go)）。
- `lcl.Init()`：`runtime.LockOSThread()` + `api.SetEventCallback` 注册 7 类回调（EctLCL/EctLCLRemove/EctMessage/EctCreateParams/EctFormCreate/EctUIThreadSync/EctUIThreadAsync）（[lcl_init.go](https://github.com/energye/lcl/blob/main/lcl/lcl_init.go)）。

**结论：架构同源（零 CGO + DLL 加载），energye/lcl 用 purego 更现代、跨平台实现更完整。小幅占优。**

### 3.3 线程与事件模型（FluxVCL D4 的核心）

**govcl**
- 唯一调度原语 `vcl.ThreadSync(fn)`（`api.DSynchronize(fn,1)`，阻塞式）（[funcs.go](https://github.com/ying32/govcl/blob/master/vcl/funcs.go)）；**Go API 无 QueueAsyncCall/AsyncExecute**（research.md §6.2 核实）。FluxVCL 需自建主线程消费队列。
- 事件回调**无 recover**，handler 内 panic 直接崩进程（research.md §6.6）。
- 事件绑定是**反射方法名 + 显式 setter 并存**（`autoBindEvents.go` 按 `On+组件名+事件` 反射收集方法）；research.md 已确认反射绑定有 garble 失效问题，FluxVCL 禁用。

**energye/lcl**
- **原生提供 `lcl.RunOnMainThreadAsync(cb)`（异步主线程队列，带 id）与 `lcl.RunOnMainThreadSync(cb)`（同步）**（[lcl_runonmain_thread.go](https://github.com/energye/lcl/blob/main/lcl/lcl_runonmain_thread.go)）——正是 govcl 缺失、FluxVCL D4 需要自建的两类原语，且直接挂钩 `EctUIThreadSync/EctUIThreadAsync`。
- **原生异常处理回调** `EctExceptionHandler`（[api.go](https://github.com/energye/lcl/blob/main/api/api.go)）——对应 govcl 缺失的"事件错误边界"。
- **事件绑定是纯显式 setter，无反射自动绑定**：`SetOnClick/SetOnMouseDown/SetOnWndProc` 显式方法；表单创建改用 `IOnFormCreate/IOnCloseQuery/IOnClose/IOnShow` 接口自动挂载（[callback.go](https://github.com/energye/lcl/blob/main/lcl/callback.go)、[button.go](https://github.com/energye/lcl/blob/main/lcl/button.go)）——规避了 govcl 反射绑定的坑。
- `IApplication` 提供 `RemoveAsyncCalls/ProcessMessages/Idle(wait)` 等（[application.go](https://github.com/energye/lcl/blob/main/lcl/application.go)）。

**结论：energye/lcl 显著占优。** FluxVCL D4（线程纪律 + 错误边界）在 energye/lcl 上是"直接用原生原语 + 原生错误钩子"，在 govcl 上是"全部自建"。直接降低 Phase 2.5 的工程量与风险。

### 3.4 Go 工具链兼容性与构建

| | govcl | energye/lcl |
|---|---|---|
| go.mod | master 无（+incompatible）；v1.2.10 为 go 1.11 | go 1.20 |
| 最低 Go | 1.9.2（README） | 1.20 |
| 构建要求 | `GOOS=windows` `CGO_ENABLED=0` `-buildmode=exe`（Go≥1.15） | 零 CGO，纯 Go |
| 现代 Go（1.22–1.27） | v2.x 可构建（community 证实）；**v1.2.10 VCL 线待实测** | 代码活跃、持续适配；**待实测** |

来源：[govcl README](https://github.com/ying32/govcl/blob/master/README.md) · [lcl go.mod](https://github.com/energye/lcl/blob/main/go.mod)

**结论：energye/lcl 面向现代 Go 维护、go 1.20，风险更低；govcl v1.2.10（go 1.11）在现代 Go 上能否构建是 research.md 已列的最高风险。energye/lcl 占优**（两者都仍需 Phase 0 实验定案）。

### 3.5 许可与 DLL 分发（FluxVCL 产品化痛点）

**govcl**
- Go 代码 Apache-2.0。
- v2.x LCL 线：`liblcl.dll` 由 Lazarus（开源）编译，GitHub release 直接发布 `liblcl-2.2.3.zip`（[下载](https://github.com/ying32/govcl/releases/download/v2.2.3/liblcl-2.2.3.zip)），无许可摩擦，但**版本固定（v2.2.3）、依赖作者发布**。
- **v1.2.10 VCL 线（真实 VCL）：`Librarys-1.2.10.zip` 今天仍可下载**（内含 libvcl/win32/libvcl.dll 1.72MB、libvcl/win64/libvclx64.dll 2.22MB，[release v1.2.10](https://github.com/ying32/govcl/releases/tag/v1.2.10)）；但 zip 内 README 写明 **"libvcl 库二进制仅供预览和测试使用，正式使用请自行编译 libvcl 源代码"**（[last-vcl-support README](https://github.com/ying32/govcl/blob/last-vcl-support/README.md)），且作者在 [z-kit.cc/about](https://z-kit.cc/about.html) 明言 Delphi 版权问题——**真实 VCL 路线的最大产品化摩擦**。

**energye/lcl**
- Go 代码 Apache-2.0。
- `libenergy` 运行时经 **SourceForge `liblcl` 项目**分发（[sourceforge.net/projects/liblcl](https://sourceforge.net/projects/liblcl/files/)），由 Lazarus/FreePascal 编译（开源、无商业授权问题）。
- **命名混乱**：SourceForge 项目名叫 liblcl；v2.5.4 的 Windows 文件仍叫 `liblcl.Windows64.zip`（内置 liblcl.dll）；v3.0.1（2026-07-06）才出现 `libenergy-linux-amd64-gtk3-147.zip` 这种命名且**仅 Linux 包、无 Windows**；而 Go 代码（默认 dev）找的是 `libenergy-amd64.dll`。**Windows 上"libenergy"新命名的 DLL 尚未随 v3.0 发布。**
- **版本脱节**：Go 模块版本（v1.0.6/v1.0.9）与运行时版本（SourceForge v3.0.1）两套体系，无文档化对应关系；下载量小（v3.0.1 累计 53 次）。
- 缓解手段：`libname.LibName` 可手动指定任意 DLL 路径/文件名；`libname.UseWS` 选择 gtk2/gtk3。加载优先级：LibName > exe 目录 > `~/.energy/runtime` > 临时目录（[initialize_dev.go](https://github.com/energye/lcl/blob/main/internal/initialize/initialize_dev.go)）。

**结论：** 若走真实 VCL，govcl v1.2.10 的"预览-only + Delphi 版权"是最差；若走 LCL，govcl v2.x 的 liblcl 分发最清晰但版本冻结，energye/lcl 的 libenergy **无授权问题但分发最混乱**（命名 + 版本 + Windows 滞后）。三者均需在 Phase 0 用实验锁定。

### 3.6 控件覆盖与 API 对齐（FluxVCL 需要的核心控件）

- energye/lcl 的 `lcl` 包含 **496 个 Go 文件**（govcl `vcl` 仅 211 个），FluxVCL 需要的核心控件**全部齐备**：TForm/TButton/TEdit/TMemo/TListView/TPaintBox/TCanvas/TImage/TLabel/TPanel/TComboBox/TTreeView/TStringGrid/TProgressBar/TPageControl/TScrollBox/TTimer/TrayIcon/对话框/菜单等（[lcl 包](https://github.com/energye/lcl/tree/main/lcl)）。
- **TListView 虚拟化原语完整**：`SetOnData/SetOnDataFind/SetOnDataHint/SetOnDataStateChange`（OwnerData 虚拟列表，[listview.go](https://github.com/energye/lcl/blob/main/lcl/listview.go)）——FluxVCL Phase 6 依赖满足。
- **额外亮点**：SynEdit 编辑器家族（syn*.go 约 100 个）、LazVirtualStringTree 虚拟树、CoolBar/ControlBar。
- **相对 govcl 的缺口**：energye/lcl **缺 TRichEdit、TGauge、TMonthCalendar、TMiniWebview、TPngImage**（govcl 有）。TRichEdit 缺口值得注意——research.md §2.3 提到 govcl 的 TRichEdit 本身有中文/emoji bug（[#212](https://github.com/ying32/govcl/issues/212)、[#234](https://github.com/ying32/govcl/issues/234)），FluxVCL 若需要富文本，LCL 侧可用 TMemo 兜底或自绘。
- **API 对齐度**：组件名、`NewXxx` 构造、事件签名完全一致（`type TNotifyEvent func(sender IObject)`）；energye/lcl 导出函数 1201 个、govcl 450 个，347 个完全重叠。**差异**：energye 构造返回接口（`NewButton(...) IButton`）、govcl 返回具体类型（`*TButton`）；energye 用具体类继承（`TButton struct{ TCustomButton }`）、govcl 用接口嵌入 + 生成式扁平函数。

**结论：能力基本相当，energye/lcl 覆盖更广但缺富文本等少量控件；LCL 语义是唯一实质差异。**（D6 隔离下差异可控。）

### 3.7 社区与生态

| | govcl | energye/lcl |
|---|---|---|
| 规模 | 2,401 stars，中文社区成熟（QQ 群 263106281、Gitee wiki、CSDN、itying bbs） | 34 stars，背靠 Energy 生态 |
| 文档 | 中文 wiki 为主、英文弱；作者自述"英语不好" | 中英双语 README、Energy 文档站 energye.github.io、locales i18n |
| 工具链 | 拖拽设计器 GoVCLUIDesigner（已冻结） | **Energy Designer** 持续开发（2026-06-30 活跃）；energy CLI |
| 生态定位 | 独立 LCL/VCL 绑定 | Energy（LCL+CEF+WebView2 三模式）的底座，可独立用 LCL |
| 认可度 | 社区成熟但停滞 | **Gitee GVP 项目**；40+ 官方示例 |

来源：[govcl README](https://github.com/ying32/govcl/blob/master/README.md) · [energy README](https://github.com/energye/energy) · [Energy Designer](https://github.com/energye/designer) · [gitee energye](https://gitee.com/energye/)

**结论：govcl 社区规模大但停滞、作者失联；energye/lcl 社区小但有活跃工具链、文档与 GVP 认可。对 FluxVCL 而言 energye/lcl 生态"更活着"。综合相当，energye/lcl 微占优。**

### 3.8 自维护风险

- govcl v1.2.10：完全冻结 6 年，任何现代 Go 工具链演进都可能破坏构建；必须 vendored + 自维护；无社区提交者。**最高。**
- govcl v2.x（LCL）：master 冻结、dev 零星演进、无 release；社区仍在用（issue 持续产生）但作者不回复。自维护负担中高。
- energye/lcl：作者活跃跟进，跟随 Energy 生态演进；**单点维护**（作者 yanghy 一人）+ **运行时由外部生态供给**（SourceForge）。但代码 Apache-2.0 可 fork，且因同源可反向吸收 govcl 修复。自维护负担中等，需主动锁版本。

**结论：energye/lcl 自维护负担最低，govcl v1.2.10 最高。**

---

## 4. 对 FluxVCL 的适配评估（对照架构不变量 D1–D7）

| 不变量 | govcl v1.2.10（VCL） | govcl v2.x（LCL） | energye/lcl（LCL） |
|---|---|---|---|
| D1 三棵树 | 适配层照常；控件是真实 Delphi VCL | 适配层照常；Lazarus LCL | 适配层照常；Lazarus LCL |
| D2 属性级 patch | `Set*` setter 齐全 | `Set*` setter 齐全 | `Set*` setter 齐全（同源 API） |
| D3 稳定 key | 框架自实现，不依赖库 | 同 | 同 |
| **D4 线程纪律** | 仅 ThreadSync，需自建队列+错误边界 | 仅 ThreadSync，需自建 | **原生 RunOnMainThreadAsync/Sync + EctExceptionHandler，直接映射 runOnUI/runOnUISync/错误路由** |
| **事件绑定** | 反射 + setter（反射有坑） | 反射 + setter | **纯 setter + IOnFormCreate 接口，无反射** |
| D5 布局 | 禁用 Align，自做几何 | 同 | 同（LCL 亦有 Align，框架禁用） |
| D6 绑定隔离 | 窄接口后藏 libvcl | 窄接口后藏 liblcl | 窄接口后藏 libenergy；**API 与 govcl 对齐 → 切换成本低** |
| D7 测试不变量 | 与库无关，mock 测试 | 同 | 同 |

关键结论：
1. **D4（线程 + 错误边界）是最大差异点**：energye/lcl 让 Phase 2.5 从"自建调度器 + 自建错误路由"降为"适配原生原语"，且无反射事件绑定的坑。
2. **D6（隔离）让选型可逆**：由于同源、API 高度兼容，即使选了 energye/lcl 发现缺某控件（如 TRichEdit），回切或双后端支持的适配层改动有限。
3. **VCL/LCL 语义差异**：FluxVCL 的声明式抽象（Widget/Node/State）不依赖具体控件实现；LCL 的 Win32 widgetset 在 Windows 上同样封装原生控件，观感接近。产品叙事需把"VCL"表述改为"VCL 风格的原生控件"或"LCL/VCL"。

---

## 5. 综合评分矩阵

评分 1–5（5 最优），权重针对 FluxVCL 的落地优先级。

| 维度 | 权重 | govcl v1.2.10（VCL） | govcl v2.x（LCL） | energye/lcl |
|---|---|---|---|---|
| 维护活跃度 | 20% | 1（冻结 6 年） | 2（master/release 冻结、dev 零星） | 5（2026 高频维护） |
| 线程/事件原语（D4） | 20% | 1 | 1 | 5（Async/Sync + 异常钩子 + 无反射） |
| Go 工具链兼容 | 15% | 1（go 1.11，待实测） | 3（可构建但冻结） | 4（go 1.20，活跃，待实测） |
| 许可/DLL 分发 | 15% | 1（libvcl 预览-only + Delphi 授权） | 4（liblcl 开源、分发清晰但冻结） | 3（libenergy 开源但命名/版本/Windows 滞后） |
| 控件覆盖 | 10% | 5（真实 VCL） | 4（LCL） | 4（LCL，覆盖更广但缺 TRichEdit 等） |
| 社区/生态 | 10% | 3（大但停滞） | 3 | 3（小但活跃 + Designer/GVP） |
| 自维护风险 | 10% | 1 | 2 | 3（单点 + 外部供给，可 fork） |
| **加权总分** | 100% | **1.6** | **2.55** | **4.05** |

> 评分是导向性的，依据见 §3 各维度；VCL/LCL 语义差异的"产品叙事"影响未计入（见 §6）。govcl v2.x 因 dev 分支仍演进从 research.md 的"完全停更"上调为 2 分，但无 release + 作者不回复 issue 使其仍不具备可持续底座价值。

---

## 6. 选型决议（明确优先级）

### 决议：FluxVCL 底座优先采用 **energye/lcl**（LCL）；govcl v1.2.10（真实 VCL）为 B 计划。

**采用 energye/lcl 的理由（按权重）：**
1. **活跃维护**：2026 年仍高频演进、跟随现代 Go；govcl 两年无 release、作者不回复 issue。
2. **原生线程原语**：`RunOnMainThreadAsync/Sync` + `EctExceptionHandler` + 无反射事件绑定，直接对应 FluxVCL D4 需求，省去自建调度器、错误边界与事件映射的主要工程量。
3. **零商业许可摩擦**：libenergy 由开源 Lazarus 编译；govcl VCL 线的 libvcl.dll "仅预览/测试" + Delphi 商业授权是产品化硬伤。
4. **同源可切换**：energye/lcl 复用 govcl 代码、API 高度兼容，D6 隔离下回切成本低——选它几乎不堵死 VCL 路线。
5. **生态工具链**：Energy Designer、energy CLI、GVP 认可、40+ 示例。

**采用 energye/lcl 必须接受的代价：**
1. **LCL 语义**（非 Delphi VCL）：控件是 Lazarus 控件集，缺 TRichEdit/TGauge 等少量控件——用 TMemo 兜底或自绘。
2. **运行时分发混乱**：libenergy 需从 SourceForge 获取、命名/版本与 Go 代码脱节、Windows 新命名滞后——Phase 0 必须锁定"用哪个运行时版本 + 如何命名/指定路径"。
3. **单点维护 + 外部供给**：作者一人 + DLL 依赖 Energy 生态持续发布——D6 隔离 + fork 预案 + 版本锁定应对。

**保留 govcl v1.2.10 为 B 计划的场景：**
- Phase 0 实验发现 energye/lcl 在 Windows 上无法运行/构建（概率低）；
- 产品必须严格呈现"Delphi VCL"语义或依赖 VCL 独有控件/行为（需清单化核对后确认为硬需求）；
- Energy 生态在 Phase 0–2 期间出现停更信号。

**B 计划启用条件（任一触发即切）：** 见 §7 实验项 E1 失败 + E2 失败，或产品需求清单中 VCL 专属项被确认为硬门槛。

### 对 design.md 的修正（已确认并执行）
- 定位表述已改为"**基于原生控件（LCL/VCL 双后端，默认 LCL）**"，见 [design.md](./design.md)（v0.2）。
- 产品叙事卖"现代声明式 Go 框架 + 原生控件"，不再强调"Delphi 封装"（research.md §8.3 已警告该标签是负债）。

---

## 7. 风险与 Phase 0 验证清单

| # | 实验项 | 目的 | 通过标准 |
|---|---|---|---|
| E1 | **energye/lcl 构建冒烟** | 现代 Go（1.22–1.27）+ Windows 构建最小"窗体+按钮+点击" | `go build` 单命令产出 exe，双击出窗、点击响应 |
| E2 | **libenergy 运行时获取与版本匹配** | 确认 Windows 上"哪个 SourceForge 版本 + 哪个文件名 + 用 dev 还是 prod 命名"；**重点核对 v2.5.4 的 liblcl.dll 重命名为 libenergy.dll 是否能跑，或 v3.0 是否已发布 Windows 包**；记录"Go tag ↔ 运行时版本 ↔ DLL 文件名"对照表 | 运行无版本错误；产出可复现的对照表；确认 `libname.LibName` 手动指定路径可行 |
| E3 | **govcl v1.2.10 对照实验（仅当需 B 计划）** | 确认 VCL 线在现代 Go 可构建、libvcl DLL 可获取 | 构建+运行通过 |
| E4 | **LCL/VCL 能力差异清单** | 列出 design.md 提到的控件/行为中 LCL 缺失的项（如 TRichEdit），确认哪些是硬需求 | 清单 + 决议 |
| E5 | **线程原语实测** | 验证 `RunOnMainThreadAsync/Sync` 行为与死锁边界（对照 govcl ThreadSync 已知死锁） | 从 goroutine 改 UI 不崩、不挂 |

**风险登记册增量：**
- energye/lcl 单点维护（作者一人）+ 运行时外部供给 → 应对：fork 预案 + D6 隔离 + Phase 0 锁定版本；监控 Energy 生态停更信号。
- libenergy 命名/版本混乱 → 应对：E2 产出对照表；用 `libname.LibName` 显式指定；后续考虑在 FluxVCL 内封装"运行时下载/校验"。
- LCL 控件与 VCL 行为差异 → 应对：E4 清单 + 适配层兜底（个别控件用 custom draw / Win32 直连 / 嵌入 govcl v2.x 同类控件）。

---

## 附录：来源索引

**仓库 / 元数据**
- [ying32/govcl](https://github.com/ying32/govcl) · [commits/master](https://github.com/ying32/govcl/commits/master) · [commits/dev](https://github.com/ying32/govcl/commits/dev) · [branches](https://github.com/ying32/govcl/branches) · [releases](https://github.com/ying32/govcl/releases) · [release v1.2.10](https://github.com/ying32/govcl/releases/tag/v1.2.10) · [last-vcl-support README](https://github.com/ying32/govcl/blob/last-vcl-support/README.md) · [commit de29e00](https://github.com/ying32/govcl/commit/de29e0080585f692604d49750411c0c379307be0)
- [ying32/liblcl](https://github.com/ying32/liblcl) · [ying32/dylib（archived）](https://github.com/ying32/dylib)
- [energye/lcl](https://github.com/energye/lcl) · [commits](https://github.com/energye/lcl/commits/main) · [releases](https://github.com/energye/lcl/releases) · [tags](https://github.com/energye/lcl/tags) · [issues](https://github.com/energye/lcl/issues) · [pkg.go.dev v1.0.9](https://pkg.go.dev/github.com/energye/lcl@v1.0.9)
- [energye/energy](https://github.com/energye/energy) · [releases](https://github.com/energye/energy/releases) · [Energy Designer](https://github.com/energye/designer) · [energye 组织](https://github.com/orgs/energye/repositories) · [SourceForge liblcl](https://sourceforge.net/projects/liblcl/files/)

**源码（raw.githubusercontent.com 逐文件核对）**
- govcl：[vcl/init.go](https://github.com/ying32/govcl/blob/master/vcl/init.go) · [vcl/funcs.go](https://github.com/ying32/govcl/blob/master/vcl/funcs.go)（ThreadSync）· [vcl/autoBindEvents.go](https://github.com/ying32/govcl/blob/master/vcl/autoBindEvents.go) · [vcl/api/dllimports/dll_windows.go](https://github.com/ying32/govcl/blob/master/vcl/api/dllimports/dll_windows.go)
- energye/lcl：[lcl_runonmain_thread.go](https://github.com/energye/lcl/blob/main/lcl/lcl_runonmain_thread.go) · [api/api.go](https://github.com/energye/lcl/blob/main/api/api.go) · [api/libname/lib.go](https://github.com/energye/lcl/blob/main/api/libname/lib.go) · [lcl_init.go](https://github.com/energye/lcl/blob/main/lcl/lcl_init.go) · [lcl/application.go](https://github.com/energye/lcl/blob/main/lcl/application.go) · [lcl/listview.go](https://github.com/energye/lcl/blob/main/lcl/listview.go) · [lcl/button.go](https://github.com/energye/lcl/blob/main/lcl/button.go) · [rtl/rtl.go（版权 ying32）](https://github.com/energye/lcl/blob/main/rtl/rtl.go) · [pkgs/win/user32dll.go](https://github.com/energye/lcl/blob/main/pkgs/win/user32dll.go) · [api/imports/load_windows.go](https://github.com/energye/lcl/blob/main/api/imports/load_windows.go) · [internal/initialize/initialize_dev.go](https://github.com/energye/lcl/blob/main/internal/initialize/initialize_dev.go) · [energye/examples lcl](https://github.com/energye/examples/tree/main/lcl)

**关键 issue / 社区**
- govcl：[#240](https://github.com/ying32/govcl/issues/240)（弃坑担忧，2026-06）· [#238](https://github.com/ying32/govcl/issues/238) · [#237](https://github.com/ying32/govcl/issues/237) · [#212](https://github.com/ying32/govcl/issues/212)（RichEdit 中文）
- energye/lcl：[#2](https://github.com/energye/lcl/issues/2)（作者 3 天回复）· [#3](https://github.com/energye/lcl/issues/3)

**架构参考**
- [DeepWiki: Energy Core Architecture](https://deepwiki.com/energye/energy/3-core-architecture)（purego、零 CGO、三模式收敛 libenergy）· [Energy 文档站](https://energye.github.io/v3/energy/Overview) · [energy CLI 安装文档](https://energye.github.io/course/install-env)

---

*本文档为独立专项调研，与 [research.md](./research.md) §1 底座选型互补；评分与决议基于 2026-08-08 事实，最终以 Phase 0 实验（§7）定案。*
