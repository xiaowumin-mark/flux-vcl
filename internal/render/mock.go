package render

import (
	"fmt"
	"sync"
)

// Mock 是 Renderer 的内存实现，用于无头测试（0.6 无头测试驱动雏形）。
//
// 不创建任何原生控件、不依赖 libenergy DLL：每次调用追加一条 Op 到日志，
// 供测试断言。state/diff 纯逻辑（Phase 1.4 起）与 CI 的 go test 都通过它
// 在无显示环境验证行为 —— 对应 Fyne test 驱动的思路。
//
// 线程安全：方法加锁，允许 State 从任意 goroutine 触发 render 的 -race 测试。
// RunOnUI 直接同步执行（mock 无独立 UI 线程）。
type Mock struct {
	mu         sync.Mutex
	ops        []Op
	next       Handle
	clientW    int // 模拟窗体客户区尺寸（缺省 400x300）
	clientH    int
	resizeFn   func(w, h int)             // 已注册的 resize 回调
	handlers   map[Handle]map[string]any  // 已注册事件回调（Phase 4 触发测试用）
	timerFn    func()                     // NewTimer 注册的回调（nil=未注册/已停止；FireTimer 驱动）
	scrolls    map[Handle]*mockScroll     // Phase 6 ListView 滚动状态（Scrollable 测试面）
	checked    map[Handle]*mockCheckable  // 可选控件的状态与回调（Checkable 测试面）
	widgetType map[Handle]string          // 已创建控件类型（RadioButton 逻辑组测试面）
	parents    map[Handle]Handle          // resolved native parent（RadioButton 逻辑组测试面）
	selectable map[Handle]*mockSelectable // 下拉选择控件的状态与回调（Selectable 测试面）
	progress   map[Handle]*mockProgress   // 进度控件的范围和值（Progressable 测试面）
	radioGroup map[Handle]int             // 单选控件原生组编号（RadioGroupable 测试面）
	pages      map[Handle]*mockPages      // 分页容器的页面顺序、受控索引与回调
}

// mockScroll 记录 ListView 滚动配置/位置（Phase 6）。与真实滚动条不同，mock 不
// 呈现滚动 UI —— 只保存状态供断言，并保存 OnScroll 回调供 FireScroll 手动驱动。
type mockScroll struct {
	content  int // ScrollConfig.Content
	step     int // ScrollConfig.Step
	pos      int // 当前滚动位置（DIP）
	onScroll func(int)
}

// mockCheckable 记录 CheckBox 等可选控件的状态与回调。
type mockCheckable struct {
	checked   bool
	onChanged func(bool)
}

// mockSelectable 记录 ComboBox 选项、受控索引和选择回调。
type mockSelectable struct {
	items    []string
	selected int
	onSelect func(int)
}

type mockProgress struct {
	minimum int
	maximum int
	value   int
}

type mockPages struct {
	pages    []Handle
	desired  int
	selected int
	onSelect func(int)
}

// NewMock 创建空的 Mock。
func NewMock() *Mock { return &Mock{clientW: 400, clientH: 300} }

func (m *Mock) Create(widgetType string) Handle {
	h := m.alloc()
	m.mu.Lock()
	if m.widgetType == nil {
		m.widgetType = make(map[Handle]string)
	}
	m.widgetType[h] = widgetType
	m.ops = append(m.ops, Op{Type: OpCreate, Handle: h, Key: widgetType})
	m.mu.Unlock()
	return h
}

func (m *Mock) Destroy(h Handle) {
	m.mu.Lock()
	delete(m.widgetType, h)
	delete(m.parents, h)
	delete(m.checked, h)
	delete(m.selectable, h)
	delete(m.progress, h)
	delete(m.pages, h)
	delete(m.radioGroup, h)
	delete(m.handlers, h)
	delete(m.scrolls, h)
	m.ops = append(m.ops, Op{Type: OpDestroy, Handle: h})
	m.mu.Unlock()
}

func (m *Mock) SetParent(child, parent Handle) {
	m.mu.Lock()
	defer m.mu.Unlock()
	childType, childExists := m.widgetType[child]
	if !childExists {
		panic(fmt.Sprintf("render.Mock: 未知子控件 %d", child))
	}
	if parent == 0 {
		if childType == "TabPage" {
			panic("render.Mock: TabPage 只能直接属于 PageControl")
		}
		return
	}
	parentType, parentExists := m.widgetType[parent]
	if !parentExists {
		panic(fmt.Sprintf("render.Mock: 未知父控件 %d", parent))
	}
	isPage := childType == "TabPage"
	isPageControl := parentType == "PageControl"
	if isPage != isPageControl {
		if isPage {
			panic("render.Mock: TabPage 只能直接属于 PageControl")
		}
		panic("render.Mock: PageControl 只能直接挂载 TabPage")
	}
	if m.parents == nil {
		m.parents = make(map[Handle]Handle)
	}
	m.parents[child] = parent
	m.ops = append(m.ops, Op{Type: OpAppendChild, Handle: child, Parent: parent})
}

func (m *Mock) SetBounds(h Handle, b Rect) {
	m.mu.Lock()
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Bounds", Value: b})
	m.mu.Unlock()
}

func (m *Mock) SetVisible(h Handle, v bool) {
	m.mu.Lock()
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Visible", Value: v})
	m.mu.Unlock()
}

func (m *Mock) SetText(h Handle, text string) {
	m.mu.Lock()
	m.ops = append(m.ops, Op{Type: OpSetText, Handle: h, Value: text})
	m.mu.Unlock()
}

func (m *Mock) SetEnabled(h Handle, enabled bool) {
	m.mu.Lock()
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Enabled", Value: enabled})
	m.mu.Unlock()
}

func (m *Mock) SetColor(h Handle, color Color) {
	m.mu.Lock()
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Color", Value: color})
	m.mu.Unlock()
}

func (m *Mock) SetFontColor(h Handle, color Color) {
	m.mu.Lock()
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "FontColor", Value: color})
	m.mu.Unlock()
}

func (m *Mock) SetTitleBarDark(h Handle, dark bool) {
	m.mu.Lock()
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "TitleBarDark", Value: dark})
	m.mu.Unlock()
}

// NewTimer 存储回调供 FireTimer 手动驱动（mock 无真实定时器/消息泵）。
// 停止函数幂等：置 timerFn 为 nil，后续 FireTimer 为 no-op。
func (m *Mock) NewTimer(intervalMs int, fn func()) (stop func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.timerFn = fn
	return func() {
		m.mu.Lock()
		m.timerFn = nil
		m.mu.Unlock()
	}
}

// FireTimer 手动触发一次已注册的定时器回调（测试驱动动画帧）。
// 未注册/已停止时为 no-op。
func (m *Mock) FireTimer() {
	m.mu.Lock()
	fn := m.timerFn
	m.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// ensureCheckable 取回（必要时创建）控件 h 的选中状态。调用方须持有 m.mu。
func (m *Mock) ensureCheckable(h Handle) *mockCheckable {
	if m.checked == nil {
		m.checked = make(map[Handle]*mockCheckable)
	}
	if m.checked[h] == nil {
		m.checked[h] = &mockCheckable{}
	}
	return m.checked[h]
}

// SetChecked 记录可选控件受控状态。RadioButton 额外模拟 Flux 的逻辑分组契约：
// 同一 resolved parent、同一 GroupIndex 内只能有一个 checked=true。
func (m *Mock) SetChecked(h Handle, checked bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.ensureCheckable(h)
	if checked && m.widgetType[h] == "RadioButton" {
		parent, group := m.parents[h], m.radioGroup[h]
		for peer, peerCheckable := range m.checked {
			if peer != h && m.widgetType[peer] == "RadioButton" &&
				m.parents[peer] == parent && m.radioGroup[peer] == group {
				peerCheckable.checked = false
			}
		}
	}
	c.checked = checked
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Checked", Value: checked})
}

// OnCheckedChange 保存选中状态变化回调供 FireCheckedChange 驱动。
func (m *Mock) OnCheckedChange(h Handle, fn func(bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureCheckable(h).onChanged = fn
	m.ops = append(m.ops, Op{Type: OpSetEvent, Handle: h, Key: "OnCheckedChange", Value: fn})
}

// Checked 返回控件 h 的当前选中状态（测试断言用；未配置返回 false）。
func (m *Mock) Checked(h Handle) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.checked[h]; c != nil {
		return c.checked
	}
	return false
}

// FireCheckedChange 模拟用户改变选中状态（未绑定时为 no-op）。RadioButton 按
// SetChecked 的 Flux 逻辑分组语义更新 peers，回调仍在 mutex 外执行。
func (m *Mock) FireCheckedChange(h Handle, checked bool) {
	m.mu.Lock()
	var fn func(bool)
	if c := m.ensureCheckable(h); c != nil {
		if checked && m.widgetType[h] == "RadioButton" {
			parent, group := m.parents[h], m.radioGroup[h]
			for peer, peerCheckable := range m.checked {
				if peer != h && m.widgetType[peer] == "RadioButton" &&
					m.parents[peer] == parent && m.radioGroup[peer] == group {
					peerCheckable.checked = false
				}
			}
		}
		c.checked = checked
		fn = c.onChanged
	}
	m.mu.Unlock()
	if fn != nil {
		fn(checked)
	}
}

func (m *Mock) ensureProgress(h Handle) *mockProgress {
	if m.progress == nil {
		m.progress = make(map[Handle]*mockProgress)
	}
	if m.progress[h] == nil {
		m.progress[h] = &mockProgress{maximum: 100}
	}
	return m.progress[h]
}

func clampProgress(value, minimum, maximum int) int {
	if maximum < minimum {
		maximum = minimum
	}
	if value < minimum {
		return minimum
	}
	if value > maximum {
		return maximum
	}
	return value
}

// SetMinimum 写入进度条最小值，并保持当前范围和值有效。
func (m *Mock) SetMinimum(h Handle, minimum int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.ensureProgress(h)
	p.minimum = minimum
	if p.maximum < minimum {
		p.maximum = minimum
	}
	p.value = clampProgress(p.value, p.minimum, p.maximum)
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Minimum", Value: minimum})
}

// SetMaximum 写入进度条最大值，并保持当前值有效。
func (m *Mock) SetMaximum(h Handle, maximum int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.ensureProgress(h)
	if maximum < p.minimum {
		maximum = p.minimum
	}
	p.maximum = maximum
	p.value = clampProgress(p.value, p.minimum, p.maximum)
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Maximum", Value: maximum})
}

// SetValue 写入已规范化的受控进度值。
func (m *Mock) SetValue(h Handle, value int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p := m.ensureProgress(h)
	p.value = clampProgress(value, p.minimum, p.maximum)
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Value", Value: p.value})
}

// Progress 返回控件 h 的进度状态副本（测试断言用）。
func (m *Mock) Progress(h Handle) (minimum, maximum, value int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.progress[h]; p != nil {
		return p.minimum, p.maximum, p.value
	}
	return 0, 100, 0
}

// SetGroupIndex 设置 RadioButton 的 Flux 逻辑组编号。若已选中的单选按钮改入
// 一个新组，Mock 与 native Renderer 一样让它成为该组的唯一选中项。
func (m *Mock) SetGroupIndex(h Handle, groupIndex int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.radioGroup == nil {
		m.radioGroup = make(map[Handle]int)
	}
	m.radioGroup[h] = groupIndex
	if c := m.checked[h]; c != nil && c.checked && m.widgetType[h] == "RadioButton" {
		parent := m.parents[h]
		for peer, peerCheckable := range m.checked {
			if peer != h && m.widgetType[peer] == "RadioButton" &&
				m.parents[peer] == parent && m.radioGroup[peer] == groupIndex {
				peerCheckable.checked = false
			}
		}
	}
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "GroupIndex", Value: groupIndex})
}

// GroupIndex 返回控件 h 的单选组编号（测试断言用；未配置返回 0）。
func (m *Mock) GroupIndex(h Handle) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.radioGroup[h]
}

// ensureSelectable 取回（必要时创建）控件 h 的选择状态。调用方须持有 m.mu。
func (m *Mock) ensureSelectable(h Handle) *mockSelectable {
	if m.selectable == nil {
		m.selectable = make(map[Handle]*mockSelectable)
	}
	if m.selectable[h] == nil {
		m.selectable[h] = &mockSelectable{items: []string{}, selected: -1}
	}
	return m.selectable[h]
}

func normalizeSelectedIndex(items []string, index int) int {
	if len(items) == 0 {
		return -1
	}
	if index < -1 {
		return -1
	}
	if index >= len(items) {
		return len(items) - 1
	}
	return index
}

func cloneItems(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	return append([]string(nil), items...)
}

// SetItems 记录 ComboBox 的选项，并使当前索引始终相对于新选项合法。
func (m *Mock) SetItems(h Handle, items []string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.ensureSelectable(h)
	c.items = cloneItems(items)
	c.selected = normalizeSelectedIndex(c.items, c.selected)
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Items", Value: cloneItems(c.items)})
}

// SetSelectedIndex 记录 ComboBox 的受控选中索引。
func (m *Mock) SetSelectedIndex(h Handle, index int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.ensureSelectable(h)
	c.selected = normalizeSelectedIndex(c.items, index)
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "SelectedIndex", Value: c.selected})
}

// OnSelectionChange 保存选择变化回调供 FireSelectionChange 驱动。
func (m *Mock) OnSelectionChange(h Handle, fn func(int)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureSelectable(h).onSelect = fn
	m.ops = append(m.ops, Op{Type: OpSetEvent, Handle: h, Key: "OnSelectionChange", Value: fn})
}

// Items 返回控件 h 的选项副本（测试断言用）。
func (m *Mock) Items(h Handle) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.selectable[h]; c != nil {
		return cloneItems(c.items)
	}
	return []string{}
}

// SelectedIndex 返回控件 h 的当前选中索引（测试断言用）。
func (m *Mock) SelectedIndex(h Handle) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if c := m.selectable[h]; c != nil {
		return c.selected
	}
	return -1
}

// FireSelectionChange 模拟用户选择选项。回调在锁外执行以允许其触发 re-render。
func (m *Mock) FireSelectionChange(h Handle, index int) {
	m.mu.Lock()
	var fn func(int)
	if c := m.selectable[h]; c != nil {
		c.selected = normalizeSelectedIndex(c.items, index)
		index = c.selected
		fn = c.onSelect
	}
	m.mu.Unlock()
	if fn != nil {
		fn(index)
	}
}

func (m *Mock) ensurePages(h Handle) *mockPages {
	if m.pages == nil {
		m.pages = make(map[Handle]*mockPages)
	}
	if m.pages[h] == nil {
		m.pages[h] = &mockPages{desired: -1, selected: -1}
	}
	return m.pages[h]
}

func normalizePageIndex(count, index int) int {
	if count == 0 {
		return -1
	}
	if index < -1 {
		return -1
	}
	if index >= count {
		return count - 1
	}
	return index
}

// SyncPages 记录 PageControl 当前按 Element 顺序排列的 TabPage 句柄，并在页面
// 全部就绪后应用先前缓存的受控索引。
func (m *Mock) SyncPages(parent Handle, pages []Handle) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.widgetType[parent] != "PageControl" {
		panic(fmt.Sprintf("render.Mock: 分页父控件 %d 非 PageControl", parent))
	}
	seen := make(map[Handle]struct{}, len(pages))
	for _, page := range pages {
		if m.widgetType[page] != "TabPage" {
			panic(fmt.Sprintf("render.Mock: PageControl 子控件 %d 非 TabPage", page))
		}
		if _, exists := seen[page]; exists {
			panic(fmt.Sprintf("render.Mock: PageControl 页面句柄 %d 重复", page))
		}
		seen[page] = struct{}{}
	}
	p := m.ensurePages(parent)
	p.pages = append([]Handle(nil), pages...)
	p.selected = normalizePageIndex(len(p.pages), p.desired)
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: parent, Key: "Pages", Value: append([]Handle(nil), pages...)})
}

// SetPageSelectedIndex 缓存并应用 PageControl 的受控索引。程序化设置不会触发回调。
func (m *Mock) SetPageSelectedIndex(parent Handle, index int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.widgetType[parent] != "PageControl" {
		return
	}
	p := m.ensurePages(parent)
	p.desired = index
	p.selected = normalizePageIndex(len(p.pages), index)
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: parent, Key: "SelectedIndex", Value: p.selected})
}

// OnPageSelectionChange 保存 PageControl 的用户选择回调；nil 表示解绑。
func (m *Mock) OnPageSelectionChange(parent Handle, fn func(int)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.widgetType[parent] != "PageControl" {
		return
	}
	m.ensurePages(parent).onSelect = fn
	m.ops = append(m.ops, Op{Type: OpSetEvent, Handle: parent, Key: "OnSelectionChange", Value: fn})
}

// Pages 返回 PageControl 的页面句柄顺序副本（测试断言用）。
func (m *Mock) Pages(parent Handle) []Handle {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.pages[parent]; p != nil {
		return append([]Handle(nil), p.pages...)
	}
	return []Handle{}
}

// PageSelectedIndex 返回 PageControl 当前选中索引（测试断言用）。
func (m *Mock) PageSelectedIndex(parent Handle) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if p := m.pages[parent]; p != nil {
		return p.selected
	}
	return -1
}

// FirePageSelectionChange 模拟用户切换 PageControl 页签，回调在锁外执行。
func (m *Mock) FirePageSelectionChange(parent Handle, index int) {
	m.mu.Lock()
	if m.widgetType[parent] != "PageControl" {
		m.mu.Unlock()
		return
	}
	p := m.ensurePages(parent)
	p.selected = normalizePageIndex(len(p.pages), index)
	index = p.selected
	fn := p.onSelect
	m.mu.Unlock()
	if fn != nil {
		fn(index)
	}
}

// TextExtent 模拟 intrinsic 测量：mock 无字体，返回按字符数的稳定伪值
// （宽=len*8、高=20，与 Phase 1 占位一致，保证布局测试断言稳定）。
// 布局引擎的真实测量在 LCL 适配层实现（design.md §6.2）。
// 查询不产生 mutation op（布局 pass 每次 render 都调用，计入会污染 diff 断言）。
func (m *Mock) TextExtent(text string) (int, int) { return len(text) * 8, 20 }

func (m *Mock) SetEvent(h Handle, event string, fn any) {
	m.mu.Lock()
	if m.handlers == nil {
		m.handlers = make(map[Handle]map[string]any)
	}
	if m.handlers[h] == nil {
		m.handlers[h] = make(map[string]any)
	}
	m.handlers[h][event] = fn
	m.ops = append(m.ops, Op{Type: OpSetEvent, Handle: h, Key: event, Value: fn})
	m.mu.Unlock()
}

// EventHandler 返回控件 h 上已注册的事件回调（Phase 4 测试触发用；未注册返回
// nil）。mock 不驱动原生消息循环，测试取回调后自行调用以模拟事件分发。
func (m *Mock) EventHandler(h Handle, event string) any {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.handlers == nil || m.handlers[h] == nil {
		return nil
	}
	return m.handlers[h][event]
}

// AttachRef 在 mock 下无真实原生对象：注入 nil 作为占位。
func (m *Mock) AttachRef(h Handle, ref Ref) {
	if ref != nil {
		ref.SetNative(nil)
	}
	m.mu.Lock()
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Ref", Value: ref})
	m.mu.Unlock()
}

// ApplyNative 在 mock 下无真实原生对象：传 nil 给逃逸函数。
func (m *Mock) ApplyNative(h Handle, fn func(obj any)) {
	if fn != nil {
		fn(nil)
	}
	m.mu.Lock()
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Native", Value: "fn"})
	m.mu.Unlock()
}

// RunOnUI 直接同步执行：mock 无独立 UI 线程，State 触发的 render
// 在调用 goroutine 内完成（测试用 WaitGroup 同步后断言）。
func (m *Mock) RunOnUI(fn func()) {
	if fn != nil {
		fn()
	}
}

// PluginCapabilitySnapshot 返回插件可安全保存和跨 goroutine 读取的只读能力快照。
func (m *Mock) PluginCapabilitySnapshot() map[string]any {
	return map[string]any{
		"flux.renderer.dpi":     96,
		"flux.renderer.backend": "mock",
	}
}

func (m *Mock) HandleAllocated(h Handle) bool { return h != 0 }

// InspectNative 返回全部 Mock 控件的只读类型、父级和分配状态。
func (m *Mock) InspectNative() NativeSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make(NativeSnapshot, len(m.widgetType))
	for h, widgetType := range m.widgetType {
		out[h] = NativeInfo{Type: "Mock" + widgetType, Parent: m.parents[h], Allocated: true}
	}
	return out
}

// ClientSize 返回模拟窗体客户区尺寸（缺省 400x300，与 Phase 1 Window 默认一致）。
func (m *Mock) ClientSize() (int, int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.clientW, m.clientH
}

// SetClientSize 设置模拟窗体客户区尺寸（测试钩子：模拟用户拖拽 resize 后的结果）。
func (m *Mock) SetClientSize(w, h int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clientW, m.clientH = w, h
}

// OnResize 注册 resize 回调（覆盖式）。mock 无独立 UI 线程，回调在 TriggerResize 内
// 同步执行（与 RunOnUI 内联一致）。
func (m *Mock) OnResize(fn func(w, h int)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resizeFn = fn
}

// TriggerResize 模拟 resize 事件（测试钩子）：更新尺寸后调用已注册回调。
func (m *Mock) TriggerResize(w, h int) {
	m.mu.Lock()
	fn := m.resizeFn
	m.clientW, m.clientH = w, h
	m.mu.Unlock()
	if fn != nil {
		fn(w, h)
	}
}

// —— Phase 6 滚动（Scrollable 实现）——

// ensureScroll 取回（必要时创建）控件 h 的滚动状态。调用方须持有 m.mu。
func (m *Mock) ensureScroll(h Handle) *mockScroll {
	if m.scrolls == nil {
		m.scrolls = make(map[Handle]*mockScroll)
	}
	if m.scrolls[h] == nil {
		m.scrolls[h] = &mockScroll{}
	}
	return m.scrolls[h]
}

// SetScrollConfig 记录滚动配置（diff 只在值变化时调用，零 mutation 兼容）。
func (m *Mock) SetScrollConfig(h Handle, cfg ScrollConfig) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.ensureScroll(h)
	s.content = cfg.Content
	s.step = cfg.Step
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "ScrollConfig", Value: cfg})
}

// SetScrollPos 记录滚动位置（DIP）。
func (m *Mock) SetScrollPos(h Handle, pos int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.ensureScroll(h)
	s.pos = pos
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "ScrollPos", Value: pos})
}

// OnScroll 保存滚动回调供 FireScroll 驱动（mock 无原生滚动条/消息循环）。
func (m *Mock) OnScroll(h Handle, fn func(int)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := m.ensureScroll(h)
	s.onScroll = fn
	m.ops = append(m.ops, Op{Type: OpSetEvent, Handle: h, Key: "Scroll", Value: fn})
}

// ScrollPos 返回控件 h 的当前滚动位置（DIP，测试断言用；未配置返回 0）。
func (m *Mock) ScrollPos(h Handle) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.scrolls[h]; s != nil {
		return s.pos
	}
	return 0
}

// ScrollContent 返回控件 h 的滚动内容总高（DIP，测试断言用；未配置返回 0）。
func (m *Mock) ScrollContent(h Handle) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s := m.scrolls[h]; s != nil {
		return s.content
	}
	return 0
}

// FireScroll 模拟滚动事件（Phase 6 测试驱动）：以 pos 调用已绑定的 OnScroll 回调
// （未绑定/未配置时为 no-op）。模拟用户滚轮/拖动滚动条 → 框架回写 State → re-render。
func (m *Mock) FireScroll(h Handle, pos int) {
	m.mu.Lock()
	var fn func(int)
	if s := m.scrolls[h]; s != nil {
		fn = s.onScroll
	}
	m.mu.Unlock()
	if fn != nil {
		fn(pos)
	}
}

func (m *Mock) alloc() Handle {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.next++
	return m.next
}

// Ops 返回操作日志的副本（断言用，避免外部修改内部状态）。
func (m *Mock) Ops() []Op {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Op, len(m.ops))
	copy(out, m.ops)
	return out
}

// Count 统计日志中某类 op 的数量。
func (m *Mock) Count(t OpType) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, op := range m.ops {
		if op.Type == t {
			n++
		}
	}
	return n
}
