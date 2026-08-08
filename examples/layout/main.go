// FluxVCL Phase 3 布局演示应用：examples/layout
//
// 布局引擎核心用例（design.md §6.2 / D5 / research.md §5.1）：
//  1. flex 分配：Expanded 拿满剩余空间、flex 因子 1:2 按比例分割（左 1/3、右 2/3）；
//  2. 主轴/交叉轴对齐：CrossAxisStretch 让面板内容撑满交叉轴；
//  3. resize 即时更新：拖动窗口边框 → 窗体 resize 事件 → invalidate → re-render，
//     左右面板按新客户区尺寸即时重分割（零控件重建，diff 只 patch Bounds）；
//  4. 滚动容器（Phase 3.6）：左面板 Expanded(ScrollBox(20 行)) —— Expanded 给
//     ScrollBox 有界 viewport，内容超高时原生 TScrollBox 滚动条滚动（SingleChildScroll
//     语义，滚动轴 unbounded 测量）；resize 后滚动范围随新 viewport 即时更新。
//
// 工程约束（Phase 0 冒烟结论）：LCL 的 TLabel 无独立 HWND，Win32 冒烟无法读
// label 文本；因此顶部 counter 按钮文本（TButton 有 HWND）作为"点击生效"信号
// （smoke.ps1 -Target layout 断言按钮文本 0→数字）。
//
// 初始化序列（E2 结论）：Init → NewRenderer(NewForms) → Mount(build) → Run。
// Go 包版本必须与 libenergy DLL 严格一致：lcl v1.0.3 ↔ libenergy-amd64.dll。
//
// 构建：scripts/build.ps1 -Target layout（生成 winres → windowsgui exe → 复制 DLL）。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/energye/lcl/lcl"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/internal/native"
)

// scrollItems 左面板滚动列表内容：20 行文本（Phase 3.6 滚动容器 demo）。
// 行高 20 + 间距 4 → 内容总高 476，超 viewport 触发原生滚动；行 key 稳定
// （si0..si19）保证 diff 只 patch Bounds 不重建。
func scrollItems() flux.Widget {
	var kids []any
	for i := range 20 {
		kids = append(kids, flux.Text(fmt.Sprintf("scroll item %d", i), flux.Key(fmt.Sprintf("si%d", i))))
	}
	return flux.Column(kids...)
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
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll（应位于 exe 旁）。请用 scripts/build.ps1 -Target layout 构建，它会复制 DLL。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}

	r := native.NewRenderer()
	app := flux.NewApp(r)

	// State 原语：Set 可跨 goroutine，re-render 自动 marshal 到 UI 线程。
	count := flux.NewState(0)        // 顶部 counter（冒烟信号：点击后按钮文本为数字）
	leftName := flux.NewState("左面板") // 左面板输入框（two-way → 文本回显）
	dpiLabel := flux.NewState(fmt.Sprintf("DPI: %d", r.DPI())) // 底部 DPI 读数（Phase 3.5）

	app.Mount(func() flux.Widget {
		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - layout (Phase 3)"),
			flux.Column(
				// 顶栏：标题 + counter 按钮（冒烟断言目标）+ 操作提示
				flux.Row(
					flux.Text("Panel demo", flux.Key("title")),
					flux.Button(flux.Bind(count), flux.Key("btn"), flux.OnClick(func() {
						count.Set(count.Get() + 1)
					})),
					flux.Text("拖动窗口边框，看左右面板即时重分割", flux.Key("hint")),
				),

				// 主区域：Expanded 拿满剩余高度，内部 1:2 左右分栏
				// （右面板宽度是左面板 2 倍；resize 时按新宽度重算分配）
				flux.Expanded(flux.Row(
					flux.Expanded(flux.Column(
						flux.Key("left"),
						flux.CrossAxis(flux.CrossAxisStretch),
						flux.Text("Left 1/3", flux.Key("lt")),
						flux.Input(flux.Bind(leftName), flux.Key("li")),
						flux.Text(flux.Bind(leftName), flux.Key("echo")),
						// Phase 3.6 滚动列表：Expanded 给 ScrollBox 有界高度，
						// 内容 20 行超高 → 原生滚动条（滚动轴 unbounded 测量）。
						flux.Expanded(flux.ScrollBox(scrollItems())),
					), 1),
					flux.Expanded(flux.Column(
						flux.Key("right"),
						flux.CrossAxis(flux.CrossAxisStretch),
						flux.Text("Right 2/3", flux.Key("rt")),
						flux.Button("Action", flux.Key("rb")),
					), 2),
				)),

				// 底栏：固定文案 + DPI 读数（Phase 3.5；跨 goroutine State.Set dogfood）
				flux.Text("Bottom bar", flux.Key("bottom")),
				flux.Text(flux.Bind(dpiLabel), flux.Key("dpi")),
			),
		)
	})

	// DPI 读数：定时器在后台 goroutine 读 r.DPI()，变化时跨线程 Set State →
	// re-render 自动 marshal 到 UI 线程（Phase 2 marshalling）。读操作经
	// r.RunOnUI 包住（UI 线程纪律：LCL 对象只在主线程访问）。
	//
	// 关机纪律（Phase 3.6）：窗体关闭时 r.OnClose 关闭 done 通道停止轮询 ——
	// 后台 goroutine 的 RunOnMainThreadSync 与窗体 teardown 竞争会触发间歇性
	// 0xC0000005（框架 RunOnUI 的 closed 门是兜底，双保险）。
	done := make(chan struct{})
	r.OnClose(func() { close(done) })
	go func() {
		var last int32
		for {
			select {
			case <-done:
				return
			case <-time.After(time.Second):
				var d int
				r.RunOnUI(func() { d = r.DPI() })
				if int32(d) != last {
					last = int32(d)
					dpiLabel.Set(fmt.Sprintf("DPI: %d", d))
				}
			}
		}
	}()

	lcl.Application.Run()
}
