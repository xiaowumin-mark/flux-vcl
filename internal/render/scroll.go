// Phase 6 列表虚拟化滚动接口（development-plan §6.2 / design.md §16）。
//
// 设计：ListView 的滚动位置由框架拥有（State），绑定层只提供"滚动输入设备"——
// 滚轮 / 滚动条拖动 → OnScroll 回调回写 State → re-render → 布局引擎重算可见区。
// FluxVCL 控件池虚拟化保证可见区外不建原生控件（内存有界），原生滚动条范围 =
// 内容总高 − 视口高（DIP，边界处换算物理像素）。
//
// D6 绑定隔离：本文件是 diff 层与绑定层之间关于"滚动"的唯一知识点。diff 层经
// 可选接口（type assertion）使用，绑定层（internal/native 与 Mock）实现；未实现
// 时 ListView 布局照常、仅无原生滚动条（退化，不 panic）。
package render

// ScrollTarget 是 ListView 滚动位置的绑定源（flux 层实现）：
//   - Current()：布局期读取当前滚动偏移（DIP）；
//   - Apply(pos)：原生滚动事件回写（UI 线程），内部 State.Set → 触发 re-render。
//
// 实现应为可比的值类型（持 *State 指针）—— Props 值可比 → D2 跨 render 零 mutation
// （同 State 不重新绑定 OnScroll，D7c 兼容）。
type ScrollTarget interface {
	Current() int
	Apply(pos int)
}

// ScrollConfig 是 ListView 滚动配置（DIP）：内容总高与滚轮每档步长。
// 布局引擎每次 render 写入（内容/步长不变则不产生 diff，D2 属性级 patch）。
type ScrollConfig struct {
	Content int // 内容总高（DIP）
	Step    int // 滚轮每档步长（DIP）
}

// Scrollable 是滚动控件的窄接口（可选能力）：diff 层配置滚动范围/位置、
// 绑定滚动回调。实现方：internal/native（TScrollBox 视口 + 内部 TScrollBar）
// 与 Mock（无头测试驱动）。renderer 未实现本接口时 ListView 无原生滚动条。
type Scrollable interface {
	// SetScrollConfig 配置滚动：内容总高与滚轮步长（DIP）。
	// 内容 <= 视口时隐藏滚动条（无可滚动）。
	SetScrollConfig(h Handle, cfg ScrollConfig)
	// SetScrollPos 设置滚动位置（DIP），滚动条据此更新。
	SetScrollPos(h Handle, pos int)
	// OnScroll 绑定滚动位置变化回调（DIP，UI 线程）。
	OnScroll(h Handle, fn func(pos int))
}
