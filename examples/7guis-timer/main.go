// FluxVCL 7GUIs Timer：Slider 控制时长，ProgressBar 展示 App.Animate 主线程进度。
//
// 设计对应 docs/design.md §21.4，Timer 的主线程 pump 边界见 docs/7guis.md。
// 构建：scripts/build.ps1 -Target 7guis-timer；
// 冒烟：scripts/smoke.ps1 -Target 7guis-timer。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/native"
)

func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法获取 exe 路径:", err)
		os.Exit(2)
	}
	dllPath := filepath.Join(filepath.Dir(exe), "libenergy-amd64.dll")
	if _, err := os.Stat(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll。请用 scripts/build.ps1 -Target 7guis-timer 构建。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}

	r := native.NewRenderer()
	app := flux.NewApp(r)
	durationTenths := flux.NewState(50)
	elapsedMillis := flux.NewState(0)

	var stopTimer func()
	var startTimer func()
	startTimer = func() {
		if stopTimer != nil {
			stopTimer()
			stopTimer = nil
		}

		limit := durationTenths.Get() * 100
		start := elapsedMillis.Get()
		if start > limit {
			start = limit
			elapsedMillis.Set(limit)
		}
		if start >= limit {
			return
		}

		remaining := limit - start
		// App.Animate 由 native TTimer 在 UI 线程泵送；回调只写 State，不启动 goroutine。
		stopTimer = app.Animate(time.Duration(remaining)*time.Millisecond, flux.LinearCurve, func(v float64) {
			next := start + int(float64(remaining)*v)
			if next > limit {
				next = limit
			}
			elapsedMillis.Set(next)
		})
	}

	if err := app.Mount(func() flux.Widget {
		limit := durationTenths.Get() * 100
		elapsed := elapsedMillis.Get()
		if elapsed > limit {
			elapsed = limit
		}
		progress := 0
		if limit > 0 {
			progress = elapsed * 100 / limit
		}

		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - 7GUIs Timer"),
			flux.Width(520),
			flux.Height(280),
			flux.Column(
				flux.CrossAxis(flux.CrossAxisStretch),
				flux.Text("Timer"),
				flux.Row(
					flux.Text("Elapsed: "),
					// Bind 建立动画 State -> render 的订阅，ProgressBar 随同一次 diff 更新。
					flux.Text(flux.Bind(elapsedMillis)),
					flux.Text(" ms"),
				),
				flux.ProgressBar(flux.Minimum(0), flux.Maximum(100), flux.Value(progress)),
				flux.Row(
					flux.Text("Duration: "),
					flux.Text(flux.Bind(durationTenths)),
					flux.Text(" x 0.1 s"),
				),
				flux.Slider(
					flux.Minimum(1),
					flux.Maximum(100),
					flux.Value(durationTenths.Get()),
					flux.Step(1),
					flux.OnValueChange(func(value int) {
						durationTenths.Set(value)
						newLimit := value * 100
						if elapsedMillis.Get() > newLimit {
							elapsedMillis.Set(newLimit)
						}
						startTimer()
					}),
				),
				flux.Button("Reset", flux.OnClick(func(_ flux.Event) {
					elapsedMillis.Set(0)
					startTimer()
				})),
			),
		)
	}); err != nil {
		fmt.Fprintln(os.Stderr, "挂载失败:", err)
		os.Exit(2)
	}

	startTimer()
	native.Run()
}
