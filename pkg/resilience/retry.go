package resilience

import (
	"time"
)

// DefaultRetryConfig 默认重试参数（与 content-service 原型一致）。
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxAttempts: DefaultMaxAttempts,
		BaseDelay:   DefaultBaseDelay,
		MaxDelay:    DefaultMaxDelay,
	}
}

// 默认重试参数。
const (
	DefaultMaxAttempts = 3                      // 单次调用最多尝试次数（含首次）
	DefaultBaseDelay   = 100 * time.Millisecond // 重试基础退避
	DefaultMaxDelay    = time.Second            // 重试退避上限
)

// RetryConfig 重试参数。
type RetryConfig struct {
	MaxAttempts int // 总尝试次数（含首次）
	BaseDelay   time.Duration
	MaxDelay    time.Duration // <=0 表示不设上限
}

// Normalize 收敛非法参数：尝试次数 <1 按 1，负延迟清零。
func (r RetryConfig) Normalize() RetryConfig {
	if r.MaxAttempts < 1 {
		r.MaxAttempts = 1
	}
	if r.BaseDelay < 0 {
		r.BaseDelay = 0
	}
	if r.MaxDelay < 0 {
		r.MaxDelay = 0
	}
	return r
}

// BackoffDelay 返回第 failedAttempts 次失败后的退避时长：base * 2^(n-1)，封顶 maxDelay。
// 确定性指数退避（无抖动），保证可精确测试。
func (r RetryConfig) BackoffDelay(failedAttempts int) time.Duration {
	if r.BaseDelay <= 0 || failedAttempts < 1 {
		return 0
	}

	d := r.BaseDelay
	for i := 1; i < failedAttempts; i++ {
		d *= 2
		if r.MaxDelay > 0 && d >= r.MaxDelay {
			return r.MaxDelay
		}
	}
	if r.MaxDelay > 0 && d > r.MaxDelay {
		return r.MaxDelay
	}
	return d
}

// IsRetryable 判断错误是否值得重试（仅瞬时错误可重试）。
func IsRetryable(err error) bool {
	return KindOf(err) == KindTransient
}

// ReportBreaker 按错误分类上报熔断器：
// 服务自身故障（瞬时错误与契约异常）计入失败；4xx 与上下文取消属于请求侧
// 问题，不影响服务健康，上报成功以释放 half-open 的探测占用
// （避免探测被请求侧错误卡死）。
func ReportBreaker(b *Breaker, err error) {
	switch KindOf(err) {
	case KindBadRequest, KindCanceled:
		b.Success()
	default:
		b.Failure()
	}
}
