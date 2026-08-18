# 维护政策 / Maintenance Policy

FluxVCL 的维护重点是可重复构建、无头可测和真实 Windows 冒烟。

- 依赖版本必须与 `packaging/dependencies.lock.json` 及 DLL SHA-256 一致。
- 新公开 API 必须有中文 doc comment、英文入口、无头 D7 测试、示例和设计记录。
- 后端能力通过 `internal/render` 的具名窄接口暴露；不得把 LCL 类型带入根包。
- 修复优先保证 D1-D7、IME、DPI、焦点/原生句柄身份和关闭流程。
- CI 必须跑 `go test`, `go test -race`, `go vet`，以及 DLL 到位后的 build/smoke
  和非空截图；性能数字只用于固定环境趋势比较。
- issue/PR 应说明 Go 版本、Windows 版本、DLL 来源、目标示例和复现步骤。

安全修复和数据丢失问题优先于新控件；弃用 API 至少保留一个 minor release，
并在 CHANGELOG 和迁移文档中标注。发布前由维护者按
[RELEASE_CHECKLIST.md](../RELEASE_CHECKLIST.md) 签字。

## Maintenance policy (English)

Reproducible dependencies, headless tests, and real Windows smoke runs are
release requirements. New public APIs need docs, a design record, D7 tests, and
an example. Backend-specific behavior belongs behind named render capabilities.
Deprecations remain for at least one minor release with a migration note.
