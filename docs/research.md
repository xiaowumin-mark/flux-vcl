# FluxVCL 调研报告

> 版本：0.1（调研稿）
> 日期：2026-08-08
> 范围：Go + VCL 绑定生态现状、已知坑点、声明式 UI 先例、reconciliation/布局/线程等核心工程问题、社区评价与生态风险。
> 方法：Web 检索 + GitHub issues/PR/仓库元数据核对，覆盖 8 个维度；全部关键结论附来源链接。今天为 2026-08-08，涉及"最新状态"的结论均以此为基准核实。

> **⚠️ 更新提示（2026-08-08）：** 本报告 §1.5 底座选型建议（默认 A：govcl v1.2.10）与 §9 战略结论第 1 条，已被后续专项调研 [govcl-vs-lcl.md](./govcl-vs-lcl.md) 取代——最终决议为**默认采用 energye/lcl（LCL），govcl v1.2.10（真实 VCL）降为 B 计划**。[design.md](./design.md) 已据此将定位修订为"LCL/VCL 双后端，默认 LCL"。本节以下内容保留为调研过程记录。

---

## 0. 摘要（TL;DR）

1. **没有活跃维护的 Go→Delphi VCL 绑定。** 最流行的 `ying32/govcl` 在 v2.0（2020）已移除 Delphi VCL、只保留 Lazarus LCL（`liblcl`），并于 2023-11 宣布"纯维护"；最后支持真实 VCL 的代码是 **`govcl v1.2.10`（2020-03 冻结，`last-vcl-support` 分支）**。`energye/golcl` 与 `energye/lcl` 都是 **LCL** 绑定（服务于 CEF 框架 Energy），不是 VCL。**这是对 design.md 前提的重要修正。**
2. **架构确认：纯 Go + syscall 加载原生 DLL（`libvcl.dll`/`libvclx64.dll`），Windows 下零 CGO。** 但预编译 DLL 官方标注"仅预览/测试"，生产需用 Delphi 10.2.1 自行编译 —— 存在许可与分发摩擦。
3. **坑点高度集中在几个方向**（govcl issues 实证）：UI 非线程安全、对象必须显式 Free（Go finalizer 默认关闭）、事件回调内 Free 会崩溃、UTF-8/中文/IME 边界、DPI 默认关闭、DLL 架构不匹配的 init panic、`-buildmode=exe` 等构建要求。
4. **空白地带确认：Go 生态中没有"原生控件上的声明式/状态驱动 UI"实现。** `lxn/walk`（7k star）有声明式 Builder 但非响应式且 2021 年起停更；Fyne/Wails/Gio 等全部走 canvas/webview。FluxVCL 是真正的 greenfield，同时意味着 reconciliation 层必须自研。
5. **reconciliation 唯一可行路线：retained + diff + 属性级 patch。** 绝不能 immediate mode —— 重建 HWND 昂贵（ComboBox 空态 ~53ms）且丢焦点/滚动/IME。推荐 Flutter 三棵树模型（Widget/Element/RenderObject）+ `canUpdate`（类型+key）+ 批量应用。
6. **布局引擎必须在"不实现控件"的前提下测量原生控件。** 采用 intrinsic-size 函数（AWT WButtonPeer 先例）+ GDI 文本测量 + 主题 API（`BCM_GETIDEALSIZE`/`GetThemePartSize`），缓存 + 脏标记（Yoga measure func 模型）。
7. **社区生态**：govcl 无英文社区、用户群在中国（QQ/Gitee/CSDN）；HN 2024-2026 一致认为 Go 原生 GUI 无生产级方案，Wails 主导，Fyne 被批"像 2003 年"。Delphi/VCL 2026 年"活着但老化"。这些决定了 FluxVCL 的定位与营销方式。

---

## 1. 技术底座：Go + VCL 绑定生态

### 1.1 `ying32/govcl` 现状（核心事实）

| 事实 | 值 | 来源 |
|---|---|---|
| Stars / Forks / Open Issues | ~2,401 / 241 / ~23 | [repo](https://github.com/ying32/govcl) |
| 最新 release | v2.2.3（2023-08-12），仅带 `liblcl-2.2.3.zip` | [releases](https://github.com/ying32/govcl/releases) |
| 维护状态 | **2023-11-20 作者宣布进入"纯维护"**：不加新特性，只修 bug | [README](https://github.com/ying32/govcl/blob/master/README.md) |
| master 最后实质提交 | 2024-05-08（typo 修复 PR #199） | [commits](https://github.com/ying32/govcl/commits) |
| 最新 issue #240（2026-06） | "好项目，希望别中途掉了"——社区公开担忧弃坑 | [issue #240](https://github.com/ying32/govcl/issues/240) |
| 模块 | `v2.2.3+incompatible`（无有效 go.mod，打包标记可疑） | [pkg.go.dev](https://pkg.go.dev/github.com/ying32/govcl) |

**关键**：govcl 的 Go 包名仍叫 `vcl`，但 **v2.0 起核心绑定是 `liblcl`（Lazarus LCL），不再是 Delphi VCL**。v2.0.0 release note 明确"no longer supports Delphi/VCL"。作者在 [z-kit.cc/about](https://z-kit.cc/about.html) 给出的弃 VCL 理由：单人维护精力有限；VCL 与 LCL 差异（图片格式、枚举顺序、属性、事件、布局）需持续兼容补丁；VCL 仅 Windows 而 LCL 跨平台；Delphi 商业授权（Community Edition 仅限个人使用）。

### 1.2 VCL 版本的唯一存续：`govcl v1.2.10`

- 最后支持真实 Delphi VCL 的代码：**`v1.2.10`（2020-03-20）及 `last-vcl-support` 分支**（最后提交 2020-04-14）。
- 官方通过 GitHub API 核对：整个 govcl 生态 **没有** 活跃的 VCL fork；fork 全部同步 master（即 LCL）。
- **结论：FluxVCL 若坚持"真实 Delphi VCL"，只能基于 govcl v1.2.10 / last-vcl-support，并把它当作自维护的 vendored 层。**

### 1.3 `energye/golcl` / `energye/lcl`：是 LCL，不是 VCL

- `energye/golcl`（8 stars，最后提交 2024-12-31）：是 CEF 框架 Energy 的 LCL 后端，**所有分支只有 `lcl/` 包，无 `vcl/` 包**；已被独立仓库 `energye/lcl` 取代。
- `energye/lcl`（34 stars，最后提交 2026-07-01，v1.0.6 2026-05-18）：**2026 年仍活跃的 LCL 绑定**，无 CGO，需 `libenergy.dll`。API 与 govcl 的 `vcl` 包 1:1 对齐（`NewButton`/`SetCaption`/`SetOnClick`/`Application.Initialize/Run`/`types/colors`）。
- 若"活跃维护"优先于"精确 VCL 语义"，LCL 路线是可选项，但控件是 Lazarus 控件而非 Delphi 控件。

### 1.4 架构确认（零 CGO）

- **Windows：纯 Go + `syscall.NewLazyDLL`** 加载 DLL（通过 `github.com/ying32/dylib` 封装），`CGO_ENABLED=0`；仅 Linux/macOS 的 LCL 后端需要 CGO。
- 部署：`libvcl.dll`（32 位）/ `libvclx64.dll`（64 位）放在 exe 旁或 PATH；**GOARCH 必须与 DLL 位数匹配**，否则 init 阶段 panic：`Failed to load libvcl.dll: The specified module could not be found`（issues [#23](https://github.com/ying32/govcl/issues/23) / [#69](https://github.com/ying32/govcl/issues/69) / [#225](https://github.com/ying32/govcl/issues/225)）。
- **构建要求**：`GOOS=windows`、`CGO_ENABLED=0`、`-buildmode=exe`（Go>=1.15 默认 PIE 与 .syso 资源/链接不兼容）。
- **DLL 许可/分发**：`Librarys-1.2.10.zip` 中 `libvcl.dll` ~1.72MB、`libvclx64.dll` ~2.22MB，**官方 README 明确"仅预览/测试用"**；生产需用 Delphi 10.2.1 编译 `UILibSources/libvcl`（输出 `..\libvcl.dll` 与 `..\x64\libvclx64.dll`）。这是 FluxVCL 必须提前解决的发布问题。
- API 惯例（v1.2.10 源码核实）：`vcl.Application.Initialize()` / `vcl.Application.Run()`；`btn := vcl.NewButton(form)`；`btn.SetCaption/SetLeft/SetTop/SetWidth/SetHeight/SetParent`；`btn.SetOnClick(f)`；颜色常量 `types/colors` 里 `ClRed = 0x0000FF`（BGR，对应 Delphi TColor）。

### 1.5 底座选型建议（三个选项）

| 选项 | 内容 | 代价 |
|---|---|---|
| **A. govcl v1.2.10（真实 VCL）** | 冻结于 2020，FluxVCL 自维护；需自建 libvcl DLL 构建管线 | 与现代 Go 工具链兼容需自测；DLL 许可 |
| B. energye/lcl（活跃 LCL） | 唯一活跃维护的 native 绑定，API 几乎一致 | 控件是 Lazarus 而非 Delphi VCL |
| C. 直接 Win32 | 放弃 VCL | 失去成熟控件集（正是 VCL 的价值） |

**建议**：按 design.md 的定位（"VCL 作为成熟后端"），默认选 **A**，但把"绑定层"隔离在窄接口后（`Create/SetBounds/SetVisible/TextWidth/HandleAllocated`），保留未来切换到 B 的余地。此为 Phase 0 决策点，需实验验证 govcl v1.2.10 在现代 Go 工具链（1.22–1.27）上能否构建运行。

---

## 2. 已知坑点（govcl/golcl issues 实证）

### 2.1 线程安全（最高风险）

- VCL/LCL **全部 UI 组件非线程安全**。任何 goroutine 触碰控件 → SIGSEGV（issue [#14](https://github.com/ying32/govcl/issues/14)）或对话框冻结（[#29](https://github.com/ying32/govcl/issues/29)）。
- Go API 中 **没有** `Application.QueueAsyncCall`/`AsyncExecute`。唯一调度原语是 **`vcl.ThreadSync(fn)`**（包装 `api.DSynchronize(fn, 1)`，阻塞式）。
- 事件只在主线程触发，**阻塞的事件处理器会冻结整个应用**（issue [#165](https://github.com/ying32/govcl/issues/165) 的规范做法：`SetEnabled(false)` → goroutine 干活 → `ThreadSync(恢复)`）。

### 2.2 内存管理

- **Go finalizer 默认关闭**（`finalizerOff.go` 是空实现，需 `-tags finalizerOn` 才启用 `runtime.SetFinalizer`）。因此**每个创建的对象必须显式 `.Free()`**，否则泄漏。
- **在事件回调内部 Free 正在处理事件的组件** → `LCLRefCount>0` 警告 + 崩溃（issues [#95](https://github.com/ying32/govcl/issues/95) / [#96](https://github.com/ying32/govcl/issues/96) / [#120](https://github.com/ying32/govcl/issues/120)）。销毁必须推迟到回调返回之后。
- 作者自述：`v1.2.7` 修过 liblcl 字符串被 Go GC 提前回收；`v2.0.2` 修过字符串内存过早释放；`v2.2.3` 增加事件回调清理以便 GC 回收 —— 说明字符串跨边界与事件闭包生命周期是历史事故高发区。

### 2.3 字符串 / 编码 / IME

- 文本以 **UTF-8** 跨 Go/LCL 边界（`StringToUTF8Ptr`/`GoStr`），不是 Delphi VCL 的 UTF-16。
- `TRichEdit` 有未解决的中文/emoji/大文本 bug：[#212](https://github.com/ying32/govcl/issues/212)（`Text()` 对 CJK 返回错误长度）、[#234](https://github.com/ying32/govcl/issues/234)（emoji 回环乱码，作者建议改用 TMemo）。
- **IME 中文输入**：`OnKeyPress` 拿不到组合中的中文，只能走 **`OnUTF8KeyPress`**（仅 TForm 级、仅 dev 分支，issue [#126](https://github.com/ying32/govcl/issues/126)）。自定义绘制控件需自行接入 Win32 IMM。

### 2.4 DPI

- 默认 manifest 中 `<dpiAware>` 是**注释掉的** → 高 DPI 下模糊。`SetScaled`/`SetFormScaled` 自 v1.2.8 起**已失效**。govcl 提供 `Scale96ToForm`/`ScaleFormTo96` 等辅助函数（v2.0.6+）。
- **Flutter 式布局引擎必须自己做 DPI 数学**（见 §6.5）。

### 2.5 构建 / 启动

- 默认构建会弹出**黑色控制台窗口**并打印 `Library Version: x`：需 `-ldflags "-H=windowsgui"` + 空导入 `_ "github.com/ying32/govcl/pkgs/winappres"`（manifest + 图标 + 版本信息）。
- `-buildmode=exe` 必须（Go>=1.15）；`CGO_ENABLED=0`。
- 主窗口 `Hide()` 后 `Show()` 只出现在任务栏 → 需 `Application.Restore()`（[#70](https://github.com/ying32/govcl/issues/70)）；关闭拦截要用 `OnCloseQuery`（[#51](https://github.com/ying32/govcl/issues/51)）。
- 事件绑定基于**反射方法名**（`OnFormCreate`/`OnButton1Click` 自动绑定）：garble 混淆会使其静默失效（[#171](https://github.com/ying32/govcl/issues/171)）；相似命名会误匹配（[#93](https://github.com/ying32/govcl/issues/93)）。**FluxVCL 的事件映射必须显式注册回调，不能用反射。**

### 2.6 渲染 / 闪烁

- 无硬件加速，`TPaintBox` 每帧重绘慢；社区方案：离屏 bitmap + `Draw` + `Invalidate`，而非逐帧画图元（[#121](https://github.com/ying32/govcl/issues/121) / [#145](https://github.com/ying32/govcl/issues/145)）。
- VCL 防闪烁原语：`DoubleBuffered`、`BeginUpdate/EndUpdate`、`WM_SETREDRAW`（TScrollBox 无 BeginUpdate 时用）。批量 patch 时包装这些原语。

---

## 3. 先例：Go 声明式 / 响应式 GUI

### 3.1 `lxn/walk`（最接近的先例，但非响应式）

- 7,099 stars，`declarative` 子包用**嵌套 Go 结构体 + 反射 Builder** 一次性构造原生 Win32 控件：创建控件 → 设属性 → 建布局（VBox/HBox/Grid）→ 递归子控件 → 绑事件 → `builder.Defer` 收尾。
- 事件是 **pub-sub**（`Event`/`EventPublisher`，`Attach/Detach/Publish`，类型化载荷）。
- 数据绑定：`DataBinder{}` + `Bind("FieldName")` + 校验（`Range/SelRequired`）+ 反射 `ReflectTableModel`，模型有 `ItemsReset/ItemChanged/RowsChanged` 通知。
- **致命局限：没有 re-render/diff 循环。** `Run()` 之后开发者必须命令式 `SetText/SetEnabled` 修改 live 控件 —— 是"一次性声明 + 事后命令式"，不是状态驱动。
- **维护状态：2021-01-12 后无 master 提交，345 个 open issues，stale PR（最早 2015 年）**。→ **用作设计参考，不作运行时依赖。**

### 3.2 govcl/golcl 生态：无任何声明式层

- 对 govcl 全包树、issues（`repo:ying32/govcl declarative` = 0 结果）、生态（energye/golcl 仅 CEF 后端的命令式 fork）三重核实：**不存在任何 govcl 声明式/响应式包装**。这是 FluxVCL 的机会，也意味着一切要从零造。

### 3.3 主流 Go GUI 的架构定位

| 框架 | Stars | 架构 | 说明 |
|---|---|---|---|
| Wails | 35,742 | WebView2 前端 + Go 后端 | 最成功，但非原生控件 |
| Fyne | 28,578 | 自绘 canvas（OpenGL） | 被批"丑、像 2003"；v2.6 引入单 goroutine UI + 泛型 data binding |
| Gio/gioui | 活跃 | immediate-ish retained | v0.10 恢复维护 |
| Energy | 610 | CEF + LCL + WebView2 | 中文社区为主，LCL 仍命令式 |
| Spot | 1,256 | 虚拟 DOM + hooks（原生后端） | **Windows 后端没做完，2024-12 后停滞** |
| Gova | 339 | `g.Scope`/`State.Set()`→re-render（Fyne 后端） | 声明式+响应式，但渲染 canvas |

**收敛共识**（Gova/loom/govinci/go-tui/gogpu/ui）：显式细粒度 `State[T]` + `Set()` 通知；订阅按调用点 key 防泄漏；**单 goroutine 跑全部 UI 回调**（Fyne v2.6、Gio 共识）；`fyne.Do` 式 marshalling API。这些模式可直接借鉴给 FluxVCL 的 State 与线程层。

**结论**：FluxVCL 的定位（声明式、状态驱动、映射到真实原生 Windows 控件）在 Go 生态**无既有实现**；walk 提供 Builder/DataBinder/事件 pub-sub 的证明，新项目提供 state/线程/reconciliation 的模式。

---

## 4. 核心工程问题：Reconciliation（协调 / diffing）

> design.md 未明确 diffing 策略，但这是整个框架成败的关键。以下为跨框架调研的收敛结论。

### 4.1 成熟框架的收敛结论

React、Flutter、SwiftUI、Compose、React Native（Fabric）、Iced、Dioxus 全部收敛到同一策略：

1. **每次渲染重建便宜的声明式树**（不可变、纯数据）；
2. 按 **TYPE + KEY** 匹配新旧节点（identity）；
3. 身份匹配 → **原地更新**（patch 变化的属性）；
4. 仅类型或 key 变化才 **销毁 + 重建**。

Flutter 的原话：`Widget.canUpdate(old,new) = old.runtimeType==new.runtimeType && old.key==new.key`。SwiftUI 的 identity 机制同理（WWDC23 "Demystify SwiftUI performance"）。

### 4.2 为什么"每次重建原生控件"是灾难（实证）

- **创建/重建 Win32 子窗口昂贵**：WinForms 基准（同为 CreateWindowEx 机制）：Button ~2ms、空 ComboBox ~53ms、100 项 ComboBox ~264ms；`EnableVisualStyles` 可再慢 ~10 倍。
- **丢失状态**：HumbleUI（Java 声明式）issue [#50](https://github.com/HumbleUI/HumbleUI/issues/50)：文本字段在每次重建后丢焦点；NativeBase issue [#5231](https://github.com/GeekyAnts/NativeBase/issues/5231)：组件函数内定义 Factory 导致每次 state 更新重建、丢焦点；hatter 文档直言"清空+重建会导致闪烁、TextInput 丢焦点、滚动复位"。
- **VCL 特有**：重建会连带丢失 caret/IME 组合状态；父控件 handle 重建会级联销毁所有子句柄。
- **egui 是反例**：immediate mode 每帧重建 GPU 场景 OK，但对 HWND 是灾难。**FluxVCL 必须是 retained + diff-and-patch，绝无第二个选项。**

### 4.3 推荐架构（直接可实施）

1. **三棵树分离**：Widget 树（每次 render 重建的不可变 Go 结构体）→ Element 树（持久 identity 节点：{控件类型, key, 父路径, 原生控件指针, 上一份配置}）→ 真实 VCL 控件树。Widget 层绝不持有原生指针。
2. **canUpdate 规则**：`旧控件类型==新控件类型 && 旧key==新key`（双 nil key 视为相等）→ 原地 patch；否则只重建该节点。
3. **属性级 patch 为渲染器心脏**：逐属性比较旧/新配置（Caption/Left/Top/Width/Height/Visible/Enabled/Font/Color/TabStop…），仅对变化者调 `SetCaption/SetLeft/...`；配置未变的控件直接跳过（hatter 的 `setStrProp/setNumProp` 教训，与 diff 策略无关的最大收益）。
4. **列表身份**：稳定数据 key（模型 ID，创建时生成一次），**绝不 index、绝不每次 render 随机**。index key 会让焦点/caret/IME 迁到错误行 —— 在真 TEdit/TMemo 上是正确性 bug 而非性能问题。
5. **作用域失效（可选增强）**：State getter 记录"被哪个 element 路径读取"，`Set()` 只标记依赖子树 dirty（Compose SlotTable 的 Go 可实现子集）；至少缓存并跳过未变子树。
6. **批量提交**：diff 遍历只收集 mutation（Create/Update/Insert/Remove/Delete，RN-Fabric 风格），按"先销毁后创建、先上后下"的安全顺序应用；用 `BeginUpdate/EndUpdate` + 窗体 `DoubleBuffered` + `WM_SETREDRAW`（TScrollBox）包裹；新控件在首个 handle 访问前设完全部属性。
7. **逃逸口**：类 Dioxus Signal 的**直接单向属性绑定**，高频热路径（TTimer 进度条/日志）绕过整树 diff。

### 4.4 三条不可妥协的测试不变量

- (a) 纯属性变化绝不重建控件；
- (b) 稳定 key 的列表重排不迁移焦点/IME；
- (c) 相同树 diff 产出零 mutation（幂等提交 golden test）。

---

## 5. 布局引擎：在原生控件上做 Measure/Layout

### 5.1 协议模型（照抄 RenderBox）

- 两遍协议：`constraints`（min/max W/H，可 ∞）**下传** → 子返回 `Size` **上抛** → 父再定子 offset。`performLayout` 必须 `constraints.constrain(...)` 钳制，否则 RenderFlex overflow。
- **RenderFlex 精确算法**（flex.dart 源码核实）：
  1. 非 flex 子控件用主轴 unbounded、交叉轴（stretch 时）tight 的约束布局，累加主轴长度 + 间距；
  2. `freeSpace = max(0, maxMain - accumulated)`，`spacePerFlex = freeSpace / totalFlex`，按 flex 因子分给 flex 子控件；
  3. `Expanded`=tight，`Flexible`=loose；
  4. 交叉轴 = max(子尺寸)；主轴按 `MainAxisAlignment`（start/center/end/spaceBetween/Evenly/Around）定位；
  5. **只增长不收缩**，溢出给诊断（Flutter 的条纹条 → FluxVCL 的 inspector 提示）。

### 5.2 核心难题：原生控件是"不透明 HWND"

- VCL 惰性创建窗口：`SetParent→CreateHandle→CreateWnd→CreateWindowEx`；读 `Handle` 会强制创建，`HandleAllocated()` 只查不建。Windows 有子窗口数量上限，框架若为测量而全部实现会撞墙。
- **VCL 没有 `TControl.GetPreferredSize`**（那是 LCL 的 API）；标准 TButton 的 AutoSize 不可用（需子类 + DrawText 测量）。
- **可行的行业模式**（已验证）：
  1. **intrinsic-size 函数**（主路径）：每个叶子 widget 提供 `Size Measure(font, text, dpi, constraints)`。AWT 的 `WButtonPeer` 就是**用 Java FontMetrics 在 Java 侧算按钮尺寸**（`stringWidth+14, getHeight+8`），而非问原生窗口 —— 这是教科书级先例。
  2. **GDI 文本测量**：`GetTextExtentPoint32` / `DrawText(DT_CALCRECT)` / `GetTextMetrics`（行高），govcl 暴露为 `TCanvas.TextWidth/TextHeight/TextExtent`。
  3. **主题/系统定义尺寸**：按钮 `BCM_GETIDEALSIZE`、主题件 `GetThemePartSize`、`GetSystemMetrics`（需要已实现控件，或静态表按 DPI 缩放）。
  4. **混合（推荐）**：intrinsic 函数为主 + 主题重叶子一次性"隐藏实现→测量→缓存"，结果按 (controlType, font, text, dpi) 缓存，`WM_DPICHANGED`/`CM_FONTCHANGED`/文本变化时失效（Yoga `cachedMeasurements` + `YGNodeMarkDirty` 模型）。
- 滚动容器给滚动轴 **unbounded** 约束（Flutter ListView 语义），内容超高时用虚拟化（见 §7.5）。

### 5.3 为什么禁用 VCL Align/Anchors

- VCL 的 Align（alTop/alClient/...）+ Anchors 是**边锚定**，无 constraints 传递、无 preferred-size 协议、无 flex 因子 —— 无法表达 Row/Column 比例 flex、间距分布、内在尺寸。
- **框架管理的控件一律 `Align=alNone`，几何只走 `SetBounds`**。`Native()` 逃逸口若设置了 Align，布局前必须还原，否则两者打架（alClient 子控件会在父 resize 时自撑，破坏自定义布局算出的 frame）。

### 5.4 DPI

- manifest 声明 **PerMonitorV2**（`<dpiAwareness>PerMonitorV2</dpiAwareness>`），处理 `WM_DPICHANGED`（wParam 新 DPI / lParam 建议矩形），`GetDpiForWindow` 取当前 DPI。
- **所有布局坐标用 DIP**，`MulDiv(dip, dpi, 96)` 转像素；自定义布局拥有几何时设 `TForm.Scaled=false` 防 VCL 双重缩放；字体用 `TForm.ScaleForPPI(NewPPI)`。
- 注意：VCL 的 `SetScaled` 已失效，DPI 数学必须自己做。

---

## 6. 线程 / 事件 / 动画模型

> 本节由专项 Agent 对照 govcl 源码（v2.2.3 模块、liblcl 源码、samples）逐条核实。含对早期结论的两处更正（见 §6.5）。

### 6.1 主线程机制（源码核实）

- govcl 的 `vcl/init.go` 在 package `init()` 里**无条件调用 `runtime.LockOSThread()`**（注释："锁定主线程，防止中间被改变"）—— Go 主 goroutine 被钉在一个 OS 线程上，**只有它允许碰 VCL 对象**。
- 消息循环在 **liblcl.dll 内部**执行：`Application.Initialize()` → `SetMainFormOnTaskBar(true)` → `CreateForm(&mainForm)` → `Application.Run()`（`Run` 是 `api.Application_Run` 的薄封装，会阻塞主 goroutine 直到退出；退出时 govcl 释放所有表单并关闭库）。便捷函数 `vcl.RunApp(values...)` 一次性完成上述流程。
- `Application.ProcessMessages()` 存在，可用于手动泵消息；**Go API 中没有 `Application.OnIdle`**。
- 模态流：`vcl.MessageDlg` / `vcl.ShowMessage` 只在主线程调用。

### 6.2 Marshalling：`ThreadSync` 是唯一原语

- `vcl.ThreadSync(fn)` → `api.DSynchronize(fn, 1)` → liblcl `DSynchronize`：**已在主线程则 fn 内联执行（不推迟）；在 goroutine 则阻塞该 goroutine，经 `TThread.Synchronize` 等主线程跑完 fn**。
- **Go API 中不存在 `QueueAsyncCall`/`AsyncExecute`**（对 v2.2.3 模块 grep 零命中）。`DSynchronize` 的 `useMsg` 参数在当前 liblcl 里已废弃（被忽略）。
- **死锁铁律**：
  1. 在 `ThreadSync` 回调内再调 `ThreadSync` → 死锁（底层是**非可重入** `sync.Mutex`）；
  2. 主线程阻塞时（忙 handler、模态框、未泵消息）调用 `ThreadSync` → 永久挂起；
  3. 主线程同步等待一个正在 `ThreadSync` 回来的 goroutine → ABBA 死锁。

### 6.3 动画：主线程 TTimer 是规范做法

- `vcl.NewTimer(owner)`：`SetInterval`/`SetEnabled`/`SetOnTimer`，`WM_TIMER` 在主线程触发，无需额外线程。规范示例 `samples/clock`：`OnTimer` → `PaintBox.Repaint()` → `OnPaint` 重绘表盘。
- ~60fps 可选：(a) `TTimer Interval=16`（WM_TIMER 粒度 ~15.6ms，主线程忙时丢帧）；(b) goroutine `time.Sleep` + 每帧 `ThreadSync`（可行但阻塞、被全局 mutex 串行，节奏脆弱）；(c) 主线程 `Application.ProcessMessages()` 泵。
- **稳健规则：帧工作在主线程做（TTimer/ProcessMessages）；计算放 goroutine，每帧只 marshal 最小 commit。** 任何长时间阻塞的事件/定时器 handler 都会停泵：定时器停发、ThreadSync 调用方挂起、无法重绘。

### 6.4 事件与统一事件抽象

- govcl 为每个控件生成显式回调 setter：`SetOnMouseMove/SetOnMouseDown(fn, shift, x, y)`、`SetOnKeyDown/SetOnKeyPress`（`key` 是 `*Char`，**uint16 = UTF-16 码元**）、`SetOnClick`、`SetOnMouseEnter/Leave` 等。
- 原生消息：`TForm.SetOnWndProc(TMessage{Msg,WParam,LParam,Result})` 可捕获未暴露的高层事件（`WM_MOUSEMOVE`/`WM_LBUTTONDOWN`/`WM_KEYDOWN`/`WM_CHAR` 等）。
- **统一 `Event{X,Y,Type,Source}` 的喂入路径**：x/y 取自 OnMouseMove/OnMouseDown；key 取自 OnKeyDown/OnKeyPress；Source 用 sender IObject（`samples/eventpublic` 的共享 handler 模式：按 `Tag()`/`Name()` 区分）。需 KeyPreview 时 `TForm.SetKeyPreview(true)`。

### 6.5 IME / 中文输入（含更正）

- **`OnUTF8KeyPress`（`TUTF8KeyPressEvent`，`TUTF8Char{Len byte; Content [7]byte}`）已在正式版 v2.2.3 中**（2023-08-13 加入，对应 issue [#126](https://github.com/ying32/govcl/issues/126)）——**更正**：此前调研记作"仅 dev 分支"是错的。
- 但它**仅绑定 TForm**（`Form_SetOnUTF8KeyPress` 在 liblcl 的 `MyLCL_Form.inc`）。自定义绘制控件拿不到，需经 form 级 `OnUTF8KeyPress` + `KeyPreview`，或在 `OnWndProc` 挂 `WM_CHAR`/`WM_IME_*`。
- 逐控件 `OnKeyPress` 拿不到 IME 组合中的中文（单 uint16 码元）。

### 6.6 State 跨线程模式与已知崩溃（含更正）

- **规范模式**（`samples/simpleIM`/`samples/memstream`）：goroutine 做 I/O 并**在纯 Go 侧算好新状态**，然后 `vcl.ThreadSync(func(){ 落地到 UI })`。**不要**在一堆 goroutine 里对每个条目各调一次 ThreadSync（阻塞 + 全局 mutex 串行，会停摆）——**批量/合并更新**。
- 崩溃实证：#14 在 `go func(){ vcl.ShowMessage(...) }()` → SIGSEGV；#29 `ShowModal` 从 goroutine 调 → 冻结。#95/#96 在事件处理中 Free 组件 → `LCLRefCount>0` 警告 + 空指针 panic（**更正**：#120 与此无关，它是"自定义 struct 如何释放"）。
- **govcl 事件回调无 `recover()`**：主线程 handler 里 panic 会直接崩掉整个进程 → FluxVCL 必须自建事件分发错误边界。
- 阻塞 handler 冻结泵（#165 场景：禁用按钮不生效、点击被排队）—— handler 必须短。

### 6.7 对 FluxVCL 的落地建议

- 框架 init 照抄 `runtime.LockOSThread()`，并在**每个碰 VCL 的入口做 debug 断言**：`rtl.CurrentThreadId() == rtl.MainThreadId()`（govcl 提供 `vcl/rtl`），违反即 panic。
- 在 `ThreadSync` 之上自建**单一 `DispatchToMainThread()` 队列**（一个主线程消费者 channel + 批量合并），提供 `runOnUI(fn)`（异步）/`runOnUISync(fn)`（阻塞）；State 跨线程变更 = 离线程更新纯 Go 模型 → 入队小 commit。
- **动画**：主线程 TTimer/ProcessMessages 驱动；直接绑定逃逸口（§4.3）承接高频属性。
- **事件**：显式回调注册（D6）→ 统一 Event；IME 走 form 级 `OnUTF8KeyPress`。
- **生命周期**：销毁一律入队延后，绝不回调内同步 Free；`LCLRefCount>0` 警告在 debug 构建中当作框架断言失败。
- **错误边界**：事件分发层包 `recover()`，路由到错误事件而非让进程崩。

---

## 7. 工程化：构建 / CI / 测试 / 打包 / 性能

### 7.1 构建

- 纯 Go 零 CGO（Windows），`GOARCH` 与 DLL 位数严格匹配；`-buildmode=exe`；`-ldflags "-H=windowsgui"`；空导入 `winappres` 供 manifest/图标。
- **module 路径 `github.com/fluxvcl/flux-vcl` 未被占用**（proxy.golang.org 404、org 未注册），可安全使用。
- Go 版本策略：工具链已是 ~1.27rc2（2026-08），而 govcl v1.2.10 go.mod 为 go 1.13 —— CI 每轮必须验证冻结绑定在现行 Go 上可构建。

### 7.2 单 EXE / DLL 分发

- VCL 单 EXE 只有 `RCDATA + windres` 内存加载一条路，且 **仅 win32、禁 UPX**；`go:embed+临时目录` 路径是 LCL 专属且依赖无许可模块 `ying32/liblclbinres`（无 go.mod、无 license）。
- **推荐**：NSIS/WiX 用 `File` 命令把 DLL 放 exe 旁；或发布"构建脚本 + 从源码编译 DLL"两条路径。**DLL 许可是必须提前写清楚的痛点。**

### 7.3 CI

- GitHub Actions `windows-latest`（Windows Server 2025，GA 2025-09-30）**可以**跑原生 GUI 冒烟：Wails v3 就在 windows-latest 上最小化启动 app 并断言日志。可用 `kbinani/screenshot` 截图上传 artifact。
- **无真无头 Windows GUI 测试**（无 xvfb 等价物）：逻辑测试走注入的无头 renderer（下节），边界测试跑真实 session。

### 7.4 测试策略

- govcl/golcl **没有任何测试框架**，也无 goroutine 安全 UI。参照 Fyne 的 `test` 无头驱动模式：**state/reconciliation/diff 全部做成纯逻辑（不泄漏 VCL 类型），测试注入 mock renderer**；govcl 边界只留少量真实 session 冒烟。
- 三条 reconciliation 不变量（§4.4）是核心测试资产。

### 7.5 性能与虚拟列表

- 虚拟列表：**`TListView.OwnerData=true` + `OnData/OnDataHint`**（数据外置，`Items.Count` 设行数，**绝不用 `Items.Add()`**——虚拟模式下静默失败并泄漏）；刷新用 `Invalidate()`/`UpdateItems`，百万行 15-20MB 无延迟。
- 防闪烁：**`WS_EX_COMPOSITED`（仅 resize 循环开启）优于 `DoubleBuffered`**（DoubleBuffered 有视觉怪癖）。
- 批量创建 ~200 控件：属性设置几十 ms、窗口句柄创建数百 ms；`BeginUpdate/EndUpdate` 约 10x 提速。
- diff 循环避免每帧 Go 分配（复用 buffer）。

---

## 8. 社区评价与生态（2022–2026）

### 8.1 govcl/VCL 的社区

- **无英文社区**：HN Algolia 搜索 govcl 仅 1 条 3 分帖（2020-12-19）；r/golang、Lobsters 均无。真实社区全在中国：**QQ 群 263106281**、Gitee 镜像与 wiki、CSDN 教程、itying bbs。作者自述"我的英语不好，可用谷歌翻译中文 wiki"。
- **中文用户共识**：原生、快、体积小（UPX 后 1-2MB）、LCL 组件丰富；但文档弱、DirectUI 式自绘难、Go 版本兼容痛苦（res2go 被 Go 1.20 移除 `-i` 打断；Go>=1.15 要 `-buildmode=exe`）。
- **弃坑担忧公开化**：issue #240（2026-06）"希望别中途掉了"。

### 8.2 Go GUI 大生态的舆论

- HN 2024-2026 共识：**没有生产可用的原生 Go GUI 工具包**；Wails 靠 web 技术主导；Fyne 被评"像 2003 年"、"丑陋"、"样式是痛苦补丁"。
- 2024-2026 明确缺失项（与 FluxVCL 功能清单几乎一一对应）：成熟声明式/响应式模型（"期待 Go 的 Dioxus"）、富数据表格、**早期多窗口支持**、原生对话框/原生观感、**首发带截图**、可访问性/高 DPI/字体打磨、显式绑定的状态管理。
- **Gova（2025 Show HN）教训**：截图是硬门槛（"读 GUI 先要看截图"，甚至有人提议建 arewescreenshottingyet.com）；必被问"这比 X 多了什么？"；多窗口要早期做；注意 AI 生成代码的刻板印象。

### 8.3 Delphi/VCL 2026 的现实

- RAD Studio 12.3 Athens（2025-03）、13 Florence（2025-09）仍在发版；DevExpress 仍在投 VCL（2025 v25.2 路线图含 accessibility/UI Automation）；Delphi TIOBE ~1.44%（2026-05，#10 左右）。
- 社区论调："没人再写新的 Win32 桌面应用了"；人才池老化、高校不教。
- **对 FluxVCL 的启示**：VCL 的控件集与成熟度是资产；"Delphi 包装"的标签是负债。定位必须卖"现代声明式 Go 框架"而非"Delphi 封装"。

### 8.4 可信度警告

大量中文"Go GUI 对比"文章（datasea.cn 等）是 **AI/SEO 内容农场**，含捏造数据（"92% 放弃 Fyne"、"N=1,842 调查"、"4.3% 采用率"等，均无一手来源）。**本项目任何对外材料不得引用此类数据**；只用仓库元数据、官方 changelog、真实 HN/CSDN/juejin 帖。

---

## 9. 对 FluxVCL 的战略结论

1. **修正定位表述**：design.md 写"基于 VCL"，但当前 Go 生态已无活跃 VCL 绑定。需在 Phase 0 明确：**A) 自维护 govcl v1.2.10（真实 VCL）** vs **B) energye/lcl（活跃 LCL）**。默认 A，绑定隔离在窄接口后。**（已被 govcl-vs-lcl.md 决议取代：默认 B = energye/lcl；A 降为 B 计划。）**
2. **Reconciliation 是最大研发风险，也是最大差异化**：自研 retained + diff + 属性级 patch + 批量提交；三条不变量做测试护栏。这是其他 Go GUI 都没做过的事。
3. **布局必须"不实现即可测量"**：intrinsic-size 函数 + GDI 文本测量 + 主题 API + 缓存脏标记；彻底禁用 VCL Align/Anchors。
4. **线程纪律是第二生命线**：自建主线程调度器，State 变更一律 marshalling；动画/异步/IME 都有既定模式。
5. **工程化前置**：DLL 分发与许可、DPI manifest、CI 冒烟、无头测试驱动 —— 从 Phase 0 就位。
6. **产品化从一开始**：中英双语文档、首发截图 + 可跑 demo、7GUIs 式完整性演示、早期多窗口、公开维护政策，正面回应"比 X 多什么"。

---

## 附录 A：来源索引（主要）

**仓库 / 元数据**
- [ying32/govcl](https://github.com/ying32/govcl) · [releases](https://github.com/ying32/govcl/releases) · [last-vcl-support 分支](https://github.com/ying32/govcl/tree/last-vcl-support) · [z-kit.cc 维护声明](https://z-kit.cc/about.html)
- [energye/golcl](https://github.com/energye/golcl) · [energye/lcl](https://github.com/energye/lcl) · [energye/energy](https://github.com/energye/energy) · [ying32/dylib](https://github.com/ying32/dylib)
- [lxn/walk](https://github.com/lxn/walk) · [walk/declarative](https://pkg.go.dev/github.com/lxn/walk/declarative) · [fyne-io/fyne](https://github.com/fyne-io/fyne) · [wailsapp/wails](https://github.com/wailsapp/wails)
- [roblillack/spot](https://github.com/roblillack/spot) · [NV404/gova](https://github.com/NV404/gova) · [facebook/yoga](https://github.com/facebook/yoga)

**关键 issues / PR**
- govcl 线程：[#14](https://github.com/ying32/govcl/issues/14) [#29](https://github.com/ying32/govcl/issues/29) [#165](https://github.com/ying32/govcl/issues/165)
- govcl 内存：[#40](https://github.com/ying32/govcl/issues/40) [#95](https://github.com/ying32/govcl/issues/95) [#96](https://github.com/ying32/govcl/issues/96) [#120](https://github.com/ying32/govcl/issues/120)
- govcl 编码/IME：[#36](https://github.com/ying32/govcl/issues/36) [#126](https://github.com/ying32/govcl/issues/126) [#212](https://github.com/ying32/govcl/issues/212) [#234](https://github.com/ying32/govcl/issues/234)
- govcl 构建/启动：[#23](https://github.com/ying32/govcl/issues/23) [#69](https://github.com/ying32/govcl/issues/69) [#114](https://github.com/ying32/govcl/issues/114) [#201](https://github.com/ying32/govcl/issues/201) [#225](https://github.com/ying32/govcl/issues/225)
- 声明式重建丢状态：[HumbleUI #50](https://github.com/HumbleUI/HumbleUI/issues/50) · [NativeBase #5231](https://github.com/GeekyAnts/NativeBase/issues/5231) · [hatter incremental-rendering](https://github.com/jappeace-sloth/hatter/blob/d6c643f887497c150e8e7a8b0781d31e986a06f1/docs/incremental-rendering-approaches.md)

**核心文档**
- [React reconciliation](https://ru.react.js.org/docs/reconciliation.html) · [React preserving-and-resetting-state](https://react.dev/learn/preserving-and-resetting-state)
- [Flutter box.dart](https://github.com/flutter/flutter/blob/master/packages/flutter/lib/src/rendering/box.dart) · [flex.dart](https://raw.githubusercontent.com/flutter/flutter/master/packages/flutter/lib/src/rendering/flex.dart) · [Widget.canUpdate](https://api.flutter.dev/flutter/widgets/Widget/canUpdate.html)
- [SwiftUI performance WWDC23 #10160](https://nonstrict.eu/wwdcindex/wwdc2023/10160/) · [Compose internals (slot table)](https://blog.stackademic.com/jetpack-compose-internals-the-slot-table-snapshot-system-recomposition-mechanics-fb8117cd2f35)
- [AWT LayoutManager](https://docs.oracle.com/en/java/javase/11/docs/api/java.desktop/java/awt/LayoutManager.html) · [WButtonPeer 源码](https://android.googlesource.com/toolchain/jdk/jdk9_jdk/+/refs/tags/jdk7-b30/src/windows/classes/sun/awt/windows/WButtonPeer.java)
- [Win32 text measurement](https://learn.microsoft.com/en-us/windows/win32/gdi/string-widths-and-heights) · [BCM_GETIDEALSIZE](https://learn.microsoft.com/en-us/windows/win32/Controls/bcm-getidealsize) · [DPI awareness](https://learn.microsoft.com/en-us/windows/win32/hidpi/dpi-awareness-context)
- [Embarcadero DoubleBuffered](https://docwiki.embarcadero.com/Libraries/Athens/en/Vcl.Controls.TWinControl.DoubleBuffered) · [TListView.OwnerData](https://docwiki.embarcadero.com/Libraries/Rio/e/index.php?title=Vcl.ComCtrls.TListView.OwnerData)

**社区讨论**
- HN：[Gova 帖 #47886272](https://news.ycombinator.com/item?id=47886272) · [Spot 帖 #40469592](https://news.ycombinator.com/item?id=40469592) · [Fyne 评价 #40399965](https://news.ycombinator.com/item?id=40399965)
- govcl 中文社区：[Gitee wiki](https://gitee.com/ying32/govcl/wikis/pages) · [itying bbs 评价](https://bbs.itying.com/topic/69163076e1e30e00420d8d1c)

---

*本文档由多路并行调研汇总；所有事实尽量以一手来源（仓库/官方文档/issue 编号）为准，个别二手转述已标注。§6 线程/事件/动画维度已对照 govcl 源码逐条核实。*
