package flux

import (
	"sync"

	"github.com/xiaowumin-mark/flux-vcl/internal/diff"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// App 管理一棵声明式 UI 树的 reconciliation 生命周期。
//
// 用法：NewApp(绑定层 renderer) → Mount(build)。build 是每次 render 调用的
// 根构建函数；任一订阅的 State 变化（State.Set）自动触发"重新调用 build →
// 布局 → diff"（经 RunOnUI marshal，合并更新）。
//
//	app := flux.NewApp(nativeAdapter)          // 绑定层 renderer
//	app.Mount(func() flux.Widget { return flux.Window(flux.Text(flux.Bind(count))) })
//
// 绑定层 renderer 必须由 internal/native 的适配器创建（D6 隔离）。
type App struct {
	r          render.Renderer
	rc         *diff.Reconciler
	build      func() Widget
	mu         sync.Mutex
	renderMu   sync.Mutex // 串行化 reconcile：即使并发 Set 也只允许一个 render 进行
	pending    bool       // 脏标志：有待处理的失效（D4 合并）
	inRender   bool       // 重入防护：当前已有 renderWidget 在栈上（生命周期钩子等 render 中触发 State.Set）
	lastDiags  []LayoutDiag // 最近一次 render 的布局溢出诊断（Phase 3.7 inspector）
	lastInspect []NodeDiag // 最近一次 render 的全节点布局诊断（Phase 3.7 inspector）
}

// NewApp 创建 App。r 为绑定层 renderer（默认 LCL 适配见 internal/native）。
// 注册窗体 resize 回调 → invalidate（pending 合并 + renderMu 串行化，
// resize 风暴安全）→ Window 布局用最新客户区尺寸。
func NewApp(r render.Renderer) *App {
	a := &App{r: r, rc: diff.New(r)}
	r.OnResize(func(w, h int) { a.invalidate() })
	return a
}

// Mount 注册根构建函数并首次渲染。之后 State.Set 自动触发 re-render。
func (a *App) Mount(build func() Widget) {
	a.mu.Lock()
	a.build = build
	a.mu.Unlock()
	a.render()
}

// Render 手动渲染一棵具体树（Phase 1 兼容路径；不触发 State 自动更新，请用 Mount）。
func (a *App) Render(w Widget) { a.renderWidget(w) }

// Root 返回当前 Element 树根（Inspector / 测试用）。
func (a *App) Root() *diff.Element { return a.rc.Root() }

// render 取当前 build 并渲染（build 为空则跳过，未 Mount）。renderMu 与重入
// 防护在 renderWidget 内统一处理（见下）。
func (a *App) render() {
	a.mu.Lock()
	b := a.build
	a.mu.Unlock()
	if b == nil {
		return
	}
	a.renderWidget(b())
}

// renderWidget 对一棵具体 Widget 树做 diff：布局（constraints 下传，写 Bounds）→
// 收集绑定依赖（订阅 State）→ reconcile。collectBindings 在 diff 前执行，保证
// State.Set 在 render 后立即能看到订阅。
//
// 重入防护（Phase 4.3 工程发现）：生命周期钩子（OnMount/OnUpdate/OnUnmount）
// 在 reconcile 内触发，若钩子回调里 Set State → invalidate → RunOnUI（主线程
// 内联）→ renderWidget，会重入当前栈：非重入 renderMu 自锁 + 无限递归。
// 因此 renderWidget 持有 inRender 守卫：重入调用只置 pending 并返回，由当前
// render 结束后 finishRender 统一 flush（D4 合并更新，递归变一次尾调）。
func (a *App) renderWidget(w Widget) {
	a.mu.Lock()
	if a.inRender {
		a.pending = true // 已有 render 在栈上：排队，由当前 render 结束时 flush
		a.mu.Unlock()
		return
	}
	a.inRender = true
	a.mu.Unlock()
	defer a.finishRender()

	a.renderMu.Lock() // 串行化 reconcile：并发 Set 也只允许一个 render 进行
	defer a.renderMu.Unlock()

	root := w.Create()
	cw, ch := a.r.ClientSize()
	d := &layoutDiags{}
	layoutTree(root, a.r, Tight(cw, ch), Point{}, d)
	d.finalize(root) // 布局完成后后序回填 Frame（record 时点早于父 setPos 平移）
	a.mu.Lock()
	a.lastDiags = d.list
	a.lastInspect = d.nodes
	a.mu.Unlock()
	a.collectBindings(root)
	a.rc.Render(root)
	// D4 延后销毁落地点：reconcile 移除的控件在此统一物理释放（在 UI 线程、
	// 事件回调触发 render 时也晚于 reconcile 完成）。
	if d, ok := a.r.(drainer); ok {
		d.DrainDestroy()
	}
}

// finishRender 结束一次 renderWidget：清 inRender，若 render 期间有重入排队
// 的 State.Set（pending），flush 一次（递归走 render() → 最新 build）。
func (a *App) finishRender() {
	a.mu.Lock()
	a.inRender = false
	again := a.pending
	a.pending = false
	a.mu.Unlock()
	if again {
		a.render()
	}
}

// LastLayoutDiags 返回最近一次 render 的布局溢出诊断（无则空切片）。
// Phase 3.7 inspector 将在此之上做溢出提示 UI。
func (a *App) LastLayoutDiags() []LayoutDiag {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]LayoutDiag(nil), a.lastDiags...)
}

// Inspect 返回最近一次 render 的全节点布局诊断（constraints/size/frame/flex），
// 与 LastLayoutDiags（溢出）互补 —— Phase 3.7 inspector 数据源。无则空切片。
func (a *App) Inspect() []NodeDiag {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]NodeDiag(nil), a.lastInspect...)
}

// invalidate 请求一次 re-render（State.Set 调用）。
//
// 合并（D4）：pending 标志保证同一周期内多次 Set 只触发一次 render。
// State.Set 在调用 invalidate 前已提交值，故被吞并的 Set 其新值仍会被
// 本次 render 读到（render 时 Get 当前值）——不丢最后一次写入。
// renderMu 串行化 reconcile：并发 Set 时即使两个 flush 都入队，也只有一个
// render 在进行。经 renderer.RunOnUI marshal 到 UI 线程，任意 goroutine 安全。
func (a *App) invalidate() {
	a.mu.Lock()
	if a.pending {
		a.mu.Unlock()
		return
	}
	a.pending = true
	a.mu.Unlock()

	a.r.RunOnUI(func() {
		a.mu.Lock()
		a.pending = false
		a.mu.Unlock()
		a.render()
	})
}

// drainer 是可选接口：实现方在每次 render 完成后统一物理释放延后销毁的
// 控件（D4 销毁入队延后，见 internal/native.Renderer.DrainDestroy）。
// Mock 不实现（同步销毁，op 记录即语义），仅真实绑定层延后物理 Free。
type drainer interface {
	DrainDestroy()
}

// collectBindings 遍历节点树，把登记了 bindKey 的绑定订阅到 App（幂等）。
func (a *App) collectBindings(n *Node) {
	if v, ok := n.Props.Get(bindKey); ok {
		if b, ok := v.(bindable); ok {
			b.bindTo(a)
		}
	}
	for _, c := range n.Children {
		a.collectBindings(c)
	}
}
