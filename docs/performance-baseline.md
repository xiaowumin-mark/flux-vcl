# 性能基线

本文记录发布前可复跑的无头性能样本，用于比较趋势和定位回归。CI 只做基准编译与
单次 smoke，不设置 hosted runner 上容易波动的绝对耗时阈值；正式发布时在固定环境
重跑并追加记录。

## P7.3a 基线（2026-08-18）

环境：Windows/amd64，Go 1.25.1，Intel Core i5-1135G7 @ 2.40GHz，`GOMAXPROCS=8`。

命令：

```powershell
go test . -run '^$' `
  -bench 'Benchmark(ControlMount|ControlPurePropertyPatch|PageSwitch|VirtualListScrollPatch)$' `
  -benchmem -benchtime=500ms -count=3
```

下表采用三次结果的中位值；`mutations/op` 是确定性的 Renderer 调用计数，耗时仅作为
同环境趋势样本。

| 基准 | ns/op | mutations/op | B/op | allocs/op |
|---|---:|---:|---:|---:|
| `BenchmarkControlMount` | 147,897 | 153 | 257,484 | 1,506 |
| `BenchmarkControlPurePropertyPatch` | 9,859 | 3 | 9,499 | 103 |
| `BenchmarkPageSwitch` | 11,552 | 1 | 10,860 | 124 |
| `BenchmarkVirtualListScrollPatch` | 236,737 | 52 | 261,062 | 2,355 |

`ControlMount` 覆盖当前发布的 native 控件全集；纯属性 patch 与 Page 切换保持原
identity；虚拟列表样本在十万行中段移动现有控件池。Grid 更新与 Paint invalidate
依赖 7.5 的 `StringGrid`、`Canvas/PaintBox`，在 7.3b 发布门加入本表。
