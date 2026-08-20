# CD1 Draw Core 实施记录

状态：完成（无头范围）
日期：2026-08-20

CD1 把冻结在 CD0 的 Draw API 落成纯值协议，不依赖 libenergy/LCL DLL，也不声称已经
完成真实像素绘制。native executor、DPI 适配和 `DrawSurface` 的真实落点保留给 CD4。

## 已交付

- 根包导出 `DrawList`、封闭 `DrawOp`、Fill/Stroke、字体与文本枚举和值类型，以及基础
  Clear/Rect/RoundRect/Line/Ellipse/Text/Clip 构造器。
- `NewDrawList`/`MustDrawList` 完成防御性复制、规范化和值比较；零值列表表示空绘制。
- 校验覆盖 CD0 上限：操作数、文本字节数、clip 深度/平衡、坐标和尺寸、圆角、描边、
  alpha、字体大小/名称和未知枚举；错误通过 `ErrInvalidDrawList` 与
  `DrawValidationError{ID, OpIndex, Field}` 判定。
- `internal/render.DrawController` 和 Mock 分离记录 `SetDrawList`、`ResetDrawList`、
  `InvalidateDraw`；diff 对 `DrawList` 做 mount、patch、remove 和同值 D7c 处理。
- 内部 adapter 提供 legacy PaintBox 到 CD1 的确定性转换；旧
  `PaintBox`/`PaintCommand` 公开路径保持不变。
- diff 已为后续 CD4 的 `DrawList` 属性接好 headless mount/patch/remove/D7c 协议；公开
  `DrawSurface` 构造器仍遵守阶段门，不在 CD1 提前发布。

## 边界与后续

CD1 只验证值语义，不执行 Canvas/HDC，不包含 Image、Path、Transform、Gradient 或部分
alpha。`DrawSurface` 的 LCL executor、文本测量、DPI、状态保存/恢复和像素探针属于 CD4。

验证命令：

```text
gofmt -w draw.go draw_test.go benchmark_test.go internal/render/draw*.go
go vet ./...
go test ./...
```
