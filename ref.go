package flux

import "github.com/xiaowumin-mark/flux-vcl/internal/render"

// Ref 是控件引用（逃逸口，design.md §11.2）：经 BindRef 绑定后，可在事件回调
// 或外部代码中读取绑定层原生控件对象。
//
//	btnRef := &flux.Ref{}
//	flux.Button("OK", flux.BindRef(btnRef))
//
//	// 事件回调 / 外部代码：
//	btn := btnRef.Get().(*lcl.TButton) // 类型断言到具体后端
//	btn.SetEnabled(false)
//
// D6 隔离：flux 不知道具体后端类型，Get() 返回 any，由用户断言。
type Ref struct {
	current any
}

// SetNative 由绑定层在控件创建后调用，注入原生控件对象。
func (r *Ref) SetNative(obj any) { r.current = obj }

// Get 返回绑定层原生控件对象（未绑定时为 nil）。调用方断言到具体类型
// （默认 LCL 为 *lcl.TButton 等）。
func (r *Ref) Get() any { return r.current }

// BindRef 绑定控件引用（design.md §11.2）。控件创建后 binding 层注入原生对象。
func BindRef(r *Ref) Opt {
	return optFn(func(n *Node) { n.Props.Set("Ref", r) })
}

// Native 原生逃逸口（design.md §11.1）：控件创建后立即调用 fn，注入绑定层
// 原生控件对象。泛型 T 由用户指定为具体后端类型（默认 LCL：*lcl.TButton）。
//
//	flux.Button("OK", flux.Native(func(b *lcl.TButton) {
//	    b.SetColor(types.ClRed)
//	}))
//
// D6 隔离：泛型包装把用户闭包桥接为 func(any)，在绑定层断言回 T。
// 默认 LCL 后端会在回调返回后将 Align 恢复为 alNone；其他原生布局属性仍不属于
// 框架支持路径，不能用来接管声明式布局（D5）。
func Native[T any](fn func(c T)) Opt {
	return optFn(func(n *Node) {
		n.Props.Set("Native", func(obj any) { fn(obj.(T)) })
	})
}

// 确保 Ref 满足绑定层的引用接口（编译期断言）。
var _ render.Ref = (*Ref)(nil)
