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

类似 Flutter Inspector。

功能：

* Widget Tree
* 属性查看
* Layout 调试
* Event 查看

---

# 19. 插件系统

允许第三方组件：

```go
RegisterWidget(
    "Chart",
    ChartWidget,
)
```

---

# 20. 项目结构

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

# 21. 开发路线

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

# 22. 最终定位

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
