package client

import (
	"github.com/prometheus/client_golang/prometheus"
)

// GetGameById 结果分类标签。
const (
	resultSuccess      = "success"
	resultBreakerOpen  = "breaker_open"
	resultTransient    = "transient_error"
	resultBadRequest   = "bad_request"
	resultCanceled     = "canceled"
	resultContractErr  = "contract_error"
	resultUnknownError = "unknown_error"
)

// clientMetrics 跨服务调用指标集合（ENHANCEMENT #3：Prometheus 监控跨服务调用）。
// 注册到 go-zero 默认 registry 后随 /metrics 端点暴露；降级语义不变，仅增加观测。
type clientMetrics struct {
	// requests 调用总次数，按 result 分类（含熔断快速失败）。
	requests *prometheus.CounterVec
	// retries 因瞬时错误发起的重试次数。
	retries prometheus.Counter
	// duration 调用总耗时（含重试与退避）。
	duration prometheus.Histogram
	// breakerState 熔断器状态：0=closed 1=half-open 2=open。
	breakerState prometheus.Gauge
}

// newClientMetrics 创建指标集合并注册到 reg（nil 表示不注册，测试可完全隔离）。
func newClientMetrics(reg prometheus.Registerer) *clientMetrics {
	m := &clientMetrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "content_service",
			Subsystem: "gamecatalog_client",
			Name:      "requests_total",
			Help:      "game-catalog 跨服务调用总次数（含快速失败），按 result 分类。",
		}, []string{"result"}),

		retries: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "content_service",
			Subsystem: "gamecatalog_client",
			Name:      "retries_total",
			Help:      "game-catalog 跨服务调用因瞬时错误发起的重试次数。",
		}),

		duration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "content_service",
			Subsystem: "gamecatalog_client",
			Name:      "request_duration_seconds",
			Help:      "game-catalog 跨服务调用总耗时（含重试与退避）。",
			Buckets:   []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
		}),

		breakerState: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "content_service",
			Subsystem: "gamecatalog_client",
			Name:      "breaker_state",
			Help:      "game-catalog 客户端熔断器状态：0=closed 1=half-open 2=open。",
		}),
	}

	if reg != nil {
		reg.MustRegister(m.requests, m.retries, m.duration, m.breakerState)
	}
	return m
}

// defaultMetrics 生产默认指标集合，注册到 prometheus 默认 registry，
// 随 go-zero Prometheus agent 的 /metrics 暴露。
var defaultMetrics = newClientMetrics(prometheus.DefaultRegisterer)

// resultLabel 将调用错误映射为结果分类标签。
func resultLabel(err error) string {
	callErr, ok := err.(*callError)
	if !ok {
		return resultUnknownError
	}
	switch callErr.kind {
	case kindTransient:
		return resultTransient
	case kindBadRequest:
		return resultBadRequest
	case kindCanceled:
		return resultCanceled
	case kindContract:
		return resultContractErr
	default:
		return resultUnknownError
	}
}

// attachBreakerMetrics 将熔断器状态迁移上报到 gauge。
// 回调由 Breaker 在锁外同步调用，只做 gauge 写入（prometheus 类型自身并发安全）。
// 状态值映射见 GaugeOpts Help：0=closed 1=half-open 2=open。
func attachBreakerMetrics(b *Breaker, m *clientMetrics) {
	b.onChange = func(s breakerState) {
		m.breakerState.Set(breakerStateValue(s))
	}
}

// breakerStateValue 将内部状态枚举映射为 gauge 语义值。
func breakerStateValue(s breakerState) float64 {
	switch s {
	case stateOpen:
		return 2
	case stateHalfOpen:
		return 1
	default:
		return 0
	}
}
