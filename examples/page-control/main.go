// FluxVCL P7.2c 分页容器演示：examples/page-control。
//
// 展示 docs/design.md §20 与 development-plan §7.2c：PageControl/TabPage 的
// 受控 SelectedIndex、稳定页面 Key、页内独立 native parent、inactive 页面保活和
// keyed 页面重排。唯一数字 Button 同时切换受控索引与页面顺序，并作为 smoke 的可观测
// 信号；页内输入框可手工聚焦后切页，验证 caret/IME 所属页面不被重建。
//
// 工程约束：窗口内恰好一个 Button，按钮 Caption 由 State 驱动为数字；其他交互使用
// PageControl 原生页签。构建：scripts/build.ps1 -Target page-control；
// 冒烟：scripts/smoke.ps1 -Target page-control。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/energye/lcl/lcl"
	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/native"
)

func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法获取 exe 路径:", err)
		os.Exit(2)
	}
	dllPath := filepath.Join(filepath.Dir(exe), "libenergy-amd64.dll")
	if _, err := os.Stat(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll。请用 scripts/build.ps1 -Target page-control 构建。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}

	r := native.NewRenderer()
	app := flux.NewApp(r)
	selected := flux.NewState(0)
	reversed := flux.NewState(false)
	clicks := flux.NewState(0)
	status := flux.NewState("当前页：第一页")
	firstInput := flux.NewState("first-input")
	secondInput := flux.NewState("second-input")
	r.OnClose(func() {
		if err := app.Close(); err != nil {
			fmt.Fprintln(os.Stderr, "关闭失败:", err)
		}
	})

	build := func() flux.Widget {
		index := selected.Get()
		first := flux.TabPage("第一页", flux.Column(
			flux.Text("第一页内容（Key=first）"),
			flux.Input(flux.Bind(firstInput), flux.Key("first-input"), flux.Width(280)),
		), flux.Key("first"))
		second := flux.TabPage("第二页", flux.Column(
			flux.Text("第二页内容（Key=second）"),
			flux.Input(flux.Bind(secondInput), flux.Key("second-input"), flux.Width(280)),
		), flux.Key("second"))
		pageArgs := make([]any, 0, 6)
		if reversed.Get() {
			pageArgs = append(pageArgs, second, first)
		} else {
			pageArgs = append(pageArgs, first, second)
		}
		pageArgs = append(pageArgs,
			flux.SelectedIndex(index),
			flux.OnSelectionChange(func(value int) {
				selected.Set(value)
				clicks.Set(clicks.Get() + 1)
				status.Set("当前页：" + pageTitle(reversed.Get(), value))
			}),
			flux.Width(520), flux.Height(300),
		)
		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - PageControl (P7.2c)"),
			flux.Column(
				flux.Text("PageControl：切换页签不会卸载页面子树"),
				flux.Button(fmt.Sprint(clicks.Get()), flux.OnClick(func(flux.Event) {
					nextReversed := !reversed.Get()
					nextIndex := 0
					if nextReversed {
						nextIndex = 1
					}
					reversed.Set(nextReversed)
					selected.Set(nextIndex)
					clicks.Set(clicks.Get() + 1)
					status.Set("当前页：第一页（keyed 页序已切换）")
				})),
				flux.PageControl(pageArgs...),
				flux.Text(flux.Bind(status)),
			),
		)
	}
	if err := app.Mount(build); err != nil {
		fmt.Fprintln(os.Stderr, "挂载失败:", err)
		os.Exit(2)
	}
	lcl.Application.Run()
}

func pageTitle(reversed bool, index int) string {
	if index < 0 || index > 1 {
		return "未选择"
	}
	if reversed {
		if index == 0 {
			return "第二页"
		}
		return "第一页"
	}
	if index == 0 {
		return "第一页"
	}
	return "第二页"
}
