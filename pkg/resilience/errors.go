// Package resilience 统一跨服务客户端韧性抽象（CLN-004）：
// 熔断器、错误分类、指数退避重试与调用循环。
// 原型为 content-service 对 game-catalog 的客户端
// （services/content-service/client，重试/熔断/指标三件套），
// 抽出与具体服务解耦的公共核，供各服务的跨服务客户端共享。
//
// 口径与原型一致：
//   - 熔断用连续失败计数（等价「窗口内失败率 100%」的确定性近似），便于精确测试；
//   - 退避无抖动（确定性指数退避），便于精确测试；
//   - 错误按 Kind 分类：瞬时错误可重试且计入熔断，4xx（429 除外）与
//     上下文取消不重试不计入熔断，契约异常不重试但计入熔断。
package resilience

import (
	"net/http"
)

// Kind 调用失败分类，决定是否重试与是否计入熔断。
type Kind int

const (
	// KindTransient 瞬时错误：网络错误、5xx、429、业务错误码 —— 可重试且计入熔断。
	KindTransient Kind = iota
	// KindBadRequest 客户端侧错误（4xx，429 除外）—— 不重试、不计入熔断（服务本身可达）。
	KindBadRequest
	// KindCanceled 调用方上下文取消/超时 —— 不重试、不计入熔断。
	KindCanceled
	// KindContract 响应契约异常（解析失败、无法识别的响应体）—— 不重试、计入熔断。
	KindContract
)

// KindUnknown KindOf 对非 KindError 的归类结果。
const KindUnknown Kind = -1

func (k Kind) String() string {
	switch k {
	case KindTransient:
		return "transient"
	case KindBadRequest:
		return "bad_request"
	case KindCanceled:
		return "canceled"
	case KindContract:
		return "contract"
	default:
		return "unknown"
	}
}

// KindError 携带错误分类的调用失败。
type KindError struct {
	Kind  Kind
	msg   string
	cause error
}

func (e *KindError) Error() string { return e.msg }

// Unwrap 透出包装的底层错误（errors.Is/As 沿链判定）。
func (e *KindError) Unwrap() error { return e.cause }

// NewError 构造带分类的调用失败。
func NewError(kind Kind, msg string) *KindError {
	return &KindError{Kind: kind, msg: msg}
}

// WrapError 构造带分类与底层原因的调用失败（保留 errors.Is/As 链）。
func WrapError(kind Kind, cause error, msg string) *KindError {
	return &KindError{Kind: kind, msg: msg, cause: cause}
}

// KindOf 提取错误的分类；非 KindError 返回 KindUnknown。
func KindOf(err error) Kind {
	if callErr, ok := err.(*KindError); ok {
		return callErr.Kind
	}
	return KindUnknown
}

// StatusKind 按 HTTP 状态码归类错误。
// 5xx 与 429 视为服务瞬时故障（可重试）；其余 4xx 视为请求侧错误。
func StatusKind(status int) Kind {
	if status == http.StatusTooManyRequests || status >= http.StatusInternalServerError {
		return KindTransient
	}
	return KindBadRequest
}

// Result 调用最终结果分类（供消费方接指标）。
type Result string

// 结果分类标签。
const (
	ResultSuccess     Result = "success"
	ResultBreakerOpen Result = "breaker_open"
	ResultTransient   Result = "transient_error"
	ResultBadRequest  Result = "bad_request"
	ResultCanceled    Result = "canceled"
	ResultContract    Result = "contract_error"
	ResultUnknown     Result = "unknown_error"
)

// ResultOf 将调用错误映射为最终结果分类。
func ResultOf(err error) Result {
	switch KindOf(err) {
	case KindTransient:
		return ResultTransient
	case KindBadRequest:
		return ResultBadRequest
	case KindCanceled:
		return ResultCanceled
	case KindContract:
		return ResultContract
	default:
		return ResultUnknown
	}
}
