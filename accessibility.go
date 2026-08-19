package flux

import "github.com/xiaowumin-mark/flux-vcl/internal/widget"

const tabOrderKey = "_tabOrder"

// assignTabOrder 把声明树顺序投影为每个原生父级内的稳定 TabOrder。透明节点
// 与插件组合不创建原生父级，因此其后代继续占用外层顺序；真实控件的子树则
// 开启新的局部顺序。该属性参与普通 diff，keyed 重排只 patch 顺序而不重建。
func assignTabOrder(root *Node) {
	if root == nil {
		return
	}
	assignTabOrderChildren(root.Children, 0)
}

func assignTabOrderChildren(children []*Node, next int) int {
	for _, child := range children {
		if child == nil {
			continue
		}
		if accessibilityTransparent(child) {
			next = assignTabOrderChildren(child.Children, next)
			continue
		}
		if accessibilityHasTabOrder(child) {
			child.Props.Set(tabOrderKey, next)
			next++
		}
		assignTabOrderChildren(child.Children, 0)
	}
	return next
}

func accessibilityHasTabOrder(n *Node) bool {
	// LCL TLabel and TPaintBox are TGraphicControl descendants without an HWND
	// or TabOrder. Every other current native Flux control is a TWinControl.
	return n != nil && n.Type != "Text" && n.Type != "PaintBox"
}

func accessibilityTransparent(n *Node) bool {
	if n == nil {
		return false
	}
	switch n.Type {
	case "Column", "Row", "Expanded", "Flexible", "Component", "ListViewRow":
		return true
	default:
		return widget.IsPlugin(n)
	}
}
