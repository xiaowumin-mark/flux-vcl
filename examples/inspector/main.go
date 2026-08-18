// FluxVCL P7.1 Inspector 演示应用：examples/inspector
//
// 展示 design.md §18 / development-plan §7.1 的只读工具链：Widget/Element/native
// 三层树、Props、布局、实际事件、mutation 提交统计与 canUpdate 重建风险。
// 点击唯一数字 Button 会依次执行纯文本 patch、key mismatch 重建、type mismatch
// 重建；Inspector 独立窗口的刷新/关闭不会触发被检查 App render。
//
// 工程约束：主窗体只有一个 Button，文本是数字计数，供通用 smoke 枚举并验证
// 点击生效；Inspector 工具窗只含只读 Memo，不增加 Button HWND。
//
// 构建：scripts/build.ps1 -Target inspector；冒烟：scripts/smoke.ps1 -Target inspector。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/inspector"
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
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll。请用 scripts/build.ps1 -Target inspector 构建。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}

	r := native.NewRenderer()
	app := flux.NewApp(r)
	count := flux.NewState(0)

	app.Mount(func() flux.Widget {
		value := count.Get()
		key := "stable-status"
		if value >= 2 {
			key = "changed-status" // canUpdate key mismatch
		}
		var status flux.Widget = flux.Text(fmt.Sprintf("纯属性 patch：计数 %d", value), flux.Key(key))
		if value >= 3 {
			status = flux.Input(flux.Key(key)) // canUpdate type mismatch
		}
		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - Inspector target"),
			flux.Column(
				flux.Text("P7.1 Inspector：观察右侧独立工具窗"),
				flux.Button(flux.Bind(count), flux.Key("counter"), flux.OnClick(func(flux.Event) {
					count.Set(count.Get() + 1)
				})),
				status,
			),
		)
	})

	inspector.Open(app)
	native.Run()
}
