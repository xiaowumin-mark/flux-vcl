# FluxVCL 开发计划

> 版本：0.1
> 日期：2026-08-08
> 配套文档：[调研报告](./research.md)（生态、坑点、先例、工程证据）、[设计文档](./design.md) 与 [底座选型调研](./govcl-vs-lcl.md)。
> 本计划把 design.md 的三大阶段拆细为 **Phase 0–7**，每阶段含子任务、交付物、验收标准与风险；并先把调研得出的**架构基线决策**固化为全项目不变量。

---

## 0. 架构基线决策（全项目不变量）

> 这些决策来自调研，跨阶段生效，写代码前必须先定死。任何偏离需单独评审。

### D1. 三棵树模型（reconciliation 的根基）
- **Widget 树**：每次 render 重建的不可变 Go 结构体（纯数据，不持有原生指针）。
- **Element 树**：持久 identity 节点 `{controlType, key, parentPath, nativePtr, prevConfig}`。
- **原生控件树**：绑定层真实控件（默认 LCL：`*lcl.TButton` 等；VCL 后端：`*vcl.TButton`）。
- 更新规则 = Flutter `canUpdate`：`旧控件类型==新控件类型 && 旧key==新key` → **原地 patch**；否则**只重建该节点**（绝不上溯重建祖先容器）。

### D2. 属性级 patch + 批量提交
- diff 只产生 mutation op 集（Create/Update/Insert/Remove/Delete），按"先删后建、先上后下"应用。
- 逐属性比较（Caption/Left/Top/Width/Height/Visible/Enabled/Font/Color/TabStop…），只对变化者调 `Set*`；未变控件直接跳过。
- 应用时用 `BeginUpdate/EndUpdate` + 窗体 `DoubleBuffered` + `WM_SETREDRAW`（TScrollBox）包裹；新控件在首个 handle 访问前设完全部属性。
- 逃逸口：直接单向属性绑定（类 Dioxus Signal），高频热路径绕过整树 diff。

### D3. 列表身份：稳定 key
- key 必须来自模型（ID 或创建时生成一次），**绝不用数组 index、绝不每次 render 随机**。index key 会让 VCL 焦点/caret/IME 迁到错误行。

### D4. 线程纪律：单一 UI 线程 + marshalling
- 所有控件访问只在主线程；框架 init 照抄绑定库（energye/lcl 与 govcl 相同）的 `runtime.LockOSThread()` 钉住 UI goroutine；**每个碰原生控件的入口做 debug 断言** `rtl.CurrentThreadId()==rtl.MainThreadId()`（违反即 panic）。
- 自建调度器：在 `ThreadSync` 之上建单一主线程消费队列，提供 `runOnUI(fn)`（异步）/`runOnUISync(fn)`（阻塞），**批量/合并更新**（禁止逐条目 ThreadSync，阻塞+全局互斥会停摆）。
- State 变更从任意 goroutine 触发时：离线程更新纯 Go 模型，落地控件的 commit 一律经 `runOnUI`。
- **销毁必须入队延后**，绝不在事件回调内同步 Free（LCLRefCount>0 崩溃）；`LCLRefCount>0` 警告在 debug 构建中视为断言失败。
- **事件分发错误边界**：govcl 事件回调无 recover，主线程 handler 内 panic 会崩进程 → 分发层统一 `recover()` 路由到错误事件。

### D5. 布局：自定义 Measure/Layout，禁用原生 Align
- 协议：constraints 下传 / size 上抛 / 父定 offset；结果 `constraints.constrain()` 钳制。
- 框架管理的控件 `Align=alNone`，几何只走 `SetBounds`；`Native()` 逃逸口设置的 Align 在布局前还原。
- 叶子尺寸用 **intrinsic-size 函数**（GDI 文本测量 + 主题 API + 缓存脏标记），不因测量而实现控件。
- 全坐标用 DIP，`MulDiv(dip, dpi, 96)` 转像素；DPI 感知 PerMonitorV2。

### D6. 绑定隔离
- Renderer 面向窄接口（`Create/SetBounds/SetVisible/TextWidth/HandleAllocated`…），默认 LCL 绑定（energye/lcl）藏在适配层后，保留切 govcl v1.2.10（真实 VCL，B 计划）的余地（决议见 [govcl-vs-lcl.md](./govcl-vs-lcl.md)）。
- **事件映射显式注册回调，禁止反射方法名绑定**（garble 失效、误匹配问题）。

### D7. 三条不可妥协的测试不变量
- (a) 纯属性变化绝不重建控件；(b) 稳定 key 列表重排不迁移焦点/IME；(c) 相同树 diff 零 mutation。

---

## Phase 0 — 地基与选型验证（Spike）· 目标：决策落地、可冒烟

> **进展（2026-08-09）**：E1（energye/lcl v1.0.3 + libenergy DLL 构建冒烟）与 E2（DLL 获取路径/版本映射）
> **已完成并验证通过**，详见 [phase0-e2-libenergy-mapping.md](./phase0-e2-libenergy-mapping.md) 与 `ref/e1-smoke/`。
> 关键结论：Go 包版本必须与 DLL 严格一致（designer 锁 v1.0.3）；标准初始化序列
> `Init → Application.Initialize → NewForms → Run`；控件须在 `NewForms` 之后创建。

| # | 子任务 | 要点 / 参考 | 状态 |
|---|---|---|---|
| 0.1 | **绑定选型实验** | 在现行 Go 工具链（1.22–1.27）上验证首选 **energye/lcl + libenergy DLL**（构建 + libenergy 获取/版本匹配，见 [govcl-vs-lcl.md](./govcl-vs-lcl.md) §7 E1/E2）；B 计划 govcl v1.2.10（`last-vcl-support`）+ libvcl DLL 仅在需要真实 VCL 时做对照。写最小"窗体+按钮+点击"用例，记录能否构建/运行/DLL 加载。决议已预倾向 LCL，实验用于定案。 | ✅ 完成 |
| 0.2 | **DLL 交付与许可方案** | 确认首选 libenergy（energye/lcl）获取路径与版本 pin（**结论见 [phase0-e2-libenergy-mapping.md](./phase0-e2-libenergy-mapping.md)**：权威来源是 energye/designer 内嵌 zip，非 SourceForge/GitHub Releases；版本锁定 lcl v1.0.3）；B 计划 libvcl.dll/libvclx64.dll 路径与"预览/测试"许可；决策分发方式（exe 旁 vs 构建脚本）。 | ✅ 完成 |
| 0.3 | **构建脚手架** | `GOOS=windows` `CGO_ENABLED=0` `-buildmode=exe` `-ldflags "-H=windowsgui"`；空导入 `winappres`（manifest/图标）；`.syso` 命名 `_windows_<arch>`；Go 版本策略 + CI 每轮验证冻结绑定可构建。 | |
| 0.4 | **仓库与模块** | 模块路径 `github.com/fluxvcl/flux-vcl`（已核实未被占用）；目录骨架；`go.mod`；README/许可。 | |
| 0.5 | **CI 骨架** | GitHub Actions：windows-latest 上 `go test ./...` + 冒烟（启动真 app、断言日志、`kbinani/screenshot` 截图 artifact）。 | |
| 0.6 | **无头测试驱动雏形** | 参照 Fyne `test` 驱动：mock renderer，state/diff 纯逻辑可无显示测试。 | |

**交付物**：选型决议文档、可运行的 Hello World、CI 绿。
**验收**：`go build` 单命令产出 exe，双击出窗口、点按钮有反应；CI 冒烟通过。
**风险**：energye/lcl 的 libenergy 运行时分发/版本匹配（预判为最高风险，E2 实验重点）；B 计划 govcl v1.2.10 与现代 Go 不兼容风险次之。两者靠 D6 隔离让切换可控。

---

## Phase 1 — 声明式核心（Widget/Node/Element/diff/Renderer 抽象）· 目标：能写普通桌面程序

| # | 子任务 | 要点 / 参考 |
|---|---|---|
| 1.1 | Widget/Node 数据结构 | `Widget` 接口（design.md §4.2）；Node `{Type, Props, Children}`。 |
| 1.2 | Element 树与 identity | `canUpdate`（D1）；Element 节点持有原生指针 + prevConfig。 |
| 1.3 | Renderer 接口 + Mutation op 集 | `Mount/Update/Remove`（design.md §5.1）+ Dioxus 风格 op：`AppendChild/InsertChildBefore/RemoveChild/SetProperty/SetText/Create/Destroy`（可 mock 测试）。 |
| 1.4 | **diff/reconciliation 引擎** | 全项目最高优先级代码。build 新树 → 按 D1 匹配 → 属性级 patch（D2）→ 批量提交。性能：diff 循环复用 buffer。 |
| 1.5 | 基础控件集 | `Window/Column/Row/Text/Button/Input`；对应原生控件 `TForm/TEdit/TButton`（默认 LCL；占位布局，Phase 3 精修）。 |
| 1.6 | 原生逃逸口 | `Native(func(btn *lcl.TButton))`（默认 LCL 后端）、`Ref`（design.md §11）。约束：逃逸口改动 Align 须在布局前还原（D5）。 |
| 1.7 | **三不变量测试** | D7 三条测试护栏上线（a/b/c）。 |

**交付物**：`examples/basic`（窗口+文本+按钮+输入框+点击）。
**验收**：按钮点击改文本/输入框内容，全程零控件重建（渲染器断言 mutation 数）。
**风险**：diff 正确性 —— 用不变量测试锁死。

---

## Phase 2 — State 系统与数据绑定 · 目标：状态驱动 UI

| # | 子任务 | 要点 / 参考 |
|---|---|---|
| 2.1 | `State[T]` 原语 | `State(0)` / `count.Set(1)` / `Bind(count)`；订阅机制（参照 Gova `State.Set()→re-render`、Compose 快照）。 |
| 2.2 | 单向绑定 | `Text(Bind(user.Name))`：属性变化→patch。 |
| 2.3 | 双向绑定 | `Input(Bind(user.Name))`：控件事件→State→patch。 |
| 2.4 | **作用域失效** | State getter 记录"被哪个 element 路径读取"，`Set()` 只 re-diff 依赖子树；至少先做"未变子树跳过"。 |
| 2.5 | **线程 marshalling** | `runOnUI`/`runOnUISync`（D4）；State 从 goroutine 触发变更的规范路径；销毁延后。 |
| 2.6 | Key 系统 | 列表 key（D3）在 State 场景落地。 |

**交付物**：计数器 demo、输入双向同步 demo。
**验收**：外部 goroutine 改 State 不崩溃、UI 正确刷新；只重建受影响子树（断言）。
**风险**：goroutine 改 UI 崩溃 —— 用 D4 调度器 + 测试覆盖。

---

## Phase 3 — 布局引擎 · 目标：现代布局

| # | 子任务 | 要点 / 参考 |
|---|---|---|
| 3.1 | 协议 | `BoxConstraints`/`Size`；`Measure`/`Layout` 两遍（design.md §6.2）。 |
| 3.2 | **intrinsic-size 函数** | `Size Measure(font, text, dpi, constraints)`；GDI 文本测量（`TCanvas.TextWidth/TextHeight/TextExtent`）；主题 API（`BCM_GETIDEALSIZE`/`GetThemePartSize`）一次实现测量+缓存；缓存失效（文本/字体/DPI 变化）。 |
| 3.3 | Flex 算法 | RenderFlex 精确实现：非 flex 主轴 unbounded、freeSpace/flex 分配、Expanded=tight/Flexible=loose、主轴对齐分布、只增不缩+溢出诊断。 |
| 3.4 | 定位应用 | `SetBounds` 写 frame；框架控件 `Align=alNone`（D5）；逃逸口 Align 还原。 |
| 3.5 | **DPI** | PerMonitorV2 manifest；DIP→像素 `MulDiv`；`WM_DPICHANGED` 全量 re-layout；`TForm.Scaled=false`。 |
| 3.6 | 滚动容器 | 滚动轴 unbounded 约束；TScrollBox 原生滚动 + `WM_SETREDRAW` 防闪烁。 |
| 3.7 | 布局调试 | inspector 预留：节点 constraints/size/frame/flex 因子、溢出提示。 |

**交付物**：表单布局、可伸缩面板、高分屏 demo。
**验收**：Row/Column 比例 flex 正确；改变窗口尺寸布局即时更新且无闪烁；125%/150% 缩放文字不糊。
**风险**：测量与真实渲染尺寸不一致（字体匹配、主题 padding）—— 用"隐藏实现一次性测量+缓存"校准。

---

## Phase 4 — 事件系统与生命周期 · 目标：统一交互

| # | 子任务 | 要点 / 参考 |
|---|---|---|
| 4.1 | 统一事件 | `Event{Source,X,Y,Type}`（design.md §10）；**显式回调注册**（D6，禁反射）。 |
| 4.2 | Mouse/Keyboard 映射 | 原生控件 OnClick/OnMouseDown/OnKeyDown → 统一事件；坐标 DIP 归一。 |
| 4.3 | 生命周期 | `OnMount/OnUpdate/OnUnmount`（design.md §12）；卸载时入队销毁（D4）。 |
| 4.4 | **IME/中文输入** | form 级路由 `OnUTF8KeyPress`（已确认在正式版 v2.2.3，仅绑定 TForm，govcl issue #126）；自定义编辑器 Win32 IMM 或 `OnWndProc` 挂 `WM_CHAR`/`WM_IME_*` 预案。 |

**交付物**：完整交互示例（hover/点击/键盘/焦点）。
**验收**：中文输入正常；事件不阻塞主线程（长时间 handler 自动离屏）；销毁不崩溃。
**风险**：IME 边界（政府已知 bug）—— 限制范围，普通输入用 TMemo/TEdit 能力内。

---

## Phase 5 — 高级特性 · 目标：表现力

| # | 子任务 | 要点 / 参考 |
|---|---|---|
| 5.1 | **动画** | 主线程 60fps（TTimer/自定义 pump）；Tween/Curve/Transition；高频属性用**直接绑定**（D2 逃逸口）避免整树 re-diff。 |
| 5.2 | Theme | `Theme{Font,Color,Radius,Animation}`（design.md §14）；Light/Dark/Fluent；主题切换=全量 re-diff（重入 diff 引擎）。 |
| 5.3 | Async | `Async(Load, OnSuccess)`（design.md §15）：后台 goroutine + `runOnUI` marshalling（D4）。 |
| 5.4 | Component | `Build() Widget`（design.md §4.1）；组件身份（**不在 Build 内定义嵌套类型/生成 key** —— React 教训）。 |

**交付物**：淡入淡出、主题切换、异步加载 demo。
**验收**：60fps 动画不冻结 UI；切换主题无闪烁；async 回调安全落地。

---

## Phase 6 — 列表与虚拟化 · 目标：大数据

| # | 子任务 | 要点 / 参考 |
|---|---|---|
| 6.1 | ListView + key | `ListView(Items, Builder)` + 稳定 key（D3）；控件池按 key 复用，重排不重建。 |
| 6.2 | **虚拟化** | 10 万行：a) FluxVCL 控件池虚拟化（可见区 N 个控件复用）；b) 或嵌入 `TListView.OwnerData=true` + `OnData/OnDataHint`（**绝不用 `Items.Add()`**）。 |
| 6.3 | 多窗口 | 第二个 Window；独立 State 作用域。 |

**交付物**：10 万条数据流畅滚动 demo。
**验收**：滚动流畅、行内控件焦点/IME 不漂移、内存有界。
**风险**：窗口句柄上限 —— 虚拟化是硬要求，非可选优化。

---

## Phase 7 — 工程化与生态 · 目标：可用、可信、可发布

| # | 子任务 | 要点 / 参考 |
|---|---|---|
| 7.1 | Inspector | Widget 树/属性/布局调试/事件查看（design.md §18）；在 mutation 层挂钩统计每次提交的 create/update/destroy/patch 数，**高亮任何重建**（重建=焦点丢失信号）。 |
| 7.2 | 插件系统 | `RegisterWidget("Chart", ChartWidget)`（design.md §19）。 |
| 7.3 | 测试与 CI 强化 | 无头逻辑测试（D7 覆盖全量控件）；Windows 冒烟 + 截图 artifact；性能基准（控件创建/更新耗时）。 |
| 7.4 | 打包 | 安装器（NSIS/WiX `File` 放 DLL）；DPI manifest/版本信息（go-winres）；单 EXE 方案评估。 |
| 7.5 | **产品化** | 中英双语文档；`examples/*` 全量示例；**首发带截图**；7GUIs 完整性演示；公开维护政策（回应弃坑担忧）；明确的"比 Fyne/Wails 多了什么"对比页。 |
| 7.6 | Accessibility / i18n | 高对比度、键盘导航、UIA（VCL 部分能力）；国际化资源。 |

**交付物**：Inspector 可用的完整框架 + 打包产物 + 文档站。
**验收**：全新用户按 README 5 分钟跑通示例；CI 全绿；发布 v0.1.0。

---

## 依赖关系与里程碑

```
P0 ──► P1 ──► P2 ──► P3 ──► P4
        │       │       │
        │       └──►(P2.4 依赖 P1.4 diff 引擎)
        └──►(P3 布局独立于 State，可与 P2 并行推进)
P5 ──► P6 ──► P7
（P5.1 动画依赖 P2 线程调度；P6 虚拟化依赖 P1.4 + P3）
```

**关键路径**：P0 → P1（diff 引擎）→ P2（State）→ P5（动画/主题）→ P7（产品化）。P3（布局）与 P4（事件）可并行。

**里程碑**：
- M1（P0 完）：选型定案 + Hello World + CI 绿。
- M2（P1 完）：可写普通桌面程序（design.md Phase 1 目标达成）。
- M3（P2+P3 完）：状态驱动 + 现代布局（核心价值点成立）。
- M4（P4+P5 完）：完整交互 + 动画/主题（设计目标覆盖）。
- M5（P6 完）：大数据虚拟化。
- M6（P7 完）：v0.1.0 可发布。

---

## 风险登记册

| 风险 | 等级 | 应对 |
|---|---|---|
| energye/lcl libenergy 运行时分发/版本匹配 | 高 | Phase 0 E2 实验锁定"Go tag ↔ 运行时版本 ↔ DLL 文件名"对照表；`libname.LibName` 显式指定；D6 隔离 → 可切 govcl v1.2.10（B 计划） |
| govcl v1.2.10（B 计划）与现代 Go 不兼容 | 中 | 仅当启用真实 VCL 时实验（govcl-vs-lcl.md §7 E3） |
| libvcl DLL 许可/分发（预览-only） | 高 | 写清许可；双路径（自带 DLL 说明 + 源码自编译） |
| diff 引擎正确性（重建/焦点漂移） | 高 | D7 三不变量测试；Inspector 高亮重建 |
| goroutine 碰 UI 崩溃 | 高 | D4 调度器 + 测试 |
| 原生控件测量不准 | 中 | intrinsic 函数 + 一次性实现测量缓存校准 |
| IME/中文边界 | 中 | 限制范围；form 级 OnUTF8KeyPress |
| 社区"Delphi 包装"标签 / 弃坑担忧 | 中 | 卖 Go 声明式叙事；截图/示例/维护政策 |
| 引用伪造数据（SEO 内容农场） | 低 | 只引用一手来源（见 research.md §8.4） |

---

## 附录：各阶段与 design.md 章节映射

| 本计划 | design.md |
|---|---|
| P1 | §4.2 Widget、§4.3 Node Tree、§5 渲染系统、§11 逃逸口 |
| P2 | §8 State、§9 数据绑定、§17 Key |
| P3 | §6 Layout、§6.2 算法 |
| P4 | §10 事件、§12 生命周期 |
| P5 | §13 动画、§14 Theme、§15 异步、§4.1 Component |
| P6 | §16 Virtual List |
| P7 | §18 Inspector、§19 插件、工程化 |
