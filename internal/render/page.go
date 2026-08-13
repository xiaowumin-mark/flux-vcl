package render

// PageController 是分页容器的窄能力接口（D6）。diff 层用它同步页面顺序、
// 受控选中索引与用户选择回调；实现方为 internal/native 和 Mock。未实现该能力的
// Renderer 会安全退化，不需要扩张基础 Renderer 接口。
type PageController interface {
	SyncPages(parent Handle, pages []Handle)
	SetPageSelectedIndex(parent Handle, index int)
	OnPageSelectionChange(parent Handle, fn func(index int))
}
