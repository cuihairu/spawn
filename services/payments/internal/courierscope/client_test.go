package courierscope

// spawn_game_id → courier scope 解析客户端单测（GP-202 集成缺口修复）：
// 覆盖 Resolve 全部分支——成功（X-Internal-Token 头 + 路径正确）、404 →
// ErrMappingNotFound、非 200 → 错误、响应缺 courierGameId/env → 错误、
// 非 JSON 响应 → 错误、传输失败 → 错误。集成侧（mapping 存根 + 下单 502）
// 由 handler 集成测试与 orderlogic 白盒覆盖。

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestResolve_Success(t *testing.T) {
	var gotToken, gotPath string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Internal-Token")
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"courierGameId":"courier-game-a","env":"dev"}`))
	}))
	defer stub.Close()

	c := NewClient(stub.URL, "s2s-tok", time.Second)
	scope, err := c.Resolve("game-a")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if scope.CourierGameId != "courier-game-a" || scope.Env != "dev" {
		t.Fatalf("scope = %+v", scope)
	}
	if gotToken != "s2s-tok" {
		t.Fatalf("X-Internal-Token = %q, want s2s-tok", gotToken)
	}
	if gotPath != "/internal/games/game-a/mapping" {
		t.Fatalf("path = %q", gotPath)
	}
}

func TestResolve_NotFound(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"message":"游戏未接入 Courier"}`))
	}))
	defer stub.Close()

	_, err := NewClient(stub.URL, "tok", time.Second).Resolve("nomap")
	if !errors.Is(err, ErrMappingNotFound) {
		t.Fatalf("err = %v, want ErrMappingNotFound", err)
	}
}

func TestResolve_Non200(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer stub.Close()

	_, err := NewClient(stub.URL, "tok", time.Second).Resolve("g")
	if err == nil || errors.Is(err, ErrMappingNotFound) {
		t.Fatalf("err = %v, want non-200 error", err)
	}
}

func TestResolve_MissingFields(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"courierGameId":"cg"}`)) // env 缺失
	}))
	defer stub.Close()

	_, err := NewClient(stub.URL, "tok", time.Second).Resolve("g")
	if err == nil || !strings.Contains(err.Error(), "缺 courierGameId/env") {
		t.Fatalf("err = %v, want missing-fields error", err)
	}
}

func TestResolve_BadJSON(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer stub.Close()

	_, err := NewClient(stub.URL, "tok", time.Second).Resolve("g")
	if err == nil {
		t.Fatal("err = nil, want bad-JSON error")
	}
}

func TestResolve_TransportError(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", "tok", 50*time.Millisecond) // 连接拒绝
	if _, err := c.Resolve("g"); err == nil {
		t.Fatal("err = nil, want transport error")
	}
}
