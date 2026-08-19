package render

// AccessibilityController 是 Renderer 的可选可访问性属性能力。基础 Renderer
// 接口保持稳定；不支持该能力的后端会安全退化，但仍可依赖自身原生控件语义。
type AccessibilityController interface {
	SetAccessibleName(h Handle, name string)
	SetAccessibleDescription(h Handle, description string)
	SetAccessibleValue(h Handle, value string)
	SetTabStop(h Handle, enabled bool)
	ResetTabStop(h Handle)
	SetDefaultButton(h Handle, enabled bool)
	SetCancelButton(h Handle, enabled bool)
}

// TabOrderController 是 Renderer 的可选焦点顺序能力。order 由声明树中同一原生
// 父级下的控件顺序产生；后端应忽略没有键盘焦点的控件。
type TabOrderController interface {
	SetTabOrder(h Handle, order int)
}
