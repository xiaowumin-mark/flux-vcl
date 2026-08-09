package render

import (
	"fmt"
	"os"
	"runtime/debug"
)

// Guard 是 D4 统一错误边界：包裹用户回调（事件/生命周期/定时器/滚动）。
// 回调 panic 不崩进程、只记 stderr（development-plan §4"分发层统一 recover 路由
// 到错误事件"的落地：本框架尚无全局错误事件通道，路由目标退化为 stderr 日志 +
// 堆栈，可读性优先）。diff 层（reconcile 触发生命周期/事件包装）与 native 层
// （原生事件入口、NewTimer、RunOnUI）共用 —— mock 直接触发回调的测试路径同样
// 受保护（回调在 diff 层已包装）。
func Guard(what string, fn func()) {
	defer func() {
		if rec := recover(); rec != nil {
			fmt.Fprintf(os.Stderr, "flux/render: 回调 %s panic（D4 错误边界捕获）: %v\n%s\n", what, rec, debug.Stack())
		}
	}()
	fn()
}
