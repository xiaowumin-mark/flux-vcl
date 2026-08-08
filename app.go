package flux

import (
	"github.com/xiaowumin-mark/flux-vcl/internal/diff"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

// App 管理一棵声明式 UI 树的 reconciliation 生命周期。
//
// 用法：NewApp(绑定层 renderer) → Render(Widget)。每次状态变化重建 Widget 树
// 并再次 Render：diff 引擎只 patch 变化的属性（D2），不重建未变化控件（D1/D7）。
// 事件回调（OnClick 等）里调用 Render 即可触发更新（State 系统属 Phase 2）。
//
//	app := flux.NewApp(nativeAdapter)          // 绑定层 renderer
//	app.Render(flux.Window(flux.Button("OK")))
//
// 绑定层 renderer 必须由 internal/native 的适配器创建（D6 隔离）。
type App struct {
	r  render.Renderer
	rc *diff.Reconciler
}

// NewApp 创建 App。r 为绑定层 renderer（默认 LCL 适配见 internal/native）。
func NewApp(r render.Renderer) *App {
	return &App{r: r, rc: diff.New(r)}
}

// Render 对整棵树做一次 diff：先占位布局（写 Bounds），再 reconcile。
func (a *App) Render(w Widget) {
	root := w.Create()
	layoutTree(root, a.r)
	a.rc.Render(root)
}

// Root 返回当前 Element 树根（Inspector / 测试用）。
func (a *App) Root() *diff.Element { return a.rc.Root() }
