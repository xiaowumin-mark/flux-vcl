// FluxVCL 常用表单控件演示：examples/form-controls。
//
// 计数与进度操作均使用 Button，便于同时用鼠标、Tab、Space 和 Enter 验证：
//   - Memo：多行编辑、中文 IME 与文本 State 回显；
//   - CheckBox：显式 Checked 状态；
//   - ComboBox：Items、SelectedIndex 与选择回写；
//   - ProgressBar：Minimum/Maximum/Value 受控范围和值；
//   - RadioButton：Flux 逻辑 GroupIndex 的同组互斥与异组独立。
//
// 设计对应 docs/design.md §16「绑定层（D6 窄接口）」与
// docs/development-plan.md「控件扩充批次 1」。
//
// 构建：scripts/build.ps1 -Target form-controls；
// 冒烟：scripts/smoke.ps1 -Target form-controls。
package main

import (
	"fmt"
	"os"
	"path/filepath"

	flux "github.com/xiaowumin-mark/flux-vcl"
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
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll。请用 scripts/build.ps1 -Target form-controls 构建。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}

	r := native.NewRenderer()
	app := flux.NewApp(r)

	counter := flux.NewState(0)
	memo := flux.NewState("第一行：可编辑文本\n第二行：可用中文输入法验证 IME")
	checked := flux.NewState(false)
	selected := flux.NewState(0)
	progress := flux.NewState(25)
	radio := flux.NewState(0)
	radioOther := flux.NewState(false)
	status := flux.NewState("")
	options := []string{"红色", "绿色", "蓝色"}

	var build func() flux.Widget
	build = func() flux.Widget {
		value := progress.Get()
		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - form controls"),
			flux.Column(
				flux.Text("常用表单控件（批次 1）"),

				// 0→1 的纯数字 Caption 供 smoke.ps1 验证点击生效。
				flux.Button(flux.Bind(counter), flux.OnClick(func(_ flux.Event) {
					counter.Set(counter.Get() + 1)
				})),

				flux.Text("Memo（编辑后下方即时回显）"),
				flux.Memo(flux.Bind(memo), flux.Width(360), flux.Height(72)),
				flux.Text("回显："+memo.Get()),

				flux.CheckBox("启用附加选项", flux.Checked(checked.Get()),
					flux.OnCheckedChange(func(value bool) {
						checked.Set(value)
						status.Set(fmt.Sprintf("CheckBox = %v", value))
					}),
				),

				flux.Text("ComboBox"),
				flux.ComboBox(
					flux.Items(options),
					flux.SelectedIndex(selected.Get()),
					flux.OnSelectionChange(func(index int) {
						selected.Set(index)
						status.Set(fmt.Sprintf("ComboBox index = %d", index))
					}),
				),

				flux.Button(fmt.Sprintf("ProgressBar：%d / 100（每次 +10）", value),
					flux.AccessibleName("增加进度值"),
					flux.OnClick(func(_ flux.Event) {
						next := value + 10
						if next > 100 {
							next = 0
						}
						progress.Set(next)
						status.Set(fmt.Sprintf("ProgressBar value = %d", next))
					}),
				),
				flux.ProgressBar(flux.Minimum(0), flux.Maximum(100), flux.Value(value)),

				flux.Text("RadioButton（同组互斥；不同 GroupIndex 可独立选择）"),
				flux.RadioButton("选项 A", flux.Checked(radio.Get() == 0), flux.GroupIndex(1),
					flux.OnCheckedChange(func(isChecked bool) {
						if isChecked {
							radio.Set(0)
							status.Set("RadioButton = A")
						}
					}),
				),
				flux.RadioButton("选项 B", flux.Checked(radio.Get() == 1), flux.GroupIndex(1),
					flux.OnCheckedChange(func(isChecked bool) {
						if isChecked {
							radio.Set(1)
							status.Set("RadioButton = B")
						}
					}),
				),
				flux.RadioButton("选项 C", flux.Checked(radio.Get() == 2), flux.GroupIndex(1),
					flux.OnCheckedChange(func(isChecked bool) {
						if isChecked {
							radio.Set(2)
							status.Set("RadioButton = C")
						}
					}),
				),

				flux.RadioButton("独立组（GroupIndex 2）", flux.Checked(radioOther.Get()), flux.GroupIndex(2),
					flux.OnCheckedChange(func(isChecked bool) {
						radioOther.Set(isChecked)
						status.Set(fmt.Sprintf("独立 RadioButton = %v", isChecked))
					}),
				),

				flux.Text(flux.Bind(status)),
			),
		)
	}

	app.Mount(build)
	native.Run()
}
