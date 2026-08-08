// FluxVCL Phase 5 高级特性演示应用：examples/phase5
//
// 四大特性同屏展示（design.md §4.1/§13/§14 / development-plan §5）：
//  1. 5.1 动画：点击按钮触发"方块滑动"—— App.Animate 主线程 16ms pump 驱动
//     AnimationController（ElasticOut 回弹曲线），每帧用 App.SetBounds 直接
//     应用几何（D2 逃逸口），不触发整树 re-diff —— 高频属性绕开 diff 落地。
//  2. 5.2 Theme：右上角"主题"文字可点击，Light/Dark 切换 = State 变 → 全量
//     re-diff → diff 引擎只 patch 变化的颜色属性（未变子树零 mutation）。
//  3. 5.3 Async：点击按钮后台 goroutine 模拟 500ms 加载，完成后经
//     renderer.RunOnUI marshal 回 UI 线程更新状态文字（D4 marshalling）。
//  4. 5.4 Component：状态卡 = Component(build, Key("card")) 透明分组，身份靠
//     外部 Key 稳定（D3）；含 async 状态行与动画方块，子树每次 render 原地复用。
//
// 工程约束（Phase 0 冒烟结论）：smoke 按 class=Button 枚举子控件取按钮文本作
// "点击生效"信号 —— 本 demo 全窗口只有一个按钮（counter），枚举唯一。
// 主题切换用可点击的 Text（TLabel 无 HWND，非 Button 类，不干扰冒烟）。
//
// 构建：scripts/build.ps1 -Target phase5；冒烟：scripts/smoke.ps1 -Target phase5。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/energye/lcl/lcl"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/diff"
	"github.com/xiaowumin-mark/flux-vcl/internal/native"
	"github.com/xiaowumin-mark/flux-vcl/internal/render"
)

func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法获取 exe 路径:", err)
		os.Exit(2)
	}
	dllPath := filepath.Join(filepath.Dir(exe), "libenergy-amd64.dll")
	if _, err := os.Stat(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll。请用 scripts/build.ps1 -Target phase5 构建。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}

	r := native.NewRenderer()
	app := flux.NewApp(r)

	count := flux.NewState(0)          // counter（smoke 断言按钮文本变数字）
	themeName := flux.NewState("light") // 主题名 light/dark（5.2）；Bind 到 chip 文字以订阅 re-render
	load := flux.NewState("idle")      // Async 加载状态（5.3）

	// 当前动画的停止函数：连续点击时先停上一段（每帧 SetBounds 是幂等的覆盖，
	// 但停止可省 60fps pump 的 CPU）。
	var stopAnim func()

	// findBounds 按稳定 key 从 Element 树读取该控件最近一次布局槽位（DIP）。
	// 动画以槽位为起点，每帧 SetBounds 覆盖原生几何；下次 render 时布局不变则
	// diff 不发 SetBounds（D2 逃逸口 + 布局零 mutation 协同），动画位置保持。
	findBounds := func(key string) (render.Rect, bool) {
		var walk func(e *diff.Element) *diff.Element
		walk = func(e *diff.Element) *diff.Element {
			if e.Key == key {
				return e
			}
			for _, c := range e.Children {
				if f := walk(c); f != nil {
					return f
				}
			}
			return nil
		}
		e := walk(app.Root())
		if e == nil {
			return render.Rect{}, false
		}
		if v, ok := e.Props.Get("Bounds"); ok {
			if b, ok := v.(render.Rect); ok {
				return b, true
			}
		}
		return render.Rect{}, false
	}

	// slide 启动方块滑动动画（5.1）：ElasticOut 回弹，从布局槽位滑到窗体右缘。
	// onStep 里直接 App.SetBounds —— 无 State.Set、无 re-render（D2 逃逸口）。
	slide := func() {
		if stopAnim != nil {
			stopAnim()
		}
		base, ok := findBounds("box")
		if !ok {
			return
		}
		cw, _ := r.ClientSize()
		endX := cw - 8 - base.W
		if endX <= base.X {
			return
		}
		stopAnim = app.Animate(700*time.Millisecond, flux.ElasticOut, func(v float64) {
			x := base.X + int(float64(endX-base.X)*v)
			app.SetBounds("box", render.Rect{X: x, Y: base.Y, W: base.W, H: base.H})
		})
	}

	// kickAsync 后台加载（5.3）：goroutine 模拟耗时 IO，完成后 marshal 回 UI 线程。
	kickAsync := func() {
		load.Set("loading…")
		flux.Async(app, func() (string, error) {
			time.Sleep(500 * time.Millisecond) // 模拟网络/磁盘 IO（非 UI 线程）
			return "加载完成（模拟 500ms）", nil
		}, func(s string) {
			load.Set(s)
		}, func(err error) {
			load.Set("加载失败: " + err.Error())
		})
	}

	app.Mount(func() flux.Widget {
		th := flux.LightTheme
		if themeName.Get() == "dark" {
			th = flux.DarkTheme
		}

		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - phase5 (高级特性)"),
			flux.Color(th.Background),
			flux.Column(
				flux.Text("动画 / 主题 / Async / 组件 —— 点击下方按钮", flux.FontColor(th.Text)),

				// 唯一按钮（smoke 约束）：点击 = 计数（0→1）+ 滑动动画 + 异步加载
				flux.Button(flux.Bind(count), flux.Key("btn"),
					flux.Color(th.Primary), flux.FontColor(flux.RGB(0xFF, 0xFF, 0xFF)),
					flux.OnClick(func(e flux.Event) {
						count.Set(count.Get() + 1)
						slide()     // 5.1 动画（D2 逃逸口，不经 re-diff）
						kickAsync() // 5.3 Async（后台 goroutine + RunOnUI）
					}),
				),

				// 主题切换（5.2）：可点击 Text chip（TLabel 无 HWND，非 Button，不扰冒烟）。
				// Bind(themeName) 把 State 订阅到 App —— Set 才触发全量 re-diff（同 count/load 路径）。
				flux.Row(
					flux.Text(flux.Bind(themeName), flux.Key("theme-chip"),
						flux.FontColor(th.Accent),
						flux.OnClick(func(e flux.Event) {
							if themeName.Get() == "light" {
								themeName.Set("dark") // State 变 → 全量 re-diff（仅 patch 颜色属性）
							} else {
								themeName.Set("light")
							}
						}),
					),
					flux.Text(" ▸ 点击切换主题", flux.FontColor(th.Text)),
					flux.Expanded(flux.Text("", flux.Width(1))),
				),

				// 组件状态卡（5.4）：透明分组，身份靠外部 Key("card") 稳定
				flux.Component(func() flux.Widget {
					return flux.Column(
						flux.Row(
							flux.Text("Async: ", flux.FontColor(th.Text)),
							flux.Text(flux.Bind(load), flux.Key("async-status"), flux.FontColor(th.Text)),
						),
						// 动画方块（滑动目标；5.1）
						flux.Text("●", flux.Key("box"),
							flux.Width(48), flux.Height(48),
							flux.Color(th.Surface), flux.FontColor(th.Accent)),
					)
				}, flux.Key("card")),
			),
		)
	})

	lcl.Application.Run()
}
