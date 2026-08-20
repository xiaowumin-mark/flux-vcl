# CD0 原生 Spike 记录

> 状态：完成。已验证路径可作为后续阶段的输入；标为 deferred 的项目不是公开能力承诺。
> 执行日期：2026-08-20
> 关联决策：[`cd0-decisions.md`](./cd0-decisions.md)、[`api-vnext.md`](./api-vnext.md)

## 可复现入口

在 Windows amd64、已获取锁定 DLL 的工作树执行：

```powershell
.\scripts\fetch-libenergy.ps1
.\scripts\cd0-native-probe.ps1 -ArtifactDir .\artifacts\cd0-native
GOARCH=386 go test -count=1 ./internal/native -run '^TestCD0Win32DrawABILayout$'
```

脚本将每个 LCL 探针放进独立 `go test` 进程，并写出 JSON、PNG 和日志到 artifact 目录。CI 的
`native-probe` job 调用同一脚本并上传 `cd0-native-evidence`。本次使用
`github.com/energye/lcl v1.0.3` 和 `libenergy-amd64.dll`，SHA-256 为
`2D13987CB5505D56C24D073F5CE8C1CE981A9BD1BD78D8BDE16C8EDBD8641300`；锁定来源见
[`dependencies.lock.json`](../packaging/dependencies.lock.json)。

`bin/` 和 `artifacts/` 是忽略的运行产物，不能替代本记录；CI artifact 是逐次运行的原始证据。

## 结果摘要

| CD0 项 | 运行证据 | 结果 | 结论 |
|---|---|---|---|
| CD0.1 | `cd0_api_names_test.go`、`cd0_legacy_theme_test.go` | supported | API 名称、值快照和 legacy 映射契约已冻结；vNext 符号仍未实现。 |
| CD0.2 | `TestCD0CanvasRuntimeProbe`、`cd0-canvas-probe.json/png` | supported | LCL bitmap Canvas 的 Rect、RoundRect、Ellipse、Text、clip、stroke、line、字体 fallback 与 96/144/192 DPI 都有像素或数值断言。 |
| CD0.3 | `TestCD0OwnerDrawAndSubclassRuntimeProbe`、ABI tests | deferred | `WM_DRAWITEM` 已真实观察到 normal/focused/pressed；hovered/disabled/default 的 `ODS_*` 位在本 widgetset run 未观察到。CD5 不得只依赖这些位。 |
| CD0.4 | 同 CD0.3 JSON | supported | Window、ScrollBox、TabPage 直接 parent 的路由、手动解绑和 `WM_NCDESTROY` 路由清理都通过。 |
| CD0.5 | `TestCD0ControlDrawRuntimeProbe`、`cd0-control-draw-probe.json` | deferred | 每种协议独立探测，见能力矩阵；不将 Combo/Grid 与 `NM_CUSTOMDRAW` 伪装为统一协议。 |
| CD0.6 | `TestValidatePaintCommands*`、`TestPaintBox*` | supported | PaintBox 对活动颜色字段拒绝 `0x00RRGGBB`（RGB 非零）和 partial alpha，并给出稳定诊断。 |
| CD0.7 | `cd0_legacy_theme_test.go` | supported | `FromLegacyTheme` 的逐字段、按位颜色与快照契约已冻结；生产函数待 CD3.6。 |

## CD0.2 Canvas

运行结果为 `supported`。画布大小为 360 x 280 px，矩形右边界的 half-open 采样、clip 内外采样、
stroke/line 采样和文本墨迹数量均通过。请求缺失字体
`__FluxVCL_CD0_Missing_Font__` 后解析为本机 fallback（本次为 `宋体`），并获得非零文字尺寸。

对 `FluxVCL`、16 DIP 字体，归一化宽度为 56/56/56 DIP（96/144/192 DPI），高度为 21/21/23 DIP；
探针允许宽高各最多 2 DIP 的字体栅格化舍入漂移。本次 PNG SHA-256：

```text
A54E60C38E46D594652B9331BA6D87E209F72FF4B6698C1D048D5275344EE164
```

PNG 已经人工检查，展示了三种填充图元、裁剪区域、描边/线段和 DPI 文字样本。它作为 CI artifact
上传，不作为仓库内稳定的跨机器像素金样，因为字体 fallback 会随 Windows 字体集变化。

## CD0.3 与 CD0.4 Owner-draw

Win32 ABI 断言使用 probe-local mirror structs，避免提前发布未验证 binding：

| 架构 | `DRAWITEMSTRUCT` | `NMHDR` | `NMCUSTOMDRAW` |
|---|---:|---:|---:|
| amd64 | 64 | 24 | 80 |
| 386 | 48 | 12 | 48 |

amd64 真实探针确认 `BS_OWNERDRAW + WM_DRAWITEM` 的 parent 消息路由：

| 直接 parent | `WM_DRAWITEM` | 手动 unbind | callback 移除 | `WM_NCDESTROY` 清理 |
|---|---|---|---|---|
| Window | observed | true | true | 单独 lifecycle pair observed |
| ScrollBox | observed | true | true | 单独 lifecycle pair observed |
| TabPage | observed | true | true | 单独 lifecycle pair observed |

真实状态位记录为 normal、focused、pressed；hovered、disabled、default 没有看到对应 `ODS_HOTLIGHT`、
`ODS_DISABLED`、`ODS_DEFAULT`。后续 Button painter 必须保存 framework visual state 或另行证明映射，
不能把缺失状态当成 native zero state 的可靠含义。

`syscall.NewCallback` 在 Go 1.25/LCL 进程的常规 `os.Exit` 路径上会触发
`exitsyscall: syscall frame is no longer valid`。测试因此通过短生命周期 helper 运行真实回调并由
`ExitProcess` 结束 helper；父测试校验 helper exit code 和 JSON。此 workaround 只保证 Spike 可复现，
**不构成 CD5 生产回调生命周期的批准**；CD5 必须提供正常 Go 进程退出的实现或替换 callback 机制。

## CD0.5 控件能力矩阵

本次锁定 DLL 运行的 callback 计数如下：

| 控件/路径 | 实际协议 | 结果 | 观察 |
|---|---|---|---|
| ComboBox DrawItem | `CsOwnerDrawFixed + OnDrawItem` | supported | 14 个带 live Canvas 的回调。 |
| ComboBox MeasureItem | `CsOwnerDrawFixed + OnMeasureItem` | deferred | 配置 handler 后本次没有收到 callback；CD6 不能据此选择可变高度协议。 |
| StringGrid Prepare | `OnPrepareCanvas` | supported | 8 个带 live Canvas 的回调。 |
| StringGrid Draw | `OnDrawCell` | supported | 8 个带 live Canvas 的回调。 |
| TrackBar | parent `WM_NOTIFY/NM_CUSTOMDRAW` | supported | 7 个回调，包含 prepaint、item-prepaint、postpaint。 |
| Progress | parent `WM_NOTIFY/NM_CUSTOMDRAW` | deferred | 本次没有收到 direct-parent custom draw 回调；必须保留 native rendering 或在 CD6 重新验证。 |

探针脚本将 ComboBox.DrawItem、StringGrid.Prepare、StringGrid.Draw 和 TrackBar 作为运行时必需证据；
MeasureItem 和 Progress 允许 `deferred`，但 JSON 必须存在，且文档中的限制不得省略。

## 非原生决策验证

CD0.6 只改变 PaintBox 和未来 Draw 路径：legacy `Theme`/普通 native 控件仍走兼容的 `TColor` 边界，
不因此改变历史颜色行为。只有在 32-bit 离屏、预乘 alpha、合成顺序和截图验证全部完成后，CD7 才能
放开 partial alpha。

CD0.7 的 reference adapter 特意保留在测试中，避免把未实现的 `DesignTheme`/`FromLegacyTheme`
提早伪装成已发布 API。CD3.6 实现后应以同一表格测试替换 reference adapter。

## 验证边界

本次 CD0 复核命令如下，四个独立 native probe 和 Paint 校验均通过：

```powershell
go test -count=1 -v ./internal/native -run '^TestCD0(CanvasRuntimeProbe|OwnerDrawAndSubclassRuntimeProbe|ControlDrawRuntimeProbe|Win32DrawABILayout)$'
go test -count=1 ./internal/render -run 'TestValidatePaintCommands|TestPaintBox'
git diff HEAD^ --check
```

本次复核的 `go test -count=1 ./...` 也通过。CD0 脚本仍将每个 LCL 探针隔离到独立进程，
以避免 native 初始化和 Go 回调退出生命周期互相污染；这属于探针可复现性约束，不改变生产
后端的生命周期结论。
