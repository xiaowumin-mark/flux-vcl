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
>
> **同日追加：0.3/0.4 完成。** `examples/basic`（窗体+文本+按钮+点击）用 go-winres 生成
> `rsrc_windows_amd64.syso`（PerMonitorV2 DPI manifest + 图标 + 版本信息，已从 exe 提取验证）；
> `scripts/build.ps1` 单命令产出 windowsgui exe 并复制 DLL；`scripts/smoke.ps1` 无头冒烟
> （窗口出现→按钮点击生效→干净退出）通过。
>
> **冒烟中发现的重要工程约束**：LCL 的 `TLabel` **无独立 HWND**（自绘在父窗体表面），
> 因此 Win32 冒烟无法读 label 文本，示例改为点击时同步更新按钮 Caption 作可观测信号；
> 这会影响 Phase 3 布局（label 尺寸/重绘）与 Phase 7 inspector 的设计。
>
> **同日追加：0.5/0.6 完成。** CI 工作流 + `scripts/fetch-libenergy.ps1`（designer commit
> `5c4ec54` 锁定 lcl v1.0.3 对应 DLL，`-Force` 实测下载 13MB 成功）；`internal/render`
> 无头测试驱动（Renderer 接口 + Mock + 无显示测试，`go test ./...` 全绿）。

| # | 子任务 | 要点 / 参考 | 状态 |
|---|---|---|---|
| 0.1 | **绑定选型实验** | 在现行 Go 工具链（1.22–1.27）上验证首选 **energye/lcl + libenergy DLL**（构建 + libenergy 获取/版本匹配，见 [govcl-vs-lcl.md](./govcl-vs-lcl.md) §7 E1/E2）；B 计划 govcl v1.2.10（`last-vcl-support`）+ libvcl DLL 仅在需要真实 VCL 时做对照。写最小"窗体+按钮+点击"用例，记录能否构建/运行/DLL 加载。决议已预倾向 LCL，实验用于定案。 | ✅ 完成 |
| 0.2 | **DLL 交付与许可方案** | 确认首选 libenergy（energye/lcl）获取路径与版本 pin（**结论见 [phase0-e2-libenergy-mapping.md](./phase0-e2-libenergy-mapping.md)**：权威来源是 energye/designer 内嵌 zip，非 SourceForge/GitHub Releases；版本锁定 lcl v1.0.3）；B 计划 libvcl.dll/libvclx64.dll 路径与"预览/测试"许可；决策分发方式（exe 旁 vs 构建脚本）。 | ✅ 完成 |
| 0.3 | **构建脚手架** | `GOOS=windows` `CGO_ENABLED=0` `-buildmode=exe` `-ldflags "-H=windowsgui"`；`examples/basic` 用 go-winres 生成 `rsrc_windows_amd64.syso`（PerMonitorV2 manifest + 图标 + 版本信息，命名 `rsrc_windows_<arch>`）；封装 `scripts/build.ps1`（资源生成→构建→DLL 复制）与 `scripts/smoke.ps1`（无头冒烟）；Go 版本策略 `go 1.22`（覆盖 1.22–1.27 工具链）；CI 复用两个脚本（0.5）。 | ✅ 完成 |
| 0.4 | **仓库与模块** | 模块路径 `github.com/xiaowumin-mark/flux-vcl`（git 远程地址）；目录骨架（根包 `flux`、`internal/{widget,diff,render}`、`examples/basic`、`scripts`、`assets`）；`go.mod` 锁 `lcl v1.0.3`；README/许可（MIT）。 | ✅ 完成 |
| 0.5 | **CI 骨架** | GitHub Actions（`.github/workflows/ci.yml`）：windows-latest 上 `go test ./...` + `go vet`；`scripts/fetch-libenergy.ps1` 从 designer 内嵌 zip 取 DLL（锁定 commit `5c4ec54`）；复用 `build.ps1` + `smoke.ps1` 冒烟；截图 artifact（用 PowerShell `CopyFromScreen`，避免引入 `kbinani/screenshot` 依赖污染根 go.mod；无头会话可能黑屏，失败不中断）。 | ✅ 完成 |
| 0.6 | **无头测试驱动雏形** | 参照 Fyne `test` 驱动：`internal/render` 定义 Renderer 窄接口（D6）+ Dioxus 风格 Op 集 + `Mock` renderer；测试不接触 energye/lcl/DLL，任意平台 `go test` 可跑。Phase 1.4 diff 引擎直接在此框架加测试。 | ✅ 完成 |

**交付物**：选型决议文档、可运行的 Hello World、CI 绿。
**验收**：`go build` 单命令产出 exe，双击出窗口、点按钮有反应；CI 冒烟通过。
**风险**：energye/lcl 的 libenergy 运行时分发/版本匹配（预判为最高风险，E2 实验重点）；B 计划 govcl v1.2.10 与现代 Go 不兼容风险次之。两者靠 D6 隔离让切换可控。

---

## Phase 1 — 声明式核心（Widget/Node/Element/diff/Renderer 抽象）· 目标：能写普通桌面程序

> **进展（2026-08-09）：全部完成。** 声明式核心落地：`internal/widget`（Widget/Node/
> Props 有序属性集，D2 属性级 diff 判定）、`internal/diff`（Element 树 + Reconciler：
> D1 canUpdate / D3 稳定 key / 透明容器）、flux 根包（控件构造器 + 占位布局 + App 入口）、
> `internal/native`（energye/lcl Renderer 适配 + 逃逸口）。D7 三不变量测试全绿。
> examples/basic 声明式改造后 build + smoke 端到端通过。
>
> **工程发现/偏差（实现记录）**：
> - **事件回调每次 render 重新绑定**：函数值无法比较相等性，D2 按"属性恒变"处理
>   （React 同款）。D7c"相同树零 mutation"的测试树**不含事件回调**；另设专门用例
>   断言事件重绑但不重建控件。
> - **Column/Row 为透明容器**：不创建原生控件，Element 句柄继承父容器，子控件直接
>   挂到祖父 —— diff 引擎用 `transparentType()` 过滤 Create/SetParent/Destroy。
> - **TextWidth 为占位实现**（`len(text)*8`，Mock 与 LCL 一致）：布局发生在 diff 之前、
>   控件尚未创建，因此测量不依赖控件句柄；Phase 3 精修为 GDI/主题测量 + 缓存。
> - **布局为占位堆叠**（Column 垂直/Row 水平，gap 4；Window 默认 400x300）：
>   每次 render 重算 Bounds 写入 Props，Phase 3 替换为 Measure/Layout 两遍算法。
> - **`flux.Window` 用 `...any` 变参**混合子节点与窗体 Opt（Title/Width/Height）。
> - LCL 适配层验证了 energye/lcl 的接口签名：`TControl.SetBounds/SetCaption/SetVisible/
>   SetEnabled/SetOnClick`（OnClick 在基类 TControl，非 TButton）、`ICustomEdit.SetText/
>   SetOnChange`、`NewButton/NewLabel/NewEdit(owner IComponent)`。

| # | 子任务 | 要点 / 参考 | 状态 |
|---|---|---|---|
| 1.1 | Widget/Node 数据结构 | `Widget` 接口（design.md §4.2）；Node `{Type, Props, Children}`。 | ✅ 完成 |
| 1.2 | Element 树与 identity | `canUpdate`（D1）；Element 节点持有原生指针 + prevConfig。 | ✅ 完成 |
| 1.3 | Renderer 接口 + Mutation op 集 | Dioxus 风格 op：`AppendChild/SetProperty/SetText/Create/Destroy/SetEvent`（可 mock 测试）；适配层 `internal/native` 实现 energye/lcl 映射。 | ✅ 完成 |
| 1.4 | **diff/reconciliation 引擎** | 全项目最高优先级代码。build 新树 → 按 D1 匹配 → 属性级 patch（D2）→ 批量提交。性能：diff 循环复用 buffer。 | ✅ 完成 |
| 1.5 | 基础控件集 | `Window/Column/Row/Text/Button/Input`；对应原生控件 `TEngForm/TLabel/TButton/TEdit`（默认 LCL；占位布局，Phase 3 精修）。 | ✅ 完成 |
| 1.6 | 原生逃逸口 | `Native(func(btn *lcl.TButton))`（默认 LCL 后端）、`Ref`（design.md §11）。约束：逃逸口改动 Align 须在布局前还原（D5）。 | ✅ 完成 |
| 1.7 | **三不变量测试** | D7 三条测试护栏上线（a/b/c）；flux 层端到端（Mock 断言零重建）。 | ✅ 完成 |

**交付物**：`examples/basic`（窗口+文本+按钮+输入框+点击）。
**验收**：按钮点击改文本/输入框内容，全程零控件重建（渲染器断言 mutation 数）—— 已达成（smoke 点击后 "Clicked 1"，diff 零重建由 flux 端到端测试断言）。
**风险**：diff 正确性 —— 用不变量测试锁死。

---

## Phase 2 — State 系统与数据绑定 · 目标：状态驱动 UI

> **进展（2026-08-09）：全部完成。** State/绑定层落地：`flux.State[T]`（mutex 保护 +
> 订阅 map，`Set/Get` 跨 goroutine 安全）、`flux.Bind(s)` 返回 `Binding[T]`（同时实现
> `bindable` 渲染接口与 `Opt`，支持 `Text/Button(Bind(s))` 单向与 `Input(Bind(s))` 双向）、
> `App.Mount(build)`（持有根构建函数，State 变化自动 re-render）、`App.invalidate`
> （pending 合并 + `RunOnUI` marshal，D4）。依赖收集：render 时 `collectBindings` 遍历
> `_bind` 隐藏 Props key，把绑定订阅到 App（幂等）。
>
> **工程发现/偏差（实现记录）**：
> - **作用域失效 = 全树 diff + 未变子树跳过（等价 D7c）**：2.4 的"只 re-diff 依赖子树"
>   以最小实现落地 —— 每次 render 全树 build+diff，diff 引擎按 D1/D2 天然跳过未变
>   子树（零 Create/Destroy、零属性 mutation），`TestStateScopeInvalidation` 断言只
>   patch 受影响的 SetText 一条。比 getter 级依赖跟踪简单且同样满足验收。
> - **`_bind` 隐藏 key 不产生 mutation**：Binding 值经 `reflect.DeepEqual` 比较，同
>   State 的两个 Binding（`state` 字段同指针）判等，diff 恒跳过。
> - **并发 Set 的正确性**：`State.Set` 先提交值再 `invalidate`，故被 pending 合并吞掉的
>   Set，其新值仍被本次 render 读到（render 时 `Get` 当前值），不丢最后一次写入；
>   `App.renderMu` 串行化 reconcile（mock `RunOnUI` 同步内联，无 UI 线程可串行）。
>   `TestStateSetFromGoroutine` 5 个 goroutine 并发 Set 在 `-race` 下通过。
> - **合并更新**：`pending` 脏标志使同一周期多次 Set 只触发一次 render（`TestStateInvalidateMerge`
>   用延迟 `RunOnUI` 制造同周期窗口断言）。
> - **`examples/basic` 改为 State 驱动**：counter（`Button(Bind(count))` 文本随 State
>   刷新）+ two-way（`Input(Bind(name))` ↔ `Text(Bind(name))` 回显）；smoke 断言按钮
>   文本 "0"→"1"（点击 +1）。

| # | 子任务 | 要点 / 参考 | 状态 |
|---|---|---|---|
| 2.1 | `State[T]` 原语 | `NewState(initial)` / `count.Set(v)` / `count.Get()`；mutex 保护 + 订阅 map（参照 Gova `State.Set()→re-render`、Compose 快照）。 | ✅ 完成 |
| 2.2 | 单向绑定 | `Text(Bind(user.Name))`：渲染取当前值，State 变化→re-render→属性 patch。 | ✅ 完成 |
| 2.3 | 双向绑定 | `Input(Bind(user.Name))`：`OnChange` 回写 State→re-render→控件文本同步。string/int 类型转换，其余类型仅单向。 | ✅ 完成 |
| 2.4 | **作用域失效** | 以"全树 diff + 未变子树跳过"落地（等价 D7c）：未变子树零 mutation，只 patch 受影响子树（测试断言 SetText 恰一条且指向该句柄）。 | ✅ 完成 |
| 2.5 | **线程 marshalling** | Renderer 接口加 `RunOnUI`；native 用 `CurrentThreadId==MainThreadId` 检查 + `lcl.RunOnMainThreadSync`（阻塞）；Mock 同步内联。`App.invalidate` pending 合并（D4）。 | ✅ 完成 |
| 2.6 | Key 系统 | 列表 key（D3）机制已在 Phase 1 diff 引擎落地（按 key 匹配复用）；State 场景直接复用。 | ✅ 完成 |

**交付物**：计数器 demo、输入双向同步 demo —— 已达成（examples/basic）。
**验收**：外部 goroutine 改 State 不崩溃（`TestStateSetFromGoroutine`，-race）、UI 正确刷新
（smoke "0"→"1"）；只重建受影响子树（`TestStateScopeInvalidation` 断言）。
**风险**：goroutine 改 UI 崩溃 —— D4 RunOnUI + `renderMu` 串行化 + 测试覆盖。

---

## Phase 3 — 布局引擎 · 目标：现代布局

> **进展（2026-08-09）：布局核心完成（3.1–3.4）；3.5 DPI 完成；3.6 滚动 / 3.7 inspector 完成，Phase 3 全部收尾。**
> 落地：`box.go`（BoxConstraints/Size/Point/对齐枚举，全 DIP）、`layout.go` 单遍 RenderFlex
> 重写（Expanded=Tight/Flexible=Loose、freeSpace 分配、主轴 spaceBetween/Around/Evenly、
> 交叉轴 Start/Center/End/Stretch、只增不缩 + 溢出诊断）、`TextExtent` GDI 测量替换占位
> （共享 bitmap canvas + `TextExtentWithStr` + 缓存）、Window resize 即时更新
> （`OnResize→invalidate→re-render`，零控件重建）。3.6 滚动容器 `ScrollBox`（SingleChildScroll
> 语义，滚动轴 unbounded 测量 + 原生 TScrollBox AutoScroll 滚动条）。3.7 inspector 数据
> 源 `App.Inspect()`（全节点 constraints/size/frame/flex）。`examples/layout` demo：resize 时
> 左右面板 1:2 即时重分割，左面板滚动列表；冒烟断言按钮 0→1 + 干净退出。
>
> **工程发现/偏差（实现记录）**：
> - **单遍 RenderFlex，非 Measure/Layout 两遍**（design.md §6.2 的两遍在交互/动画场景
>   才有区分价值，本轮统一为单遍：约束下传同时量出尺寸写 Bounds）。
> - **Window 是布局根，子控件收到"有界主轴约束"**（`layoutRoot`）：非 flex 子惯用
>   unbounded 主轴测量，但布局根必须给内容一个有界盒子（Flutter Scaffold 语义）——
>   否则根 Column 在 unbounded 主轴下 `freeSpace=0`，内部 Expanded 全部被压成 0。
>   这是嵌套 flex + Expanded 能工作的前提。
> - **`TextWidth`→`TextExtent(text)(w,h)`** 替换占位：布局在 diff 前执行、控件未创建，
>   测量用共享 1x1 bitmap canvas + 窗体默认字体（`SetFontToFont(form.Font())`），按文本
>   缓存；`w<=0` 兜底 `len*8`、`h<=0` 兜底 20。3.5 DPI/字体变化时需失效缓存。
> - **Window 尺寸取自渲染器客户区**（`ClientSize`，native 默认 640x480、mock 400x300），
>   不再固定 400x300；`NewRenderer` 设默认窗体尺寸。
> - **diff Bounds 修复（resize 后必暴露的 bug）**：透明容器/Window 的 Bounds 只用于定位
>   与诊断，`applyProp` "Bounds" case 对 `transparentType(e.Type) || e.Type=="Window"`
>   跳过 —— 否则透明节点（Handle 继承父）的 Bounds 会 `SetBounds` 到父容器句柄把父控件
>   搬走 / 把窗体边框收缩。
> - **溢出诊断钩子**：`layoutDiags` 收集 `App.LastLayoutDiags()`（Type/Key/OverflowW/H），
>   本轮供测试断言，3.7 inspector 在其上做溢出提示 UI。
> - **DPI 换算收在 native 边界，接口保持 DIP**（`internal/render/dip.go` 纯函数，
>   `DIPToPX`/`PXToDIP` 四舍五入、`math.Round` 远离零语义，与 Win32 MulDiv 一致）：布局
>   引擎与 Renderer 接口（ClientSize/TextExtent/SetBounds）零改动，mock/既有测试全绿。
> - **DPI 源 fallback 链**：`GetDpiForWindow`（user32 syscall，perMonitorV2 下返回窗口
>   所在显示器真实 DPI）→ `Monitor().PixelsPerInch()` → 96；结果缓存，`WM_DPICHANGED`
>   时 `invalidateDPI` 清零强制重查。
> - **WM_DPICHANGED 钩子**：`SetOnWndProc` 中先 `InheritedWndProc(msg)` 放行 LCL 默认
>   （窗体按建议矩形 resize、字体随 widgetset 缩放；InheritedWndProc 走 Pascal 父类，
>   不会递归），再清 DPI + 文本测量缓存并 `emitResize` 触发全量 re-layout。
>   字体策略：不调 `ScaleForPPI`、不改 `Application.Scaled` —— 需手动 125%/150% 验证。
> - **TextExtent 显示器无关**：bitmap DC 的 DPI（`GetDeviceCaps(LOGPIXELSX=88)`）进程内
>   固定、缓存一次；测量到的物理像素经 `PXToDIP` 归一化为 DIP，`measureCache` 无需随
>   显示器 DPI 失效（仅字体随 DPI 变化时清空，钩子统一处理）。
> - **demo 加 DPI 读数**：底部 Text 绑 State，后台 goroutine 每秒经 `RunOnUI` 读
>   `r.DPI()`（UI 线程纪律），变化时跨线程 Set → re-render 自动 marshal（Phase 2
>   marshalling dogfood）。
>
> **3.6/3.7 工程发现（实现记录）**：
> - **真实容器的局部坐标空间**：ScrollBox 是第二个"真实容器"（有原生句柄）后，透明容器
>   链"窗体绝对坐标"假设被打破。坐标规则：真实容器（Window/ScrollBox）子树坐标相对自身
>   客户区（局部，原生 `SetBounds` 相对父）；透明容器（Column/Row/Expanded/Flexible，
>   diff 不建句柄、子挂祖父）子树为窗体绝对坐标。`setPos` 只定位真实容器自身 Bounds
>   （不平移子树），`offsetSubtree` 在真实容器边界停止下钻。`layoutScrollBox` 内容用
>   `Point{}` 局部原点布局。
> - **滚动语义（SingleChildScrollView）**：单子内容用 `{交叉轴 0..crossMax, 滚动轴(高)
>   unbounded}` 约束测量 → 内容总高；自身 = viewport = `c.Constrain(内容)`：内容超高被
>   钳制出现原生滚动条、内容偏矮收缩到内容（自适应）。滚动轴溢出不记溢出诊断（滚动是
>   目的），交叉轴溢出记。已知限制：滚动内容内 `Expanded` 在 unbounded 主轴下被压 0
>   （Flutter 同需 IntrinsicHeight）—— demo 滚动内容只用非 flex 行。
> - **TScrollBox 配置**：`NewScrollBox(form)` + `SetAutoScroll(true)`（LCL 按子包围盒
>   自动算滚动范围、滚动条自动出现）+ `SetDoubleBuffered(true)`（防闪烁）。无
>   OnScrollViewChanged 事件（滚动条位置回写需轮询/钩 WM_VSCROLL，MVP 不做）。
>   `WM_SETREDRAW` 批量防闪烁留 Phase 5 虚拟化。
> - **NodeDiag 的 Frame 时机**：`record` 在布局递归内执行，早于父容器 `setPos` 平移子树，
>   故 `NodeDiag.Frame` 留空、由 `finalize(root)` 在整棵布局完成后后序回填（与 record
>   同序），与 diff 应用的一致 Bounds。
> - **关机竞态（0xC0000005，ScrollBox+DPI goroutine 触发）**：后台 goroutine 的
>   `lcl.RunOnMainThreadSync` 与窗体 teardown 竞争，在 `Application.Run()` 内间歇崩溃
>   （纯 LCL 最小复现不崩、FluxVCL 集成层崩、加探针即消失 → heisenbug 竞态；50ms tick
>   压力下必现、修复后 8/8 干净）。修复（纵深防御）：`Renderer.RunOnUI` 在窗体
>   `OnClose` 置 `closed` 门后丢弃后台线程任务（不再产生 DLL sync 调用），但 UI 主线程
>   回调仍内联执行，以便完成 `App.Close` 清理；
>   `emitResize` 同门控；demo 用 `r.OnClose` 关闭 `done` 通道停止后台轮询 goroutine。
>   教训：LCL 对象只在主线程访问是必要不充分 —— 关机期间连"主线程同步调用"本身都要避免。

| # | 子任务 | 要点 / 参考 | 状态 |
|---|---|---|---|
| 3.1 | 协议 | `BoxConstraints`/`Size`；`Measure`/`Layout` 两遍（design.md §6.2）。 | ✅ 完成（单遍） |
| 3.2 | **intrinsic-size 函数** | `Size Measure(font, text, dpi, constraints)`；GDI 文本测量（`TCanvas.TextWidth/TextHeight/TextExtent`）；主题 API（`BCM_GETIDEALSIZE`/`GetThemePartSize`）一次实现测量+缓存；缓存失效（文本/字体/DPI 变化）。 | ✅ 完成（GDI 测量+缓存；主题 API 待 3.5） |
| 3.3 | Flex 算法 | RenderFlex 精确实现：非 flex 主轴 unbounded、freeSpace/flex 分配、Expanded=tight/Flexible=loose、主轴对齐分布、只增不缩+溢出诊断。 | ✅ 完成 |
| 3.4 | 定位应用 | `SetBounds` 写 frame；框架控件 `Align=alNone`（D5）；逃逸口 Align 还原。 | ✅ 完成（Bounds 写 Props，diff 应用；逃逸口 Align 还原待 3.5 校量） |
| 3.5 | **DPI** | PerMonitorV2 manifest（已就位）；DIP→像素换算（`render.DIPToPX/PXToDIP`）；`WM_DPICHANGED` 钩子（先 `InheritedWndProc` 放行再清缓存 + 全量 re-layout）；字体策略：不调 `ScaleForPPI`、不改 `Application.Scaled`（测量归一化自洽）。 | ✅ 完成 |
| 3.6 | 滚动容器 | 滚动轴 unbounded 约束；TScrollBox 原生滚动 + DoubleBuffered 防闪烁（`WM_SETREDRAW` 留 Phase 5 虚拟化）。 | ✅ 完成 |
| 3.7 | 布局调试 | inspector 预留：节点 constraints/size/frame/flex 因子、溢出提示。 | ✅ 完成（`App.Inspect()` 全节点 + `App.LastLayoutDiags` 溢出） |

**交付物**：表单布局、可伸缩面板、滚动列表、高分屏 demo。
**验收**：Row/Column 比例 flex 正确；改变窗口尺寸布局即时更新且无闪烁；125%/150% 缩放文字不糊（3.5 完成换算与钩子，冒烟回归通过；125%/150% 观感待手动验证，demo 底部有 DPI 读数）。滚动列表可拖滚动条、内容超高滚动、resize 后滚动范围即时更新、干净退出。
**风险**：测量与真实渲染尺寸不一致（字体匹配、主题 padding）—— 用"隐藏实现一次性测量+缓存"校准。TScrollBox 原生滚动无法 headless 验证 —— 冒烟回归 + 手动验证兜底。

---

## Phase 4 — 事件系统与生命周期 · 目标：统一交互

> **进展（2026-08-09）：全部完成。** 统一事件模型落地：`internal/render/event.go`
> `Event{Type,X,Y,Key,Text,Button,Mods,Source}`（全 DIP），flux 根包以 `type Event = render.Event`
> 别名 + 事件/生命周期 Opt（`OnClick/OnMouse*/OnKey*/OnPress` 收 `func(Event)`，
> `OnMount/OnUpdate/OnUnmount` 收 `func()`）。native 边界把 LCL 鼠标/键盘事件映射为
> 统一事件并做 DIP 归一（`mouseEvent/mapButton/mapShift` 纯函数可单测）；diff 引擎对
> `func(render.Event)` 回调注入稳定 `Source="Type#Key"`（D3）后转发，共享 handler 可区分来源。
> 生命周期：`OnMount`（子树挂载完成后，父后于子）`/OnUpdate`（Flutter didUpdateWidget 语义，
> 仅真实属性变化）`/OnUnmount`（物理释放前）；卸载控件入队延后销毁（D4，
> `App.DrainDestroy` 在 render 后统一 Free）。中文输入（4.4）：energye/lcl v1.0.3 的
> `SetOnUTF8KeyPress` 在 **TWinControl** 上可用（非 TForm-only，计划原担忧不成立），控件级
> 逐字符路由，含 IME 组合结果。`examples/events` demo 全链路演示 + CI 冒烟（click 0→1）通过。
>
> **工程发现/偏差（实现记录）**：
> - **统一事件放 `internal/render`，不是 flux**：`Event` 需被 native 适配层构造（跨包），
>   放 render 后 flux 以 `type Event = render.Event` 别名转发，`func(flux.Event)` 与
>   `func(render.Event)` 同一类型（别名不产生新类型），diff 注入与用户签名天然一致。
> - **LCL 的 `IControl` 只声明 `SetOnClick`**：鼠标/键盘 setter（`SetOnMouseDown`、
>   `SetOnKeyDown`、`SetOnUTF8KeyPress`…）是**具体类型**（TButton/TEdit/TScrollBox）的方法，
>   不在 IControl 接口上 → native 适配层用两个结构接口（`mouseEvents`/`keyEvents`）做
>   类型断言，控件类型不支持的事件静默忽略（不 panic）。
> - **DIP 归一收在 native 边界，接口保持 DIP**：`render.Event.X/Y` 是 DIP；适配层拿到
>   原生像素坐标后经 `PXToDIP` 换算（与 Phase 3.5 同一换算，`render.PXToDIP`）。mock
>   测试用 144 DPI 场景断言 144px→96DIP。
> - **生命周期钩子是函数值，diff 恒判变化**：为保 D7c"相同树零 mutation"，`applyProp`
>   对 `OnMount/OnUpdate/OnUnmount` 显式跳过（不落 SetEvent），由
>   `mount/reconcile/destroySubtree` 显式触发；`patchProps` 返回"真实属性变化"bool
>   （值非函数且非 `_bind`/生命周期键），`OnUpdate` 只在真实变化时触发 —— Flutter
>   didUpdateWidget 语义，避免"每次 render 都回调 → 钩子里 Set State → 无限 re-render"。
> - **重入 render 死锁（Phase 4.3 最关键的工程发现）**：`OnMount` 等钩子在 reconcile 内
>   触发，若钩子回调里 `State.Set` → `invalidate` → `RunOnUI`（mock 内联 / 主线程同步）
>   → `renderWidget` 重入当前栈：非重入 `renderMu` **自锁** + 无限递归。修复：App 增加
>   `inRender` 重入守卫 —— 重入调用只置 `pending` 返回，由当前 render 结束时的
>   `finishRender` 统一 flush 一次（D4 合并更新，递归变尾调）。测试
>   `TestStateSetInsideLifecycleNoDeadlock` 用 `Mount`（有 build）验证收敛。
> - **生命周期钩子内 Set State 测试必须走 `Mount` 而非 `Render`**：`Render` 是单树手动
>   路径（`App.build==nil`），flush 为 no-op —— State 自动更新只存在于 `Mount` 路径
>   （`Render` 文档本就注明"不触发 State 自动更新"）。
> - **`OnUpdate` 仅在真实属性变化触发**（同上）：`examples/events` 里按钮 OnUpdate 靠
>   `count.Set` 驱动（点击 +1，文本真变）；hover 去重（同坐标跳过 Set）避免状态栏文本
>   无谓刷新。挂载时序：`OnMount` 在子树挂载完成后触发（父后于子），钩子可访问完整子树。
> - **`OnUTF8KeyPress` 的 Text 是 `*string`（UTF-8 逐字符，含 IME 组合结果）**：绑定层
>   `SetOnUTF8KeyPress` 收到字符串直接存入 `Event.Text`；`TKeyDown/KeyUp` 的 `*uint16`
>   虚拟键码存入 `Event.Key`。中文输入路径独立于虚拟键码 —— 4.4 落地为
>   "TEdit 原生 IME（组合窗/候选）→ `OnUTF8KeyPress` 逐字符路由"，无需自定义 IMM。

| # | 子任务 | 要点 / 参考 | 状态 |
|---|---|---|---|
| 4.1 | 统一事件 | `Event{Type,X,Y,Key,Text,Button,Mods,Source}`（design.md §10）；**显式回调注册**（D6，禁反射）。 | ✅ 完成（`internal/render/event.go` + flux 别名 + `flux/event_opts.go`） |
| 4.2 | Mouse/Keyboard 映射 | 原生控件 OnClick/OnMouseDown/OnKeyDown → 统一事件；坐标 DIP 归一（native 边界 `PXToDIP`）。 | ✅ 完成（`internal/native` 结构接口 + `mapping_test.go`） |
| 4.3 | 生命周期 | `OnMount/OnUpdate/OnUnmount`（design.md §12）；卸载时入队销毁（D4）。 | ✅ 完成（diff 触发 + `App.DrainDestroy`） |
| 4.4 | **IME/中文输入** | form 级路由 `OnUTF8KeyPress`（计划担忧 TForm-only；实测 energye/lcl v1.0.3 在 **TWinControl** 上可用，控件级路由即可）。 | ✅ 完成（`SetOnUTF8KeyPress` → `Event.Text`） |

**交付物**：完整交互示例（hover/点击/键盘/焦点/中文 IME）—— 已达成（examples/events）。
**验收**：中文输入正常；事件不阻塞主线程（长时间 handler 自动离屏）；销毁不崩溃。
**风险**：IME 边界（政府已知 bug）—— 限制范围，普通输入用 TMemo/TEdit 能力内（4.4 实测控件级路由可用，风险降级）。

---

## Phase 5 — 高级特性 · 目标：表现力

> **进展（2026-08-09）：全部完成。** 动画（纯逻辑状态机 + 主线程 pump + D2 逃逸口落地）、
> 主题（数据化调色板 + 属性级 patch）、Async（后台 goroutine + RunOnUI marshal）、
> Component（透明分组 + 外部 Key 身份）落地。工程发现：
>
> - **Go 限制：方法不支持泛型** → `Async` 为包级泛型函数 `Async[T](a *App, load, onSuccess, onError…)`
>   （方法泛型会编译失败，design.md §15 的 `Async(Load, OnSuccess)` 以自由函数落地）。
> - **Go 限制：type/func 包级同名冲突** → 颜色类型命名 `ColorValue`（别名 `render.Color`），
>   让出 `Color` 标识符给 Opt 构造器 `Color(th.Primary)`；用户几乎不写类型名（`RGB()`/主题字段即够）。
> - **pump 分层**：`AnimationController` 是纯状态机（elapsed/duration/curve/onStep/onEnd，mutex 保护、
>   可无头测试），**不持有定时器**；`App.Animate` 用主线程 TTimer（~16ms）驱动 Step，回调天然在
>   UI 线程（无 goroutine/marshalling 成本）。动画落地走 `App.SetBounds`（D2 逃逸口，`diff.Lookup`
>   按 key 定位真实控件句柄；透明容器/Window 跳过），**不触发整树 re-diff**。
> - **主题=数据不是运行时对象**：构建函数按当前 Theme 显式传颜色（Color/FontColor Opt）与标题栏
>   暗色（DarkTitleBar Opt）；切换 = State 变 → 全量 re-diff → diff 只 patch 变化的颜色属性
>   （未变子树零 mutation，D7c）。`FontSize/Radius` 为文档字段（native 未接入字体大小/圆角）——
>   `DarkTitleBar` 为**已接入**字段（诚实标注）。
> - **标题栏暗色（win32 DWM）**：`Theme.DarkTitleBar`（Light=false / Dark=true）+ `Window(DarkTitleBar(..))`
>   Opt → diff 属性级 patch → 绑定层 `DwmSetWindowAttribute`（dwmapi.dll，零 CGO syscall；
>   `DWMWA_USE_IMMERSIVE_DARK_MODE` 属性 20，1809 前回退 19）。DWM 即时重绘标题栏，无需
>   Recreate/Redraw —— 继窗体背景、文字色之后主题切换的第三个可见信号。
> - **win32 渲染限制（探针实测）**：LCL `TButton` 由 OS 主题绘制（原生 Win32 按钮），`Color`/
>   `FontColor` 均为空操作 —— LCL 内部状态正确更新、屏幕像素不变；`TSpeedButton`/`TBitBtn` 背景
>   同样不渲染（含显式 Invalidate/Repaint）；`TLabel` 背景 `Color` 也不渲染。主题切换的可见信号
>   实际来自窗体背景（`Window(Color)`）与文字 `FontColor`。按钮支持主题色需 owner-draw 改造
>   （设计路径见 design.md §14），本轮**接受限制**暂不实现 —— 按钮保持系统默认外观（demo 中已移除
>   按钮上的无效颜色属性，避免误导）。
> - **Component 透明化**：与 Column/Row/Expanded 同为透明分组节点（无原生控件、Element 句柄继承
>   父、子挂祖父），diff/layout 各加 "Component" 分支。身份靠**外部** `Key`（D3）：Component 接受
>   `opts ...Opt`（典型仅 `Key("card")`），子控件按 key 跨 render 原地 patch 不重建。

| # | 子任务 | 要点 / 参考 | 状态 |
|---|---|---|---|
| 5.1 | **动画** | 主线程 60fps（TTimer/自定义 pump）；Curve（Linear/EaseIn/Out/InOut/ElasticOut）/Tween/AnimationController（0..1 状态机，不持定时器）；高频属性用**直接绑定**（`App.SetBounds` D2 逃逸口）避免整树 re-diff。 | ✅ 完成（`flux/animation.go` + `App.Animate/SetBounds`；`flux/phase5_test.go` Mock FireTimer 驱动 pump） |
| 5.2 | Theme | `Theme{Font,Color,Radius,Animation}`（design.md §14）；Light/Dark；标题栏沉浸式暗色（`DarkTitleBar` → DWM）；主题切换=全量 re-diff（重入 diff 引擎，只 patch 变化颜色）。 | ✅ 完成（`flux/theme.go`：`ColorValue`/`RGB`/`Theme`/Light/Dark + `Color`/`FontColor`/`DarkTitleBar` Opt + diff 分发；`DarkTitleBar` 已接入 native，FontSize/Radius 为文档字段） |
| 5.3 | Async | `Async(Load, OnSuccess)`（design.md §15）：后台 goroutine + `RunOnUI` marshalling（D4）。 | ✅ 完成（包级泛型 `Async[T]`；失败走 onError 可选回调） |
| 5.4 | Component | `Build() Widget`（design.md §4.1）；组件身份（**不在 Build 内定义嵌套类型/生成 key** —— React 教训，Key 由外部经 opts 传入）。 | ✅ 完成（`flux.Component` 透明分组 + diff/layout "Component" 分支） |

**交付物**：`examples/phase5` —— 点击按钮：计数（冒烟目标）+ ElasticOut 方块滑动（App.SetBounds 逐帧直接落地）+ 500ms 异步加载（RunOnUI 回 UI 线程）；点击"主题" chip 切换 Light/Dark（State → 全量 re-diff）。
**验收**：60fps 动画不冻结 UI（无整树 re-diff，SetBounds 逃逸口；`go test -race` 全绿）；切换主题无闪烁（只 patch 变化颜色，未变子树零 mutation）；async 回调安全落地（D4 + `closed` 门，关机竞态防护复用 Phase 3.6）—— **已达成**：`go test ./...`（含 `-race`）全绿，`smoke.ps1 -Target phase5` PASS（按钮 0→1），主题 chip 点击 light→dark→light 经探针 marker 验证。
> **验收结果（已接受限制）**：主题切换的**可见**验证于窗体背景（亮 F5F5F5 / 暗 121212）、文字
> `FontColor`、主题 chip（light/dark 文字色切换）与标题栏（`DarkTitleBar` → DWM 沉浸式暗色，
> 亮/暗标题栏随主题切换）；**按钮保持系统默认外观** —— win32 后端 TButton
> 由 OS 主题绘制，`Color`/`FontColor` 不渲染（探针实测内部状态正确、像素不变），此为后端能力限制而非
> 框架缺陷。支持按钮主题色需 owner-draw 改造（design.md §14），本轮**接受限制**，不阻塞 Phase 5 验收。

---

## Phase 6 — 列表与虚拟化 · 目标：大数据

> **进展（2026-08-09）：全部完成。** `ListView` 虚拟滚动列表（控件池虚拟化 +
> 稳定 slot key）+ 滚动双向绑定 + 第二个窗体落地。工程发现：
>
> - **控件池 = slot key，不是数据 index**：布局只把"可见区 ± overscan"的行构建为
>   slot 子节点，key = `row-0..row-N`（**槽位**身份，D3）—— 滚动时同一批槽位跨
>   render 复用（原生控件不重建、焦点/IME 不漂移），内容随槽内 `builder(index)`
>   原地 patch（SetText/SetBounds）。10 万行也只建 ~20 个原生控件（内存有界）。
>   行内容（builder 产物）**不得**带数据依赖 key（否则滚动换内容时重建，破坏池）。
> - **`Builder` func 不可比**：每次 render 新函数 → Props 恒判不等 → diff 需显式
>   ignore `"ItemCount"/"ItemHeight"/"Builder"`（漏过 default 会误走 SetEvent panic）。
>   相同树仍零 mutation（Builder 不计入 changed，不触发 OnUpdate）。
> - **D7c 与 OnScroll 重绑的解法 = 值类型 scrollTarget**：`scrollTarget{s *State[int]}`
>   是值类型 → Props 值可比（reflect.DeepEqual 对同一 State 指针为真）→ `Scroll`
>   属性跨 render 不产生 diff → OnScroll 只绑一次，零 mutation。
> - **滚动位置钳制回写 State**：布局读 `ScrollTarget.Current()` → 钳到
>   `[0, 内容−视口]` → 实际变化时 `Apply` 回写（触发一次 re-render 收敛）——
>   `scroll.Get()` 与原生滚动条读数永不漂移；值已合法时零 Apply（无额外 render）。
> - **D6 滚动窄接口**：`render.Scrollable`（SetScrollConfig/SetScrollPos/OnScroll）为
>   diff 层与绑定层间唯一知识点；native 以 TScrollBox 视口（AutoScroll=false、隐藏
>   内建双滚动条、DoubleBuffered）+ 内部 TScrollBar 实现（滚动条范围 = 内容−视口，
>   页尺寸 = 视口高，全 DIP）；Mock 无头实现供测试 FireScroll 驱动。
> - **6.3 多窗口 = 第二个 Renderer/App**：`NewRenderer()` 再注册一个
>   `Application.NewForms`（首个=主窗体），次要窗体须显式 `Show()`（native.Renderer 新
>   增 `Show()` 方法）；第二窗体独立 `flux.NewApp` + 独立 State → 各自触发各自
>   re-render。主窗体关闭 → `Application.Run` 退出（含第二窗体打开时），进程干净退出。

| # | 子任务 | 要点 / 参考 | 状态 |
|---|---|---|---|
| 6.1 | ListView + key | `ListView(Items, Builder)` + 稳定 key（D3）；控件池按 key 复用，重排不重建。 | ✅ 完成（`flux.ListView(count, itemH, builder, ScrollOffset(scroll))`；slot key=`row-i` 控件池；`list_test.go` 验证槽位复用 + D7c 零 mutation） |
| 6.2 | **虚拟化** | 10 万行：a) FluxVCL 控件池虚拟化（可见区 N 个控件复用）；b) 或嵌入 `TListView.OwnerData=true` + `OnData/OnDataHint`（**绝不用 `Items.Add()`**）。 | ✅ 完成（控件池方案；`layoutListView` 只建可见区±overscan 槽位；滚轮/滚动条拖动 → OnScroll → State → re-render，属性 patch 不重建） |
| 6.3 | 多窗口 | 第二个 Window；独立 State 作用域。 | ✅ 完成（第二个 `NewRenderer`/`NewApp` + `r2.Show()`；`examples/virtual-list` 启动即开第二窗体，计数互不相干） |

**交付物**：`examples/virtual-list` —— 10 万行虚拟列表（行号 + 内容 + 可点击选中标记），
头部实时滚动位置读数（Bind(scroll)），"滚到顶/底/选中第 50000 行" 编程滚动（点击 Text，
非 Button 类，不扰冒烟），启动即开第二窗体。
**验收**：滚动流畅（滚动 = 属性 patch，控件池复用，内存有界）、行内控件焦点/IME 不漂移
（D3 槽位身份，D7b）、内存有界（10 万行只建 ~20 控件）—— **已达成**：`go test ./...`
（含 `-race`）全绿；`smoke.ps1 -Target virtual-list` PASS（按钮 0→1、双窗体干净退出）；
`bin/virtual-list-smoke.png` 截图 506 色（内容非空）。
**风险**：窗口句柄上限 —— 虚拟化是硬要求，非可选优化（本轮以控件池虚拟化落实）。

---

## 控件扩充批次 1 — 常用表单基线 · 目标：补齐最小可用表单能力

> **状态（2026-08-11）：`Memo`、`CheckBox`、`ComboBox`、`ProgressBar` 与 `RadioButton` 已完成公开 API、D6 窄能力、属性对称 diff、布局、Mock/LCL 适配和无头测试；`examples/form-controls` 提供聚合人工验证与唯一数字 Button smoke 信号。`ComboBox` 采用 `[]string` Items、受控 `SelectedIndex` 与 `OnSelectionChange(func(int))`，不扩张 `Bind`；`ProgressBar` 固定为 `Minimum/Maximum/Value` 范围模型；`RadioButton` 由 native Renderer 按 resolved native parent + `GroupIndex` 维护逻辑互斥，并以逐控件内部 host 隔离 LCL 原生互斥范围。** 本批次位于 P6 与 P7 之间，
> 是 P7.3 全控件 D7 覆盖和 P7.5 7GUIs 示例的前置准备，不构成无限制的控件库扩张。
> 实现严格按下列顺序推进，并在整批完成后新增一个聚合 example 供人工验证；在此之前不以
> “已有 LCL 原生控件”宣称 FluxVCL 已支持对应控件。
>
> **自动验收（2026-08-11）**：`gofmt`、`git diff --check`、`go vet ./...`、
> `go test -count=1 ./...`、`go test -race -count=1 ./...`、
> `scripts/build.ps1 -Target form-controls` 与 `scripts/smoke.ps1 -Target form-controls`
> 均通过；smoke 验证唯一 Button `0 → 1` 且进程干净退出。剩余项仅为使用者对
> Memo/IME、CheckBox、ComboBox、ProgressBar 与 RadioButton 的人工交互确认。

### 范围与裁剪规则

| 顺序 | 控件 | 最小目标 | 优先级 |
|---|---|---|---|
| 1 | `Memo` | 多行可编辑文本、文本属性与 `OnChange`；明确默认 intrinsic 尺寸，不承诺自动换行/富文本布局。 | 核心 |
| 2 | `CheckBox` | Caption、`Checked` 值属性与 `OnCheckedChange(func(bool))`；不扩张 `Bind` 的布尔双向绑定语义。 | 核心 |
| 3 | `ComboBox` | `[]string` Items、选中索引、选择变更与值类型 State 绑定；不引入富数据源。 | 核心 |
| 4 | `ProgressBar` | 最小 `Minimum/Maximum/Value` 范围模型与确定性进度显示。 | 已完成 |
| 5 | `RadioButton` | Caption、`Checked` 与最小组语义（`GroupIndex`）。 | 已完成 |

- 时间或验证成本不足时，允许在文档中明确裁掉 **ProgressBar / RadioButton**；不得裁掉
  `Memo / CheckBox / ComboBox` 后仍把本批标记为完成。
- `TabControl/PageControl`（真实容器与每页子树语义）、`Canvas/PaintBox`（Painter/自绘机制）、
  `StringGrid`（native `TStringGrid`，单元格编辑/数据模型）、`Slider`（范围/拖拽交互模型）留到 P7 的插件、
  组件或 7GUIs 专项设计阶段，不能借本批次顺带实现。

### 统一实现与验收矩阵

每一个纳入本批次的控件必须同时完成以下项目；只完成 native `Create` 不算支持该控件：

1. **公开 API**：在 `controls.go` 增加构造器，在 `opts.go` 增加最小、类型明确的 Opt；仅当
   现有统一事件无法表达时，才在 `event_opts.go` 增加对应事件 API。
2. **D6 隔离**：控件专属属性不得持续膨胀 `render.Renderer` 主接口；参照
   `render.Scrollable` 建立可选能力接口，由 native 与 Mock 共同实现，diff 通过类型断言调用。
3. **属性对称性**：在 `internal/diff/diff.go` 同时实现 `applyProp` 与 `applyRemoved`；撤掉
   `Checked`、Items、选择索引、范围或进度等 Opt 后，原生状态必须回到文档化的默认值，不能残留。
4. **布局**：在 `layout.go` 为每个叶子控件定义 intrinsic 尺寸与 `Width/Height` 覆盖规则；
   `Memo` 采用明确的多行默认尺寸策略，而非伪造已支持的富文本或自动高度测量。
5. **无头测试**：扩展 Mock 的能力状态/操作记录；覆盖首次挂载、纯属性 patch 不重建、属性移除重置、
   相同值树零 mutation（D7c）、状态回写及不支持能力的安全降级。
6. **LCL 适配与 Windows 验证**：在 `internal/native` 完成构造、属性、事件和解除事件绑定；
   以真实 `energye/lcl` API 编译验证，并跑 build/smoke。OS 主题不渲染的颜色或字体色限制须如实记录。
7. **文档与示例**：同步 `design.md`、README 的控件清单/限制；整批结束后新增聚合 example，
   让使用者手工验证输入、切换、选择、进度与单选组行为。

### API 与数据模型边界

- `ComboBox` 的 Items 固定为 `[]string`，选择状态固定为索引值；本批不做对象 Items、显示字段、
  异步数据源、可编辑 ComboBox 或插件级 adapter。空 Items 的选中索引为 `-1`；非空时索引钳制到
  `[-1, len(Items)-1]`，其中 `-1` 明确表示未选择。传入 Items 时须防御性复制，避免调用方后续修改
  slice 绕开 diff。
- `ProgressBar` 使用后台无关的范围不变量：`Minimum <= Maximum`，`Value` 钳制在该闭区间；默认值为
  `Minimum=0`、`Maximum=100`、`Value=0`。实现应在 Flux/diff 层统一规范化，不把后端差异暴露给用户。
- 新的绑定目标必须使用稳定的值类型包装，遵守 P2/P6 的 `reflect.DeepEqual` 语义；不得因每次
  render 新建 slice、闭包或临时对象而破坏 D7c。本批**不扩张既有 `Bind` 的隐式语义**：布尔勾选与
  选择索引由 `Checked`/`SelectedIndex` 和类型化回调配合 `State.Set` 显式维护；若后续需要双向绑定，
  必须另行设计 `BindChecked`/`BindSelection` 等专用 API，不能临时重载。
- `OnChange(func(string))` 保持文本专用，仅用于 `Input`/`Memo`；`CheckBox` 与 `RadioButton` 使用
  `OnCheckedChange(func(bool))`，`ComboBox` 使用 `OnSelectionChange(func(int))`。这些回调均须有
  diff 包装、native 接线及移除时的 nil 解绑，不得让未知函数类型落入通用 `SetEvent` 分支。
- D6 可选能力接口为 `Checkable`（`SetChecked`/`OnCheckedChange`）、`Selectable`
  （`SetItems`/`SetSelectedIndex`/`OnSelectionChange`）、`Progressable`
  （`SetMinimum`/`SetMaximum`/`SetValue`）及 `RadioGroupable`（`SetGroupIndex`）；基础
  `Renderer` 保持不变，Mock 与 native 必须同时实现。
- `ComboBox` 不能直接复用当前仅断言 `lcl.ICustomEdit` 的 `OnChange` 接线；必须使用经编译核实的
  ComboBox 专用接口/事件分支，并测试 nil 回调解除绑定，避免运行时 panic。
- `RadioButton` 的组行为以同一 resolved native parent 与 `GroupIndex` 为边界；跨透明包装器不改变该逻辑父级关系。energye/lcl v1.0.3 没有分组 setter，native Renderer 必须自行维护逻辑互斥，并确保内部隔离 host 不进入公开 Element 树。

### 聚合 example 与人工验收

整批实现完成后新增专门 example（名称在实现时确定），展示五项控件的最小联动：Memo 文本回显、
CheckBox 状态、ComboBox 选项、ProgressBar 数值、RadioButton 组。现有 smoke 脚本通过
`class=Button` 定位并点击唯一原生 Button，因此该 example 的窗口中必须**恰好一个** `Button`；
其余可交互入口使用控件自身交互或可点击 `Text`，并让该唯一 Button 的 Caption/可观测状态作为
smoke 信号。人工验收还应确认：多行编辑与 IME、勾选/取消、下拉选择、进度范围、单选互斥、
State 驱动更新，以及窗口关闭后无异常。

**完成条件**：范围内每个未裁剪控件均满足上述七项矩阵；`go test ./...`、`go test -race ./...`、`go vet ./...`、Windows build/smoke 通过；文档与 `examples/form-controls` 聚合 example 已提交到工作区供人工验证。完成后再进入 P7，不把结构性控件的欠账隐含在“基础控件已补齐”的表述中。

---

## Phase 7 — 工程化与生态 · 目标：可用、可信、可发布

> **状态：进行中（7.1、7.2、7.2c 与 7.3a 基线门已完成，更新于 2026-08-18）。** 入口门槛是“控件扩充批次 1”完成并通过人工验收。P7 的“控件补齐”
> 指为 Inspector、插件验证与 7GUIs 首发示例补齐必要的内建控件和机制，不等于包装
> energye/lcl 的全部控件。菜单、对话框、TreeView、图像/媒体、托盘等不属于 v0.1.0
> 发布阻塞项，后续按真实用例或插件生态增量加入。

### P7 总体任务与状态

| # | 子任务 | 要点 / 参考 | 状态 |
|---|---|---|---|
| 7.1 | Inspector | Widget/Element/native 树、属性、布局、实际事件和 mutation 查看（design.md §18）；高亮任何原生控件重建。 | ✅ 完成 |
| 7.2 | 插件系统 | `RegisterWidget` 注册、生命周期、布局与可选 Renderer 能力（design.md §19）；内建控件与第三方 builder 双轨隔离。 | ✅ 完成 |
| 7.2c | 控件扩充批次 2 | 插件模型定案后实现 `PageControl/TabPage` 结构性容器，验证每页子树与 native parent 模型。 | ✅ 完成 |
| 7.3 | 测试与 CI 强化 | 分 7.3a 基线门和 7.3b 发布门；D7 覆盖全量已发布控件、Windows 冒烟/截图、性能基准。 | 🟨 7.3a 完成；7.3b 待 7.5/7.6 |
| 7.4 | 打包 | 安装器、DPI manifest/版本资源、DLL 版本校验、单 EXE 方案评估。 | ⬜ 未开始 |
| 7.5 | 产品化与控件扩充批次 3 | 中英双语文档、全量示例、7GUIs；按示例机制逐项实现 `Slider`、`StringGrid`（native `TStringGrid`）、`Canvas/PaintBox`。 | ⬜ 未开始 |
| 7.6 | Accessibility / i18n | 高对比度、键盘导航、焦点顺序、可访问名称/UIA 能力清单、国际化资源。 | ⬜ 未开始 |

### 固定执行顺序与门禁

单维护者按下列顺序推进；未通过前一门禁不得把后续项标记完成：

1. **7.1 Inspector**：先让重建、属性 patch、布局和事件流可观察，作为后续结构性控件的排错工具。
2. **7.2 插件系统**：冻结注册、builder、生命周期和布局接口；内建 `native.Create` switch 与第三方注册表保持正交。
3. **控件批次 2（7.2c）**：实现 `PageControl/TabPage`，用真实多页子树验证容器与插件边界。
4. **7.3a 基线门**：对批次 1、批次 2 和既有控件完成统一 D7/CI 覆盖；未通过不得进入产品示例扩张。
5. **7.4 打包**：可与 7.3a 后半并行，但必须在 7.5 发布文档冻结前产出可安装测试包。
6. **7.5 产品化 + 控件批次 3**：按 7GUIs 示例需要逐项实现机制型控件，不单独开启“无限补控件”支线。
7. **7.6 Accessibility/i18n**：在公开控件/API 稳定后做全量键盘、高对比度、可访问名称和文案资源验收。
8. **7.3b 发布门**：批次 3 和 7.6 结果纳入全量 D7、Windows 冒烟、截图和性能基准，形成最终发布矩阵。
9. **v0.1.0 发布**：全新环境安装、README 5 分钟路径、全部 7GUIs、文档站和维护政策同时通过。

```text
控件批次 1（已完成）
        ├──► 7.1 Inspector ───────────────┐
        └──► 7.2 插件系统 ─► 批次 2 ─► 7.3a
                                          ├──► 7.4 打包
                                          └──► 7.5 + 批次 3 ─► 7.6 ─► 7.3b ─► v0.1.0
```

### P7 控件补齐范围

#### 控件扩充批次 2（7.2 插件模型定案后）

批次 2 只处理结构性容器，不混入自绘或表格数据模型。公开 API 在实现前先通过
energye/lcl 探针确认 `TPageControl/TTabSheet` 能力，再决定暴露一个统一 `PageControl`
抽象还是同时保留 `TabControl` 名称；不得先承诺两个重复概念。

| 目标 | 最小语义 | 必须验证 |
|---|---|---|
| `PageControl/TabPage` | 受控 `SelectedIndex`、选择回调、带稳定 Key 的页面列表、每页标题与唯一子树。 | 切页只 patch 选择状态，不重建页面或子控件；焦点/IME 不迁移到错误页面。 |
| 页面子树 | 每页拥有独立 native parent；inactive 页面保留 Element/native 子树，仅隐藏而不卸载。 | 页面重排按 Key 复用；增删只影响目标页；透明容器不改变页面归属。 |
| 布局 | PageControl 参与普通 constraints；活动页内容填充扣除 tab header 后的客户区。 | resize/DPI/显式 Width/Height 下 bounds 正确，无重叠、负尺寸或坐标越界。 |
| 插件边界 | 内建容器继续走 native switch；插件注册表只负责第三方类型及 builder。 | 注册同名、未知类型、插件卸载/失败有确定错误；内建控件不依赖插件初始化顺序。 |

**批次 2 完成条件**：公开 API、页面 identity/native parent 设计、diff 对称性、布局、Mock、
LCL 适配、无头测试、Windows 多页 smoke 和专门 example 全部完成；Inspector 能显示页面层级，
连续切页与 keyed 重排均为零 create/destroy。

#### 控件扩充批次 3（7.5 各 7GUIs 任务内）

批次 3 不是一个先做完再写示例的独立阶段；每个控件与使用它的 7GUIs 任务一起设计、实现和验收，
避免脱离真实用例提前固化错误 API。建议按机制风险从低到高推进：`Slider` → `StringGrid` →
`Canvas/PaintBox`。

| 控件/机制 | 对应 7GUIs | v0.1.0 最小范围 | 明确不做 |
|---|---|---|---|
| `Slider` | Timer | `Minimum/Maximum/Value/Step`、受控 Value、`OnValueChange(func(int))`、水平布局与键盘步进。 | 刻度标签、垂直方向、范围双滑块、富绑定。 |
| `StringGrid`（native `TStringGrid`） | CRUD、Cells | 行列数、字符串 Cells 防御性复制、受控选中行/单元格、选择/编辑回调、表头与基本列宽。 | 通用 ORM、无限数据源、复杂单元格 renderer、Excel 兼容层。 |
| `Canvas/PaintBox` | Circle Drawer | 自绘 surface、DIP 坐标、paint/invalidate 生命周期、鼠标命中；支持圆形新增/选择/半径更新。 | 通用矢量引擎、GPU 后端、场景图、任意富媒体。 |

`Canvas/PaintBox` 的绘制回调不能直接当作普通可比 Props 参与 D7c；实现前必须先选定稳定命令值、
Painter 对象 identity 或专用 invalidate 逃逸口之一，并在 design.md 记录。`StringGrid` 的二维 slice
必须深复制，调用方修改源数据不能绕开 diff；`Slider` 沿用显式受控模式，不临时扩张 `Bind`。

### P7 新控件统一实现矩阵

批次 2/3 的每个控件必须逐项满足；任何一项缺失都只能标记为实验性，不能进入 v0.1.0 控件清单：

1. **设计记录**：先在 design.md 写清 Widget/Node/Element/native 对应关系、受控状态、默认值、移除语义和已知限制。
2. **公开 API**：构造器、Opt、类型化事件与中文 doc comment；非法参数有确定 panic/error，不接受 `map[string]any` 式逃逸 API。
3. **结构与身份**：容器/重复项必须定义稳定 Key 和 native parent；切换、重排、增删不得迁移焦点、caret 或 IME。
4. **D6 隔离**：专属能力走可选窄接口；native 与 Mock 同时实现；第三方插件不迫使基础 `Renderer` 膨胀。
5. **diff 对称性**：mount、属性 patch、属性移除默认、事件 nil 解绑、同值零 mutation 全覆盖；范围属性按确定顺序下发。
6. **布局与 DPI**：intrinsic/constraints、显式尺寸、resize、DPI、容器客户区和溢出诊断均有测试；禁止原生 Align 接管。
7. **原生与事件**：真实 energye/lcl API 编译探针、主线程约束、Guard/recover、延后销毁、键盘/鼠标/IME 行为明确。
8. **测试与示例**：D7a/b/c、State 回写、能力缺失安全退化、Windows build/smoke/截图、专门 example 和 README/design 同步。

### 7GUIs 完整性映射

7GUIs 用于证明控件与机制形成闭环，不允许用静态截图或 native escape hatch 绕开缺失能力：

| 7GUIs 任务 | 主要 FluxVCL 能力 | 控件状态/依赖 |
|---|---|---|
| Counter | State、Text、Button | 既有能力；纳入最终回归。 |
| Temperature Converter | Input 双向绑定、数值转换 | 既有能力；补非法输入与焦点测试。 |
| Flight Booker | ComboBox、Input、受控 Enabled | 批次 1 已满足控件前置。 |
| Timer | Animation/Timer、ProgressBar、Slider | Slider 在该任务内实现。 |
| CRUD | Input、Button、StringGrid、稳定选择 | StringGrid 在该任务内先实现最小行选择/编辑模型。 |
| Circle Drawer | Canvas/PaintBox、鼠标命中、undo/redo 状态 | 自绘机制在该任务内实现，不用预生成图片替代。 |
| Cells | StringGrid、公式依赖图、增量更新 | 复用 StringGrid；公式解析/依赖图属于示例业务层，不塞入控件 API。 |

每个示例必须独立可运行、带说明和截图；共享 smoke 脚本仍遵守“每窗口唯一 Button”约束，
若任务天然需要多个按钮，则为该示例增加按 Key/AutomationId 定位的专用 smoke，不能削弱业务 UI。

### 7.1–7.6 分项完成条件

#### 7.1 Inspector

- mutation 层提供只读 observer，记录 create/destroy/reparent/property/event/bounds，Inspector 不反向修改 diff 状态。
- 展示 Widget/Element/native 三层对应关系、Key/Path、Props、Bounds/constraints/诊断和最近一次提交统计。
- 重建节点醒目标记，能定位“哪次 render、哪个 canUpdate 失败条件”导致焦点风险。
- Inspector 自身关闭/刷新不得触发被检查应用重建；提供无头 observer 测试和 `examples/inspector`。

> **完成记录（2026-08-12）**：`App.ObserveInspector` / `InspectorSnapshot` 发布深复制
> 只读数据；render commit 与 `App.SetBounds` direct commit 覆盖全部 mutation，实际
> 事件在用户 handler 前记录；diff 同时识别 canUpdate type/key mismatch 和 keyed
> no-candidate replacement。`render.NativeInspectable` 提供 D6 可选原生元数据；
> `inspector.Open` 独立工具窗只读取目标快照，关闭仅取消订阅。`inspector_test.go`
> 覆盖首次挂载不误报、属性/零 mutation、type/key 重建、事件/direct bounds、深复制、
> unsubscribe 与有界历史；`examples/inspector` 提供人工与 Windows smoke 验证。

#### 7.2 插件系统

- 冻结注册 API、唯一类型名、builder 输入/输出、生命周期、布局测量和可选能力获取方式。
- 注册表并发安全；重复注册、未知类型、插件 panic、初始化失败和关闭顺序有确定错误边界。
- 至少提供一个不修改 `internal/native` switch 的第三方 Chart/Badge 示例，验证插件确实走注册路径。
- 插件不能 import 不稳定 internal 包；需要的扩展点必须从公开、最小接口暴露并写兼容政策。

> **完成记录（2026-08-12）**：公开 API 已落地 `RegisterWidget` / `UnregisterWidget` /
> `RegisteredWidgets`、`PluginWidget`、`WidgetPlugin`、四种类型化 property、DIP
> `Measure`、App 级 Init/Close、实例 Mount/Update/Unmount 与类型安全 capability；
> 内建控件仍走 native Create switch，第三方 builder 只返回公开 Widget 子树。错误边界覆盖
> 非法/保留/重复/未知/在用注销、Init/Build/Measure/Close 失败、插件 panic、递归循环，
> prepare 失败在 native commit 前回滚，实例提交期错误经 Render/`LastError` 可观测。
> `plugin_test.go` 覆盖并发注册、能力缺失退化、生命周期/逆序关闭、事务回滚、DIP 布局和
> D7a/D7b/D7c；capability 按回调捕获不可变快照，覆盖保存旧上下文后的并发读取与动态值刷新；
> 插件透明身份由不可导出 marker 证明而非信任 `Plugin:` 字符串；`examples/plugin-badge/badge`
> 仅依赖公开根包且未修改 native switch。收尾审查进一步覆盖 prepare 失败时的重入 State
> 更新不得递归重试、Close UI 任务未执行时可重试，以及 App 关闭后解除 State 订阅。
> CI 已纳入 `go test ./...`、`go vet ./...` 以及 Windows `plugin-badge` build/smoke/非空截图。
> **7.2c 控件扩充批次 2 已完成（2026-08-13）**：基于 energye/lcl v1.0.3 的
> `TPageControl/TTabSheet`，公开 `PageControl` + `TabPage`，实现稳定 Key 页面、受控
> `SelectedIndex`/选择回调、每页 native parent、inactive 保活、keyed 重排与 DIP 客户区
> 布局。prepare 会在 commit 前拦截手写 Node/插件 builder 的非法结构并回滚本次 runtime；
> Mock/diff 无头回归覆盖挂载顺序、默认值/钳制、事件抑制及重排零 create/destroy。
> `examples/page-control` 的 Windows smoke 连续执行索引切换和 keyed 重排，确认 PageControl、
> 两个 TabSheet parent 与 Edit HWND 全程不变，并产出经像素检查的目标窗口截图。`TabControl`
> 未暴露，因为当前 LCL 版本没有对应的后端类型。
>
> **复核记录（2026-08-13）：P7.2 插件系统确认完成。** 已逐项复核公开注册与 builder API、
> 类型化属性、DIP 测量、App/实例生命周期、Renderer 可选能力、并发注册表与可判定错误边界；
> 第三方 `examples/plugin-badge/badge` 仍只依赖公开根包，`internal/native` 未增加插件类型分支。
> 本地执行 `go test ./...`、`go test -race ./...`、`go vet ./...`、`git diff --check` 均通过；
> `scripts/build.ps1 -Target plugin-badge` 与对应 smoke 通过，Win32 验证窗口出现、按钮文本
> `0 -> 1` 且进程正常退出。smoke 的全屏截图未保证目标窗口置前，故本次不把截图内容作为
> UI 视觉验收证据。结论仅覆盖 7.2 插件系统；7.2c 分页容器按上方独立完成记录验收。

#### 7.3 测试与 CI 强化

> **7.3a 基线门已完成（2026-08-18）**：`control_contract_test.go` 固定 18 个内建公开
> 控件的 inventory、mount、纯属性 patch 零重建和无事件同树零 mutation 基线；
> 可配置 native 控件另测移除重置/事件解绑，具交互语义控件另测 State 回写，
> `PluginWidget` 的 D7/生命周期由 `plugin_test.go` 独立覆盖。矩阵补出并修复了
> `ListView` 移除 `ScrollOffset` 后旧原生回调残留及重复解绑的问题。CI 在 Go
> 1.22–1.26 与当前 1.27rc3 上跑无头测试，并增加
> vet/race、DLL 哈希验证、DLL 到位后的 non-race native probe，以及 9 个公开 examples
> 的独立 Windows build/smoke/像素有效截图 artifact。控件挂载、纯属性 patch、Page
> 切换和十万行列表更新基准及首份样本见 [performance-baseline.md](./performance-baseline.md)。
> 本地逐项复跑 9 个公开 examples 均通过 build/smoke、专属交互断言与退出码 0，
> 9 张像素有效 PNG 均非空（9,202–46,650 bytes）。
> **7.3b 仍待 7.5/7.6**：`StringGrid` 更新、`Canvas/PaintBox` invalidate、全部 7GUIs
> 以及键盘/高对比度/i18n 复测必须在对应实现完成后进入最终发布矩阵。

- **7.3a**：批次 1/2 + 既有控件统一跑 mount、patch 不重建、移除重置、事件解绑、同树零 mutation、State 回写。
- **7.3b**：批次 3、7.6 与全部 7GUIs 纳入同一矩阵；容器额外测 keyed 重排/native parent，绘制额外测 invalidate 不重建，并复跑键盘/高对比度/i18n smoke。
- Windows CI 构建全部公开 examples，执行 smoke 并上传非空截图；native 探针在 DLL 可用且非 race 时运行。
- 增加控件创建、纯属性 patch、Page 切换、Grid 更新、Paint invalidate 基准，并记录发布基线而非设置脆弱绝对阈值。

#### 7.4 打包

- NSIS/WiX 至少选定一个主方案，安装/卸载包含 exe、严格匹配版本的 DLL、许可证和示例入口。
- 构建时校验 Go module 的 energye/lcl 版本与打包 DLL 来源；不允许静默拿“最新 DLL”。
- 保留 perMonitorV2 manifest、版本信息和 common controls v6；干净 Windows VM 安装、启动、卸载通过。
- 单 EXE 仅做可行性与许可证评估；若不能可靠落地，v0.1.0 明确采用 exe + DLL，不阻塞发布。

#### 7.5 产品化

- README、快速开始、API/设计/限制、迁移和维护政策提供中英双语入口；所有公开 API 有可检索示例。
- 完成上表 7GUIs，并为批次 2/3 各提供聚合或专门 example；首发截图来自真实运行窗口。
- 发布对比页只陈述可验证能力：原生控件、声明式 diff、IME、DPI、虚拟化、多窗口、Inspector/插件。
- 冻结 v0.1.0 公开 API 清单和 breaking-change 政策，生成 changelog 与 release checklist。

#### 7.6 Accessibility / i18n

- 全控件键盘可达，Tab 顺序、方向键、Space/Enter、Esc 行为有 Windows smoke；焦点指示不可被主题隐藏。
- 高对比度下不以自定义颜色覆盖系统可读性；Canvas/Grid 补可访问名称或明确记录后端限制。
- 示例文案和框架诊断从可替换资源读取；至少验证中英文切换不重建有状态控件、不破坏布局。
- 形成 UIA/屏幕阅读器能力表：原生继承能力、框架补充能力、energye/lcl 限制分别列出。

**P7 最终交付物**：Inspector、插件 SDK、批次 2/3 控件、7GUIs、全量 D7/CI、Windows 安装包、
中英双语文档站、Accessibility/i18n 能力表。

**P7 最终验收**：全新 Windows 环境从安装到运行示例不超过 5 分钟；全部公开 examples 与
7GUIs 可交互运行；CI/race/vet/native smoke 全绿；Inspector 未发现非预期重建；发布 v0.1.0。

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
| IME/中文边界 | 低 | Phase 4.4 实测 energye/lcl v1.0.3 `OnUTF8KeyPress` 控件级可用（含 IME 组合结果），风险降级；长文输入留 TMemo |
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
