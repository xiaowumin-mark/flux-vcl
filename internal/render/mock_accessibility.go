package render

type mockA11y struct {
	name        string
	description string
	value       string
	tabOrder    int
	tabStop     bool
	defaultBtn  bool
	cancelBtn   bool
}

func (m *Mock) ensureA11y(h Handle) *mockA11y {
	if m.a11y == nil {
		m.a11y = make(map[Handle]*mockA11y)
	}
	state := m.a11y[h]
	if state == nil {
		state = &mockA11y{tabStop: defaultMockTabStop(m.widgetType[h])}
		m.a11y[h] = state
	}
	return state
}

func defaultMockTabStop(widgetType string) bool {
	switch widgetType {
	case "Button", "Input", "Memo", "CheckBox", "RadioButton", "ComboBox",
		"Slider", "StringGrid", "PageControl", "ListView":
		return true
	default:
		return false
	}
}

func (m *Mock) SetAccessibleName(h Handle, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureA11y(h).name = name
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "AccessibleName", Value: name})
}

func (m *Mock) SetAccessibleDescription(h Handle, description string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureA11y(h).description = description
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "AccessibleDescription", Value: description})
}

func (m *Mock) SetAccessibleValue(h Handle, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureA11y(h).value = value
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "AccessibleValue", Value: value})
}

func (m *Mock) SetTabOrder(h Handle, order int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureA11y(h).tabOrder = order
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "TabOrder", Value: order})
}

func (m *Mock) SetTabStop(h Handle, enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureA11y(h).tabStop = enabled
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "TabStop", Value: enabled})
}

func (m *Mock) ResetTabStop(h Handle) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureA11y(h).tabStop = defaultMockTabStop(m.widgetType[h])
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "TabStop", Value: "default"})
}

func (m *Mock) SetDefaultButton(h Handle, enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureA11y(h).defaultBtn = enabled
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "DefaultButton", Value: enabled})
}

func (m *Mock) SetCancelButton(h Handle, enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ensureA11y(h).cancelBtn = enabled
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "CancelButton", Value: enabled})
}

func (m *Mock) AccessibleSnapshot(h Handle) (name, description, value string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.ensureA11y(h)
	return state.name, state.description, state.value
}

func (m *Mock) FocusSnapshot(h Handle) (order int, tabStop, defaultButton, cancelButton bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := m.ensureA11y(h)
	return state.tabOrder, state.tabStop, state.defaultBtn, state.cancelBtn
}
