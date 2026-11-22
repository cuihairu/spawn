package resilience

import (
	"sync"
	"time"
)

// State 熔断器状态。
type State int

const (
	// StateClosed 关闭：请求正常放行。
	StateClosed State = iota
	// StateHalfOpen 半开：冷却结束后放行单个探测请求。
	StateHalfOpen
	// StateOpen 开启：请求快速失败，不再发起真实调用。
	StateOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateHalfOpen:
		return "half_open"
	case StateOpen:
		return "open"
	default:
		return "unknown"
	}
}

// MetricValue 状态的指标语义值：0=closed 1=half-open 2=open（gauge 口径）。
func (s State) MetricValue() float64 {
	switch s {
	case StateOpen:
		return 2
	case StateHalfOpen:
		return 1
	default:
		return 0
	}
}

// Breaker 基于连续失败次数的熔断器。
// 连续失败达到 maxFailures 后进入 open；冷却 cooldown 后转 half-open，
// half-open 仅放行一个探测请求——成功则恢复 closed，失败则重新 open。
// 使用连续失败计数而非带时间窗口的失败率，等价于「窗口内失败率 100%」
// 的确定性近似，便于精确测试与故障复现。
type Breaker struct {
	mu sync.Mutex
	// now 可注入时钟，便于测试推进冷却时间。
	now func() time.Time

	maxFailures int
	cooldown    time.Duration

	state     State
	failures  int
	openSince time.Time
	probing   bool // half-open 下是否已有探测请求在途

	// onTransition 状态迁移回调（在锁外调用），用于导出指标等副作用。
	onTransition func(from, to State)
}

// DefaultMaxFailures / DefaultCooldown 默认熔断参数（与 content-service 原型一致）。
const (
	DefaultMaxFailures = 3
	DefaultCooldown    = 5 * time.Second
)

// NewBreaker 创建熔断器，maxFailures < 1 时按 1 处理。
func NewBreaker(maxFailures int, cooldown time.Duration) *Breaker {
	if maxFailures < 1 {
		maxFailures = 1
	}
	if cooldown < 0 {
		cooldown = 0
	}
	return &Breaker{
		now:         time.Now,
		maxFailures: maxFailures,
		cooldown:    cooldown,
		state:       StateClosed,
	}
}

// SetOnTransition 设置状态迁移回调（用于指标导出等副作用）。
func (b *Breaker) SetOnTransition(cb func(from, to State)) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.onTransition = cb
}

// SetNow 注入时钟（测试推进冷却时间用）。
func (b *Breaker) SetNow(fn func() time.Time) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.now = fn
}

// State 返回当前状态（观测与测试断言用）。
func (b *Breaker) State() State {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}

// Allow 判断是否允许发起请求。
// open 且未过冷却期时拒绝；冷却结束后转入 half-open 并放行探测请求；
// half-open 下已有探测在途时拒绝其余请求。
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	prevState := b.state
	var transition State
	transitioned := false
	allowed := false

	switch b.state {
	case StateClosed:
		allowed = true
	case StateOpen:
		if b.now().Before(b.openSince.Add(b.cooldown)) {
			b.mu.Unlock()
			return false
		}
		// 冷却结束：进入半开并放行一个探测请求。
		b.state = StateHalfOpen
		b.probing = true
		allowed = true
		transition, transitioned = StateHalfOpen, true
	default: // StateHalfOpen
		if !b.probing {
			b.probing = true
			allowed = true
		}
	}
	cb := b.onTransition
	b.mu.Unlock()

	if transitioned && cb != nil {
		cb(prevState, transition)
	}
	return allowed
}

// Success 记录一次成功：closed 下复位连续失败计数；half-open 下恢复为 closed。
func (b *Breaker) Success() {
	b.mu.Lock()
	prevState := b.state
	b.probing = false
	b.failures = 0
	var transition State
	transitioned := false
	if b.state == StateHalfOpen {
		b.state = StateClosed
		transition, transitioned = StateClosed, true
	}
	cb := b.onTransition
	b.mu.Unlock()

	if transitioned && cb != nil {
		cb(prevState, transition)
	}
}

// Failure 记录一次失败：closed 下累计连续失败，达到阈值转 open；
// half-open 下探测失败直接转回 open。
func (b *Breaker) Failure() {
	b.mu.Lock()
	prevState := b.state
	b.probing = false
	var transition State
	transitioned := false
	switch b.state {
	case StateHalfOpen:
		b.state = StateOpen
		b.openSince = b.now()
		transition, transitioned = StateOpen, true
	case StateClosed:
		b.failures++
		if b.failures >= b.maxFailures {
			b.state = StateOpen
			b.openSince = b.now()
			transition, transitioned = StateOpen, true
		}
	}
	cb := b.onTransition
	b.mu.Unlock()

	if transitioned && cb != nil {
		cb(prevState, transition)
	}
}
