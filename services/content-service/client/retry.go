package client

import (
	"net/http"
	"time"
)

// errorKind 调用失败分类，决定是否重试与是否计入熔断。
type errorKind int

const (
	// kindTransient 瞬时错误：网络错误、5xx、429、业务错误码 —— 可重试且计入熔断。
	kindTransient errorKind = iota
	// kindBadRequest 客户端侧错误（4xx，429 除外）—— 不重试、不计入熔断（服务本身可达）。
	kindBadRequest
	// kindCanceled 调用方上下文取消/超时 —— 不重试、不计入熔断。
	kindCanceled
	// kindContract 响应契约异常（解析失败、无法识别的响应体）—— 不重试、计入熔断。
	kindContract
)

// callError 携带错误分类的调用失败。
type callError struct {
	kind errorKind
	msg  string
}

func (e *callError) Error() string { return e.msg }

// statusKind 按 HTTP 状态码归类错误。
// 5xx 与 429 视为服务瞬时故障（可重试）；其余 4xx 视为请求侧错误。
func statusKind(status int) errorKind {
	if status == http.StatusTooManyRequests || status >= http.StatusInternalServerError {
		return kindTransient
	}
	return kindBadRequest
}

// retryConfig 重试参数。
type retryConfig struct {
	maxAttempts int // 总尝试次数（含首次）
	baseDelay   time.Duration
	maxDelay    time.Duration // <=0 表示不设上限
}

// backoffDelay 返回第 failedAttempts 次失败后的退避时长：base * 2^(n-1)，封顶 maxDelay。
// 确定性指数退避（无抖动），保证可精确测试。
func (r retryConfig) backoffDelay(failedAttempts int) time.Duration {
	if r.baseDelay <= 0 || failedAttempts < 1 {
		return 0
	}

	d := r.baseDelay
	for i := 1; i < failedAttempts; i++ {
		d *= 2
		if r.maxDelay > 0 && d >= r.maxDelay {
			return r.maxDelay
		}
	}
	if r.maxDelay > 0 && d > r.maxDelay {
		return r.maxDelay
	}
	return d
}

// isRetryable 判断错误是否值得重试。
func isRetryable(err error) bool {
	callErr, ok := err.(*callError)
	return ok && callErr.kind == kindTransient
}

// reportBreaker 按错误分类上报熔断器：
// 服务自身故障计入失败；4xx 与上下文取消属于请求侧问题，不影响服务健康，
// 上报成功以释放 half-open 的探测占用（避免探测被请求侧错误卡死）。
func reportBreaker(b *Breaker, err error) {
	if callErr, ok := err.(*callError); ok {
		if callErr.kind == kindBadRequest || callErr.kind == kindCanceled {
			b.Success()
			return
		}
	}
	b.Failure()
}

// clientOptions 客户端可注入项（测试用）。
type clientOptions struct {
	retry   retryConfig
	breaker *Breaker
	sleep   func(time.Duration)
}

// defaultClientOptions 默认熔断与重试参数。
func defaultClientOptions() clientOptions {
	return clientOptions{
		retry: retryConfig{
			maxAttempts: defaultMaxAttempts,
			baseDelay:   defaultBaseDelay,
			maxDelay:    defaultMaxDelay,
		},
		breaker: NewBreaker(defaultMaxFailures, defaultCooldown),
		sleep:   nil, // 由构造函数回退为 time.Sleep
	}
}
