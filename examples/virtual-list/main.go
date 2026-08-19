// FluxVCL Phase 6 列表与虚拟化演示应用：examples/virtual-list
//
// 大数据量三件套同屏展示（design.md §16 / development-plan §6）：
//  1. 6.1 ListView + 稳定 slot key：10 万行数据，每行 = 序号 + 内容 + 选中标记。
//  2. 6.2 虚拟化（控件池）：布局引擎只挂载"可见区 ± overscan"的 ~20 个行控件，
//     滚动 = 行内容属性 patch（SetText/SetBounds）不重建 —— 内存有界（10 万行
//     也只建 ~20 个原生控件）、滚动流畅、行内控件焦点/IME 不漂移（D3/D7b）。
//     滚动位置经 ScrollOffset 双向绑定：滚轮/滚动条拖动 → 原生 OnScroll 回写
//     State → re-render；"滚到顶/底" 编程 Set → 布局钳制到有效范围。
//  3. 6.3 多窗口：启动即开第二个窗体（独立 App/State 作用域，计数互不相干），
//     主窗体与第二窗体的 State 各自触发各自的 re-render。
//
// 键盘可达性：滚动、选中和第二窗口计数均使用原生 Button，Tab/Space/Enter
// 与鼠标路径等价。第二窗体标题不以 "FluxVCL " 开头（smoke 只匹配主窗体标题前缀）。
// State 订阅约束（design §9）：行 builder 里读取的 sel 必须 Bind 出来才响应
// Set —— 头部"已选中"读数即为此订阅（点击标记 → sel.Set → 触发 re-render）。
//
// 构建：scripts/build.ps1 -Target virtual-list；冒烟：scripts/smoke.ps1 -Target virtual-list。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/native"
)

const (
	rows = 100_000 // 数据行数（虚拟化对象：10 万行）
	rowH = 24      // 每行高度（DIP，等行高是虚拟化的前提）
)

func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法获取 exe 路径:", err)
		os.Exit(2)
	}
	dllPath := filepath.Join(filepath.Dir(exe), "libenergy-amd64.dll")
	if _, err := os.Stat(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll。请用 scripts/build.ps1 -Target virtual-list 构建。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}

	// —— 主窗体：10 万行虚拟列表 ——
	r := native.NewRenderer()
	app := flux.NewApp(r)

	scroll := flux.NewState(0) // 滚动位置（DIP，ScrollOffset 双向绑定）
	count := flux.NewState(0)  // counter（smoke 断言按钮文本变数字）
	sel := flux.NewState(-1)   // 当前选中行（-1 = 无）
	maxOffset := rows * rowH   // "滚到底" 目标（布局钳制到 内容−视口）

	app.Mount(func() flux.Widget {
		th := flux.LightTheme
		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - phase6 (虚拟列表/多窗口)"),
			flux.Color(th.Background),
			// 静态树零 Key（D3：寻址与身份解耦）—— 头/控制行固定不重排，位置匹配即可；
			// 虚拟列表行身份由 ListView 内部控制池 slot key（row-N）提供，builder 产物
			// 不写 key（否则滚动换内容时重建，破坏控件池）。
			flux.Column(
				// 头部：标题 + 计数按钮 + 滚动位置读数
				flux.Row(
					flux.Text("10 万行虚拟列表", flux.FontColor(th.Text), flux.Width(140)),
					flux.Button(flux.Bind(count),
						flux.OnClick(func(e flux.Event) { count.Set(count.Get() + 1) })),
					flux.Expanded(flux.Text("滚动: ", flux.FontColor(th.Text))),
					flux.Text(flux.Bind(scroll), flux.FontColor(th.Accent)),
					flux.Text(" DIP", flux.FontColor(th.Text)),
					// 选中行读数：把 sel 绑出来 = 订阅 App（design §9）。行 builder 里
					// 的 sel.Get() 只是普通读、不订阅；未 Bind 的 State 其 Set 只改内存
					// 值、不触发 re-render（phase5 主题 chip 同坑）—— 点击行标记后
					// 必须有这次 render 才看得到 ○→●。
					flux.Text("  | 已选中: ", flux.FontColor(th.Text)),
					flux.Text(flux.Bind(sel), flux.FontColor(th.Accent)),
					flux.Text(" 行", flux.FontColor(th.Text)),
				),
				// 控制行：原生按钮同时提供鼠标与键盘操作路径。
				flux.Row(
					flux.Button("滚到顶", flux.AccessibleName("滚动到列表顶部"),
						flux.OnClick(func(flux.Event) { scroll.Set(0) })),
					flux.Button("滚到底", flux.AccessibleName("滚动到列表底部"),
						flux.OnClick(func(flux.Event) { scroll.Set(maxOffset) })),
					flux.Button("选中第 50000 行",
						flux.OnClick(func(flux.Event) {
							sel.Set(50000)
							scroll.Set(50000 * rowH) // 滚动到目标行（可见区中部）
						})),
					flux.Button("清除选中",
						flux.OnClick(func(flux.Event) { sel.Set(-1) })),
				),
				// 虚拟列表：占满剩余高度（Expanded → 有界约束）
				flux.Expanded(
					flux.ListView(rows, rowH, func(idx int) flux.Widget {
						mark := "○"
						if sel.Get() == idx {
							mark = "●"
						}
						return flux.Row(
							flux.Text(fmt.Sprintf("%6d", idx), flux.Width(70),
								flux.FontColor(th.Text)),
							flux.Text(fmt.Sprintf("第 %d 条数据（点击右侧标记选中）", idx),
								flux.FontColor(th.Text)),
							flux.Expanded(flux.Text("", flux.Width(1))),
							flux.Button(mark, flux.Width(40),
								flux.AccessibleName(fmt.Sprintf("选择第 %d 行", idx)),
								flux.OnClick(func(e flux.Event) {
									if sel.Get() == idx {
										sel.Set(-1)
									} else {
										sel.Set(idx)
									}
								})),
						)
					}, flux.ScrollOffset(scroll)),
				),
			),
		)
	})

	// —— 6.3 第二个窗体：独立 App/State 作用域 ——
	// 与主窗体各自持有独立 State（count2）：计数互不相干，各自触发各自 re-render。
	r2 := native.NewRenderer()
	app2 := flux.NewApp(r2)
	count2 := flux.NewState(0)

	app2.Mount(func() flux.Widget {
		th := flux.DarkTheme
		return flux.Window(
			flux.Title("第二窗口 - 独立 State 作用域"),
			flux.Color(th.Background),
			flux.Column(
				flux.Text("Phase 6.3 多窗口：第二个窗体", flux.FontColor(th.Text)),
				flux.Row(
					flux.Text("第二窗口计数: ", flux.FontColor(th.Text)),
					flux.Text(flux.Bind(count2), flux.FontColor(th.Accent)),
				),
				flux.Button("计数 +1（不影响主窗口）",
					flux.OnClick(func(e flux.Event) { count2.Set(count2.Get() + 1) })),
			),
		)
	})

	r2.Show() // 次要窗体须显式 Show（主窗体由 Application.Run() 自动显示）

	native.Run()
}
