package render

// Selectable 是下拉选择控件的窄接口（D6）：diff 层通过它设置选项、受控索引并绑定
// 选择变化。实现方：internal/native（TComboBox）与 Mock。未实现时属性和事件安全
// 退化，不扩张主 Renderer 接口。
type Selectable interface {
	SetItems(h Handle, items []string)
	SetSelectedIndex(h Handle, index int)
	OnSelectionChange(h Handle, fn func(index int))
}
