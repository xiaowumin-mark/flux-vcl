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

	"github.com/energye/lcl/lcl"
	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/native"
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
	count := flux.NewState(0)        // counter 计数（单向绑定 → Button 文本）
	name := flux.NewState("FluxVCL") // two-way 绑定目标（Input ↔ Text 回显）

	app.Mount(func() flux.Widget {
		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - basic (Phase 2)"),
			// 静态树（结构固定、不重排）零 Key：无 key 控件按位置匹配，测试/排查用
			// App.FindByPath("Window/0/Column/1/Button") 定位 —— 寻址与身份解耦（D3）。
			flux.Column(
				flux.Text("Hello, FluxVCL! State 驱动的最小用例"),

				flux.Text("1) counter：点按钮 +1，文本由 State 驱动刷新"),
				flux.Button(flux.Bind(count), flux.OnClick(func(_ flux.Event) {
					count.Set(count.Get() + 1) // 外部修改 State → 自动 re-render
				})),

				flux.Text("2) two-way：输入框 ↔ State ↔ 文本回显"),
				flux.Input(flux.Bind(name)),
				flux.Text(flux.Bind(name)),
			),
		)
	})

	lcl.Application.Run()
}
