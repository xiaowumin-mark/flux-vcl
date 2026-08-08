// Package diff 实现 FluxVCL 的 diff / reconciliation 引擎
// （design.md §5，开发计划 Phase 1.4）。
//
// 全项目最高优先级代码：按 D1 canUpdate 匹配、D2 属性级 patch + 批量提交、
// D7 三不变量测试。Phase 1 实现，当前仅占位。
package diff
