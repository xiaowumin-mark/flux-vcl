// FluxVCL 7GUIs Circle Drawer：PaintBox 自绘、DIP 鼠标命中与示例层 undo/redo。
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	flux "github.com/xiaowumin-mark/flux-vcl"
	"github.com/xiaowumin-mark/flux-vcl/native"
)

const (
	defaultRadius = 30
	minimumRadius = 5
	maximumRadius = 150
)

type circle struct {
	x, y   int
	radius int
	color  flux.ColorValue
}

type drawingState struct {
	circles  []circle
	selected int
}

func cloneDrawing(d drawingState) drawingState {
	return drawingState{
		circles:  append([]circle(nil), d.circles...),
		selected: d.selected,
	}
}

func hitCircle(circles []circle, x, y int) int {
	best := -1
	bestDistance := 0
	for i, c := range circles {
		dx, dy := x-c.x, y-c.y
		distance := dx*dx + dy*dy
		if distance <= c.radius*c.radius && (best < 0 || distance < bestDistance) {
			best = i
			bestDistance = distance
		}
	}
	return best
}

func main() {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "无法获取 exe 路径:", err)
		os.Exit(2)
	}
	dllPath := filepath.Join(filepath.Dir(exe), "libenergy-amd64.dll")
	if _, err := os.Stat(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "缺少 libenergy-amd64.dll。请用 scripts/build.ps1 -Target 7guis-circle-drawer 构建。")
		os.Exit(2)
	}
	if err := native.Init(dllPath); err != nil {
		fmt.Fprintln(os.Stderr, "初始化失败:", err)
		os.Exit(2)
	}

	r := native.NewRenderer()
	app := flux.NewApp(r)
	radiusText := flux.NewState(strconv.Itoa(defaultRadius))

	drawing := drawingState{selected: -1}
	var undoStack []drawingState
	var redoStack []drawingState
	radiusValid := true

	setRadiusText := func() {
		radiusValid = true
		if drawing.selected < 0 || drawing.selected >= len(drawing.circles) {
			radiusText.Set(strconv.Itoa(defaultRadius))
			return
		}
		radiusText.Set(strconv.Itoa(drawing.circles[drawing.selected].radius))
	}
	commit := func(change func()) {
		undoStack = append(undoStack, cloneDrawing(drawing))
		redoStack = nil
		change()
	}

	palette := []flux.ColorValue{
		flux.RGB(0x3B, 0x82, 0xF6),
		flux.RGB(0x10, 0xB9, 0x81),
		flux.RGB(0xF5, 0x9E, 0x0B),
		flux.RGB(0xEF, 0x44, 0x44),
	}

	if err := app.Mount(func() flux.Widget {
		commands := make([]flux.PaintCommand, 0, len(drawing.circles)+1)
		commands = append(commands, flux.PaintCommand{
			Kind:  flux.PaintClear,
			Color: flux.RGB(0xFA, 0xFA, 0xFA),
		})
		for i, c := range drawing.circles {
			strokeWidth := 1
			strokeColor := flux.RGB(0x4B, 0x55, 0x63)
			if i == drawing.selected {
				strokeWidth = 4
				strokeColor = flux.RGB(0x11, 0x18, 0x27)
			}
			commands = append(commands, flux.PaintCommand{
				Kind:        flux.PaintCircle,
				X:           c.x,
				Y:           c.y,
				Radius:      c.radius,
				FillColor:   c.color,
				StrokeColor: strokeColor,
				StrokeWidth: strokeWidth,
			})
		}

		selected := "none"
		if drawing.selected >= 0 {
			selected = strconv.Itoa(drawing.selected + 1)
		}
		radiusColor := flux.LightTheme.Text
		if !radiusValid {
			radiusColor = flux.RGB(0xB9, 0x1C, 0x1C)
		}
		canvas := flux.Widget(flux.PaintBox(commands,
			flux.Key("drawing"),
			flux.OnMouseDown(func(e flux.Event) {
				if e.Button != flux.ButtonLeft {
					return
				}
				if index := hitCircle(drawing.circles, e.X, e.Y); index >= 0 {
					drawing.selected = index
					setRadiusText()
					return
				}

				radius := defaultRadius
				if value, err := strconv.Atoi(radiusText.Get()); err == nil && value >= minimumRadius && value <= maximumRadius {
					radius = value
				}
				commit(func() {
					drawing.circles = append(drawing.circles, circle{
						x: e.X, y: e.Y, radius: radius,
						color: palette[len(drawing.circles)%len(palette)],
					})
					drawing.selected = len(drawing.circles) - 1
				})
				setRadiusText()
			}),
		))

		return flux.Window(
			flux.Title("FluxVCL "+flux.Version+" - 7GUIs Circle Drawer"),
			flux.Column(
				flux.CrossAxis(flux.CrossAxisStretch),
				flux.Row(
					flux.Button("Undo", flux.Enabled(len(undoStack) > 0), flux.OnClick(func(_ flux.Event) {
						if len(undoStack) == 0 {
							return
						}
						redoStack = append(redoStack, cloneDrawing(drawing))
						drawing = undoStack[len(undoStack)-1]
						undoStack = undoStack[:len(undoStack)-1]
						setRadiusText()
					})),
					flux.Button("Redo", flux.Enabled(len(redoStack) > 0), flux.OnClick(func(_ flux.Event) {
						if len(redoStack) == 0 {
							return
						}
						undoStack = append(undoStack, cloneDrawing(drawing))
						drawing = redoStack[len(redoStack)-1]
						redoStack = redoStack[:len(redoStack)-1]
						setRadiusText()
					})),
					flux.Expanded(flux.Text(fmt.Sprintf("Circles: %d   Selected: %s", len(drawing.circles), selected))),
				),
				flux.Expanded(canvas),
				flux.Row(
					flux.Text("Radius"),
					flux.Input(
						flux.Bind(radiusText),
						flux.Width(80),
						flux.Enabled(drawing.selected >= 0),
						flux.OnChange(func(text string) {
							value, err := strconv.Atoi(text)
							radiusValid = err == nil && value >= minimumRadius && value <= maximumRadius
							if radiusValid && drawing.selected >= 0 && drawing.selected < len(drawing.circles) &&
								drawing.circles[drawing.selected].radius != value {
								commit(func() {
									drawing.circles[drawing.selected].radius = value
								})
							}
							radiusText.Set(text)
						}),
					),
					flux.Text(fmt.Sprintf("%d-%d DIP", minimumRadius, maximumRadius), flux.FontColor(radiusColor)),
				),
			),
		)
	}); err != nil {
		fmt.Fprintln(os.Stderr, "挂载失败:", err)
		os.Exit(2)
	}

	native.Run()
}
