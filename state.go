package flux

import (
	"fmt"
	"strconv"
	"sync"
)

// State 是响应式状态原语（design.md §8.1，Phase 2.1）。
//
//	count := flux.NewState(0)
//	flux.Text(flux.Bind(count))
//	count.Set(1) // → 通知订阅它的 App 重新渲染（diff 引擎只 patch 变化的属性）
//
// 线程安全：Set/Get 可跨 goroutine（D4）。Set 触发的 re-render 经
// renderer.RunOnUI marshal 到 UI 线程，从任意 goroutine 调用均安全。
type State[T any] struct {
	mu   sync.Mutex
	val  T
	subs map[*App]struct{}
}

// NewState 创建状态，initial 为初值。
func NewState[T any](initial T) *State[T] {
	return &State[T]{val: initial, subs: make(map[*App]struct{})}
}

// Get 返回当前值（线程安全）。
func (s *State[T]) Get() T {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.val
}

// Set 设置新值并通知所有订阅的 App 重新渲染。
//
// 若从非 UI 线程调用，re-render 会阻塞等待 UI 线程执行完成
// （renderer.RunOnUI，D4 marshalling）。
func (s *State[T]) Set(v T) {
	s.mu.Lock()
	s.val = v
	subs := make([]*App, 0, len(s.subs))
	for a := range s.subs {
		subs = append(subs, a)
	}
	s.mu.Unlock()
	for _, a := range subs {
		a.invalidateFor(s)
	}
}

// subscribe 登记 App 订阅（App.collectBindings 在每次 render 时调用，幂等）。
func (s *State[T]) subscribe(a *App) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subs[a] = struct{}{}
}

// unsubscribe removes an App from this State. App replaces its complete
// subscription set after each successful tree build, so bindings removed from
// a conditional branch no longer keep the App alive or schedule re-renders.
func (s *State[T]) unsubscribe(a *App) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, a)
}

// stateSubscription is the stable State identity behind a bindable. Bindings
// are reconstructed on every render, so App tracks this identity rather than
// the transient Binding value.
type stateSubscription interface {
	subscribe(*App)
	unsubscribe(*App)
}

// bindable 是绑定参数（Bind 返回值）：渲染时取当前文本值，输入时回写 State。
// Text/Button 构造器按类型分支处理；Input 走 Opt.apply 设置回写。
type bindable interface {
	renderText() string
	onChange() func(string)
	subscription() stateSubscription
}

// Binding 是 State 的绑定（design.md §9 数据绑定）。
//
//	Text(Bind(count))   // 单向：渲染值随 State 变化
//	Input(Bind(name))   // 双向：输入 → State → 重新渲染
//
// 同时实现 Opt（Input 用）与 bindable（Text/Button 用）。
type Binding[T any] struct {
	state *State[T]
}

// Bind 创建 State 绑定。
func Bind[T any](s *State[T]) *Binding[T] { return &Binding[T]{state: s} }

func (b *Binding[T]) renderText() string { return fmt.Sprint(b.state.Get()) }

// onChange 生成文本回写函数（双向绑定）。对 string/int 提供类型转换，
// 其余类型返回 nil（该 Binding 仅作单向显示）。
func (b *Binding[T]) onChange() func(string) {
	var zero T
	switch any(zero).(type) {
	case string:
		return func(v string) { b.state.Set(any(v).(T)) }
	case int:
		return func(v string) {
			if n, err := strconv.Atoi(v); err == nil {
				b.state.Set(any(n).(T))
			}
		}
	default:
		return nil
	}
}

func (b *Binding[T]) subscription() stateSubscription { return b.state }

// apply 实现 Opt：Input(Bind(s)) 时设置显示文本、回写事件与依赖标记。
func (b *Binding[T]) apply(n *Node) {
	n.Props.Set("Text", b.renderText())
	n.Props.Set(bindKey, b)
	if cb := b.onChange(); cb != nil {
		n.Props.Set("OnChange", cb)
	}
}

// bindKey 是节点 Props 里登记绑定依赖的隐藏键（collectBindings 遍历用）。
// 值相同（同一 Binding 指针），diff 恒跳过，不产生 mutation。
const bindKey = "_bind"
