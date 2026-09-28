package metrics

import (
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestMetrics_Registration(t *testing.T) {
	// 使用隔离注册表避免污染默认注册表
	reg := prometheus.NewRegistry()

	// 创建指标实例（手动注册到隔离 registry）
	gameCatalogCalls := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "content_game_catalog_client_calls_total",
		Help: "Total number of calls to game-catalog service",
	}, []string{"method", "status"})
	gameCatalogDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "content_game_catalog_client_call_duration_seconds",
		Help:    "Latency of calls to game-catalog service in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method"})
	circuitBreakerState := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "content_circuit_breaker_state",
		Help: "Current state of circuit breaker (0=closed, 1=half-open, 2=open)",
	}, []string{"name"})
	circuitBreakerTransitions := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "content_circuit_breaker_transitions_total",
		Help: "Total number of circuit breaker state transitions",
	}, []string{"name", "from_state", "to_state"})

	reg.MustRegister(gameCatalogCalls, gameCatalogDuration, circuitBreakerState, circuitBreakerTransitions)

	// 写入样本数据，确保指标在采集时可见（CounterVec/HistogramVec 无样本时不输出）
	gameCatalogCalls.WithLabelValues("GetGameById", "success").Inc()
	gameCatalogDuration.WithLabelValues("GetGameById").Observe(0.1)
	circuitBreakerState.WithLabelValues("game_catalog").Set(float64(CircuitStateClosed))
	circuitBreakerTransitions.WithLabelValues("game_catalog", "closed", "half_open").Inc()

	// 验证指标已注册：采集器输出应包含指标名
	metricsFamilies, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}

	found := map[string]bool{}
	for _, mf := range metricsFamilies {
		found[mf.GetName()] = true
	}

	expected := []string{
		"content_game_catalog_client_calls_total",
		"content_game_catalog_client_call_duration_seconds",
		"content_circuit_breaker_state",
		"content_circuit_breaker_transitions_total",
	}
	for _, exp := range expected {
		if !found[exp] {
			t.Errorf("expected metric %s not found in gathered metrics", exp)
		}
	}
}

func TestMetrics_GameCatalogCallsTotal_Increments(t *testing.T) {
	reg := prometheus.NewRegistry()
	calls := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "content_game_catalog_client_calls_total",
		Help: "Total number of calls to game-catalog service",
	}, []string{"method", "status"})
	reg.MustRegister(calls)

	// 模拟几次调用
	calls.WithLabelValues("GetGameById", "success").Inc()
	calls.WithLabelValues("GetGameById", "success").Inc()
	calls.WithLabelValues("GetGameById", "transient_error").Inc()
	calls.WithLabelValues("GetGameById", "circuit_open").Inc()

	// 断言计数
	if err := testutil.CollectAndCompare(reg, strings.NewReader(`
# HELP content_game_catalog_client_calls_total Total number of calls to game-catalog service
# TYPE content_game_catalog_client_calls_total counter
content_game_catalog_client_calls_total{method="GetGameById",status="success"} 2
content_game_catalog_client_calls_total{method="GetGameById",status="transient_error"} 1
content_game_catalog_client_calls_total{method="GetGameById",status="circuit_open"} 1
`)); err != nil {
		t.Errorf("metric values mismatch: %v", err)
	}
}

func TestMetrics_CircuitBreakerState_Transitions(t *testing.T) {
	reg := prometheus.NewRegistry()
	state := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "content_circuit_breaker_state",
		Help: "Current state of circuit breaker (0=closed, 1=half-open, 2=open)",
	}, []string{"name"})
	transitions := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "content_circuit_breaker_transitions_total",
		Help: "Total number of circuit breaker state transitions",
	}, []string{"name", "from_state", "to_state"})
	reg.MustRegister(state, transitions)

	// 模拟状态迁移：closed -> half-open -> open -> half-open -> closed
	state.WithLabelValues("game_catalog").Set(float64(CircuitStateClosed))
	transitions.WithLabelValues("game_catalog", "closed", "half_open").Inc()

	state.WithLabelValues("game_catalog").Set(float64(CircuitStateHalfOpen))
	transitions.WithLabelValues("game_catalog", "half_open", "open").Inc()

	state.WithLabelValues("game_catalog").Set(float64(CircuitStateOpen))
	transitions.WithLabelValues("game_catalog", "open", "half_open").Inc()

	state.WithLabelValues("game_catalog").Set(float64(CircuitStateHalfOpen))
	transitions.WithLabelValues("game_catalog", "half_open", "closed").Inc()

	state.WithLabelValues("game_catalog").Set(float64(CircuitStateClosed))

	// 断言最终状态与迁移计数（Prometheus 会按字母顺序排序标签：from_state, name, to_state）
	if err := testutil.CollectAndCompare(reg, strings.NewReader(`
# HELP content_circuit_breaker_state Current state of circuit breaker (0=closed, 1=half-open, 2=open)
# TYPE content_circuit_breaker_state gauge
content_circuit_breaker_state{name="game_catalog"} 0
# HELP content_circuit_breaker_transitions_total Total number of circuit breaker state transitions
# TYPE content_circuit_breaker_transitions_total counter
content_circuit_breaker_transitions_total{from_state="closed",name="game_catalog",to_state="half_open"} 1
content_circuit_breaker_transitions_total{from_state="half_open",name="game_catalog",to_state="closed"} 1
content_circuit_breaker_transitions_total{from_state="half_open",name="game_catalog",to_state="open"} 1
content_circuit_breaker_transitions_total{from_state="open",name="game_catalog",to_state="half_open"} 1
`)); err != nil {
		t.Errorf("metric values mismatch: %v", err)
	}
}

func TestMetrics_GameCatalogCallDuration_RecordsLatency(t *testing.T) {
	reg := prometheus.NewRegistry()
	duration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "content_game_catalog_client_call_duration_seconds",
		Help:    "Latency of calls to game-catalog service in seconds",
		Buckets: prometheus.DefBuckets,
	}, []string{"method"})
	reg.MustRegister(duration)

	// 记录几个延迟样本
	duration.WithLabelValues("GetGameById").Observe(0.05)
	duration.WithLabelValues("GetGameById").Observe(0.12)
	duration.WithLabelValues("GetGameById").Observe(0.03)

	// 验证 histogram 有数据（count=3, sum=0.2）
	metricsFamilies, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}
	for _, mf := range metricsFamilies {
		if mf.GetName() == "content_game_catalog_client_call_duration_seconds" {
			for _, m := range mf.GetMetric() {
				h := m.GetHistogram()
				if h.GetSampleCount() != 3 {
					t.Errorf("expected sample count 3, got %d", h.GetSampleCount())
				}
				if h.GetSampleSum() < 0.19 || h.GetSampleSum() > 0.21 {
					t.Errorf("expected sample sum ~0.2, got %f", h.GetSampleSum())
				}
			}
		}
	}
}
