package render

// Mock 是 Renderer 的内存实现，用于无头测试（0.6 无头测试驱动雏形）。
//
// 不创建任何原生控件、不依赖 libenergy DLL：每次调用追加一条 Op 到日志，
// 供测试断言。state/diff 纯逻辑（Phase 1.4 起）与 CI 的 go test 都通过它
// 在无显示环境验证行为 —— 对应 Fyne test 驱动的思路。
type Mock struct {
	ops  []Op
	next Handle
}

// NewMock 创建空的 Mock。
func NewMock() *Mock { return &Mock{} }

func (m *Mock) Create(widgetType string) Handle {
	h := m.alloc()
	m.ops = append(m.ops, Op{Type: OpCreate, Handle: h, Key: widgetType})
	return h
}

func (m *Mock) Destroy(h Handle) {
	m.ops = append(m.ops, Op{Type: OpDestroy, Handle: h})
}

func (m *Mock) SetParent(child, parent Handle) {
	m.ops = append(m.ops, Op{Type: OpAppendChild, Handle: child, Parent: parent})
}

func (m *Mock) SetBounds(h Handle, b Rect) {
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Bounds", Value: b})
}

func (m *Mock) SetVisible(h Handle, v bool) {
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Visible", Value: v})
}

func (m *Mock) SetText(h Handle, text string) {
	m.ops = append(m.ops, Op{Type: OpSetText, Handle: h, Value: text})
}

func (m *Mock) SetEnabled(h Handle, enabled bool) {
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Enabled", Value: enabled})
}

// TextWidth 模拟 intrinsic 测量：mock 无字体，返回按字符数的伪宽度。
// 布局引擎的真实测量在 LCL 适配层实现（design.md §6.2）。
func (m *Mock) TextWidth(h Handle, text string) int {
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "MeasureText", Value: text})
	return len(text) * 8
}

func (m *Mock) HandleAllocated(h Handle) bool { return h != 0 }

func (m *Mock) alloc() Handle { m.next++; return m.next }

// Ops 返回操作日志的副本（断言用，避免外部修改内部状态）。
func (m *Mock) Ops() []Op {
	out := make([]Op, len(m.ops))
	copy(out, m.ops)
	return out
}

// Count 统计日志中某类 op 的数量。
func (m *Mock) Count(t OpType) int {
	n := 0
	for _, op := range m.ops {
		if op.Type == t {
			n++
		}
	}
	return n
}
