package proxy

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewUpstream_RejectsBadBaseURL(t *testing.T) {
	t.Parallel()

	if _, err := NewUpstream("   ", time.Second); err == nil || !strings.Contains(err.Error(), "empty") {
		t.Fatalf("empty err = %v", err)
	}
	if _, err := NewUpstream("/relative/only", time.Second); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("no-scheme err = %v", err)
	}
	if _, err := NewUpstream("http://[::1", time.Second); err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("parse err = %v", err)
	}
}

func TestUpstream_DefaultTimeoutAndTrailingSlash(t *testing.T) {
	t.Parallel()

	up, err := NewUpstream("http://example.com///", 0)
	if err != nil {
		t.Fatalf("NewUpstream: %v", err)
	}
	if up.client.Timeout != 5*time.Second {
		t.Fatalf("timeout = %v, want default 5s", up.client.Timeout)
	}
	if up.baseURL.String() != "http://example.com" {
		t.Fatalf("baseURL = %q", up.baseURL.String())
	}
}

func TestUpstream_Ping(t *testing.T) {
	t.Parallel()

	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/health" {
			t.Errorf("ping path = %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ok.Close)

	up, err := NewUpstream(ok.URL, time.Second)
	if err != nil {
		t.Fatalf("NewUpstream: %v", err)
	}
	if err := up.Ping(context.Background()); err != nil {
		t.Fatalf("healthy ping: %v", err)
	}

	sick := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(sick.Close)
	up2, _ := NewUpstream(sick.URL, time.Second)
	if err := up2.Ping(context.Background()); err == nil || !strings.Contains(err.Error(), "upstream unhealthy: 503") {
		t.Fatalf("sick ping err = %v", err)
	}

	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	up3, _ := NewUpstream(closed.URL, time.Second)
	closed.Close()
	if err := up3.Ping(context.Background()); err == nil {
		t.Fatal("unreachable ping must fail")
	}

	// Ping 用 baseURL 原样拼接（含路径时也不带尾斜杠处理），连接失败返回错误
	if err := up3.Ping(context.Background()); err == nil {
		t.Fatal("second unreachable ping must fail")
	}
}

func TestUpstream_UnreachableReturnsJSONError(t *testing.T) {
	t.Parallel()

	closed := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	up, err := NewUpstream(closed.URL, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("NewUpstream: %v", err)
	}
	closed.Close()

	rr := httptest.NewRecorder()
	up.Handler()(rr, httptest.NewRequest(http.MethodGet, "http://gw/api/v1/posts", nil))

	if rr.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", rr.Code)
	}
	body := rr.Body.String()
	if !strings.Contains(body, `"code":502`) || !strings.Contains(body, "upstream unavailable") {
		t.Fatalf("body = %q", body)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "application/json") {
		t.Fatalf("content-type = %q", ct)
	}
}

func TestUpstream_StripsHopByHopHeaders(t *testing.T) {
	t.Parallel()

	var gotReq http.Header
	respHeaders := http.Header{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotReq = r.Header.Clone()
		respHeaders = r.Header.Clone() // 响应回显同名头以驱动 copyHeader 剥离
		w.Header().Set("X-Custom", "kept")
		for k, vv := range map[string][]string{
			"Keep-Alive":         {"timeout=5"},
			"Proxy-Authenticate": {"Basic"},
		} {
			w.Header()[k] = vv
		}
		io.WriteString(w, `{}`)
	}))
	t.Cleanup(upstream.Close)

	up, err := NewUpstream(upstream.URL, time.Second)
	if err != nil {
		t.Fatalf("NewUpstream: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "http://gw/api/v1/topics", nil)
	for _, k := range []string{"Connection", "Proxy-Connection", "Keep-Alive",
		"Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer",
		"Transfer-Encoding", "Upgrade", "X-Custom"} {
		req.Header.Set(k, "strip-me")
	}
	rr := httptest.NewRecorder()
	up.Handler()(rr, req)

	for _, k := range []string{"Connection", "Proxy-Connection", "Keep-Alive",
		"Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer",
		"Transfer-Encoding", "Upgrade"} {
		if gotReq.Get(k) != "" {
			t.Fatalf("request header %q must be stripped, got %q", k, gotReq.Get(k))
		}
		if rr.Header().Get(k) != "" {
			t.Fatalf("response header %q must be stripped, got %q", k, rr.Header().Get(k))
		}
	}
	if gotReq.Get("X-Custom") != "strip-me" {
		t.Fatal("custom header must be forwarded")
	}
	if rr.Header().Get("X-Custom") != "kept" {
		t.Fatal("custom response header must be copied")
	}
	_ = respHeaders
}
