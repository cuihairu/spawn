package client

import (
	"sync"
	"time"
)

// breakerState 熔断器状态。
type breakerState int

const (
	// stateClosed 关闭：请求正常放行。
	stateClosed breakerState = iota
	// stateOpen 开启：请求快速失败，不再发起真实调用。
	stateOpen
	// stateHalfOpen 半开：冷却结束后放行单个探测请求。
	stateHalfOpen
)

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

	state     breakerState
	failures  int
	openSince time.Time
	probing   bool // half-open 下是否已有探测请求在途

	// onChange 状态迁移回调（在锁外调用），用于导出指标等副作用。
	onChange func(breakerState)
}

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
		state:       stateClosed,
	}
}

// Allow 判断是否允许发起请求。
// open 且未过冷却期时拒绝；冷却结束后转入 half-open 并放行探测请求；
// half-open 下已有探测在途时拒绝其余请求。
func (b *Breaker) Allow() bool {
	b.mu.Lock()
	var transition breakerState
	transitioned := false
	allowed := false

	switch b.state {
	case stateClosed:
		allowed = true
	case stateOpen:
		if b.now().Before(b.openSince.Add(b.cooldown)) {
			b.mu.Unlock()
			return false
		}
		// 冷却结束：进入半开并放行一个探测请求。
		b.state = stateHalfOpen
		b.probing = true
		allowed = true
		transition, transitioned = stateHalfOpen, true
	default: // stateHalfOpen
		if !b.probing {
			b.probing = true
			allowed = true
		}
	}
	cb := b.onChange
	b.mu.Unlock()

	if transitioned && cb != nil {
		cb(transition)
	}
	return allowed
}

// Success 记录一次成功：closed 下复位连续失败计数；half-open 下恢复为 closed。
func (b *Breaker) Success() {
	b.mu.Lock()
	b.probing = false
	b.failures = 0
	var transition breakerState
	transitioned := false
	if b.state == stateHalfOpen {
		b.state = stateClosed
		transition, transitioned = stateClosed, true
	}
	cb := b.onChange
	b.mu.Unlock()

	if transitioned && cb != nil {
		cb(transition)
	}
}

// Failure 记录一次失败：closed 下累计连续失败，达到阈值转 open；
// half-open 下探测失败直接转回 open。
func (b *Breaker) Failure() {
	b.mu.Lock()
	b.probing = false
	var transition breakerState
	transitioned := false
	switch b.state {
	case stateHalfOpen:
		b.state = stateOpen
		b.openSince = b.now()
		transition, transitioned = stateOpen, true
	case stateClosed:
		b.failures++
		if b.failures >= b.maxFailures {
			b.state = stateOpen
			b.openSince = b.now()
			transition, transitioned = stateOpen, true
		}
	}
	cb := b.onChange
	b.mu.Unlock()

	if transitioned && cb != nil {
		cb(transition)
	}
}

// state 返回当前状态，供测试断言使用。
func (b *Breaker) stateValue() breakerState {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.state
}
