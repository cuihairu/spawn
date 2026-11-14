package client

// CLN-004：错误分类/退避/重试判定公共核上移 pkg/resilience，
// 此处保留历史内部名别名与薄包装，客户端行为与历史版本一致。

import (
	"time"

	"github.com/tappi/tappi/pkg/resilience"
)

// errorKind 调用失败分类，决定是否重试与是否计入熔断。
type errorKind = resilience.Kind

const (
	// kindTransient 瞬时错误：网络错误、5xx、429、业务错误码 —— 可重试且计入熔断。
	kindTransient = resilience.KindTransient
	// kindBadRequest 客户端侧错误（4xx，429 除外）—— 不重试、不计入熔断（服务本身可达）。
	kindBadRequest = resilience.KindBadRequest
	// kindCanceled 调用方上下文取消/超时 —— 不重试、不计入熔断。
	kindCanceled = resilience.KindCanceled
	// kindContract 响应契约异常（解析失败、无法识别的响应体）—— 不重试、计入熔断。
	kindContract = resilience.KindContract
)

// callError 携带错误分类的调用失败（pkg/resilience.NewError 构造）。
type callError = resilience.KindError

// statusKind 按 HTTP 状态码归类错误。
// 5xx 与 429 视为服务瞬时故障（可重试）；其余 4xx 视为请求侧错误。
func statusKind(status int) errorKind {
	return resilience.StatusKind(status)
}

// retryConfig 重试参数。
type retryConfig = resilience.RetryConfig

// backoffDelay 返回第 failedAttempts 次失败后的退避时长（确定性指数退避）。
func backoffDelay(r retryConfig, failedAttempts int) time.Duration {
	return r.BackoffDelay(failedAttempts)
}

// isRetryable 判断错误是否值得重试。
func isRetryable(err error) bool {
	return resilience.IsRetryable(err)
}

// reportBreaker 按错误分类上报熔断器：
// 服务自身故障计入失败；4xx 与上下文取消属于请求侧问题，不影响服务健康，
// 上报成功以释放 half-open 的探测占用（避免探测被请求侧错误卡死）。
func reportBreaker(b *Breaker, err error) {
	resilience.ReportBreaker(b, err)
}

// clientOptions 客户端可注入项（测试用）。
type clientOptions struct {
	retry   retryConfig
	breaker *Breaker
	sleep   func(time.Duration)
	metrics *clientMetrics // nil 时使用注册到默认 registry 的 defaultMetrics
}

// defaultClientOptions 默认熔断与重试参数。
func defaultClientOptions() clientOptions {
	return clientOptions{
		retry: retryConfig{
			MaxAttempts: defaultMaxAttempts,
			BaseDelay:   defaultBaseDelay,
			MaxDelay:    defaultMaxDelay,
		},
		breaker: NewBreaker(defaultMaxFailures, defaultCooldown),
		sleep:   nil, // 由构造函数回退为 time.Sleep
	}
}
