package client

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

const metricsPrefix = "content_service_gamecatalog_client_"

// newIsolatedMetrics 创建注册到独立 registry 的指标集合与对应客户端构造选项，
// 避免与并行用例共享的 defaultMetrics 相互干扰。
func newIsolatedMetrics() (*clientMetrics, *clientOptions) {
	reg := prometheus.NewRegistry()
	m := newClientMetrics(reg)
	opts := defaultClientOptions()
	opts.metrics = m
	return m, &opts
}

func TestClientMetrics_RegisteredInDefaultRegistry(t *testing.T) {
	// 注册断言：默认指标集合应已注册到默认 registry（随 go-zero /metrics 暴露）。
	// CounterVec 需先创建子样本，否则 gather 不输出该 family。
	defaultMetrics.requests.WithLabelValues(resultSuccess)

	want := []string{
		metricsPrefix + "requests_total",
		metricsPrefix + "retries_total",
		metricsPrefix + "request_duration_seconds",
		metricsPrefix + "breaker_state",
	}
	count, err := testutil.GatherAndCount(prometheus.DefaultGatherer, want...)
	if err != nil {
		t.Fatalf("gather default registry: %v", err)
	}
	if count < len(want) {
		t.Fatalf("expected at least %d metric samples in default registry, got %d", len(want), count)
	}
}

func TestClientMetrics_RequestCountedByResult(t *testing.T) {
	t.Parallel()

	m, opts := newIsolatedMetrics()
	failing := true
	cl := newTestServerWithOptions(t, func(w http.ResponseWriter, r *http.Request) {
		if failing {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"game":{"id":"g1"}}`))
	}, *opts)

	successBefore := testutil.ToFloat64(m.requests.WithLabelValues(resultSuccess))
	transientBefore := testutil.ToFloat64(m.requests.WithLabelValues(resultTransient))

	// 瞬时错误（5xx）→ transient_error。
	cl.retry = retryConfig{maxAttempts: 1, baseDelay: 0, maxDelay: 0}
	if _, err := cl.GetGameById(context.Background(), "g1"); err == nil {
		t.Fatal("expected transient error")
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues(resultTransient)) - transientBefore; got != 1 {
		t.Fatalf("expected transient_error delta 1, got %v", got)
	}

	// 恢复服务 → success。
	failing = false
	if _, err := cl.GetGameById(context.Background(), "g1"); err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues(resultSuccess)) - successBefore; got != 1 {
		t.Fatalf("expected success delta 1, got %v", got)
	}
}

func TestClientMetrics_BreakerOpenCounted(t *testing.T) {
	t.Parallel()

	m, opts := newIsolatedMetrics()
	opts.breaker = NewBreaker(1, time.Minute)
	opts.retry = retryConfig{maxAttempts: 3, baseDelay: 0, maxDelay: 0}

	cl := newTestServerWithOptions(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}, *opts)

	before := testutil.ToFloat64(m.requests.WithLabelValues(resultBreakerOpen))

	// 第一次调用：尝试失败后熔断打开，快速失败。
	if _, err := cl.GetGameById(context.Background(), "g1"); !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("expected ErrBreakerOpen, got %v", err)
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues(resultBreakerOpen)) - before; got != 1 {
		t.Fatalf("expected breaker_open delta 1, got %v", got)
	}

	// 熔断期间再次调用：仍按 breaker_open 计数。
	if _, err := cl.GetGameById(context.Background(), "g1"); !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("expected ErrBreakerOpen, got %v", err)
	}
	if got := testutil.ToFloat64(m.requests.WithLabelValues(resultBreakerOpen)) - before; got != 2 {
		t.Fatalf("expected breaker_open delta 2, got %v", got)
	}
}

func TestClientMetrics_RetriesAndDurationCollected(t *testing.T) {
	t.Parallel()

	reg := prometheus.NewRegistry()
	m := newClientMetrics(reg)
	opts := defaultClientOptions()
	opts.metrics = m
	opts.retry = retryConfig{maxAttempts: 3, baseDelay: time.Millisecond, maxDelay: 0}

	cl := newTestServerWithOptions(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}, opts)

	retriesBefore := testutil.ToFloat64(m.retries)
	durationBefore := histogramSampleCount(t, reg, metricsPrefix+"request_duration_seconds")

	if _, err := cl.GetGameById(context.Background(), "g1"); err == nil {
		t.Fatal("expected error")
	}

	// 3 次尝试 → 2 次重试。
	if got := testutil.ToFloat64(m.retries) - retriesBefore; got != 2 {
		t.Fatalf("expected retries delta 2, got %v", got)
	}
	// duration histogram 采集断言：调用后新增 1 次观测（sample_count +1）。
	durationAfter := histogramSampleCount(t, reg, metricsPrefix+"request_duration_seconds")
	if got := durationAfter - durationBefore; got != 1 {
		t.Fatalf("expected duration observation delta 1, got %v (before=%d after=%d)", got, durationBefore, durationAfter)
	}
}

// histogramSampleCount 从 registry 采集结果中读取直方图样本计数。
func histogramSampleCount(t *testing.T, reg *prometheus.Registry, name string) uint64 {
	t.Helper()

	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() == name && len(mf.GetMetric()) > 0 {
			return mf.GetMetric()[0].GetHistogram().GetSampleCount()
		}
	}
	return 0
}

func TestClientMetrics_BreakerStateGaugeViaOnChange(t *testing.T) {
	t.Parallel()

	m, opts := newIsolatedMetrics()
	b, advance := fakeClockBreaker(1, time.Minute)
	opts.breaker = b
	opts.retry = retryConfig{maxAttempts: 1, baseDelay: 0, maxDelay: 0}
	attachBreakerMetrics(b, m)

	if got := testutil.ToFloat64(m.breakerState); got != 0 {
		t.Fatalf("expected gauge 0 (closed) initially, got %v", got)
	}

	// closed → open。
	b.Failure()
	if got := testutil.ToFloat64(m.breakerState); got != 2 {
		t.Fatalf("expected gauge 2 (open) after failure, got %v", got)
	}

	// open → half-open（冷却结束）。
	advance(time.Minute)
	if !b.Allow() {
		t.Fatal("expected probe allowed")
	}
	if got := testutil.ToFloat64(m.breakerState); got != 1 {
		t.Fatalf("expected gauge 1 (half-open) after cooldown, got %v", got)
	}

	// half-open → closed（探测成功）。
	b.Success()
	if got := testutil.ToFloat64(m.breakerState); got != 0 {
		t.Fatalf("expected gauge 0 (closed) after probe success, got %v", got)
	}
}

func TestClientMetrics_RenderedInPrometheusFormat(t *testing.T) {
	t.Parallel()

	// 以 promhttp 渲染独立 registry，断言指标以 Prometheus 文本格式暴露
	//（与 go-zero agent 的 promhttp.Handler() 输出格式一致）。
	reg := prometheus.NewRegistry()
	m := newClientMetrics(reg)
	m.requests.WithLabelValues(resultSuccess).Inc()
	m.retries.Inc()
	m.duration.Observe(0.01)
	m.breakerState.Set(0)

	server := httptest.NewServer(promhttp.HandlerFor(reg, promhttp.HandlerOpts{}))
	defer server.Close()

	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}

	for _, want := range []string{
		`# TYPE content_service_gamecatalog_client_requests_total counter`,
		`content_service_gamecatalog_client_requests_total{result="success"} 1`,
		`content_service_gamecatalog_client_retries_total 1`,
		`content_service_gamecatalog_client_request_duration_seconds_bucket`,
		`content_service_gamecatalog_client_breaker_state 0`,
	} {
		if !strings.Contains(string(body), want) {
			t.Fatalf("expected %q in /metrics output", want)
		}
	}
}
