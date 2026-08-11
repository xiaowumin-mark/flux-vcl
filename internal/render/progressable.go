package render

// Progressable 是进度控件的窄接口（D6）：diff 层通过它按范围优先、值在后的
// 固定顺序写入进度状态。实现方：internal/native（TProgressBar）与 Mock；未实现时
// 属性安全退化，不扩张主 Renderer 接口。
type Progressable interface {
	SetMinimum(h Handle, minimum int)
	SetMaximum(h Handle, maximum int)
	SetValue(h Handle, value int)
}

// RadioGroupable 是单选控件的窄分组接口。GroupIndex 表达 Flux 逻辑组编号；支持
// 该能力的 Renderer 负责在同一 resolved native parent 下保证同组互斥、异组独立。
// diff 不直接遍历或修改兄弟节点。
type RadioGroupable interface {
	SetGroupIndex(h Handle, groupIndex int)
}
