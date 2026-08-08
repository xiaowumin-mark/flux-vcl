package flux

import "github.com/xiaowumin-mark/flux-vcl/internal/render"

// 统一事件 API（design.md §10，Phase 4.1/4.2）。
//
// Event 及事件类型/修饰键/鼠标按键定义在 internal/render（绑定层构造事件
// 的依赖面，D6 隔离），此处 re-export 给用户。坐标 X/Y 为 DIP，相对事件源
// 控件客户区；Source 为 "Type#Key"（稳定身份，D3），由 diff 引擎注入。
//
// 事件回调每次 render 重新绑定（函数值无法比较相等性，D2 逃逸口行为）。

type Event = render.Event

type EventType = render.EventType

type MouseButton = render.MouseButton

type Modifier = render.Modifier

// 事件类型常量。
const (
	EventClick      = render.EventClick
	EventMouseDown  = render.EventMouseDown
	EventMouseUp    = render.EventMouseUp
	EventMouseMove  = render.EventMouseMove
	EventMouseEnter = render.EventMouseEnter
	EventMouseLeave = render.EventMouseLeave
	EventKeyDown    = render.EventKeyDown
	EventKeyUp      = render.EventKeyUp
	EventKeyPress   = render.EventKeyPress
)

// 鼠标按键常量。
const (
	ButtonNone   = render.ButtonNone
	ButtonLeft   = render.ButtonLeft
	ButtonRight  = render.ButtonRight
	ButtonMiddle = render.ButtonMiddle
)

// 修饰键常量（按位组合）。
const (
	ModShift = render.ModShift
	ModCtrl  = render.ModCtrl
	ModAlt   = render.ModAlt
	ModWin   = render.ModWin
)
