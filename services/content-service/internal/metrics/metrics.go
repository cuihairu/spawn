package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// 跨服务调用指标（content-service 专用）
var (
	// GameCatalogClientCallsTotal 记录对 game-catalog 服务的调用总数
	// label: method (GetGameById), status (success/timeout/circuit_open/retry_exhausted/parse_error)
	GameCatalogClientCallsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "content_game_catalog_client_calls_total",
			Help: "Total number of calls to game-catalog service",
		},
		[]string{"method", "status"},
	)

	// GameCatalogClientCallDurationSeconds 记录跨服务调用延迟（秒）
	// label: method (GetGameById)
	GameCatalogClientCallDurationSeconds = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "content_game_catalog_client_call_duration_seconds",
			Help:    "Latency of calls to game-catalog service in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method"},
	)

	// CircuitBreakerState 熔断器当前状态（0=closed, 1=half-open, 2=open）
	// label: name (game_catalog)
	CircuitBreakerState = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "content_circuit_breaker_state",
			Help: "Current state of circuit breaker (0=closed, 1=half-open, 2=open)",
		},
		[]string{"name"},
	)

	// CircuitBreakerTransitionsTotal 熔断器状态迁移总数
	// label: name (game_catalog), from_state, to_state
	CircuitBreakerTransitionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "content_circuit_breaker_transitions_total",
			Help: "Total number of circuit breaker state transitions",
		},
		[]string{"name", "from_state", "to_state"},
	)
)

const (
	// 熔断器状态常量（与 Gauge 值对应，匹配 client/metrics.go 的映射）
	CircuitStateClosed   = 0
	CircuitStateHalfOpen = 1
	CircuitStateOpen     = 2
)
