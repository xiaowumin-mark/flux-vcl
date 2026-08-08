// FluxVCL Phase 0 冒烟应用：examples/basic
//
// 最小"窗体 + 文本 + 按钮 + 点击"用例，验证：
//  1. 库 import（github.com/xiaowumin-mark/flux-vcl 根包）
//  2. energye/lcl v1.0.3 + libenergy-amd64.dll 的构建与加载
//  3. 构建脚手架：go-winres 生成的 rsrc_windows_amd64.syso
//     （PerMonitorV2 DPI manifest + 图标 + 版本信息）+ windowsgui 构建
//
// 关键约束（E2 结论）：Go 包版本必须与 libenergy DLL 严格一致。
// lcl v1.0.3 ↔ libenergy-amd64.dll（取自 energye/designer 内嵌资源）。
// 版本错位时 SysCall 表名与 DLL 导出不匹配，窗口将无法创建。
//
// 构建（详见 scripts/build.ps1）：脚本会生成 .syso 资源、构建 exe、
// 并把 libenergy-amd64.dll 复制到 exe 旁。也可手动：
//   cd examples/basic && go run github.com/tc-hib/go-winres@latest make --arch amd64
//   cd ../.. && CGO_ENABLED=0 GOOS=windows go build -buildmode=exe -ldflags "-H=windowsgui" -o bin/basic.exe ./examples/basic
package main

import (
	"fmt"
	"os"
	"path/filepath"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/energye/lcl/api"
	"github.com/energye/lcl/api/libname"
	"github.com/energye/lcl/lcl"
)

// basicForm 嵌入 TEngForm（designer 的 TAppWindow 同款），
// 经 Application.NewForms 注册为主窗体。
type basicForm struct {
	lcl.TEngForm
}

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
	libname.LibName = dllPath

	// 初始化（LockOSThread + 加载 libenergy）
	lcl.Init(nil, nil)
	if api.Widget() != api.WtWIN32 {
		fmt.Fprintln(os.Stderr, "期望 WtWIN32 widgetset，实际 =", api.Widget())
		os.Exit(2)
	}

	// 标准启动序列：Initialize → SetMainFormOnTaskBar → NewForms → Run
	lcl.Application.Initialize()
	lcl.Application.SetMainFormOnTaskBar(true)

	f := &basicForm{}
	// 先 NewForms 注册（内部创建真实 form 实例并绑定到 f），
	// 之后再创建控件 —— 否则控件绑定到被替换的旧实例上。
	lcl.Application.NewForms(f)

	f.SetCaption("FluxVCL " + flux.Version + " - basic")
	f.SetWidth(420)
	f.SetHeight(240)

	label := lcl.NewLabel(f)
	label.SetParent(f)
	label.SetBounds(20, 20, 380, 28)
	label.SetCaption("Hello, FluxVCL! 点下面按钮试试")

	btn := lcl.NewButton(f)
	btn.SetParent(f)
	btn.SetBounds(150, 100, 120, 40)
	btn.SetCaption("Click me")
	clicks := 0
	btn.SetOnClick(func(sender lcl.IObject) {
		clicks++
		label.SetCaption(fmt.Sprintf("已点击 %d 次", clicks))
		// LCL 的 TLabel 无独立 HWND（自绘在父窗体表面），Win32 冒烟
		// 无法读它的文本；按钮文本可读，作为"点击生效"的可观测信号。
		btn.SetCaption(fmt.Sprintf("Clicked %d", clicks))
	})

	lcl.Application.Run()
}
