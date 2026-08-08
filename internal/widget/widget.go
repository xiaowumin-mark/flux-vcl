// Package widget 定义 FluxVCL 的 Widget 声明树（design.md §4.2）。
//
// Widget 是每次 render 重建的不可变 Go 结构体（纯数据，不持有原生指针），
// 是 reconciliation 的输入。Node 是 Widget.Create() 的产物（design.md §4.3），
// 由 diff 引擎（internal/diff）消费：按 D1 canUpdate 匹配 → D2 属性级 patch。
package widget

import "reflect"

// Widget 是 UI 描述。声明式 API（flux.Window/Column/Text/...）返回实现本接口
// 的值；每次 render 调用 Create() 重建节点树，不持有任何原生状态（D1）。
type Widget interface {
	Create() *Node
}

// Node 是一次控件树构建结果（design.md §4.3）。
//
// Type 是控件类型名（"Window"/"Column"/"Button"/...），由 Renderer 适配层
// 映射到原生控件。Props 是属性集合；Children 是子节点；Key 是稳定身份（D3），
// 用于列表 diff 时的 canUpdate 匹配，可空。
type Node struct {
	Type     string
	Props    *Props
	Children []*Node
	Key      string
}

// NewNode 创建带空 Props 的节点（Props 恒非 nil，保证属性 diff 稳定）。
func NewNode(t string) *Node {
	return &Node{Type: t, Props: NewProps()}
}

// Add 追加子节点并返回自身（内部链式构造用）。
func (n *Node) Add(c *Node) *Node {
	n.Children = append(n.Children, c)
	return n
}

// IsFunc 报告值是否为函数类型。函数值（事件回调、逃逸口）无法比较相等性，
// diff 时恒判定为需要重新绑定（D2 逃逸口行为）。
func IsFunc(v any) bool {
	return v != nil && reflect.TypeOf(v).Kind() == reflect.Func
}

// ValuesEqual 报告两个属性值是否相等，用于 D2 属性级 diff。
//
// 函数值视为不相等（总是重新绑定）；其余基本类型直接比较，未知类型
// 回退到 reflect.DeepEqual。
func ValuesEqual(a, b any) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	if IsFunc(a) || IsFunc(b) {
		return false
	}
	switch av := a.(type) {
	case string:
		bv, ok := b.(string)
		return ok && av == bv
	case bool:
		bv, ok := b.(bool)
		return ok && av == bv
	case int:
		bv, ok := b.(int)
		return ok && av == bv
	default:
		return reflect.DeepEqual(a, b)
	}
}
