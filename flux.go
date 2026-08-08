// Package flux 是 FluxVCL 框架的主包。
//
// FluxVCL 是一个基于 Go 的现代声明式 UI 框架，提供类似 Flutter / Vue /
// SwiftUI 的开发体验，底层使用原生 Windows 控件（默认 LCL 后端，energye/lcl
// + libenergy DLL；VCL 为 B 计划，见 docs/govcl-vs-lcl.md）。
//
// 设计文档见 docs/design.md，开发计划见 docs/development-plan.md。
// 声明式核心（Widget / Element / diff / Renderer）将在 Phase 1 实现，
// 当前（Phase 0）仅提供版本与构建期标识，用于验证模块与构建链路。
package flux

// Version 是当前框架版本。采用 semver 风格；"-dev" 后缀表示未发布开发版。
const Version = "0.1.0-dev"
