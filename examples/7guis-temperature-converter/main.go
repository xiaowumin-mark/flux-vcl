// FluxVCL 7GUIs Temperature Converter：两个受控 Input 保持摄氏/华氏双向换算。
//
// 设计对应 docs/design.md §21.4，非法输入的示例层边界见 docs/7guis.md。
// 构建：scripts/build.ps1 -Target 7guis-temperature-converter；
// 冒烟：scripts/smoke.ps1 -Target 7guis-temperature-converter。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/native"
)

func formatTemperature(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法获取 exe 路径:", err)
		os.Exit(2)
	}
	dllPath := filepath.Join(filepath.Dir(exe), "libenergy-amd64.dll")
	if _, err := os.Stat(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll。请用 scripts/build.ps1 -Target 7guis-temperature-converter 构建。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}

	r := native.NewRenderer()
	app := flux.NewApp(r)
	celsius := flux.NewState("0")
	fahrenheit := flux.NewState("32")
	status := flux.NewState("Enter a number in either field.")
	syncing := false

	if err := app.Mount(func() flux.Widget {
		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - 7GUIs Temperature Converter"),
			flux.Width(520),
			flux.Height(220),
			flux.Column(
				flux.CrossAxis(flux.CrossAxisStretch),
				flux.Text("Temperature Converter"),
				flux.Row(
					flux.Input(
						flux.Bind(celsius),
						flux.Width(180),
						flux.OnChange(func(text string) {
							if syncing {
								return
							}
							syncing = true
							defer func() { syncing = false }()
							celsius.Set(text)
							value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
							if err != nil {
								status.Set("Celsius is not a valid number; the input remains editable.")
								return
							}
							fahrenheit.Set(formatTemperature(value*9/5 + 32))
							status.Set("Converted from Celsius.")
						}),
					),
					flux.Text(" Celsius = "),
					flux.Input(
						flux.Bind(fahrenheit),
						flux.Width(180),
						flux.OnChange(func(text string) {
							if syncing {
								return
							}
							syncing = true
							defer func() { syncing = false }()
							fahrenheit.Set(text)
							value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
							if err != nil {
								status.Set("Fahrenheit is not a valid number; the input remains editable.")
								return
							}
							celsius.Set(formatTemperature((value - 32) * 5 / 9))
							status.Set("Converted from Fahrenheit.")
						}),
					),
					flux.Text(" Fahrenheit"),
				),
				flux.Text(flux.Bind(status)),
			),
		)
	}); err != nil {
		fmt.Fprintln(os.Stderr, "挂载失败:", err)
		os.Exit(2)
	}

	native.Run()
}
