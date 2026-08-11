package render

// Checkable 是可选控件的窄接口（D6）：diff 层通过它设置选中状态并绑定状态变化。
// 实现方：internal/native（TCheckBox/TRadioButton）与 Mock。未实现时控件
// 属性和事件安全退化，不扩张主 Renderer 接口。
type Checkable interface {
	SetChecked(h Handle, checked bool)
	OnCheckedChange(h Handle, fn func(checked bool))
}
