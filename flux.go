// Package flux 是 FluxVCL 框架的主包，向用户暴露声明式 UI API。
//
// FluxVCL 是一个基于 Go 的现代声明式 UI 框架，提供类似 Flutter / Vue /
// SwiftUI 的开发体验，底层使用原生 Windows 控件（默认 LCL 后端，energye/lcl
// + libenergy DLL；VCL 为 B 计划，见 docs/govcl-vs-lcl.md）。
//
// 编程模型（design.md）：用户每次状态变化重建 Widget 树（flux.Window/Column/
// Text/Button/...），经 flux.App.Render 交给 diff 引擎 —— 按 D1 canUpdate 匹配
// Element、D2 属性级 patch，只把变化的属性落到绑定层原生控件（零重建）。
//
//   - Widget / Node 为 type alias（内部实现见 internal/widget），用户可实现
//     Create() 返回节点树来自定义组件。
//   - 绑定层隔离（D6）：flux 只面向 internal/render.Renderer 窄接口；默认 LCL
//     适配见 internal/native。
//   - 事件回调（OnClick 等）每次 render 重新绑定（函数值无法比较相等性）。
//
// 设计文档见 docs/design.md，开发计划见 docs/development-plan.md。
package flux

// Version 是当前框架版本。采用 semver 风格；"-dev" 后缀表示未发布开发版。
const Version = "0.1.0-dev"
