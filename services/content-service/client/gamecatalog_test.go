package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// newTestServer 启动一个模拟 game-catalog 的测试服务器，并返回对应的客户端。
func newTestServer(t *testing.T, handler http.HandlerFunc) *GameCatalogClient {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return NewGameCatalogClient(server.URL, 5*time.Second)
}

// newTestServerWithOptions 同 newTestServer，但允许注入客户端选项（重试参数、熔断器、休眠函数）。
func newTestServerWithOptions(t *testing.T, handler http.HandlerFunc, opts clientOptions) *GameCatalogClient {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	return newGameCatalogClient(server.URL, 5*time.Second, opts)
}

// recordingOptions 返回带退避记录的客户端选项，以及读取已记录退避时长的函数。
// 使用阈值极大的熔断器，使重试用例与熔断器行为解耦。
func recordingOptions() (clientOptions, func() []time.Duration) {
	var delays []time.Duration
	opts := defaultClientOptions()
	opts.breaker = NewBreaker(1000, 0)
	opts.sleep = func(d time.Duration) { delays = append(delays, d) }
	return opts, func() []time.Duration { return append([]time.Duration(nil), delays...) }
}

// fakeClockBreaker 创建带可推进时钟的熔断器，返回熔断器与推进时钟的函数。
func fakeClockBreaker(maxFailures int, cooldown time.Duration) (*Breaker, func(time.Duration)) {
	now := time.Now()
	b := NewBreaker(maxFailures, cooldown)
	b.now = func() time.Time { return now }
	return b, func(d time.Duration) { now = now.Add(d) }
}

func TestGetGameById_BareGameResponse(t *testing.T) {
	t.Parallel()

	var gotPath string
	cl := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"game": map[string]any{
				"id":          "game-001",
				"title":       "星穹幻境",
				"cover_image": "https://cdn.example.com/game-001.jpg",
				"genres":      []string{"RPG"},
				"platforms":   []string{"PC", "iOS"},
			},
		})
	})

	game, err := cl.GetGameById(context.Background(), "game-001")
	if err != nil {
		t.Fatalf("GetGameById() error = %v", err)
	}
	if gotPath != "/games/game-001" {
		t.Fatalf("expected request path /games/game-001, got %q", gotPath)
	}
	if game.Id != "game-001" || game.Title != "星穹幻境" {
		t.Fatalf("unexpected game: %+v", game)
	}
	if game.CoverImage != "https://cdn.example.com/game-001.jpg" {
		t.Fatalf("unexpected cover image: %q", game.CoverImage)
	}
	if len(game.Genres) != 1 || game.Genres[0] != "RPG" {
		t.Fatalf("unexpected genres: %v", game.Genres)
	}
	if len(game.Platforms) != 2 {
		t.Fatalf("unexpected platforms: %v", game.Platforms)
	}
}

func TestGetGameById_LegacyWrappedResponse(t *testing.T) {
	t.Parallel()

	cl := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusOK,
			"message": "ok",
			"data": map[string]any{
				"id":    "game-002",
				"title": "旧格式游戏",
			},
		})
	})

	game, err := cl.GetGameById(context.Background(), "game-002")
	if err != nil {
		t.Fatalf("GetGameById() error = %v", err)
	}
	if game.Id != "game-002" || game.Title != "旧格式游戏" {
		t.Fatalf("unexpected game: %+v", game)
	}
}

func TestGetGameById_Non200Status(t *testing.T) {
	t.Parallel()

	cl := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "游戏不存在", http.StatusNotFound)
	})

	game, err := cl.GetGameById(context.Background(), "missing")
	if err == nil {
		t.Fatalf("expected error for 404 response, got game %+v", game)
	}
	if !strings.Contains(err.Error(), "404") {
		t.Fatalf("expected status code in error, got %q", err.Error())
	}
}

func TestGetGameById_BusinessErrorCode(t *testing.T) {
	t.Parallel()

	cl := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    http.StatusInternalServerError,
			"message": "查询失败",
		})
	})

	game, err := cl.GetGameById(context.Background(), "game-003")
	if err == nil {
		t.Fatalf("expected error for business error code, got game %+v", game)
	}
	if !strings.Contains(err.Error(), "查询失败") {
		t.Fatalf("expected business message in error, got %q", err.Error())
	}
}

func TestGetGameById_InvalidJSON(t *testing.T) {
	t.Parallel()

	cl := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	})

	game, err := cl.GetGameById(context.Background(), "game-004")
	if err == nil {
		t.Fatalf("expected error for invalid json, got game %+v", game)
	}
	if !strings.Contains(err.Error(), "解析响应失败") {
		t.Fatalf("expected parse error, got %q", err.Error())
	}
}

func TestGetGameById_UnrecognizedBody(t *testing.T) {
	t.Parallel()

	cl := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"unexpected": true})
	})

	game, err := cl.GetGameById(context.Background(), "game-005")
	if err == nil {
		t.Fatalf("expected error for unrecognized body, got game %+v", game)
	}
	if !strings.Contains(err.Error(), "响应格式无法识别") {
		t.Fatalf("expected unrecognized-format error, got %q", err.Error())
	}
}

func TestGetGameById_UnreachableServer(t *testing.T) {
	t.Parallel()

	// 指向一个确定不可用的地址，模拟服务下线。
	cl := NewGameCatalogClient("http://127.0.0.1:1", time.Second)

	game, err := cl.GetGameById(context.Background(), "game-006")
	if err == nil {
		t.Fatalf("expected error for unreachable server, got game %+v", game)
	}
}

func TestGetGameById_ContextCanceled(t *testing.T) {
	t.Parallel()

	cl := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		_, _ = w.Write([]byte(`{"game":{"id":"game-007"}}`))
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	game, err := cl.GetGameById(ctx, "game-007")
	if err == nil {
		t.Fatalf("expected error for canceled context, got game %+v", game)
	}
}

func TestNewGameCatalogClient_TrimsTrailingSlash(t *testing.T) {
	t.Parallel()

	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"game":{"id":"game-008"}}`))
	}))
	defer server.Close()

	cl := NewGameCatalogClient(server.URL+"/", time.Second)
	if _, err := cl.GetGameById(context.Background(), "game-008"); err != nil {
		t.Fatalf("GetGameById() error = %v", err)
	}
	if gotPath != "/games/game-008" {
		t.Fatalf("expected request path /games/game-008, got %q", gotPath)
	}
}

// ---- 重试与退避 ----

func TestGetGameById_RetriesTransientWithBackoff(t *testing.T) {
	t.Parallel()

	hits := 0
	opts, delays := recordingOptions()
	opts.retry = retryConfig{maxAttempts: 3, baseDelay: 10 * time.Millisecond, maxDelay: time.Second}

	cl := newTestServerWithOptions(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if hits < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"game": map[string]any{"id": "g1", "title": "T"}})
	}, opts)

	game, err := cl.GetGameById(context.Background(), "g1")
	if err != nil {
		t.Fatalf("GetGameById() error = %v", err)
	}
	if hits != 3 {
		t.Fatalf("expected 3 attempts (2 failures then success), got %d", hits)
	}
	got := delays()
	if len(got) != 2 || got[0] != 10*time.Millisecond || got[1] != 20*time.Millisecond {
		t.Fatalf("expected backoff [10ms 20ms], got %v", got)
	}
	if game.Title != "T" {
		t.Fatalf("unexpected game: %+v", game)
	}
}

func TestGetGameById_RetriesExhausted(t *testing.T) {
	t.Parallel()

	hits := 0
	opts, delays := recordingOptions()
	opts.retry = retryConfig{maxAttempts: 4, baseDelay: time.Millisecond, maxDelay: 3 * time.Millisecond}

	cl := newTestServerWithOptions(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("boom"))
	}, opts)

	game, err := cl.GetGameById(context.Background(), "g1")
	if err == nil {
		t.Fatalf("expected error after exhausting retries, got game %+v", game)
	}
	if hits != 4 {
		t.Fatalf("expected 4 attempts, got %d", hits)
	}
	if !strings.Contains(err.Error(), "500") {
		t.Fatalf("expected status in error, got %q", err.Error())
	}
	if got := delays(); len(got) != 3 || got[0] != time.Millisecond || got[1] != 2*time.Millisecond || got[2] != 3*time.Millisecond {
		t.Fatalf("expected backoff [1ms 2ms 3ms], got %v", got)
	}
}

func TestGetGameById_BackoffCappedAtMaxDelay(t *testing.T) {
	t.Parallel()

	opts, delays := recordingOptions()
	opts.retry = retryConfig{maxAttempts: 6, baseDelay: time.Millisecond, maxDelay: 3 * time.Millisecond}

	cl := newTestServerWithOptions(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}, opts)

	if _, err := cl.GetGameById(context.Background(), "g1"); err == nil {
		t.Fatal("expected error")
	}
	// 5 次失败后退避序列：1ms, 2ms, 3ms(cap), 3ms(cap), 3ms(cap)
	want := []time.Duration{time.Millisecond, 2 * time.Millisecond, 3 * time.Millisecond, 3 * time.Millisecond, 3 * time.Millisecond}
	if got := delays(); len(got) != 5 {
		t.Fatalf("expected 5 delays, got %d: %v", len(got), got)
	} else {
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("delay[%d] = %v, want %v", i, got[i], want[i])
			}
		}
	}
}

func TestGetGameById_NoRetryOnClientError(t *testing.T) {
	t.Parallel()

	hits := 0
	opts, delays := recordingOptions()
	opts.retry = retryConfig{maxAttempts: 5, baseDelay: time.Millisecond, maxDelay: 0}

	cl := newTestServerWithOptions(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		http.Error(w, "游戏不存在", http.StatusNotFound)
	}, opts)

	game, err := cl.GetGameById(context.Background(), "missing")
	if err == nil {
		t.Fatalf("expected error, got game %+v", game)
	}
	if hits != 1 {
		t.Fatalf("expected no retry on 404, got %d attempts", hits)
	}
	if got := delays(); len(got) != 0 {
		t.Fatalf("expected no backoff sleeps, got %v", got)
	}
}

func TestGetGameById_NoRetryOnCanceledContext(t *testing.T) {
	t.Parallel()

	hits := 0
	opts, _ := recordingOptions()
	opts.retry = retryConfig{maxAttempts: 5, baseDelay: time.Millisecond, maxDelay: 0}

	cl := newTestServerWithOptions(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"game":{"id":"g1"}}`))
	}, opts)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := cl.GetGameById(ctx, "g1"); err == nil {
		t.Fatal("expected error for canceled context")
	}
	if hits != 0 {
		t.Fatalf("expected no request with canceled context, got %d", hits)
	}
}

func TestGetGameById_NoRetryOnContractViolation(t *testing.T) {
	t.Parallel()

	hits := 0
	opts, delays := recordingOptions()
	opts.retry = retryConfig{maxAttempts: 5, baseDelay: time.Millisecond, maxDelay: 0}

	cl := newTestServerWithOptions(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte("not-json"))
	}, opts)

	if _, err := cl.GetGameById(context.Background(), "g1"); err == nil {
		t.Fatal("expected error")
	}
	if hits != 1 {
		t.Fatalf("expected no retry on contract violation, got %d attempts", hits)
	}
	if got := delays(); len(got) != 0 {
		t.Fatalf("expected no backoff sleeps, got %v", got)
	}
}

// ---- 熔断器集成（通过 GetGameById 触发）----

func TestGetGameById_BreakerOpensAndFastFails(t *testing.T) {
	t.Parallel()

	hits := 0
	opts, _ := recordingOptions()
	// 阈值 2：一次请求内的 2 次连续失败即可熔断。
	opts.retry = retryConfig{maxAttempts: 3, baseDelay: time.Millisecond, maxDelay: 0}
	opts.breaker = NewBreaker(2, time.Minute)

	cl := newTestServerWithOptions(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusInternalServerError)
	}, opts)

	game, err := cl.GetGameById(context.Background(), "g1")
	if err == nil {
		t.Fatalf("expected error, got game %+v", game)
	}
	if !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("expected ErrBreakerOpen, got %q", err.Error())
	}
	if hits != 2 {
		t.Fatalf("expected 2 attempts before breaker opens, got %d", hits)
	}

	// 熔断开启后快速失败，不再发起真实请求。
	game, err = cl.GetGameById(context.Background(), "g1")
	if err == nil {
		t.Fatalf("expected error, got game %+v", game)
	}
	if !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("expected ErrBreakerOpen, got %q", err.Error())
	}
	if hits != 2 {
		t.Fatalf("expected no network request while open, got %d hits", hits)
	}
	if got := cl.breaker.stateValue(); got != stateOpen {
		t.Fatalf("expected breaker open, got %v", got)
	}
}

func TestGetGameById_NoNetworkWhenBreakerPreOpened(t *testing.T) {
	t.Parallel()

	hits := 0
	b := NewBreaker(1, time.Minute)
	b.Failure() // 直接打开熔断器

	opts := defaultClientOptions()
	opts.breaker = b
	opts.retry = retryConfig{maxAttempts: 3, baseDelay: time.Millisecond, maxDelay: 0}
	opts.sleep = func(d time.Duration) {}

	cl := newTestServerWithOptions(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		_, _ = w.Write([]byte(`{"game":{"id":"g1"}}`))
	}, opts)

	if _, err := cl.GetGameById(context.Background(), "g1"); !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("expected ErrBreakerOpen, got %v", err)
	}
	if hits != 0 {
		t.Fatalf("expected no network request while open, got %d hits", hits)
	}
}

func TestGetGameById_HalfOpenProbeSuccessRecovers(t *testing.T) {
	t.Parallel()

	hits := 0
	failMode := true
	b, advance := fakeClockBreaker(2, time.Minute)

	opts := defaultClientOptions()
	opts.breaker = b
	opts.retry = retryConfig{maxAttempts: 1, baseDelay: 0, maxDelay: 0}
	opts.sleep = func(d time.Duration) {}

	cl := newTestServerWithOptions(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if failMode {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"game":{"id":"g1"}}`))
	}, opts)

	// 连续 2 次单次尝试失败 → 熔断开启。
	for i := 0; i < 2; i++ {
		if _, err := cl.GetGameById(context.Background(), "g1"); err == nil {
			t.Fatalf("call %d: expected error while failing", i+1)
		}
	}
	if hits != 2 || b.stateValue() != stateOpen {
		t.Fatalf("expected breaker open after 2 failures (hits=%d)", hits)
	}

	// 冷却期内继续快速失败。
	if _, err := cl.GetGameById(context.Background(), "g1"); !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("expected ErrBreakerOpen during cooldown, got %v", err)
	}
	if hits != 2 {
		t.Fatalf("expected no probe during cooldown, got %d hits", hits)
	}

	// 冷却结束：放行探测请求，服务恢复 → 熔断器回到 closed。
	advance(2 * time.Minute)
	failMode = false

	game, err := cl.GetGameById(context.Background(), "g1")
	if err != nil || game == nil {
		t.Fatalf("expected probe success, got game=%+v err=%v", game, err)
	}
	if hits != 3 {
		t.Fatalf("expected exactly 1 probe request, got %d hits", hits)
	}
	if b.stateValue() != stateClosed {
		t.Fatalf("expected breaker closed after probe success, got %v", b.stateValue())
	}

	// 恢复后请求正常放行。
	if _, err := cl.GetGameById(context.Background(), "g1"); err != nil {
		t.Fatalf("expected success after recovery, got %v", err)
	}
	if hits != 4 {
		t.Fatalf("expected normal request after recovery, got %d hits", hits)
	}
}

func TestGetGameById_HalfOpenProbeFailureReopens(t *testing.T) {
	t.Parallel()

	hits := 0
	b, advance := fakeClockBreaker(2, time.Minute)

	opts := defaultClientOptions()
	opts.breaker = b
	opts.retry = retryConfig{maxAttempts: 1, baseDelay: 0, maxDelay: 0}
	opts.sleep = func(d time.Duration) {}

	cl := newTestServerWithOptions(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		w.WriteHeader(http.StatusInternalServerError)
	}, opts)

	for i := 0; i < 2; i++ {
		if _, err := cl.GetGameById(context.Background(), "g1"); err == nil {
			t.Fatalf("call %d: expected error while failing", i+1)
		}
	}

	// 冷却结束后探测仍失败 → 重新 open。
	advance(2 * time.Minute)
	if _, err := cl.GetGameById(context.Background(), "g1"); err == nil {
		t.Fatal("expected probe failure")
	}
	if b.stateValue() != stateOpen {
		t.Fatalf("expected breaker re-opened after failed probe, got %v", b.stateValue())
	}
	if hits != 3 {
		t.Fatalf("expected exactly 1 failed probe, got %d hits", hits)
	}

	// 重新冷却期内快速失败，不再请求网络。
	if _, err := cl.GetGameById(context.Background(), "g1"); !errors.Is(err, ErrBreakerOpen) {
		t.Fatalf("expected ErrBreakerOpen after failed probe, got %v", err)
	}
	if hits != 3 {
		t.Fatalf("expected no network request while open, got %d hits", hits)
	}
}

func TestGetGameById_HalfOpenProbeClientErrorRecovers(t *testing.T) {
	t.Parallel()

	hits := 0
	failMode := true
	b, advance := fakeClockBreaker(2, time.Minute)

	opts := defaultClientOptions()
	opts.breaker = b
	opts.retry = retryConfig{maxAttempts: 1, baseDelay: 0, maxDelay: 0}
	opts.sleep = func(d time.Duration) {}

	cl := newTestServerWithOptions(t, func(w http.ResponseWriter, r *http.Request) {
		hits++
		if failMode {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		http.Error(w, "游戏不存在", http.StatusNotFound)
	}, opts)

	for i := 0; i < 2; i++ {
		if _, err := cl.GetGameById(context.Background(), "g1"); err == nil {
			t.Fatalf("call %d: expected error while failing", i+1)
		}
	}

	// 探测返回 4xx：服务可达，熔断器恢复 closed。
	advance(2 * time.Minute)
	failMode = false

	if _, err := cl.GetGameById(context.Background(), "g1"); err == nil {
		t.Fatal("expected 404 error")
	}
	if b.stateValue() != stateClosed {
		t.Fatalf("expected breaker closed after 4xx probe, got %v", b.stateValue())
	}

	// 恢复后请求正常放行（可达网络）：2 次失败 + 1 次探测 + 1 次恢复后请求。
	if _, err := cl.GetGameById(context.Background(), "g1"); err == nil {
		t.Fatal("expected 404 error")
	}
	if hits != 4 {
		t.Fatalf("expected requests to reach network after recovery, got %d hits", hits)
	}
}

// ---- 退避计算单元测试 ----

func TestRetryConfig_BackoffDelay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		retry          retryConfig
		failedAttempts int
		want           time.Duration
	}{
		{"first failure", retryConfig{baseDelay: 10 * time.Millisecond, maxDelay: time.Second}, 1, 10 * time.Millisecond},
		{"second failure", retryConfig{baseDelay: 10 * time.Millisecond, maxDelay: time.Second}, 2, 20 * time.Millisecond},
		{"third failure", retryConfig{baseDelay: 10 * time.Millisecond, maxDelay: time.Second}, 3, 40 * time.Millisecond},
		{"capped at max", retryConfig{baseDelay: 10 * time.Millisecond, maxDelay: 30 * time.Millisecond}, 3, 30 * time.Millisecond},
		{"max cap exceeded", retryConfig{baseDelay: 10 * time.Millisecond, maxDelay: 30 * time.Millisecond}, 4, 30 * time.Millisecond},
		{"no cap", retryConfig{baseDelay: 10 * time.Millisecond, maxDelay: 0}, 5, 160 * time.Millisecond},
		{"zero base", retryConfig{baseDelay: 0, maxDelay: time.Second}, 3, 0},
		{"zero attempts", retryConfig{baseDelay: 10 * time.Millisecond, maxDelay: time.Second}, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.retry.backoffDelay(tt.failedAttempts); got != tt.want {
				t.Fatalf("backoffDelay(%d) = %v, want %v", tt.failedAttempts, got, tt.want)
			}
		})
	}
}
