// FluxVCL 7GUIs Flight Booker：受控 ComboBox、日期 Input、校验与 Enabled 闭环。
//
// 设计对应 docs/design.md §21.4，日期字符串校验的业务边界见 docs/7guis.md。
// 构建：scripts/build.ps1 -Target 7guis-flight-booker；
// 冒烟：scripts/smoke.ps1 -Target 7guis-flight-booker。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/native"
)

const dateLayout = "2006-01-02"

func parseDate(text string) (time.Time, bool) {
	value, err := time.Parse(dateLayout, text)
	return value, err == nil
}

func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法获取 exe 路径:", err)
		os.Exit(2)
	}
	dllPath := filepath.Join(filepath.Dir(exe), "libenergy-amd64.dll")
	if _, err := os.Stat(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll。请用 scripts/build.ps1 -Target 7guis-flight-booker 构建。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}

	r := native.NewRenderer()
	app := flux.NewApp(r)
	flightTypes := []string{"One-way flight", "Return flight"}
	flightType := flux.NewState(flightTypes[0])
	today := time.Now().Format(dateLayout)
	outbound := flux.NewState(today)
	returnDate := flux.NewState(time.Now().AddDate(0, 0, 1).Format(dateLayout))
	status := flux.NewState("Choose a flight type and valid travel date.")

	if err := app.Mount(func() flux.Widget {
		selectedIndex := 0
		isReturn := flightType.Get() == flightTypes[1]
		if isReturn {
			selectedIndex = 1
		}

		outboundDate, outboundValid := parseDate(outbound.Get())
		backDate, returnValid := parseDate(returnDate.Get())
		bookEnabled := outboundValid
		validation := "Outbound date is valid."
		if !outboundValid {
			validation = "Outbound date must use YYYY-MM-DD."
		}
		if isReturn {
			bookEnabled = outboundValid && returnValid && !backDate.Before(outboundDate)
			switch {
			case !returnValid:
				validation = "Return date must use YYYY-MM-DD."
			case outboundValid && backDate.Before(outboundDate):
				validation = "Return date cannot precede the outbound date."
			case bookEnabled:
				validation = "Both travel dates are valid."
			}
		}

		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - 7GUIs Flight Booker"),
			flux.Width(520),
			flux.Height(300),
			flux.Column(
				flux.CrossAxis(flux.CrossAxisStretch),
				flux.Text("Flight Booker"),
				flux.ComboBox(
					flux.Items(flightTypes),
					flux.SelectedIndex(selectedIndex),
					flux.OnSelectionChange(func(index int) {
						if index >= 0 && index < len(flightTypes) {
							flightType.Set(flightTypes[index])
						}
					}),
				),
				flux.Row(
					flux.Text("Selected: "),
					// 绑定模式文字，使选择 State 的变化直接触发受控属性重渲染。
					flux.Text(flux.Bind(flightType)),
				),
				flux.Row(
					flux.Text("Outbound date (YYYY-MM-DD): "),
					flux.Expanded(flux.Input(flux.Bind(outbound))),
				),
				flux.Row(
					flux.Text("Return date (YYYY-MM-DD): "),
					flux.Expanded(flux.Input(flux.Bind(returnDate), flux.Enabled(isReturn))),
				),
				flux.Text(validation),
				flux.Button("Book", flux.Enabled(bookEnabled), flux.OnClick(func(_ flux.Event) {
					if isReturn {
						status.Set(fmt.Sprintf("Booked return flight: %s to %s.", outbound.Get(), returnDate.Get()))
						return
					}
					status.Set("Booked one-way flight for " + outbound.Get() + ".")
				})),
				flux.Text(flux.Bind(status)),
			),
		)
	}); err != nil {
		fmt.Fprintln(os.Stderr, "挂载失败:", err)
		os.Exit(2)
	}

	native.Run()
}
