package flux

import (
	"fmt"

	"github.com/xiaowumin-mark/flux-vcl/internal/widget"
)

// ListView 虚拟滚动列表（design.md §16 Virtual List / development-plan §6）。
//
//	ListView(total, itemHeight, func(index int) Widget, ScrollOffset(scroll))
//
// 语义：
//   - count 数据行数；itemHeight 每行高度（DIP，所有行等高 —— 虚拟化的前提）。
//   - builder(index) 按数据下标构建行内容（可含 Bind/事件/焦点控件）。
//   - 布局引擎只把"可见区 ± overscan"的行交给 diff 挂载（6.2 虚拟化），控件池按
//     slot key（row-0..row-N）跨 render 复用 —— 10 万行也只创建 ~20 个原生控件，
//     内存有界、滚动流畅（滚动 = 属性 patch，不重建；行内控件焦点/IME 不漂移，
//     D3/D7b）。行内容更新 = 同一 slot 的 builder 产物原地 patch（SetText 等）。
//   - 滚动位置双向绑定（ScrollOffset）：编程滚动 scroll.Set(px) / 用户滚动
//     （滚轮、拖动滚动条）原生回写 State → re-render。
//
// 约束：ListView 必须有界约束（虚拟列表必须有 viewport）—— 请放在 Expanded 或
// 固定 Height 容器内；直接放 Column 且未给高度会 panic（明确提示，勿静默退化）。
func ListView(count, itemHeight int, builder func(index int) Widget, opts ...Opt) Widget {
	n := widget.NewNode("ListView")
	n.Props.Set("ItemCount", count)
	n.Props.Set("ItemHeight", itemHeight)
	n.Props.Set("Builder", builder)
	applyOpts(n, opts)
	return widgetNode{n}
}

// ScrollOffset 把列表滚动位置绑定到 State（双向，Phase 6）：
//
//   - 编程滚动：scroll.Set(px) → re-render → 列表跳到偏移 px（布局钳制到有效范围）；
//   - 用户滚动：滚轮 / 滚动条拖动 → 原生 OnScroll 回写 State → re-render。
//
// scroll 为滚动偏移（DIP）。缺省不绑定时列表不可滚动（内容截断在视口内）。
func ScrollOffset(s *State[int]) Opt {
	return optFn(func(n *Node) {
		st := scrollTarget{s: s}
		n.Props.Set("Scroll", st) // 布局读 Current / diff 绑 OnScroll
		n.Props.Set(bindKey, st)  // collectBindings 订阅 → Set 触发 re-render
	})
}

// scrollTarget 是滚动位置的绑定适配器：实现 render.ScrollTarget（布局/原生滚动
// 回调）与 bindable（collectBindings 订阅）。
//
// 值类型（唯一字段是 *State[int]）→ Props 值可比（reflect.DeepEqual 对同一
// State 指针为真）→ D2 跨 render 不产生 mutation、不重复绑定 OnScroll（D7c）。
type scrollTarget struct{ s *State[int] }

func (st scrollTarget) Current() int                    { return st.s.Get() }
func (st scrollTarget) Apply(pos int)                   { st.s.Set(pos) }
func (st scrollTarget) renderText() string              { return fmt.Sprint(st.s.Get()) }
func (st scrollTarget) onChange() func(string)          { return nil }
func (st scrollTarget) subscription() stateSubscription { return st.s }
