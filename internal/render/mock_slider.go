package render

func (m *Mock) ensureSlider(h Handle) *mockSlider {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.sliders == nil {
		m.sliders = make(map[Handle]*mockSlider)
	}
	state := m.sliders[h]
	if state == nil {
		state = &mockSlider{step: 1}
		m.sliders[h] = state
	}
	return state
}

// SetSliderStep 实现 SliderController，并记录可断言的属性 mutation。
func (m *Mock) SetSliderStep(h Handle, step int) {
	m.ensureSlider(h)
	m.mu.Lock()
	m.sliders[h].step = step
	m.ops = append(m.ops, Op{Type: OpSetProperty, Handle: h, Key: "Step", Value: step})
	m.mu.Unlock()
}

// OnSliderValueChange 保存 Slider 用户值变化回调；nil 表示解除绑定。
func (m *Mock) OnSliderValueChange(h Handle, fn func(int)) {
	m.ensureSlider(h)
	m.mu.Lock()
	m.sliders[h].onChange = fn
	m.ops = append(m.ops, Op{Type: OpSetEvent, Handle: h, Key: "OnValueChange", Value: fn})
	m.mu.Unlock()
}

// SliderStep 返回当前模拟键盘步长。
func (m *Mock) SliderStep(h Handle) int {
	m.ensureSlider(h)
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.sliders[h].step
}

// FireValueChange 模拟用户拖动或键盘步进。值按 Progressable 范围钳制，
// 但不按 Step 吸附；程序化 SetValue 不会调用本回调。
func (m *Mock) FireValueChange(h Handle, value int) {
	m.ensureSlider(h)
	m.mu.Lock()
	p := m.ensureProgress(h)
	if value < p.minimum {
		value = p.minimum
	}
	if value > p.maximum {
		value = p.maximum
	}
	p.value = value
	fn := m.sliders[h].onChange
	m.mu.Unlock()
	if fn != nil {
		fn(value)
	}
}
