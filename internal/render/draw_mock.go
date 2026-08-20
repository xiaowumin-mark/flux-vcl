package render

import "fmt"

type mockDraw struct {
	list          DrawList
	invalidations int
}

var _ DrawController = (*Mock)(nil)

// SetDrawList stores an independent validated snapshot for a DrawSurface.
func (m *Mock) SetDrawList(h Handle, list DrawList) {
	if err := ValidateDrawList(list); err != nil {
		panic(fmt.Sprintf("render.Mock: invalid DrawList: %v", err))
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.draws == nil {
		m.draws = make(map[Handle]*mockDraw)
	}
	if m.draws[h] == nil {
		m.draws[h] = &mockDraw{}
	}
	m.draws[h].list = list.Clone()
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "DrawList", Value: list.Clone()})
}

// ResetDrawList clears the current DrawSurface list without rebuilding it.
func (m *Mock) ResetDrawList(h Handle) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.draws == nil {
		m.draws = make(map[Handle]*mockDraw)
	}
	if m.draws[h] == nil {
		m.draws[h] = &mockDraw{}
	}
	m.draws[h].list = DrawList{}
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "DrawList", Value: DrawList{}})
}

// InvalidateDraw records a repaint request for a DrawSurface.
func (m *Mock) InvalidateDraw(h Handle) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.draws == nil {
		m.draws = make(map[Handle]*mockDraw)
	}
	if m.draws[h] == nil {
		m.draws[h] = &mockDraw{}
	}
	m.draws[h].invalidations++
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "InvalidateDraw", Value: m.draws[h].invalidations})
}

// DrawList returns a defensive snapshot for headless assertions.
func (m *Mock) DrawList(h Handle) DrawList {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d := m.draws[h]; d != nil {
		return d.list.Clone()
	}
	return DrawList{}
}

// DrawInvalidations returns the number of repaint requests for h.
func (m *Mock) DrawInvalidations(h Handle) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if d := m.draws[h]; d != nil {
		return d.invalidations
	}
	return 0
}
