package flux_test

import (
	"fmt"

	flux "github.com/xiaowumin-mark/flux-vcl"
)

func ExampleTween() {
	position := flux.Tween(10, 20, 0.25)
	fmt.Println(position)
	// Output:
	// 12
}

func ExampleNewCapability() {
	export := flux.NewCapability[bool]("example.chart.export")
	fmt.Println(export.Name())
	// Output:
	// example.chart.export
}

func ExampleLookupCapability() {
	dpi, ok := flux.LookupCapability(flux.PluginContext{}, flux.RendererDPI)
	if !ok {
		dpi = 96
	}
	fmt.Println(dpi)
	// Output:
	// 96
}
