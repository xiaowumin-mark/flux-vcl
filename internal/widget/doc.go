// Package widget 定义 FluxVCL 的 Widget 声明树（design.md §4.2）。
//
// Widget 是每次 render 重建的不可变 Go 结构体（纯数据，不持有原生指针），
// 是 reconciliation 的输入。Phase 1 实现，当前仅占位。
package widget
