package flux

import "github.com/xiaowumin-mark/flux-vcl/internal/widget"

// Widget 是 UI 描述（design.md §4.2）。
//
// 面向用户的声明式 API（Window/Column/Text/Button/Input...）返回本接口的值。
// 自定义组件：实现 Create() 返回节点树（组合既有构造器即可），或直接复用
// 构造器。每次 render 重建，不持有原生状态（D1）。
type Widget = widget.Widget

// Node 是一次控件树构建结果（design.md §4.3）。
//
// 一般由构造器生成，无需用户手写；自定义控件构造器可用 widget 包类型组合。
type Node = widget.Node

// widgetNode 包装一个已构建节点，作为不可变 Widget 返回。
type widgetNode struct{ n *Node }

func (w widgetNode) Create() *Node { return w.n }
