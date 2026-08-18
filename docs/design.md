# FluxVCL

## 基于 Go + 原生控件（LCL/VCL 双后端，默认 LCL）的现代声明式 UI 框架设计文档

> 版本：0.2（修订稿）
> 日期：2026-08-08
> 关联文档：[底座选型调研](./govcl-vs-lcl.md)（默认后端决议）、[调研报告](./research.md)、[开发计划](./development-plan.md)
>
> 修订说明：0.1 版定位"基于 VCL"。专项调研（govcl-vs-lcl.md）后确认 Go 生态已无活跃的 Delphi VCL 绑定，改为 **LCL/VCL 双后端、默认 LCL**（energye/lcl）。本版据此统一修正表述，并将设计文档纳入 docs/ 统一管理。

---

# 1. 项目简介

## 1.1 项目定位

FluxVCL 是一个基于 Go 语言的现代声明式 UI 框架。

目标：

> 在保留桌面原生控件能力的同时，提供类似 Flutter / Vue / SwiftUI 的现代 UI 开发体验。

核心理念：

* 声明式 UI
* 状态驱动
* 现代布局
* 原生控件能力
* 可扩展渲染
* 高级用户可访问底层

## 1.2 后端策略（LCL/VCL 双后端）

| 后端 | 绑定库 | 语义 | 状态 |
|---|---|---|---|
| **LCL（默认）** | `energye/lcl`（LibEnergy 运行时） | Lazarus LCL，跨 Windows/macOS/Linux，零 CGO | 活跃维护（2026）；选用决议见 [govcl-vs-lcl.md §6](./govcl-vs-lcl.md) |
| **VCL（备选 / B 计划）** | `ying32/govcl v1.2.10`（LibVCL 运行时） | Delphi VCL，仅 Windows | 冻结（2020）；仅当产品硬性需要真实 VCL 语义时启用 |

* **绑定隔离**：所有控件访问收敛到窄接口（`Create/SetBounds/SetVisible/TextWidth/HandleAllocated`…），真实绑定库藏在适配层后，切换后端不改动上层声明式代码。
* **事件映射**：显式注册回调，禁用反射方法名绑定（govcl 反射绑定有 garble 失效与误匹配问题）。
* 后续文档中的 `lcl.TButton` 等类型名指绑定层的统一控件抽象；本框架默认面向 LCL，VCL 后端经适配层保持同一抽象。

---

# 2. 设计目标

## 2.1 用户体验目标

传统命令式（以 LCL/VCL 绑定库为例）：

```go
button := lcl.NewButton(form)

button.SetLeft(20)
button.SetTop(20)
button.SetCaption("OK")

button.SetOnClick(func(sender lcl.IObject) {
    // ...
})
```

问题：

* 命令式
* 状态分散
* 布局困难
* 组件复用困难

FluxVCL：

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

特点：

* UI 即结构
* 状态自动同步
* 组件可复用

---

# 3. 总体架构

```text

              Application

                    |

              Component Layer

                    |

              Widget Tree

                    |

        ---------------------------

        |                         |

   Layout Engine            State System


        ---------------------------

                    |

             Render Layer

                    |

        ---------------------------

        |                         |

  Native Renderer          Custom Renderer
  (LCL / VCL)

                    |

             Win32 / 平台原生 API

```

> Native Renderer 把 Widget 映射到绑定层的原生控件（默认 LCL 后端）；Custom Renderer 用于 Canvas / 自定义绘制。

---

# 4. 核心概念

## 4.1 Component

组件负责：

* 业务逻辑
* 状态管理
* UI 组合

示例：

```go
type LoginPage struct{}

func (p LoginPage) Build() Widget {
    return Column(
        Text("Login"),
        Input(),
        Button("Submit"),
    )
}
```

## 4.2 Widget

Widget 是 UI 描述。

接口：

```go
type Widget interface {
    Create() Node
}
```

例如：

```go
Button("OK")
```

生成：

```text
Widget
Button
{
 text: "OK"
}
```

## 4.3 Node Tree

内部结构：

```go
type Node struct {
    Type     string
    Props    map[string]any
    Children []*Node
}
```

示例：

```text
Window
 |
 Column
 |
 + Text
 + Button
```

---

# 5. 渲染系统

## 5.1 Renderer 抽象

```go
type Renderer interface {
    Mount(node *Node)
    Update(node *Node)
    Remove(node *Node)
}
```

## 5.2 Native Renderer

负责：

```text
Widget
  ↓
原生控件（绑定层）
```

例如：

```text
Button
  ↓
TButton（LCL / VCL 同用该控件名）
```

## 5.3 Custom Renderer

用于：

* Canvas
* 自定义控件
* GPU 绘制

接口：

```go
type Painter interface {
    DrawRect()
    DrawText()
    DrawImage()
}
```

---

# 6. Layout 系统

不使用原生 Align。

采用现代布局模型。

## 6.1 基础布局

### Row

水平：

```go
Row(
    Button("A"),
    Button("B"),
)
```

### Column

垂直：

```go
Column(
    Text("Name"),
    Input(),
)
```

## 6.2 Layout 算法

`Text` 与 `Memo` 的 intrinsic 测量会先规范化 CRLF/CR，再按显式换行逐行调用
`Renderer.TextExtent`：宽度取最长行，高度累加各行行高。该规则只处理显式换行，
不推断原生控件的软换行；显式 `Width`/`Height` 仍优先。

支持：

* Measure
* Layout

流程：

```text
Parent
 |
Measure children
 |
Calculate size
 |
Assign position
```

类似 Flutter RenderBox。

---

# 7. Modifier 系统

用于属性扩展。

示例：

```go
Button(
    "OK",
    Width(100),
    Height(40),
    Margin(10),
)
```

内部：

```go
type Modifier interface {
    Apply(node *Node)
}
```

---

# 8. State 系统

核心：状态驱动 UI。

## 8.1 State

```go
count := State(0)
```

绑定：

```go
Text(Bind(count))
```

修改：

```go
count.Set(1)
```

流程：

```text
State
 |
Subscriber
 |
Widget
 |
Renderer
 |
Control update
```

---

# 9. 数据绑定

支持单向 / 双向：

```go
// 单向
Text(Bind(user.Name))

// 双向
Input(Bind(user.Name))
```

`Input`/`Memo` 的文本是受控值。锁定的 LCL 后端会在 `SetText` 边界进入
`textState.applying`，抑制 setter 同步派发的原生 `OnChange`；只有用户编辑才进入
公开回调。该行为由 native probe 覆盖，避免双向转换或带副作用的回调因受控值
回写而重复执行。

**订阅即响应（核心规则）**：re-render 只由**被订阅的** State 触发 —— `State.Set` 只通知
`App.collectBindings` 在 render 时登记过的 App（`state.go`）。State 须经 `Bind(s)`
（或 `ScrollOffset(s)`，§16）出现在当前树里才算订阅；只被**读取**（如 `ListView` 行
builder 里的 `sel.Get()`）或只在事件回调里 **Set** 而未渲染的 State，其 `Set` 只更新
内存值、不触发 re-render —— 下一次无关 render（如 resize）才会读到新值。这是"只观察
声明了的绑定"的刻意设计（D6 窄绑定）；要观察一个 State，把它 `Bind` 出来渲染（哪怕
只是展示读数）。Phase 5 主题 chip 与 Phase 6 选中标记（点击行标记后须 resize 才见
反应）都踩过此坑。

---

# 10. Event 系统

统一事件：

```go
type Event struct {
    Source Widget
    X, Y   float32
    Type   EventType
}
```

支持：

* Mouse
* Keyboard
* Touch

---

# 11. Native Escape Hatch

解决高级需求。

## 11.1 Native

```go
Button(
    "OK",
    Native(func(btn *lcl.TButton) {
        btn.SetColor(types.ClRed)
    }),
)
```

> 默认绑定为 `energye/lcl`（`lcl` 包）；若启用 VCL 后端，适配层提供等价的 `*vcl.TButton` 逃逸。

## 11.2 Ref

```go
var ref Ref[TButton]

Button(BindRef(&ref))
```

使用：

```go
ref.Current.SetEnabled(false)
```

## 11.3 Custom Widget

```go
Canvas(
    func(p Painter) {
        p.DrawCircle()
    },
)
```

---

# 12. 生命周期

组件：

```go
OnMount()
OnUpdate()
OnUnmount()
```

例如：

```go
OnMount(func() {
    initDatabase()
})
```

---

# 13. 动画系统

目标：类似 Flutter Animation。

API：

```go
Animate(
    Opacity(0, 1),
    Duration(300),
)
```

支持：

* Tween
* Curve
* Transition

---

# 14. Theme 系统

统一管理：

```go
Theme{
    Font
    Color
    Radius
    Animation
}
```

支持：

* Light
* Dark
* Windows Fluent

实现取舍（Phase 5.2）：主题是**数据**不是运行时对象 —— 构建函数按当前 `Theme` 显式传颜色
（`Color`/`FontColor` Opt）与标题栏暗色（`DarkTitleBar` Opt），切换 = 换一个 Theme 值 → State
触发全量 re-diff → diff 引擎按属性级 patch 只改变化的颜色属性（未变子树零 mutation）。
`FontSize/Radius` 为文档字段（native 未接入字体大小/圆角）；`DarkTitleBar` 为**已接入**字段
（win32 DWM 沉浸式暗色，见下），诚实标注。

**标题栏暗色（win32，已接入）**：`Window(DarkTitleBar(true))` → 绑定层 `DwmSetWindowAttribute`
（dwmapi.dll，零 CGO syscall）设 `DWMWA_USE_IMMERSIVE_DARK_MODE`（Win10 1809+ 属性 20，更早
回退 19）。DWM 即时重绘标题栏，无需 Recreate/Redraw；老系统不支持时静默保持系统默认。

**已知限制（win32 后端，探针实测）**：LCL `TButton` 由 OS 主题绘制（原生 Win32 按钮控件），其
`Color`/`FontColor` 均为空操作 —— LCL 内部状态（`SetColor`/`GetColorResolvingParent`）正确更新、
屏幕像素不变；`TSpeedButton`/`TBitBtn` 背景色同样不渲染（含显式 `Invalidate`/`Repaint`）；`TLabel`
背景 `Color` 也不渲染。主题切换的可见信号实际来自窗体背景（`Window(Color(...))`）、文字 `FontColor`
（Text 的 `FontColor` 经字体对象渲染 ✓）与标题栏（`DarkTitleBar`，DWM 沉浸式暗色）。

**升级路径（可选，暂未实现）**：让按钮支持主题色需 **owner-draw** 改造 —— 用 `TWinControl.Handle()`
取得按钮 HWND 设 `BS_OWNERDRAW` 样式，经窗体 `SetOnWndProc` 钩子处理 `WM_DRAWITEM`，用 GDI 按
Theme 绘制背景/文字。代价：失去系统 hover/按下原生观感，需自行处理 DPI/禁用态，工作量与回归风险
不小，故本轮保持系统默认外观，主题以窗体背景/文字色落地。

---

# 15. 异步系统

解决：网络、文件、AI。

API：

```go
Async(
    func() { return Load() },
    OnSuccess(func(data) {}),
)
```

---

# 16. Virtual List

解决大量数据（如 100000 条），只创建可见区域控件 —— **控件池虚拟化**
（Phase 6，实现见 `flux.ListView` / `flux/layout.go layoutListView` /
`internal/render/scroll.go` / `internal/native` ListView 分支）。

## API（已落地）

```go
ListView(count int, itemHeight int, builder func(index int) Widget,
    ScrollOffset(scroll *State[int]),  // 可选：滚动位置双向绑定
)
```

## 语义

- **控件池 + 稳定 slot key（D3）**：布局引擎只把"可见区 ± overscan"的数据行构建为
  slot 子节点（`ListViewRow` 透明包装），key = `row-0..row-N`（**槽位**身份，非数据
  下标）。滚动时同一批槽位跨 render 复用：原生控件不创建/不销毁，行内容随槽内
  `builder(index)` 原地属性 patch（`SetText`/`SetBounds`）—— 内存有界（10 万行只建
  ~20 个原生控件）、行内控件焦点/IME 不漂移（D7b）。**约束**：行内容（builder 产物）
  不得带数据依赖 key（否则滚动换内容时重建，破坏控件池）。
- **滚动位置由框架拥有**：`scrollTarget{s *State[int]}` 值类型实现
  `render.ScrollTarget`（`Current()`/`Apply()`），可比值 → `Scroll` 属性跨 render 零
  mutation（D7c，OnScroll 只绑一次）。滚动输入（滚轮 / 滚动条拖动）→ 原生 `OnScroll`
  回写 State → re-render → 布局重算可见区。布局读偏移并**钳制回写** State（值变化时
  才 Apply，触发一次 re-render 收敛；`scroll.Get()` 与滚动条读数不漂移）。
- **等行高是前提**：`itemHeight`（DIP）固定；行高不等需自行保证（或按行分组定高）。
- **必须有界约束**：虚拟列表必须知道 viewport —— 请放 `Expanded` 或固定高度容器内；
  直接放 `Column` 且未给高度会 panic（明确提示，勿静默退化）。

## 绑定层（D6 窄接口）

控件专属能力不加入基础 `render.Renderer`，而是由 diff 按可选窄接口下发；未实现某项能力的 Renderer 安全退化为 no-op，不 panic：

- `render.Scrollable`：`SetScrollConfig`/`SetScrollPos`/`OnScroll`（全 DIP），供 `ListView` 使用。native 实现 = `TScrollBox` 视口（`AutoScroll=false`、隐藏内建双滚动条、`DoubleBuffered` 防闪烁）+ 内部 `TScrollBar`（`SetKind(SbVertical)`，范围 = 内容−视口，页尺寸 = 视口高）。
- `render.Checkable`：`SetChecked`/`OnCheckedChange`，由 `CheckBox` 与 `RadioButton` 复用。
- `render.Selectable`：`SetItems`/`SetSelectedIndex`/`OnSelectionChange`，供 `ComboBox` 使用。
- `render.Progressable`：`SetMinimum`/`SetMaximum`/`SetValue`，供 `ProgressBar` 使用；diff 固定按范围优先、值在后的顺序下发。
- `render.RadioGroupable`：`SetGroupIndex`，将 Flux 逻辑组编号下发给 `RadioButton`；energye/lcl v1.0.3 没有可用的分组 setter，因此 native Renderer 按 resolved native parent + `GroupIndex` 维护同组互斥，并用逐控件内部 host 隔离 LCL 的“同 parent 全部互斥”行为；diff 不遍历或修改兄弟节点。

`Memo`、`CheckBox`、`ComboBox`、`ProgressBar` 和 `RadioButton` 组成当前常用表单基线。`Memo` 的 intrinsic 按显式换行逐行测量（最小编辑区 180×96 DIP），不承诺富文本或软换行自动扩高；`ComboBox` 固定使用 `[]string` Items 和显式受控 `SelectedIndex`（空列表为 `-1`）；`ProgressBar` 默认 `Minimum=0`、`Maximum=100`、`Value=0`，构造时保证 `Minimum ≤ Maximum` 且把 Value 钳制到该闭区间；`RadioButton` 仅在相同原生父容器且 `GroupIndex` 相同时具有原生互斥语义。布尔和选择状态均由调用方在类型化回调中写入 State，并在下次 render 以 `Checked` 或 `SelectedIndex` 回写；本阶段不扩张 `Bind` 的隐式双向语义。

**已知限制（win32，实测）**：`TLabel`（无 HWND，自绘在父表面）的 caption 变化在
`DoubleBuffered` 容器（ListView 视口）内**不保证触发父容器重绘** —— 仅改文字、不改
尺寸时画面会滞留旧文本，直到 resize 强制整窗重绘（Phase 6 实测：点行标记无反应、缩放
窗口才见 ○→●）。native `SetText` 已加显式 `Invalidate()` 保险（无效化合并进同一次
WM_PAINT，无额外开销）。另见 §14 win32 颜色不渲染限制。

## 多窗口（6.3）

第二个窗体 = 第二个 `NewRenderer`/`NewApp`（`Application.NewForms` 首个=主窗体），
次要窗体显式 `native.Renderer.Show()`；独立 State 作用域（各自触发各自 re-render）。
主窗体关闭 → `Application.Run` 退出（含第二窗体打开时）。

---

# 17. Key 系统

`Key` 用于**节点身份**（reconciliation 的 identity），与**寻址**（如何定位一个控件）解耦。

## 身份（identity，跨 render 稳定）

用于 List / Tree / Table 等**动态/可变子节点**，保证重排不重建、焦点/caret/IME 不漂移：

```go
Text(
    user.Name,
    Key(user.ID),   // 必须来自模型，绝不 index、绝不每次 render 随机
)
```

只在以下场景必须写 Key：

- **动态列表/可变子节点**：key 必须来自模型（ID）；index key 会让 VCL 焦点/caret/IME 迁到错行（正确性 bug，不只是性能）。
- **Component 身份**：透明分组跨 render 复用子树，身份靠外部 Key 稳定。
- **`App.SetBounds` 动画目标**：跨 render 保持同一控件。
- **同类型多 handler 需用事件 `e.Source` 区分**。

## 寻址（addressing，静态树定位）

静态树（结构固定、不重排）**可不写 Key**：无 key 控件按位置匹配，且框架为每个
Element 维护树路径 `Path`（如 `"Window/0/Column/1/Text"`），提供隐式寻址：

- `App.FindByPath("Window/0/Column/1/Text")` / `diff.Element.FindByPath` —— 定位 Element
  （测试/排查用），未命中返回 nil。
- 事件 `Event.Source` 无 Key 时回落为 `"Type@路径"`（如 `"Button@Window/0/Column/1/Button"`）。

路径是**位置身份**：结构重排后随之漂移 —— 这正是它只适用于静态树、身份敏感控件
仍须用稳定 Key 的原因。路径格式：首段为根类型（校验），其后交替 `下标/类型`。

---

# 18. Inspector 开发工具

类似 Flutter Inspector，但观测对象是 FluxVCL 的三棵树与提交，而不是直接枚举
Win32 HWND。`TLabel` 没有独立 HWND，透明 Widget/Element 也共享祖先句柄，因此
Inspector 从 `App` / diff 的只读快照构造 Widget → Element → native 对应关系。

## 18.1 只读 observer 与提交边界

`App.ObserveInspector(observer)` 订阅两类记录：

- 每次 render 都发布一个 `InspectorCommit`，包含递增 `RenderID`、mutation 列表、
  create/destroy/reparent/property/event/bounds 统计和 canUpdate 重建原因；相同树也
  发布零 mutation commit，便于证明 D7c。
- 实际事件在调用用户 handler 前发布 `InspectorEventRecord`，覆盖统一 Event、文本
  change、checked/selection change 与滚动回写；handler panic 仍能在日志中定位。

`App.SetBounds` 动画绕过 diff，因此发布 `Direct=true` 的 bounds commit，但不递增
`RenderID`，避免把直接 mutation 误报成 render。observer 得到的 Props/value/slice
均已清洗并深复制，不含函数闭包、State/Ref/原生指针，不能反向修改 diff 状态。

## 18.2 快照与重建风险

`App.InspectorSnapshot()` 返回当前三层树：Type、Key、Path、父路径、Props、DIP
constraints/size/bounds/flex、溢出、native 数字 ID/类型/父级/allocated。原生信息走
`render.NativeInspectable` 可选窄能力一次返回元数据副本，基础 Renderer 接口不因
Inspector 膨胀，快照遍历也不会在后台线程逐项触碰原生控件。

重建只标记非首次挂载的 replacement，并记录 render 序号、旧/新 Type/Key 与
`type-mismatch` / `key-mismatch` / `type-and-key-mismatch`。keyed child 改 key 会绕过
普通 canUpdate（新 key 无旧候选），diff 在 children 匹配点显式识别该风险。

## 18.3 独立工具窗

`inspector.Open(target)` 创建只含只读 Memo 的独立 LCL 工具窗，展示三层树、属性、
布局、事件和 mutation 时间线；重建用 `REBUILD / FOCUS RISK` 醒目标记。工具窗刷新
只读取 `InspectorSnapshot()`，关闭只 unsubscribe，均不调用 target Render/State.Set。
历史使用有界 `InspectorHistory`，默认工具窗保留最近 80 条提交和事件。

---

# 19. 插件系统

FluxVCL 的插件是**编译并链接进同一 Go 进程的组合式 Widget 扩展**。插件通过公开
`flux` API 注册 builder，builder 只返回公开 `Widget` 子树；插件节点自身是透明
Element，不创建原生句柄，也不会进入 `internal/native.Renderer.Create` 的内建控件
switch。这里的“注册/注销”只管理进程内类型目录和 App 使用计数，**不是** DLL、Go
`plugin`、脚本或热加载/卸载机制。

## 19.1 公开 API 与最小示例

```go
const BadgeType = "example.badge"

err := flux.RegisterWidget(BadgeType, flux.WidgetPlugin{
    Build: func(ctx flux.PluginBuildContext) (flux.Widget, error) {
        label, ok := ctx.Properties.String("label")
        if !ok {
            return nil, errors.New("缺少 label")
        }
        return flux.Text("[" + label + "]"), nil
    },
})

badge := flux.PluginWidget(BadgeType, flux.NewPluginProperties(
    flux.PluginString("label", "Ready"),
), flux.Key("status-badge"))
```

公开入口如下：

- `RegisterWidget(name string, descriptor WidgetPlugin) error`：向进程全局注册表注册
  类型。`Build` 必填；注册成功后 descriptor 按值保存。
- `UnregisterWidget(name string) error`：逻辑注销未被 App 使用的类型，不卸载代码。
- `RegisteredWidgets() []string`：返回当前名称的字典序副本，调用方不能修改注册表。
- `PluginWidget(name string, properties PluginProperties, args ...any) Widget`：声明插件
  节点；`args` 只接受 `Widget` 子节点与通用 `Opt`，其他类型 panic。插件解析和 builder
  展开发生在布局/diff/native commit 之前。
- `WidgetPlugin`：包含必填 `Build`，以及可选 `Measure`、App 级 `Init/Close` 和
  实例级 `OnMount/OnUpdate/OnUnmount`。

## 19.2 类型名与类型化 properties

插件类型名区分大小写、进程内唯一，长度不超过 128，格式为
`^[A-Za-z][A-Za-z0-9_.-]*$`。内建类型名（例如 `Button`、`Column`）和框架前缀
`Plugin:` 保留；推荐第三方使用反向域名或项目命名空间（如 `com.example.chart`），避免
依赖注册顺序争抢短名称。非法名称、保留名称、缺少 `Build` 和重复注册分别返回可由
`errors.Is` 判定的错误。

属性不使用 `map[string]any` 逃逸口，而由 `PluginString`、`PluginInt`、
`PluginBool`、`PluginFloat` 构造不可变 `PluginProperty`，再交给
`NewPluginProperties`。属性名采用与类型名相同的格式和 128 字符上限。重名属性最后一个
值生效，`Keys()` 仍保留名称首次出现的稳定顺序；`Keys()`、传入/传出上下文和节点保存
均作防御性复制。插件用 `String/Int/Bool/Float(name)` 读取，缺失或类型不匹配时
`ok=false`。属性构造器收到非法名称，或调用方绕过构造器提交无效零值属性时 panic，
用于尽早暴露编程错误。

当前只承诺上述四种标量属性。复杂数据应由插件在其公开包中定义稳定、可比较的标量编码
或拆分字段，不应把可变 slice/map/原生对象藏进框架 Props。

## 19.3 builder 与组合语义

`Build(PluginBuildContext) (Widget, error)` 收到只读上下文：`Type`、节点 `Key`、
防御性复制的 `Properties`、调用方声明的 `Children` 副本，以及受限的
`PluginContext`。builder 必须返回一个非 nil 的公开 `Widget` 根；调用方子节点是否以及
如何组合进结果，由插件自行决定。框架把返回值递归展开为唯一子树，因此一个插件可以组合
另一个已注册插件，但递归展开深度超过 64 会返回 `ErrPluginCycle`。插件不得构造或依赖
`internal/widget.Node`、`internal/diff.Element`、Renderer、LCL/VCL 对象或原生句柄。

展开后的插件节点保留为透明 Element，下面恰有 builder 的唯一根子树。这让 Inspector、
Key 和实例生命周期仍能观察插件边界，同时原生后端只看见已有的内建 Widget。插件更新时
再次执行 builder，后续复用与 patch 完全交给普通 reconciliation。透明身份由框架在注册
解析成功后写入 `internal/widget.Node` 的不可导出 marker，diff 不信任公开 `Node.Type` 的
`Plugin:` 字符串前缀；手工伪造类型不会绕过注册/展开边界。

## 19.4 DIP Measure 与布局

插件默认尺寸等于 builder 子树在当前 `BoxConstraints` 下的布局尺寸，偏移为 `(0,0)`。
可选 `Measure(PluginMeasureContext) (PluginLayout, error)` 在子树完成布局后调用，输入
包括 `Constraints`、`ChildSize`、类型、Key、properties 和 capability 上下文；全部尺寸
与坐标均为 DIP。返回的 `PluginLayout.Size` 决定透明插件框，`ChildOffset` 将 builder
子树相对插件框左上角平移。

布局顺序固定为：先测 builder 子树 → 调用 `Measure` → 应用插件节点的通用
`Width/Height` Opt（正值覆盖 Measure 对应轴）→ 用当前 constraints 再次
`Constrain` → 应用 `ChildOffset`。Measure 不创建第二棵子树，也不能访问原生测量 API；
插件必须接受最终尺寸被约束钳制。当前不会自动裁剪、居中或校正越界/负偏移，插件负责让
返回尺寸与偏移自洽。

## 19.5 App 与实例生命周期

生命周期分两层：

- **App 级**：某 App 第一次在准备阶段遇到某类型时，取得注册 descriptor 并调用一次
  `Init(PluginContext)`；同一 App 后续实例复用该 runtime。`App.Close()` 先卸载整棵
  Element 树，再按成功 Init/首次使用的逆序调用各插件 `Close`，最后释放注册占用。
  `Close` 幂等且并发安全，插件 Close 失败或 panic 不阻止其余插件关闭，错误最终聚合。
- **实例级**：插件透明 Element 首次提交后调用 `OnMount`，可复用更新时调用
  `OnUpdate`，卸载时调用 `OnUnmount`；上下文包含 Type、Key、properties 和 capability。
  实例钩子没有返回资源句柄，状态与资源由插件按 App/Key 自行管理；Key 必须由调用方按
  D3 提供稳定业务身份。

若 `Init` 失败，该 runtime 不进入 App，也不会调用对应 `Close`。若同次准备中新取得的
插件在后续 Build/Measure 失败，框架只对本次新增 runtime 按逆序 Close 并释放占用；此前
成功 render 已在 App 中使用的插件继续存活。应用必须在 Renderer 消息循环结束前调用
`App.Close()`，保证 unmount/Close 仍可经 Renderer 在 UI 线程执行；关闭后 Mount/Render 返回
`ErrAppClosed`。`App.Close()` 不得在 build/render 栈或用户/插件实例的
Mount/Update/Unmount 生命周期回调内同步调用；提交尚未结束时调用会返回
`ErrAppCloseDuringRender`，App 保持可用，调用方应在回调返回后再关闭。

## 19.6 可选 Renderer capability

`PluginContext` 不公开 Renderer，只能通过类型安全令牌
`LookupCapability[T](ctx, capability)` 读取具名只读能力。框架预定义：

- `RendererDPI Capability[int]`：当前 DPI；默认后端/Mock 通常为 96 或实际 DPI。
- `RendererBackend Capability[string]`：后端标识；当前 LCL 后端为 `"lcl"`，Mock
  为 `"mock"`。

第三方可用 `NewCapability[T]("example.chart.export")` 声明点分命名令牌；名称长度不
超过 160，必须含命名空间分隔。后端仅通过可选的窄 provider 提供值，基础 Renderer
接口不膨胀。能力缺失或运行时类型不匹配统一返回 `ok=false`，插件必须提供安全退化路径，
不得把 capability 存在性当作跨后端保证。capability 不得返回可变原生对象或允许绕过
声明式提交的句柄。

框架在每个插件回调开始前经后端可选的 `PluginCapabilitySnapshot() map[string]any`
一次捕获能力，并复制为该 `PluginContext` 私有的不可变快照。`LookupCapability` 只读该
快照，不会再次调用 Renderer；因此插件可保存上下文并跨 goroutine 读取，App 关闭后也只会
得到历史标量值，不会触碰已销毁的后端。DPI 等动态能力在下一次插件回调创建的新上下文中
刷新，旧上下文保持原值。provider 返回值只接受 nil、bool、string、整数与浮点标量；map、
slice、指针、函数、Renderer、原生对象、句柄或其他可变后端状态会按能力缺失丢弃。框架再
对顶层 map 作复制，因此公开上下文没有可变后端引用。

## 19.7 错误、事务与并发边界

公开哨兵错误包括 `ErrPluginInvalid`、`ErrPluginReserved`、
`ErrPluginAlreadyRegistered`、`ErrPluginNotRegistered`、`ErrPluginInUse`、
`ErrPluginPanic`、`ErrPluginCycle`、`ErrAppClosed` 和 `ErrAppCloseDuringRender`。插件错误包装为
`*PluginError{Name, Stage, Err}`；Stage 为 `register`、`unregister`、`resolve`、`init`、
`build`、`measure`、`mount`、`update`、`unmount` 或 `close`，可同时用 `errors.As`
取得上下文、用 `errors.Is` 判断原因。
所有插件回调 panic 都在边界 recover 并包装为 `ErrPluginPanic`。

事务边界按阶段区分：

- resolve、Init、Build、Measure 属于 **prepare**。任何失败都发生在 diff/native commit
  前，本次不产生半提交 mutation，并回滚本次新取得的插件 runtime。
- Mount/Update/Unmount 属于 **commit/卸载** 生命周期。此时原生 mutation 可能已经发生，
  错误不能回滚已提交 UI；框架捕获后写入 `App.LastError()`，同步 Render 也会返回本次
  观察到的错误。
- App Close 收集卸载阶段的 `LastError` 并聚合全部插件 Close 错误；无论错误或 panic 都
  继续释放注册占用。成功的新 render 会清除旧的 `LastError`。

进程注册表由读写锁保护；并发同名注册至多一个成功，查询返回副本。成功 Init 的每个 App
增加该类型使用计数，存在活跃 App 时 `UnregisterWidget` 返回 `ErrPluginInUse`。App 内
render 由 `renderMu` 串行化；直接 `Mount/Render` 不自动 marshal，调用方必须遵守 App 的
UI 线程纪律，State 失效路径由 `RunOnUI` marshal，`App.Close` 也经 `RunOnUI` 执行。
native 后端的 `RunOnUI` 在当前线程已经是 UI 主线程时内联执行，因此窗体 `OnClose` 置
closed 门后，当前主线程回调仍可完成 `App.Close`；非主线程在 closed 后直接丢弃任务，不再
调用 `RunOnMainThreadSync`。这个关闭期例外不放宽上述 render/实例生命周期的重入限制。
插件回调自身的共享状态、多个 App 间状态及其启动的 goroutine 仍由插件负责同步。注册表线程
安全不意味着任意线程可以触碰 UI。

## 19.8 D1-D7 对照

- **D1 三棵树/canUpdate**：插件声明节点、透明 Element 和 builder 原生子树边界明确；
  Type/Key 不变时复用，改变时只替换目标节点。
- **D2 属性 patch**：插件 properties 驱动 builder 产生普通 Widget，diff 只提交实际变化；
  插件无直接 Renderer 写入口。
- **D3 稳定 Key**：插件实例身份沿用 `flux.Key`；列表/重排场景必须使用模型 key，不能用
  index 或每次 render 生成的随机值。
- **D4 UI 线程**：插件回调跟随 App render/close 的执行边界；直接 Mount/Render 必须在
  UI 线程调用，State 失效和 Close 使用 `RunOnUI`，后台工作经既有 `Async`/State
  marshalling 回到 UI。
- **D5 自定义布局**：Measure 与偏移只用 DIP constraints/Size/Point，不暴露原生 Align
  或物理像素几何。
- **D6 绑定隔离**：第三方仅 import 根包公开 API；Renderer 能力走具名只读窄接口，不能
  import `internal/*` 或 LCL/VCL。
- **D7 测试不变量**：插件属性变化应只 patch builder 子树（D7a）；稳定 Key 重排不得迁移
  实例/焦点（D7b）；同一插件树再次 render 必须零 native mutation（D7c，实例 Update
  回调本身不属于 native mutation）。

## 19.9 兼容政策与已知限制

`RegisterWidget`、`PluginWidget`、`WidgetPlugin` 各上下文、类型化 properties、
capability 令牌和上述错误语义属于公开插件 SDK。`v0.1.x` 补丁版本不删除/改名公开符号，
不改变既有回调顺序、名称规则、DIP/事务语义或哨兵错误判定；只允许 bug 修复和保持源码兼容
的增补。预 1.0 的 minor 版本仍可能 breaking，但必须在 changelog 标注并提供迁移说明；
1.0 后遵循 SemVer。插件必须用具名字段构造 `WidgetPlugin`，不要依赖公开 struct 的字段顺序，
以便后续增加可选回调或上下文字段。
自定义 capability 应使用所有者命名空间；后端是否实现它不构成 SDK 稳定承诺。`internal/*`、
`Plugin:` 编码、`_pluginRuntime` 等内部 Props 以及 LCL 对象均不属于兼容面。

当前限制：仅支持进程内静态链接和逻辑注销；没有插件发现、依赖/版本解析、沙箱、权限、
签名、动态卸载、跨进程隔离或热替换；注册表为进程全局且不能按 App 覆盖同名类型；properties
仅有四种标量；builder 每次 render 可重执行，插件应保持纯且快速；Measure 为单一 builder
子树的后置测量，不能创建多原生根或原生自绘控件；实例回调错误不提供 UI 回滚。需要真正的
新原生控件、绘制 surface 或后端 setter 时，应先设计新的公开窄能力或内建控件，不能借插件
绕过 D5/D6。

---

# 20. 分页容器（P7.2c）

## 20.1 公开 API 与受控选择

LCL v1.0.3 提供 `TPageControl`/`TTabSheet`，没有独立的 `TTabControl`。
FluxVCL 因此只公开一个 `PageControl` 抽象，不提供语义重复的 `TabControl` 别名：

```go
PageControl(
    TabPage("概览", Text("内容"), Key("overview")),
    TabPage("编辑", Input(Key("editor")), Key("editor")),
    SelectedIndex(1),
    OnSelectionChange(func(index int) { /* 写回 State */ }),
)
```

`TabPage(title, child, opts...)` 要求恰好一个非空 `child` 和非空稳定 `Key`；同一
`PageControl` 内 Key 必须唯一。`PageControl` 的子节点只能是 `TabPage`。页面列表为空时
`SelectedIndex=-1`；非空列表默认索引为 `0`；声明的索引统一钳制到 `[-1, pageCount-1]`。
移除 `SelectedIndex` 回落到同一默认值（空列表 `-1`，否则 `0`）。选择是受控状态：原生
页签点击触发 `OnSelectionChange`，程序化 patch 不触发该回调，避免 State 回写形成事件环。
用户切页会把选择标记为待校正；即使下一次 render 仍声明同一索引，框架也会重施该值，
因此回调可以通过不接受新索引来维持受控选择，而不会因属性值“未变化”漏掉 patch。
公开构造器先做快速参数检查；App 在插件 builder 全部展开后再次校验整棵树并规范化索引，
因此手写 Node 或插件不能绕过直系子节点、Key、唯一子树、标题和索引类型约束。校验失败
发生在 diff/native commit 前，本次新增的插件 runtime 同步回滚，不产生半提交 mutation。

## 20.2 三棵树与 native parent

每个 `TabPage` 都是独立的真实原生 parent（LCL `TTabSheet`），页内普通控件直接挂到
对应 `TabSheet`；`Column`/`Row`/`Component` 和插件等透明节点只继承该页面句柄，不能把
子控件提升到 `PageControl` 或 Window。inactive 页面只切换可见页，不设 `TabVisible=false`
也不销毁 HWND，因此 Element、输入焦点、caret 和 IME 状态持续保留。

页面按 Key reconciliation。重排调用原生页索引同步，页面及其子树句柄原地复用；插入/删除
只影响目标页面。`TabPage` 只能直接挂到 `PageControl`，普通控件不能绕过页面直接挂到
`PageControl`；Mock 与 LCL 后端都执行同一防御。同步期间以可嵌套 guard 屏蔽旧、新容器的
原生 `OnChange`，只有用户选择才向 Flux 回调。

分页操作通过 `render.PageController` 可选窄接口实现，基础 `render.Renderer` 不增加页面
专属方法；Mock 和 LCL 后端均实现该能力，缺失能力时 diff 安全退化。

## 20.3 布局与已知限制

`PageControl` 参与普通 DIP constraints，默认尺寸为 `320x220 DIP`，显式 `Width/Height`
优先。布局使用固定的水平总 inset `8 DIP` 与垂直表头/边框预算 `32 DIP`，所有页面内容
填充剩余客户区；页面子树坐标相对于页面客户区 `(0,0)`。极小约束下客户区尺寸钳为零，
不产生负 bounds。`TabPage` 自身
几何由 LCL/widgetset 的 `TCM_AdjustRect` 管理，Flux 不再以普通 `SetBounds` 覆盖它；不同
主题/DPI 下表头实际像素可能与固定 DIP 预算有细微差异，这是当前 v1.0.3 后端限制。

连续切页、resize、DPI 变化和 keyed 重排均不应创建/销毁页面或页内控件。Inspector 会显示
`PageControl -> TabPage -> 子树` 层级及各节点的原生 parent。

---

# 21. 批次 3 机制控件（P7.5）

## 21.1 Slider：显式受控整数范围

`Slider` 的 Widget/Node/Element 类型均为 `Slider`，native 对应水平
`TTrackBar`。`Minimum/Maximum/Value/Step` 在构造结束后统一规范化：默认
`0/100/0/1`，`Maximum < Minimum` 时收敛到 `Minimum`，`Value` 钳制到闭区间，
`Step <= 0` 为确定性 panic。程序化属性 patch 不等于用户输入；只有 TrackBar
的鼠标或键盘变化才通过 `OnValueChange(func(int))` 回写业务 State。

范围按 `Minimum → Maximum → Step → Value` 的固定顺序下发。移除属性分别回落
到 `0/100/1/0`，移除事件必须 nil 解绑。专属 setter 与事件通过
`render.SliderController` 可选接口隔离；Renderer 缺少能力时安全跳过。
布局只支持水平，intrinsic 为 `180×32 DIP`，显式尺寸和普通 constraints 优先。

## 21.2 StringGrid：有界字符串矩阵

`StringGrid` 映射 native `TStringGrid`。公开模型是构造器声明的逻辑 Rows/Columns、
严格矩形 `[][]string`、可选单行表头、列宽、可编辑标志和受控 `GridCell` 选择；
所有 slice 在 Opt、构造器、diff、Mock 和 native 边界深复制。传入的 `Cells` 外层
长度必须精确等于 Rows，每一行长度必须精确等于 Columns；Rows 大于 0 时的空矩阵，
以及短行、长行、缺行或多行均确定性 panic，不做补齐或截断。非法行列、表头或
列宽同样确定性 panic，业务对象和公式不会进入 Grid API。

结构属性按“行列 → 表头/列宽 → Cells → Editable → SelectedCell”下发，避免
先写越界单元格或选择。用户选择由 `OnCellSelect(func(GridCell))` 回写；编辑提交
由 `OnCellEdit(func(GridCell,string))` 回写新的受控 Cells。native 在程序化写
Cells/选择期间设置 applying 门，屏蔽同步事件回环。Grid 没有子 Widget，行身份
属于二维值模型；CRUD 模型的稳定业务 ID 仍由示例保存，排序或过滤不能用行号
替代业务身份。

属性移除采用确定的安全默认值：`GridSize` 回落为 `Rows=0, Columns=1`；`Headers`
和 `ColumnWidths` 回落为空 slice（后者由 native 使用 96 DIP 默认列宽）；`Cells`
回落为当前 shape 的全空字符串矩阵；`Editable` 回落为 `false`；选择回落到有数据
时的 `{Row:0, Column:0, RowOnly:false}`，无数据时为 `{-1,-1}`；两个事件均 nil
解绑。若 shape 与依赖属性在同一次提交中一起移除，先应用安全 shape，再按新 shape
清空 Cells 和选择，不能把旧维度或旧编辑草稿带入下一棵树。

锁定的 energye/lcl v1.0.3 不会为真实鼠标/键盘稳定派发 TStringGrid 的
`OnAfterSelection`。默认 Renderer 保留该事件作为低延迟路径，同时用一个主线程
16ms TTimer 读取 Row/Col，并在同一出口按受控选择去重。轮询器只在至少一个 Grid
绑定选择回调时启用；空闲时禁用并复用同一窗体拥有的实例，窗口关闭时解除回调，
避免在 TTimer 自身回调栈中 Free。它不占用普通鼠标/键盘事件，也不启动 goroutine。
受控 shape、Cells、Editable 或 Selection patch 会丢弃尚未提交的原生编辑草稿，
防止同步结束编辑被 applying 门拦截后留下陈旧提交。

Grid 专属操作走 `render.GridController`；默认 intrinsic 为 `360×220 DIP`。
它继承 TStringGrid 的原生键盘、焦点与编辑器 IME。复杂 cell renderer、无限
数据源、ORM 与 Excel 兼容不在 v0.1.0 范围。列宽以 DIP 保存，并在
`WM_DPICHANGED` 后按新 DPI 重施。

## 21.3 PaintBox：稳定命令值与 invalidate

`PaintBox` 映射 native `TPaintBox`。绘制输入不是回调 Props，而是防御性复制的
`[]PaintCommand` 稳定值；命令目前覆盖清屏和圆形，几何全部使用 DIP。Props 的
深值相等保证相同树 D7c 零 mutation；命令值变化由
`render.PaintController.SetPaintCommands` 更新缓存，并调用 `InvalidatePaint`，
下一次 `OnPaint` 才把 DIP 命令按当前 DPI 转换到 Canvas。事件回调不在 paint
栈内修改原生控件，用户通过普通 DIP `OnMouseDown` 做命中测试并更新 State。

native paint 回调由适配层持有，用户没有拿到 LCL Canvas 的逃逸口。移除命令
回落为空列表并 invalidate；invalidate 只请求重绘，不重建 PaintBox。默认尺寸为
`360×260 DIP`，可由 Width/Height 和 constraints 覆盖。TPaintBox 是无独立 HWND
的 graphic control；UIA/屏幕阅读器不能自动读取图元，v0.1.0 由邻接原生文字
表达选择状态，完整可访问补偿留在 P7.6。`WM_DPICHANGED` 会明确 invalidate，
下一次 paint 使用新 DPI 重算命令几何。

## 21.4 7GUIs 的边界

Timer 只把时间推进放在主线程 timer/动画 pump；CRUD 与 Cells 的过滤、业务 ID、
公式解析和依赖图属于示例层；Circle Drawer 的圆列表、命中、半径编辑和
undo/redo 都是不可变业务 State，PaintBox 仅消费命令。这样三个控件验证了真实
机制，同时没有把示例业务固化为框架 API。

---

# 22. 项目结构

```text
fluxvcl/
├── core
│   ├── widget
│   ├── component
│   └── node
├── layout
├── state
├── event
├── render
│   ├── renderer
│   └── native          # 原生控件后端适配（LCL/VCL）
├── animation
├── theme
├── native
├── inspector
└── examples
```

---

# 23. 开发路线

## Phase 1

基础框架：

* Widget
* Node
* Layout
* Native Renderer（默认 LCL）
* State

目标：可以写普通桌面程序。

## Phase 2

增强：

* Component
* Theme
* Animation
* Native API
* Custom Draw

## Phase 3

工程化：

* Inspector
* Plugin
* Virtual List
* Accessibility
* 国际化

---

# 24. 最终定位

FluxVCL 不追求替代 VCL/LCL。

而是：

```text
原生控件（LCL/VCL）
  ↓
现代 UI 框架层
  ↓
开发者
```

类似：

```text
React     → DOM
Flutter   → Skia
SwiftUI   → UIKit

FluxVCL   → 原生控件（LCL/VCL）
```

---

# 总结

FluxVCL 的核心不是"封装 VCL"。

而是建立：

> 一个现代声明式 UI 编程模型，并利用 LCL/VCL 作为成熟的原生控件后端。

设计重点：

1. Widget Tree
2. State Driven UI
3. Modern Layout
4. Renderer 抽象
5. Native Escape Hatch
6. Custom Drawing
7. Component 化
8. 工程化工具链
