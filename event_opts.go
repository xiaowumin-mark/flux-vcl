package flux

// 事件与生命周期 Opt（design.md §10/§12，Phase 4）。
//
// 统一事件回调签名为 func(Event)：native 映射层组装坐标/按键/修饰键后经
// diff 引擎注入 Source（Type#Key）转发到用户回调。每次 render 重新绑定
// （函数值无法比较相等性，D2 逃逸口行为）。
//
// OnChange 保持 func(string)（文本专用，Input 双向绑定路径，见 state.go）。

// OnClick 绑定点击事件（统一事件）。func() 旧签名已迁移为 func(Event)：
// 事件参数携带 Type（EventClick）与 Source，坐标/按键为空。
func OnClick(fn func(Event)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnClick", fn) })
}

// OnMouseDown 绑定鼠标按下事件。Event 携带 DIP 坐标（相对控件客户区）、
// 按键（Button）与修饰键（Mods）。
func OnMouseDown(fn func(Event)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnMouseDown", fn) })
}

// OnMouseUp 绑定鼠标释放事件。字段同 OnMouseDown。
func OnMouseUp(fn func(Event)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnMouseUp", fn) })
}

// OnMouseMove 绑定鼠标移动事件。Event 携带 DIP 坐标与修饰键。
func OnMouseMove(fn func(Event)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnMouseMove", fn) })
}

// OnMouseEnter 绑定鼠标进入事件。Event 只有 Type 与 Source。
func OnMouseEnter(fn func(Event)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnMouseEnter", fn) })
}

// OnMouseLeave 绑定鼠标离开事件。Event 只有 Type 与 Source。
func OnMouseLeave(fn func(Event)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnMouseLeave", fn) })
}

// OnKeyDown 绑定键盘按下事件（控件有焦点时）。Event 携带虚拟键码 Key
// 与修饰键 Mods。
func OnKeyDown(fn func(Event)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnKeyDown", fn) })
}

// OnKeyUp 绑定键盘释放事件。字段同 OnKeyDown。
func OnKeyUp(fn func(Event)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnKeyUp", fn) })
}

// OnKeyPress 绑定字符输入事件（含 IME 组合结果）。Event.Text 携带 UTF-8
// 字符（中文输入经 form/控件级 OnUTF8KeyPress 路由，Phase 4.4）。
// 常规输入框（Input）的中文由原生 TEdit IME 处理，OnChange 已可回写；
// OnKeyPress 用于需要逐字符观察/拦截的场景。
func OnKeyPress(fn func(Event)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnKeyPress", fn) })
}

// OnMount 绑定挂载生命周期钩子（design.md §12，Phase 4.3）。
// 节点（含子树）首次创建并挂载后触发一次；canUpdate 不匹配而重建时重新触发。
func OnMount(fn func()) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnMount", fn) })
}

// OnUpdate 绑定更新生命周期钩子。每次 diff 成功复用该节点（canUpdate 匹配，
// React componentDidUpdate 语义）后触发。
func OnUpdate(fn func()) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnUpdate", fn) })
}

// OnUnmount 绑定卸载生命周期钩子。节点被销毁前触发（先于原生控件释放，
// D4 卸载路径），可在此清理订阅/资源。
func OnUnmount(fn func()) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnUnmount", fn) })
}

// OnChange 绑定文本变化事件（Input 等可编辑控件）。fn 接收新文本。
func OnChange(fn func(text string)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnChange", fn) })
}

// OnCheckedChange 绑定选中状态变化事件（CheckBox 等可选控件）。fn 接收变化后的状态。
func OnCheckedChange(fn func(checked bool)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnCheckedChange", fn) })
}

// OnSelectionChange 绑定 ComboBox 选择变化事件。fn 接收用户选择后的索引；-1 表示
// 当前未选择。该事件不复用文本控件的 OnChange。
func OnSelectionChange(fn func(index int)) Opt {
	return optFn(func(n *Node) { n.Props.Set("OnSelectionChange", fn) })
}
