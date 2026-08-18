// FluxVCL P7.2 插件系统演示应用：examples/plugin-badge
//
// 展示 design.md §19 / development-plan §7.2：第三方 Badge 仅依赖公开插件 SDK，
// builder 组合 Row/Text，布局回调增加 DIP padding，完全不修改 native Create switch。
// 点击唯一数字 Button 后 State 驱动 Badge 属性更新；同 Type+Key 只 patch 内建 Text。
//
// 工程约束：主窗体只有一个 Button，按钮文本从 0 变 1，供通用 smoke 枚举断言；
// Badge 本身只组合 Text，不额外创建 Button HWND。窗体关闭时 App.Close 逆序关闭插件。
//
// 构建：scripts/build.ps1 -Target plugin-badge；
// 冒烟：scripts/smoke.ps1 -Target plugin-badge。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/examples/plugin-badge/badge"
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
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll。请用 scripts/build.ps1 -Target plugin-badge 构建。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}
	if err := badge.Register(); err != nil {
		fmt.Fprintln(os.Stderr, "Badge 插件注册失败:", err)
		os.Exit(2)
	}

	r := native.NewRenderer()
	app := flux.NewApp(r)
	count := flux.NewState(0)
	r.OnClose(func() {
		if err := app.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "插件关闭失败:", err)
		}
	})

	if err := app.Mount(func() flux.Widget {
		value := count.Get()
		tone := "info"
		if value > 0 {
			tone = "success"
		}
		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - Plugin Badge (P7.2)"),
			flux.Column(
				flux.Text("第三方 Badge：公开 SDK + 内建 Widget builder"),
				badge.Widget(fmt.Sprintf("插件属性已更新 %d 次", value), tone, flux.Key("status-badge")),
				flux.Button(flux.Bind(count), flux.OnClick(func(flux.Event) {
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
