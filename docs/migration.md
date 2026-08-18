# 迁移与兼容政策 / Migration Policy

## v0.1.0

v0.1.0 的公开构造器、Opt、事件签名和 `State` 语义以
[候选 API 冻结清单](api-v0.1.0.md) 为首发基线。当前版本仍是 `0.1.0-dev`；正式
兼容承诺从 v0.1.0 标签开始。`Slider`、`StringGrid`、`PaintBox` 的范围是最小
产品化范围；未列出的 LCL 属性不属于兼容 API。

## 迁移规则

1. 先阅读 `docs/development-plan.md` 和设计章节，确认是否触及 D1-D7。
2. 由 `Input`/`Memo` 迁移文本时继续使用 `Bind(State[string])`；选择和滑块保持
   显式 `SelectedIndex`/`Value` 与类型化回调，不把 `Bind` 重载为隐式双向绑定。
3. Grid 数据必须通过 `Cells` 传入新的二维值；调用方不得在 render 后修改原 slice。
4. 自绘从旧的原生逃逸迁移到 `PaintBox` 的稳定 `PaintCommand` 列表，命令变化
   会触发 invalidate，不应在 paint 回调里修改 State。
5. 任何 breaking change 都要同时更新本页、API 清单、CHANGELOG 和 release
   checklist，并给出旧 API 到新 API 的代码片段。

## English

The v0.1.0 candidate API surface is frozen while the build remains
`0.1.0-dev`; the formal compatibility commitment starts at the v0.1.0 tag.
Use explicit controlled values for Slider and Grid selection,
pass defensive-copying matrices to StringGrid, and represent PaintBox drawing as
stable commands. Reopening the candidate surface requires the P7.5 gate and must
update this page, the API list, `CHANGELOG.md`, and the release checklist. After
v0.1.0, it requires a new minor release until 1.0.
