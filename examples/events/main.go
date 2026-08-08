// FluxVCL Phase 4 事件系统与生命周期演示应用：examples/events
//
// 统一事件（design.md §10 / D6）：
//  1. 点击：OnClick(func(Event)) —— 事件参数携带 Type/Source（Type#Key 稳定
//     身份，D3）；counter 按钮文本随 State 刷新（冒烟断言目标）。
//  2. hover：OnMouseMove —— 坐标经 native 边界从物理像素归一为 DIP
//     （D5 全坐标 DIP），相对控件客户区。
//  3. 键盘：OnKeyDown —— 按钮聚焦后按任意键，状态栏显示虚拟键码 + 修饰键。
//  4. 中文输入（4.4 IME）：Input 双向绑定（原生 TEdit IME 处理组合与候选窗，
//     OnChange 回写 State）+ OnKeyPress（form/控件级 OnUTF8KeyPress 逐字符
//     路由，含 IME 组合结果）→ 状态栏回显。
//  5. 生命周期（4.3）：按钮 OnMount/OnUpdate/OnUnmount 计数 —— 状态栏显示
//     挂载/更新/卸载次数；窗口拖 resize 触发 OnUpdate（复用 Phase 3 re-layout）。
//
// 工程约束（Phase 0 冒烟结论）：smoke 按 class=Button 枚举子控件取按钮文本作
// "点击生效"信号 —— 本 demo 全窗口只有一个按钮（counter），枚举唯一。
//
// 构建：scripts/build.ps1 -Target events；冒烟：scripts/smoke.ps1 -Target events。
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
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll。请用 scripts/build.ps1 -Target events 构建。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}

	r := native.NewRenderer()
	app := flux.NewApp(r)

	count := flux.NewState(0)      // counter（smoke 断言按钮文本变数字）
	name := flux.NewState("中文")    // two-way 绑定：Input ↔ Text 回显（原生 IME）
	status := flux.NewState("hover / click / press keys / type Chinese")
	life := flux.NewState("mount:0 update:0 unmount:0") // 生命周期读数

	var mount, update, unmount int
	lastX, lastY := -1, -1 // hover 去重：同坐标不触发 re-render

	app.Mount(func() flux.Widget {
		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - events (Phase 4)"),
			flux.Column(
				flux.Text("统一事件：hover（坐标） / click（Source） / 聚焦后按键盘", flux.Key("hint")),

				// 唯一按钮（smoke 约束）：OnClick / OnMouseMove / OnKeyDown / 生命周期钩子
				flux.Button(flux.Bind(count), flux.Key("btn"),
					flux.OnClick(func(e flux.Event) {
						count.Set(count.Get() + 1)
						status.Set(fmt.Sprintf("click: %s (%s)", e.Source, e.Type))
					}),
					flux.OnMouseMove(func(e flux.Event) {
						if e.X == lastX && e.Y == lastY {
							return
						}
						lastX, lastY = e.X, e.Y
						status.Set(fmt.Sprintf("mousemove@(%d,%d) mods=%d", e.X, e.Y, e.Mods))
					}),
					flux.OnKeyDown(func(e flux.Event) {
						status.Set(fmt.Sprintf("keydown VK=0x%02X mods=%d", e.Key, e.Mods))
					}),
					flux.OnMount(func() { mount++; life.Set(fmt.Sprintf("mount:%d update:%d unmount:%d", mount, update, unmount)) }),
					flux.OnUpdate(func() { update++; life.Set(fmt.Sprintf("mount:%d update:%d unmount:%d", mount, update, unmount)) }),
					flux.OnUnmount(func() { unmount++; life.Set(fmt.Sprintf("mount:%d update:%d unmount:%d", mount, update, unmount)) }),
				),

				flux.Text("中文输入（原生 IME）：在输入框打字，下面回显", flux.Key("ime-hint")),
				flux.Input(flux.Bind(name), flux.Key("input"),
					flux.OnKeyPress(func(e flux.Event) {
						status.Set(fmt.Sprintf("keypress: %q", e.Text))
					}),
				),
				flux.Text(flux.Bind(name), flux.Key("echo")),
				flux.Text(flux.Bind(status), flux.Key("status")),
				flux.Text(flux.Bind(life), flux.Key("life")),
			),
		)
	})

	lcl.Application.Run()
}
