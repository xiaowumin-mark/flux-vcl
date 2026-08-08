// FluxVCL Phase 2 冒烟应用：examples/basic
//
// State 系统与数据绑定最小用例（design.md §8-9）：
//  1. counter：Button(Bind(count)) + OnClick 修改 State → 自动 re-render，
//     按钮文本随 State 刷新（零控件重建，diff 只 patch 文本）。
//  2. two-way：Input(Bind(name)) ↔ Text(Bind(name)) —— 输入经 OnChange 回写
//     State，State 变化又驱动 Text 回显（单向渲染流 + 双向绑定）。
//
// 工程约束（Phase 0 冒烟结论）：LCL 的 TLabel 无独立 HWND，Win32 冒烟无法读
// label 文本；因此 counter 的"点击生效"信号直接落在按钮文本（TButton 有 HWND）。
//
// 初始化序列（E2 结论）：Init → NewRenderer(NewForms) → Mount(build) → Run。
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

	// State 原语：Set 可跨 goroutine，re-render 自动 marshal 到 UI 线程。
	count := flux.NewState(0)       // counter 计数（单向绑定 → Button 文本）
	name := flux.NewState("FluxVCL") // two-way 绑定目标（Input ↔ Text 回显）

	app.Mount(func() flux.Widget {
		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - basic (Phase 2)"),
			flux.Column(
				flux.Text("Hello, FluxVCL! State 驱动的最小用例", flux.Key("label")),

				flux.Text("1) counter：点按钮 +1，文本由 State 驱动刷新", flux.Key("c-hint")),
				flux.Button(flux.Bind(count), flux.Key("btn"), flux.OnClick(func(_ flux.Event) {
					count.Set(count.Get() + 1) // 外部修改 State → 自动 re-render
				})),

				flux.Text("2) two-way：输入框 ↔ State ↔ 文本回显", flux.Key("t-hint")),
				flux.Input(flux.Bind(name), flux.Key("input")),
				flux.Text(flux.Bind(name), flux.Key("echo")),
			),
		)
	})

	lcl.Application.Run()
}
