// FluxVCL Phase 1 冒烟应用：examples/basic
//
// 声明式 UI 最小用例（design.md §4）：Window(Column(Text, Button, Input))。
// 演示 diff/reconciliation 引擎（Phase 1.4）：
//  1. 点击按钮 → 重建 Widget 树 → App.Render → 只 patch 变化的属性，零控件重建。
//  2. 输入框 OnChange → 把输入同步到 Text（单向事件流，State 属 Phase 2）。
//
// 工程约束（Phase 0 冒烟结论）：LCL 的 TLabel 无独立 HWND，Win32 冒烟无法读
// label 文本；因此点击时同步更新按钮文本作为"点击生效"的可观测信号。
//
// 初始化序列（E2 结论）：Init → NewRenderer(NewForms) → 声明式 Render → Run。
// Go 包版本必须与 libenergy DLL 严格一致：lcl v1.0.3 ↔ libenergy-amd64.dll。
//
// 构建：scripts/build.ps1（生成 winres 资源 → windowsgui exe → 复制 DLL）。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/native"
	"github.com/energye/lcl/lcl"
)

func main() {
	// DLL 由构建脚本复制到 exe 旁；显式指定绝对路径（E2 验证项）
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法获取 exe 路径:", err)
		os.Exit(2)
	}
	dllPath := filepath.Join(filepath.Dir(exe), "libenergy-amd64.dll")
	if _, err := os.Stat(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll（应位于 exe 旁）。请用 scripts/build.ps1 构建，它会复制 DLL。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}

	r := native.NewRenderer()
	app := flux.NewApp(r)

	var count int
	var label = "Hello, FluxVCL! 点下面按钮试试"
	btnText := "Click me"
	var build func() flux.Widget // 闭包自引用需先声明再赋值
	build = func() flux.Widget {
		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - basic"),
			flux.Column(
				flux.Text(label, flux.Key("label")),
				flux.Button(btnText, flux.Key("btn"), flux.OnClick(func() {
					count++
					label = fmt.Sprintf("已点击 %d 次", count)
					btnText = fmt.Sprintf("Clicked %d", count) // 可观测信号（TLabel 无 HWND）
					app.Render(build())
				})),
				flux.Input(flux.Key("input"), flux.OnChange(func(v string) {
					label = "输入: " + v
					app.Render(build())
				})),
			),
		)
	}
	app.Render(build())

	lcl.Application.Run()
}
