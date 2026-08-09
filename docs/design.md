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
（`Color`/`FontColor` Opt），切换 = 换一个 Theme 值 → State 触发全量 re-diff → diff 引擎按属性级
patch 只改变化的颜色属性（未变子树零 mutation）。`FontSize/Radius` 为文档字段（native 未接入字体
大小/圆角），诚实标注。

**已知限制（win32 后端，探针实测）**：LCL `TButton` 由 OS 主题绘制（原生 Win32 按钮控件），其
`Color`/`FontColor` 均为空操作 —— LCL 内部状态（`SetColor`/`GetColorResolvingParent`）正确更新、
屏幕像素不变；`TSpeedButton`/`TBitBtn` 背景色同样不渲染（含显式 `Invalidate`/`Repaint`）；`TLabel`
背景 `Color` 也不渲染。主题切换的可见信号实际来自窗体背景（`Window(Color(...))`）与文字 `FontColor`
（Text 的 `FontColor` 经字体对象渲染 ✓）。

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

解决大量数据（如 100000 条），只创建可见区域控件。

API：

```go
ListView(
    Items(data),
    Builder(func(item) {}),
)
```

---

# 17. Key 系统

用于节点身份：

```go
Text(
    user.Name,
    Key(user.ID),
)
```

用于：List / Tree / Table。

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
