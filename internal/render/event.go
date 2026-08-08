package render

import "fmt"

// 统一事件负载（design.md §10，开发计划 Phase 4.1/4.2）。
//
// 放在 render 包（而非 flux）是因为事件由绑定层构造（native 从 LCL 回调里
// 组装 X/Y/Key/Button/Mods，D6 隔离下绑定层只能依赖本包），flux 对用户侧
// re-export（type Event = render.Event）。字段约定：
//
//   - X/Y 为 DIP 坐标，相对事件源控件客户区。native 在边界把 LCL 回调的物理
//     像素经 PXToDIP 归一（4.2）。
//   - Key 为 Win32 虚拟键码（VK_*，OnKeyDown/OnKeyUp）。
//   - Text 为 UTF-8 字符/组合串（OnKeyPress，经 OnUTF8KeyPress 提供，含 IME
//     组合结果 —— 4.4 中文输入路径）。
//   - Source 为事件源稳定标识 "Type#Key"（D3 稳定身份）。与 design.md 的
//     Source Widget 偏差：Widget 每次 render 重建、不可持久持有，改用
//     Type#Key 字符串由 diff 引擎注入，供共享 handler 区分来源。

// EventType 统一事件类型。
type EventType int

const (
	EventClick EventType = iota
	EventMouseDown
	EventMouseUp
	EventMouseMove
	EventMouseEnter
	EventMouseLeave
	EventKeyDown
	EventKeyUp
	EventKeyPress // UTF-8 字符输入（含 IME 组合结果），Text 携带字符
)

// String 返回事件类型名（demo/inspector 展示用）。
func (t EventType) String() string {
	switch t {
	case EventClick:
		return "click"
	case EventMouseDown:
		return "mousedown"
	case EventMouseUp:
		return "mouseup"
	case EventMouseMove:
		return "mousemove"
	case EventMouseEnter:
		return "mouseenter"
	case EventMouseLeave:
		return "mouseleave"
	case EventKeyDown:
		return "keydown"
	case EventKeyUp:
		return "keyup"
	case EventKeyPress:
		return "keypress"
	default:
		return fmt.Sprintf("EventType(%d)", int(t))
	}
}

// MouseButton 鼠标按键。
type MouseButton uint8

const (
	ButtonNone MouseButton = iota
	ButtonLeft
	ButtonRight
	ButtonMiddle
)

// Modifier 修饰键掩码（按位组合）。
type Modifier uint8

const (
	ModShift Modifier = 1 << iota
	ModCtrl
	ModAlt
	ModWin
)

// Event 是一次统一事件的负载。
type Event struct {
	Type   EventType
	X, Y   int // DIP，相对事件源控件客户区（鼠标事件）
	Key    uint16
	Text   string
	Button MouseButton
	Mods   Modifier
	Source string // "Type#Key"，diff 引擎注入
}
