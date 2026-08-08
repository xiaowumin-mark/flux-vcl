package flux

import (
	"math"
	"sync"
	"time"
)

// 动画系统（design.md §13，Phase 5.1）。
//
// 分层：
//  1. 纯逻辑（本文件，无头可测）：Curve（缓动）/ Tween（插值）/
//     AnimationController（0..1 进度状态机，不持有定时器）。
//  2. pump（App.Animate）：主线程 ~16ms 定时器（native TTimer，D4）驱动
//     controller.Step，每帧回调 onStep。
//  3. 落地（App.SetBounds 等逃逸口）：onStep 里直接 SetBounds，不重跑 diff ——
//     "高频属性用直接绑定（D2 逃逸口）避免整树 re-diff"（development-plan §5.1）。

// Curve 把线性进度 t∈[0,1] 映射为动画进度（输出通常也在 [0,1]，可非单调如回弹）。
type Curve func(t float64) float64

// 内置缓动曲线。t 入参假设已 clamp 到 [0,1]。
func LinearCurve(t float64) float64 { return t }

// EaseIn 缓入（慢→快，quadratic）。
func EaseIn(t float64) float64 { return t * t }

// EaseOut 缓出（快→慢，quadratic）。
func EaseOut(t float64) float64 { return 1 - (1-t)*(1-t) }

// EaseInOut 缓入缓出（S 形，quadratic）。
func EaseInOut(t float64) float64 {
	if t < 0.5 {
		return 2 * t * t
	}
	return 1 - 2*(1-t)*(1-t)
}

// ElasticOut 缓出弹性（末端过冲回弹，峰值 >1）。非单调曲线的代表：
// 输出可短暂超过 [0,1]，Tween 插值会随之越界 —— 调用方如约束范围需自行 clamp。
func ElasticOut(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	const c4 = (2 * math.Pi) / 3
	return math.Pow(2, -10*t)*math.Sin((t*10-0.75)*c4) + 1
}

// Tween 在 from..to 间按进度 t∈[0,1] 线性插值（数值类型）。
//
//	flux.Tween(0, 160, 0.5)   // 80（int 类型推断自实参）
//	flux.Tween(0.0, 1.0, 0.5) // 0.5
func Tween[T ~float64 | ~int | ~int32 | ~int64](from, to T, t float64) T {
	return from + T(float64(to-from)*t)
}

// AnimationController 是 0..1 动画进度的状态机（Flutter AnimationController
// 简化版）：维护 elapsed/duration 与 running，Step 每帧推进并回调。
//
// 不持有定时器（纯逻辑可无头测试）：由 App.Animate 用主线程定时器驱动，
// 或测试直接调 Step 手动推进。线程安全（mutex），可在任意 goroutine 推进。
type AnimationController struct {
	mu       sync.Mutex
	duration time.Duration
	elapsed  time.Duration
	curve    Curve
	running  bool
	onStep   func(v float64)
	onEnd    func()
}

// NewAnimationController 创建控制器。curve 为缓动（nil 时用 LinearCurve）。
func NewAnimationController(duration time.Duration, curve Curve) *AnimationController {
	if curve == nil {
		curve = LinearCurve
	}
	return &AnimationController{duration: duration, curve: curve}
}

// Start 启动动画（重置 elapsed）。onStep 每帧收到曲线进度值；onEnd（可选）
// 在到达终点时调用一次。
func (c *AnimationController) Start(onStep func(v float64), onEnd ...func()) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = true
	c.elapsed = 0
	c.onStep = onStep
	c.onEnd = nil
	if len(onEnd) > 0 {
		c.onEnd = onEnd[0]
	}
}

// Stop 提前停止（不再推进；onEnd 不会触发）。
func (c *AnimationController) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.running = false
}

// Running 报告动画是否仍在推进。
func (c *AnimationController) Running() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.running
}

// Step 推进一帧：elapsed += dt → t = clamp(elapsed/duration, 0, 1) → v = curve(t)
// → 回调 onStep(v)（含终点 1.0）。到终点（t>=1）自动停表并触发 onEnd。
// 返回 (v, done)：done 表示本帧到达终点（调用方可停止 pump）。
func (c *AnimationController) Step(dt time.Duration) (v float64, done bool) {
	c.mu.Lock()
	if !c.running {
		c.mu.Unlock()
		return 0, false
	}
	c.elapsed += dt
	t := 1.0
	if c.duration > 0 {
		t = float64(c.elapsed) / float64(c.duration)
		if t > 1 {
			t = 1
		}
	}
	v = c.curve(t)
	if t >= 1 {
		c.running = false
	}
	fn, end := c.onStep, c.onEnd
	c.mu.Unlock() // 回调在锁外执行，避免回调内重入 Step/Stop 自锁

	if fn != nil {
		fn(v)
	}
	if t >= 1 {
		if end != nil {
			end()
		}
		return v, true
	}
	return v, false
}
