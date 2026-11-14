package client

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tappi/tappi/pkg/resilience"
)

// TestNewGameCatalogClient_NormalizesInvalidOptions 零值/负值注入项全部被
// 构造函数归一化：maxAttempts<1→1、负退避→0、nil sleep/breaker/metrics 走默认。
func TestNewGameCatalogClient_NormalizesInvalidOptions(t *testing.T) {
	c := newGameCatalogClient("http://127.0.0.1:1", time.Second, clientOptions{
		retry: retryConfig{MaxAttempts: 0, BaseDelay: -time.Second, MaxDelay: -time.Second},
	})

	if c.retry.MaxAttempts != 1 {
		t.Fatalf("MaxAttempts = %d, want 1", c.retry.MaxAttempts)
	}
	if c.retry.BaseDelay != 0 || c.retry.MaxDelay != 0 {
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
		retry: retryConfig{MaxAttempts: 5, BaseDelay: time.Hour},
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
	if resilience.KindOf(err) != kindCanceled {
		t.Fatalf("err = %v (%T), want kindCanceled", err, err)
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
	if resilience.KindOf(err) != kindTransient {
		t.Fatalf("err = %v (%T), want kindTransient", err, err)
	}
}

// TestGetGameByIdOnce_InvalidGameId gameId 含控制字符 → 构造请求即失败
// → kindCanceled「创建请求失败」。
func TestGetGameByIdOnce_InvalidGameId(t *testing.T) {
	c := NewGameCatalogClient("http://127.0.0.1:1", time.Second)
	_, err := c.getGameByIdOnce(context.Background(), "bad\x00id")
	if resilience.KindOf(err) != kindCanceled {
		t.Fatalf("err = %v (%T), want kindCanceled", err, err)
	}
}

// TestStatusForInternalMetrics_Mappings 非 callError → unknown_error；
// canceled → canceled。
func TestStatusForInternalMetrics_Mappings(t *testing.T) {
	if got := statusForInternalMetrics(errors.New("plain")); got != "unknown_error" {
		t.Fatalf("plain error → %q, want unknown_error", got)
	}
	if got := statusForInternalMetrics(resilience.NewError(kindCanceled, "")); got != "canceled" {
		t.Fatalf("canceled → %q, want canceled", got)
	}
}

// TestResultLabel_Mappings 非 callError → resultUnknownError；
// canceled → resultCanceled。
func TestResultLabel_Mappings(t *testing.T) {
	if got := resultLabel(errors.New("plain")); got != resultUnknownError {
		t.Fatalf("plain error → %q, want %q", got, resultUnknownError)
	}
	if got := resultLabel(resilience.NewError(kindCanceled, "")); got != resultCanceled {
		t.Fatalf("canceled → %q, want %q", got, resultCanceled)
	}
}
