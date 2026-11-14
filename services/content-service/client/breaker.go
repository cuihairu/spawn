package client

// CLN-004：熔断器公共核上移 pkg/resilience（与重试/错误分类一并统一），
// 此处保留类型别名与指标接线，客户端行为与历史版本一致。

import (
	"time"

	"github.com/tappi/tappi/pkg/resilience"
)

// Breaker 基于连续失败次数的熔断器（pkg/resilience 公共实现）。
type Breaker = resilience.Breaker

// breakerState 熔断器状态（历史内部名，别名保留减少扩散）。
type breakerState = resilience.State

const (
	// stateClosed 关闭：请求正常放行。
	stateClosed = resilience.StateClosed
	// stateOpen 开启：请求快速失败，不再发起真实调用。
	stateOpen = resilience.StateOpen
	// stateHalfOpen 半开：冷却结束后放行单个探测请求。
	stateHalfOpen = resilience.StateHalfOpen
)

// NewBreaker 创建熔断器，maxFailures < 1 时按 1 处理。
func NewBreaker(maxFailures int, cooldown time.Duration) *Breaker {
	return resilience.NewBreaker(maxFailures, cooldown)
}

// breakerStateValue 状态的 gauge 语义值：0=closed 1=half-open 2=open。
func breakerStateValue(s breakerState) float64 {
	return s.MetricValue()
}
