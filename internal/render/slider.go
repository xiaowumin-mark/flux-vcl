package render

// SliderController 是水平整数 Slider 的窄能力接口（D6）。范围和值复用
// Progressable；本接口只承载 Slider 专属步长和用户值变化事件。未实现该能力的
// Renderer 会由 diff 安全跳过，不扩张基础 Renderer。
type SliderController interface {
	SetSliderStep(h Handle, step int)
	OnSliderValueChange(h Handle, fn func(value int))
}
