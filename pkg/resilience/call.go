package resilience

import (
	"context"
	"errors"
	"time"
)

// ErrBreakerOpen 熔断器处于开启状态时快速失败返回的错误。
var ErrBreakerOpen = errors.New("circuit breaker open: fail fast")

// CallOptions 调用循环可注入项（测试与消费方指标接线用）。
type CallOptions struct {
	// Retry 重试参数；零值按 1 次尝试处理。
	Retry RetryConfig
	// Breaker 熔断器；nil = 不熔断（全部放行）。
	Breaker *Breaker
	// Sleep 退避休眠；nil = time.Sleep。
	Sleep func(time.Duration)
	// OnResult 调用终态回调（success / breaker_open / 各类错误分类），
	// 消费方接 Prometheus 计数等。
	OnResult func(Result)
	// OnRetry 发起重试前的回调。
	OnRetry func()
}

// Call 统一跨服务调用循环：熔断检查 → 单次调用 → 按错误分类决定
// 重试/上报熔断 → 指数退避。原型为 content-service GetGameById 的调用环。
//
// once 发起单次真实调用，错误须为 *KindError（NewError/StatusKind 构造）
// 才能参与重试与熔断分类；非 KindError 按未知错误处理（不重试、计入熔断）。
func Call[T any](ctx context.Context, opts CallOptions, once func(context.Context) (T, error)) (T, error) {
	opts.Retry = opts.Retry.Normalize()
	if opts.Sleep == nil {
		opts.Sleep = time.Sleep
	}
	record := func(r Result) {
		if opts.OnResult != nil {
			opts.OnResult(r)
		}
	}

	// 调用方上下文已不可用：直接失败，不触发重试或熔断。
	if err := ctx.Err(); err != nil {
		record(ResultCanceled)
		var zero T
		return zero, err
	}

	var lastErr error
	for attempt := 1; attempt <= opts.Retry.MaxAttempts; attempt++ {
		// 熔断检查：open 时快速失败；half-open 时仅放行探测请求。
		if opts.Breaker != nil && !opts.Breaker.Allow() {
			record(ResultBreakerOpen)
			var zero T
			return zero, ErrBreakerOpen
		}

		value, err := once(ctx)
		if err == nil {
			if opts.Breaker != nil {
				opts.Breaker.Success()
			}
			record(ResultSuccess)
			return value, nil
		}

		lastErr = err
		if opts.Breaker != nil {
			ReportBreaker(opts.Breaker, err)
		}

		if attempt == opts.Retry.MaxAttempts || !IsRetryable(err) {
			record(ResultOf(err))
			var zero T
			return zero, lastErr
		}
		if opts.OnRetry != nil {
			opts.OnRetry()
		}

		// 退避期间上下文失效则放弃后续重试。
		if delay := opts.Retry.BackoffDelay(attempt); delay > 0 {
			opts.Sleep(delay)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			record(ResultCanceled)
			var zero T
			return zero, ctxErr
		}
	}
	// 循环内所有分支均已 return，此处仅为编译完整性兜底。
	var zero T
	return zero, lastErr
}
