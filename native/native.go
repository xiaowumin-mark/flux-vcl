// Package native 提供 FluxVCL 默认的 Windows 原生后端入口。
//
// 下游应用应通过本包初始化 libenergy、创建 Renderer 并启动消息循环；
// 具体的 LCL 适配实现保留在 internal/native，避免应用依赖不稳定实现细节。
package native

import (
	"github.com/energye/lcl/lcl"

	backend "github.com/xiaowumin-mark/flux-vcl/internal/native"
)

// Renderer 是默认 energye/lcl 后端的 Renderer。
// 使用 NewRenderer 创建；一个 Renderer 对应一个原生窗口。
type Renderer = backend.Renderer

// Init 加载与 energye/lcl v1.0.3 匹配的 libenergy DLL，并初始化原生应用。
// 必须在 NewRenderer 及任何控件创建之前调用一次。
func Init(dllPath string) error { return backend.Init(dllPath) }

// NewRenderer 创建一个原生窗口及其 FluxVCL Renderer。
// 主窗口由 Run 自动显示；其他窗口调用 Renderer.Show 显示。
func NewRenderer() *Renderer { return backend.NewRenderer() }

// Run 启动原生应用消息循环，并阻塞到主窗口关闭。
// 调用前必须已经 Init、创建 Renderer 并挂载根 Widget。
func Run() { lcl.Application.Run() }
