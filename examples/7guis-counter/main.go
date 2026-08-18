// FluxVCL 7GUIs Counter：用声明式 State、Text 和 Button 实现可回写计数器。
//
// 设计对应 docs/design.md §21.4，任务边界见 docs/7guis.md。
// 构建：scripts/build.ps1 -Target 7guis-counter；
// 冒烟：scripts/smoke.ps1 -Target 7guis-counter。
package main

import (
	"fmt"
	"os"
	"path/filepath"

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
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll。请用 scripts/build.ps1 -Target 7guis-counter 构建。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}

	r := native.NewRenderer()
	app := flux.NewApp(r)
	count := flux.NewState(0)

	if err := app.Mount(func() flux.Widget {
		return flux.Window(
			flux.Title(fmt.Sprintf("FluxVCL %s - 7GUIs Counter (%d)", flux.Version, count.Get())),
			flux.Width(360),
			flux.Height(180),
			flux.Column(
				flux.CrossAxis(flux.CrossAxisCenter),
				flux.Text("Counter"),
				flux.Row(
					flux.Text("Current value: "),
					flux.Text(flux.Bind(count)),
				),
				flux.Button("Count", flux.Width(120), flux.OnClick(func(_ flux.Event) {
					count.Set(count.Get() + 1)
				})),
			),
		)
	}); err != nil {
		fmt.Fprintln(os.Stderr, "挂载失败:", err)
		os.Exit(2)
	}

	native.Run()
}
