package render

import "sync"

// Mock 是 Renderer 的内存实现，用于无头测试（0.6 无头测试驱动雏形）。
//
// 不创建任何原生控件、不依赖 libenergy DLL：每次调用追加一条 Op 到日志，
// 供测试断言。state/diff 纯逻辑（Phase 1.4 起）与 CI 的 go test 都通过它
// 在无显示环境验证行为 —— 对应 Fyne test 驱动的思路。
//
// 线程安全：方法加锁，允许 State 从任意 goroutine 触发 render 的 -race 测试。
// RunOnUI 直接同步执行（mock 无独立 UI 线程）。
type Mock struct {
	mu       sync.Mutex
	ops      []Op
	next     Handle
	clientW  int // 模拟窗体客户区尺寸（缺省 400x300）
	clientH  int
	resizeFn func(w, h int) // 已注册的 resize 回调
	handlers map[Handle]map[string]any // 已注册事件回调（Phase 4 触发测试用）
}

// NewMock 创建空的 Mock。
func NewMock() *Mock { return &Mock{clientW: 400, clientH: 300} }

func (m *Mock) Create(widgetType string) Handle {
	h := m.alloc()
	m.mu.Lock()
	m.ops = append(m.ops, Op{Type: OpCreate, Handle: h, Key: widgetType})
	m.mu.Unlock()
	return h
}

func (m *Mock) Destroy(h Handle) {
	m.mu.Lock()
	m.ops = append(m.ops, Op{Type: OpDestroy, Handle: h})
	m.mu.Unlock()
}

func (m *Mock) SetParent(child, parent Handle) {
	m.mu.Lock()
	m.ops = append(m.ops, Op{Type: OpAppendChild, Handle: child, Parent: parent})
	m.mu.Unlock()
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

func (m *Mock) HandleAllocated(h Handle) bool { return h != 0 }

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
