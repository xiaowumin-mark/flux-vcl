# 贡献指南（FluxVCL）

> 本文约定如何为 FluxVCL 做贡献：从 issue、分支、提交到 PR 合并的完整工作流。
> 代码风格与架构约束见 [docs/development-guide.md](docs/development-guide.md)；
> 各类命名规则见 [docs/naming-conventions.md](docs/naming-conventions.md)。

## 目录

1. [参与方式](#1-参与方式)
2. [环境准备](#2-环境准备)
3. [工作流](#3-工作流)
4. [提交信息规范](#4-提交信息规范)
5. [分支命名](#5-分支命名)
6. [PR 规范](#6-pr-规范)
7. [提交前检查清单](#7-提交前检查清单)
8. [规范文档索引](#8-规范文档索引)

---

## 1. 参与方式

- **报告 Bug**：提 issue，说明复现步骤、期望行为 / 实际行为、相关 commit 或版本。
- **提交修复 / 新特性**：按下面工作流操作，改动需带无头测试（见开发规范 §6）。
- **文档改进**：同样走 PR；中文主文档之外，仅 `README.en.md`、惯例发布元数据
  `CHANGELOG.md`/`RELEASE_CHECKLIST.md` 和明确标记的双语映射页适用开发规范中的
  语言豁免。

---

## 2. 环境准备

| 项 | 要求 |
|---|---|
| Go | 1.22+（`go.mod` 锁 `go 1.22`，覆盖 1.22–1.27 工具链） |
| 依赖 | `github.com/energye/lcl v1.0.3`（须与 `libenergy-amd64.dll` 版本严格一致） |
| 运行时 DLL | `libenergy-amd64.dll`（获取方式见 `docs/phase0-e2-libenergy-mapping.md`；可用环境变量 `FVCL_LIBENERGY_DLL` 指定路径） |
| 工具 | `go-winres`（生成 `.syso`）、`goimports`（可选） |

常用命令：

```powershell
# 全量无头测试（不依赖 DLL，任意平台可跑）
go test ./...

# 构建并冒烟某个示例（公开目标见下表）
.\scripts\build.ps1 -Target <name>
.\scripts\smoke.ps1 -Target <name>
```

### 2.1 公开示例与 smoke 契约

| 目标 | 主要覆盖 |
|---|---|
| `basic` | State、Button、Input 双向绑定 |
| `layout` | Flex、滚动、resize、DPI |
| `events` | 鼠标/键盘事件、IME、生命周期 |
| `phase5` | 动画、主题、Async、Component |
| `form-controls` | Memo、CheckBox、ComboBox、ProgressBar、RadioButton |
| `virtual-list` | 十万行虚拟化、滚动回写、多窗口 |
| `inspector` | 三层快照、mutation/event、重建风险 |
| `plugin-badge` | 公开插件 SDK、布局与生命周期 |
| `page-control` | PageControl、TabPage、稳定页面 Key |
| `7guis-counter` | Counter |
| `7guis-temperature-converter` | Temperature Converter |
| `7guis-flight-booker` | Flight Booker |
| `7guis-timer` | Timer、Slider、ProgressBar、主线程动画 pump |
| `7guis-crud` | CRUD、StringGrid 选择与编辑 |
| `7guis-circle-drawer` | Circle Drawer、PaintBox、撤销/重做 |
| `7guis-cells` | Cells、StringGrid、公式依赖 |

基础/通用示例使用通用 smoke 时，保留且只保留一个 Caption 为纯数字的按钮：初始
Caption 为 `0`，点击一次后由 State 更新为 `1`。7GUIs 不添加测试专用控件；其
专用 smoke 必须按业务 Caption/class/位置定位真实控件，并断言任务状态变化。

---

## 3. 工作流

```
issue（报告/提议） → 分支 → 开发（提交规范见 §4） → 本地验证 → PR → CI → 合并 main
```

1. **issue**：新特性或 bug 先在 issue 里对齐目标（尤其可能触及架构不变量 D1–D7 时）。
2. **分支**：从 main 切出 `feat/...`、`fix/...` 等（见 §5）。main 上仅允许 trivial 变更（文档、格式）。
3. **开发**：遵守 [开发规范](docs/development-guide.md) 与 [命名规范](docs/naming-conventions.md)。
4. **本地验证**：`gofmt` → `go vet ./...` → `go test ./...`（并发相关加 `-race`）→ 必要时 `build.ps1` + `smoke.ps1`。
5. **PR**：标题 = 提交信息，正文说明动机 / 影响面 / 测试（见 §6）。
6. **合并**：CI 全绿后合并；合并用 squash，保留单条规范提交信息。

---

## 4. 提交信息规范

采用 **Conventional Commits** 风格：`type(scope): subject`，`subject` 一律**中文**。

### 4.1 type（必填）

| type | 用途 | 本仓库示例 |
|---|---|---|
| `feat` | 新功能 / 新特性 / 阶段交付 | `feat: Phase 5 高级特性（动画/Theme/Async/Component）` |
| `fix` | 修复缺陷 | `fix(example): phase5 主题 chip 点击不生效 —— State 须 Bind 才触发 re-render` |
| `docs` | 文档、注释、验收记录 | `docs: 记录 win32 后端按钮/子控件颜色不渲染限制（design §14 + Phase 5 验收）` |
| `test` | 测试新增 / 修正 | — |
| `refactor` | 重构（不改行为） | — |
| `perf` | 性能优化 | — |
| `build` | 构建脚本 / 依赖改动 | — |
| `ci` | CI 工作流改动 | — |
| `chore` | 杂项（格式化、脚手架、仓库配置） | — |
| `revert` | 回滚 | — |

### 4.2 scope（可选）

- `phaseN`：阶段主推进（如 `phase0` `phase1` `phase4`）。
- `example`：示例改动。
- 子系统名：`theme` / `animation` / `state` / `layout` / `list` / `inspector` / `plugin` / `event` / `diff` / `native` / `render` / `widget` / `docs` / `ci` / `scripts`。

### 4.3 subject 规则

- **中文**，概括为先；一行放不下用全角括号补充分组。
- 并列用「、」；因果 / 转折用「——」。
- 引用设计文档用 `design §N.M` / `development-plan §N`。
- 阶段收尾：`docs: 标记 Phase N 完成，更新 README 与开发计划`。

合法示例：

```
feat(phase4): 事件系统与生命周期（统一事件/鼠标键盘映射/生命周期钩子/中文 IME）
docs(theme): 记录 win32 后端按钮/子控件背景色不渲染的实测限制
feat(phase1): LCL 适配层 + examples/basic 声明式改造（1.3/1.5 绑定落地）
```

### 4.4 正文（可选）

- 需要时用正文解释**为什么**；每行 ≤ 72 字符。
- 破坏性变更：正文注明 `BREAKING CHANGE:` 并说明迁移方式。

### 4.5 反例

- `update code`、`fix bug` —— 不具体，看不出改了什么。
- 非中文 subject —— 本项目统一中文提交。
- 多个无关变更塞进一个提交 —— 应拆分（如代码 + 文档可同提交，格式化与功能不可混）。

---

## 5. 分支命名

`<type>/<kebab-case 描述>`：

```
feat/animation-controller
fix/example-phase5-chip
docs/naming-conventions
chore/ci-windows-smoke
```

- `type` 与 §4.1 词汇一致；描述用英文 kebab-case、语义化。
- 不跨 subsystem 的大混合分支（避免无法归入单一 type）。

---

## 6. PR 规范

- **标题** = 一条提交信息（`type(scope): subject`）。
- **正文**：动机（为什么）、影响面（改了哪些包 / 是否动 API）、测试（新增了哪些无头测试、是否手动冒烟）。
- 关联 issue：`Closes #123`。
- 触发 CI：`go test ./...` + `go vet ./...` + §2.1 表中全部公开示例的独立构建、冒烟
  与截图检查。
- CI 还会在 Go 1.22–1.26 与当前 1.27rc3 上跑无头测试，在 1.27rc3 跑 `-race`，并在已校验 DLL
  到位后执行 native probe；全部 16 个公开示例必须产出
  经像素校验的非空截图 artifact。
- 触及 API 或行为：必须同步更新 `design.md` / `README.md`。

---

## 7. 提交前检查清单

- [ ] `gofmt` 通过，`go vet ./...` 无警告
- [ ] `go test ./...` 全绿；并发相关已用 `-race`
- [ ] 导出 API 有中文 doc comment（新 API 必写）
- [ ] 行为变化已同步 `design.md` / `README.md` / 开发计划状态表
- [ ] 新增示例符合 [命名规范 §2](docs/naming-conventions.md)（目录 / winres / smoke 约束）
- [ ] 未提交生成物（`bin/`、`*.syso`、`*.dll`、`ref/` 均已 gitignore）
- [ ] 提交信息符合 §4（type / scope / 中文 subject）

---

## 8. 规范文档索引

| 文档 | 内容 |
|---|---|
| **CONTRIBUTING.md**（本文） | 工作流、提交信息、分支 / PR 规范 |
| [docs/development-guide.md](docs/development-guide.md) | 开发规范：代码风格、架构不变量 D1–D7、测试、文档 |
| [docs/naming-conventions.md](docs/naming-conventions.md) | 命名规范：example / 包文件 / 标识符 / 资源 / 提交词汇 |
| [docs/design.md](docs/design.md) | 架构与设计（三棵树、布局、State、事件、主题…） |
| [docs/development-plan.md](docs/development-plan.md) | Phase 0–7 计划、架构基线决策 D1–D7 与验收标准 |
| [docs/7guis.md](docs/7guis.md) | 七个 7GUIs 任务与公开 API 映射 |
| [docs/api-v0.1.0.md](docs/api-v0.1.0.md) | v0.1.0 候选 API 冻结清单 |
| [docs/capability-comparison.md](docs/capability-comparison.md) | 八项可验证能力、仓库证据与边界 |
| [docs/migration.md](docs/migration.md) | 预 1.0 迁移规则与批次 3 注意事项 |
| [docs/maintenance.md](docs/maintenance.md) | 维护范围、兼容和安全响应政策 |
| [docs/performance-baseline.md](docs/performance-baseline.md) | 发布性能样本、复跑命令与趋势比较规则 |
| [RELEASE_CHECKLIST.md](RELEASE_CHECKLIST.md) | v0.1.0 发布前检查项与未关闭门禁 |
