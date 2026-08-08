# FluxVCL

## 基于 Go + VCL 的现代声明式 UI 框架设计文档

**版本：0.1 Design Draft**

---

# 1. 项目简介

## 1.1 项目定位

FluxVCL 是一个基于 Go 语言的现代声明式 UI 框架。

目标：

> 在保留 Windows 原生 VCL 控件能力的同时，提供类似 Flutter / Vue / SwiftUI 的现代 UI 开发体验。

核心理念：

* 声明式 UI
* 状态驱动
* 现代布局
* 原生控件能力
* 可扩展渲染
* 高级用户可访问底层

---

# 2. 设计目标

## 2.1 用户体验目标

传统 VCL：

```go
button := vcl.NewTButton(form)

button.Left = 20
button.Top = 20
button.Caption = "OK"

button.OnClick = func(sender TObject){

}
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
            OnClick(func(){
                
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

    VCL Renderer          Custom Renderer


                    |

                 Win32

```

---

# 4. 核心概念

---

# 4.1 Component

组件负责：

* 业务逻辑
* 状态管理
* UI组合

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

---

# 4.2 Widget

Widget 是 UI 描述。

接口：

```go
type Widget interface {

    Create() Node

}
```

例如：

```go
Button(
    "OK",
)
```

生成：

```
Widget

Button

{
 text:"OK"
}

```

---

# 4.3 Node Tree

内部结构：

```go
type Node struct {

    Type string

    Props map[string]any

    Children []*Node

}
```

示例：

```
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

---

## 5.2 VCL Renderer

负责：

Widget

↓

VCL Control

例如：

```
Button

↓

TButton

```

---

## 5.3 Custom Renderer

用于：

* Canvas
* 自定义控件
* GPU绘制

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

不使用 VCL Align。

采用现代布局模型。

---

# 6.1 基础布局

## Row

水平：

```go
Row(

 Button("A"),

 Button("B"),

)
```

## Column

垂直：

```go
Column(

 Text("Name"),

 Input(),

)
```

---

# 6.2 Layout 算法

支持：

* Measure
* Layout

流程：

```
Parent

 |

Measure children

 |

Calculate size

 |

Assign position

```

类似：

Flutter RenderBox。

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
type Modifier interface{

    Apply(node *Node)

}
```

---

# 8. State 系统

核心：

状态驱动 UI。

---

## 8.1 State

```go
count:=State(0)
```

绑定：

```go
Text(
    Bind(count),
)
```

修改：

```go
count.Set(1)
```

流程：

```
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

支持：

单向：

```go
Text(
 Bind(user.Name),
)
```

双向：

```go
Input(

 Bind(user.Name),

)
```

---

# 10. Event 系统

统一事件。

```go
type Event struct {


    Source Widget


    X,Y float32


    Type EventType


}
```

支持：

* Mouse
* Keyboard
* Touch

---

# 11. Native Escape Hatch

解决高级需求。

---

## 11.1 Native

```go
Button(

 "OK",

 Native(func(btn *vcl.TButton){

    btn.Color = clRed

 }),

)
```

---

## 11.2 Ref

```go
var ref Ref[TButton]


Button(

 BindRef(&ref),

)
```

使用：

```go
ref.Current.Enabled=false
```

---

## 11.3 Custom Widget

```go
Canvas(

 func(p Painter){

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
OnMount(func(){

    initDatabase()

})
```

---

# 13. 动画系统

目标：

类似 Flutter Animation。

API：

```go
Animate(

 Opacity(0,1),

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

---

# 15. 异步系统

解决：

网络、文件、AI。

API：

```go
Async(

 func(){

    return Load()

 },


 OnSuccess(func(data){

 }),

)
```

---

# 16. Virtual List

解决大量数据。

例如：

100000条数据。

只创建：

```
可见区域控件

```

API：

```go
ListView(

 Items(data),

 Builder(func(item){

 }),

)
```

---

# 17. Key 系统

用于节点身份。

```go
Text(

 user.Name,

 Key(user.ID),

)
```

用于：

* List
* Tree
* Table

---

# 18. Inspector 开发工具

类似：

Flutter Inspector。

功能：

* Widget Tree
* 属性查看
* Layout 调试
* Event 查看

---

# 19. 插件系统

允许：

第三方组件。

例如：

```go
RegisterWidget(

 "Chart",

 ChartWidget,

)
```

---

# 20. 项目结构

```
fluxvcl/


├── core

│   ├── widget

│   ├── component

│   ├── node


├── layout

│

├── state

│

├── event

│

├── render

│   ├── renderer

│   └── vcl


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
* VCL Renderer
* State

目标：

可以写普通桌面程序。

---

## Phase 2

增强：

* Component
* Theme
* Animation
* Native API
* Custom Draw

---

## Phase 3

工程化：

* Inspector
* Plugin
* Virtual List
* Accessibility
* 国际化

---

# 22. 最终定位

FluxVCL 不追求替代 VCL。

而是：

```
VCL

↓

现代 UI 框架层

↓

开发者

```

类似：

```
React     → DOM
Flutter   → Skia
SwiftUI   → UIKit

FluxVCL   → VCL
```

---

# 总结

FluxVCL 的核心不是“封装 VCL”。

而是建立：

> 一个现代声明式 UI 编程模型，并利用 VCL 作为成熟的 Windows 原生后端。

设计重点：

1. Widget Tree
2. State Driven UI
3. Modern Layout
4. Renderer 抽象
5. Native Escape Hatch
6. Custom Drawing
7. Component 化
8. 工程化工具链
