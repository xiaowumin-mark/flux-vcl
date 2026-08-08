// Package render 定义 FluxVCL 的 Renderer 抽象与原生控件适配
// （design.md §5.1，D6 绑定隔离）。
//
// Renderer 面向窄接口（Create/SetBounds/SetVisible/TextWidth/HandleAllocated…），
// 默认 LCL 绑定（energye/lcl）藏在适配层后，保留切 govcl v1.2.10（B 计划）的余地。
// Phase 1 实现，当前仅占位。
package render
