package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestNewGameCatalogClient_NormalizesInvalidOptions 零值/负值注入项全部被
// 构造函数归一化：maxAttempts<1→1、负退避→0、nil sleep/breaker/metrics 走默认。
func TestNewGameCatalogClient_NormalizesInvalidOptions(t *testing.T) {
	c := newGameCatalogClient("http://127.0.0.1:1", time.Second, clientOptions{
		retry: retryConfig{maxAttempts: 0, baseDelay: -time.Second, maxDelay: -time.Second},
	})

	if c.retry.maxAttempts != 1 {
		t.Fatalf("maxAttempts = %d, want 1", c.retry.maxAttempts)
	}
	if c.retry.baseDelay != 0 || c.retry.maxDelay != 0 {
		t.Fatalf("negative delays not normalized: %+v", c.retry)
	}
	if c.sleep == nil {
		t.Fatal("sleep must default to time.Sleep")
	}
	if c.breaker == nil {
		t.Fatal("breaker must default to a fresh breaker")
	}
	if c.metrics == nil {
		t.Fatal("metrics must default to defaultMetrics")
	}
	if c.breaker.onChange == nil {
		t.Fatal("default breaker must get metrics attached via onChange")
	}
}

// TestGetGameById_CanceledDuringBackoff 退避休眠期间上下文被取消 →
// 放弃重试并返回 context.Canceled。
func TestGetGameById_CanceledDuringBackoff(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := newGameCatalogClient(server.URL, time.Second, clientOptions{
		retry: retryConfig{maxAttempts: 5, baseDelay: time.Hour},
		sleep: func(time.Duration) { cancel() }, // 首次退避即取消
	})

	_, err := c.GetGameById(ctx, "g-1")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
}

// TestGetGameById_DoFailsWithCanceledContext 调用前上下文已取消：
// Do 直接失败且 ctx.Err() 非空 → kindCanceled（不计入熔断）。
func TestGetGameById_DoFailsWithCanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	c := NewGameCatalogClient(server.URL, time.Second)
	_, err := c.getGameByIdOnce(ctx, "g-1")
	ce, ok := err.(*callError)
	if !ok || ce.kind != kindCanceled {
		t.Fatalf("err = %v (%T), want kindCanceled callError", err, err)
	}
}

// TestGetGameById_TruncatedResponseBody 响应头声明 Content-Length 大于实际
// 写出字节数且连接提前关闭 → 读取响应体失败 → kindTransient。
func TestGetGameById_TruncatedResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, buf, err := w.(http.Hijacker).Hijack()
		if err != nil {
			t.Errorf("hijack: %v", err)
			return
		}
		buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\nshort")
		buf.Flush()
		conn.Close()
	}))
	defer server.Close()

	c := NewGameCatalogClient(server.URL, 5*time.Second)
	_, err := c.GetGameById(context.Background(), "g-1")
	ce, ok := err.(*callError)
	if !ok || ce.kind != kindTransient {
		t.Fatalf("err = %v (%T), want kindTransient callError", err, err)
	}
}

// TestGetGameByIdOnce_InvalidGameId gameId 含控制字符 → 构造请求即失败
// → kindCanceled「创建请求失败」。
func TestGetGameByIdOnce_InvalidGameId(t *testing.T) {
	c := NewGameCatalogClient("http://127.0.0.1:1", time.Second)
	_, err := c.getGameByIdOnce(context.Background(), "bad\x00id")
	ce, ok := err.(*callError)
	if !ok || ce.kind != kindCanceled {
		t.Fatalf("err = %v (%T), want kindCanceled callError", err, err)
	}
}

// TestStatusForInternalMetrics_Mappings 非 callError → unknown_error；
// canceled → canceled。
func TestStatusForInternalMetrics_Mappings(t *testing.T) {
	if got := statusForInternalMetrics(errors.New("plain")); got != "unknown_error" {
		t.Fatalf("plain error → %q, want unknown_error", got)
	}
	if got := statusForInternalMetrics(&callError{kind: kindCanceled}); got != "canceled" {
		t.Fatalf("canceled → %q, want canceled", got)
	}
}

// TestResultLabel_Mappings 非 callError → resultUnknownError；
// canceled → resultCanceled。
func TestResultLabel_Mappings(t *testing.T) {
	if got := resultLabel(errors.New("plain")); got != resultUnknownError {
		t.Fatalf("plain error → %q, want %q", got, resultUnknownError)
	}
	if got := resultLabel(&callError{kind: kindCanceled}); got != resultCanceled {
		t.Fatalf("canceled → %q, want %q", got, resultCanceled)
	}
}

// TestBreaker_SetOnChangeAndHalfOpenProbe 白盒驱动 half-open：
// 直接置 half-open 且无探测在途 → 放行一个探测；第二个探测被拒。
func TestBreaker_SetOnChangeAndHalfOpenProbe(t *testing.T) {
	var transitions []breakerState
	b := NewBreaker(1, time.Minute)
	b.SetOnChange(func(s breakerState) { transitions = append(transitions, s) })

	// maxFailures=1：一次失败即 open，触发回调
	b.Failure()
	if len(transitions) != 1 || transitions[0] != stateOpen {
		t.Fatalf("transitions = %v, want [open]", transitions)
	}

	// 白盒置为 half-open 且无探测在途
	b.mu.Lock()
	b.state = stateHalfOpen
	b.probing = false
	b.mu.Unlock()

	if !b.Allow() {
		t.Fatal("first half-open probe must be allowed")
	}
	if b.Allow() {
		t.Fatal("second concurrent probe must be rejected")
	}
}

// TestBreakerStateNames_Unknown 非法状态值 → "unknown"。
func TestBreakerStateNames_Unknown(t *testing.T) {
	if got := fromStateName(breakerState(42)); got != "unknown" {
		t.Fatalf("stateName = %q, want unknown", got)
	}
	if got := transitionStateName(breakerState(42)); got != "unknown" {
		t.Fatalf("transitionStateName = %q, want unknown", got)
	}
}

// TestBackoffDelay_CapsInsideDoublingLoop 倍增过程中命中 maxDelay 即返回
// （走循环内封顶分支）。
func TestBackoffDelay_CapsInsideDoublingLoop(t *testing.T) {
	r := retryConfig{baseDelay: time.Millisecond, maxDelay: 3 * time.Millisecond}
	if got := r.backoffDelay(10); got != 3*time.Millisecond {
		t.Fatalf("backoffDelay(10) = %v, want 3ms cap", got)
	}
}

// TestBackoffDelay_FirstAttemptBaseOverMax failedAttempts=1 时循环不执行，
// baseDelay 本身超过 maxDelay → 走循环后封顶分支。
func TestBackoffDelay_FirstAttemptBaseOverMax(t *testing.T) {
	r := retryConfig{baseDelay: 10 * time.Millisecond, maxDelay: 5 * time.Millisecond}
	if got := r.backoffDelay(1); got != 5*time.Millisecond {
		t.Fatalf("backoffDelay(1) = %v, want 5ms cap", got)
	}
}
