// Package badge 演示只依赖 FluxVCL 公开 SDK 的第三方组合式插件。
// 本包不导入 internal/native、internal/render 或 energye/lcl。
package badge

import (
	flux "github.com/xiaowumin-mark/flux-vcl"
)

// TypeName 是 Badge 在进程插件注册表中的全局唯一类型名。
const TypeName = "example.badge"

// Register 注册 Badge 插件。调用方应在创建 App 前调用一次，并处理重复注册错误。
func Register() error {
	return flux.RegisterWidget(TypeName, flux.WidgetPlugin{
		Build: func(ctx flux.PluginBuildContext) (flux.Widget, error) {
			label, _ := ctx.Properties.String("label")
			tone, _ := ctx.Properties.String("tone")
			mark := "[i]"
			if tone == "success" {
				mark = "[ok]"
			}
			return flux.Row(
				flux.Text(mark, flux.FontColor(flux.RGB(0x00, 0x78, 0x3C))),
				flux.Text(label),
			), nil
		},
		Measure: func(ctx flux.PluginMeasureContext) (flux.PluginLayout, error) {
			// 给 builder 子树增加紧凑的左右/上下留白；全部单位均为 DIP。
			return flux.PluginLayout{
				Size:        flux.Size{W: ctx.ChildSize.W + 16, H: ctx.ChildSize.H + 8},
				ChildOffset: flux.Point{X: 8, Y: 4},
			}, nil
		},
	})
}

// Widget 创建一个 Badge 插件节点。label/tone 使用类型化属性，不暴露 map[string]any。
func Widget(label, tone string, opts ...flux.Opt) flux.Widget {
	args := make([]any, len(opts))
	for i, opt := range opts {
		args[i] = opt
	}
	return flux.PluginWidget(TypeName, flux.NewPluginProperties(
		flux.PluginString("label", label),
		flux.PluginString("tone", tone),
	), args...)
}
