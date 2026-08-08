package flux

import "github.com/xiaowumin-mark/flux-vcl/internal/render"

// 占位布局（Phase 1.5：design.md §6 的简化先行实现；Phase 3 精修为
// Flutter 风格 BoxConstraints/Measure/Layout 两遍算法）。
//
// 规则：
//   - intrinsic 尺寸：Text 宽=文本测量、高=20；Button 宽=文本宽+32（≥88）、高=32；
//     Input 宽=180、高=28。Width/Height Opt 覆盖 intrinsic。
//   - Column 垂直堆叠（gap 4）、Row 水平堆叠（gap 4）；容器宽高 = children 堆叠和。
//   - Window 固定 400x300（Width/Height Opt 可改），children 按列堆叠。
//   - 每个节点的最终位置写入 Props["Bounds"]（render.Rect），供 diff 引擎 SetBounds。

const layoutGap = 4

// layoutTree 布局一棵子树，返回其内容尺寸；并把节点最终 Bounds 写入 Props。
func layoutTree(n *Node, r render.Renderer) render.Rect {
	switch n.Type {
	case "Window":
		w, h := 400, 300
		if v := n.Props.Int("Width"); v > 0 {
			w = v
		}
		if v := n.Props.Int("Height"); v > 0 {
			h = v
		}
		b := render.Rect{X: 0, Y: 0, W: w, H: h}
		n.Props.Set("Bounds", b)
		stackChildren(n, r, false)
		return b
	case "Column":
		b := stackChildren(n, r, false)
		n.Props.Set("Bounds", b)
		return b
	case "Row":
		b := stackChildren(n, r, true)
		n.Props.Set("Bounds", b)
		return b
	case "Text":
		b := render.Rect{W: r.TextWidth(n.Props.String("Text")), H: 20}
		applySizeOpts(n, &b)
		n.Props.Set("Bounds", b)
		return b
	case "Button":
		w := r.TextWidth(n.Props.String("Text")) + 32
		if w < 88 {
			w = 88
		}
		b := render.Rect{W: w, H: 32}
		applySizeOpts(n, &b)
		n.Props.Set("Bounds", b)
		return b
	case "Input":
		b := render.Rect{W: 180, H: 28}
		applySizeOpts(n, &b)
		n.Props.Set("Bounds", b)
		return b
	default: // 未知类型（含第三方控件）：默认尺寸
		b := render.Rect{W: 100, H: 32}
		applySizeOpts(n, &b)
		n.Props.Set("Bounds", b)
		return b
	}
}

// stackChildren 把 children 依次堆叠（垂直或水平），返回容器内容尺寸。
func stackChildren(n *Node, r render.Renderer, horizontal bool) render.Rect {
	var main, cross int
	for _, c := range n.Children {
		sz := layoutTree(c, r)
		if b, ok := c.Props.Get("Bounds"); ok {
			br := b.(render.Rect)
			if horizontal {
				br.X, br.Y = main, 0
			} else {
				br.X, br.Y = 0, main
			}
			c.Props.Set("Bounds", br)
		}
		if horizontal {
			main += sz.W + layoutGap
			if sz.H > cross {
				cross = sz.H
			}
		} else {
			main += sz.H + layoutGap
			if sz.W > cross {
				cross = sz.W
			}
		}
	}
	main -= layoutGap
	if main < 0 {
		main = 0
	}
	if horizontal {
		return render.Rect{W: main, H: cross}
	}
	return render.Rect{W: cross, H: main}
}

// applySizeOpts 用 Width/Height Opt 覆盖 intrinsic 尺寸。
func applySizeOpts(n *Node, b *render.Rect) {
	if v := n.Props.Int("Width"); v > 0 {
		b.W = v
	}
	if v := n.Props.Int("Height"); v > 0 {
		b.H = v
	}
}
