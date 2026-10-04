package proxy

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

// TestUpstream_ForwardsBodyWithContentLength 缓冲后定长转发：
// 上游必须收到与请求一致的内容 + 明确的 Content-Length（非 chunked），
// go-zero 上游对 chunked 请求体解析失败是加缓冲的直接动因。
func TestUpstream_ForwardsBodyWithContentLength(t *testing.T) {
	var gotBody, gotTE, gotCL string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		if len(r.TransferEncoding) > 0 {
			gotTE = r.TransferEncoding[0]
		} else {
			gotTE = "identity(CL)"
		}
		gotCL = r.Header.Get("Content-Length")
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(upstreamServer.Close)

	up, err := NewUpstream(upstreamServer.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewUpstream: %v", err)
	}

	body := `{"title":"t","content":"c"}`
	req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/posts", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	up.Handler()(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if gotBody != body {
		t.Fatalf("upstream body = %q, want %q", gotBody, body)
	}
	if gotTE != "identity(CL)" {
		t.Fatalf("upstream transfer encoding = %q, want identity Content-Length (chunked 会打崩 go-zero 上游)", gotTE)
	}
	// 服务端框架填充的 Content-Length 必须等于缓冲体长度（定长直发的证明）
	if gotCL != strconv.Itoa(len(body)) {
		t.Fatalf("upstream content-length = %q, want %d", gotCL, len(body))
	}
}

// TestUpstream_RejectsOversizedBody 超过 1 MiB 上限 → 413，不打上游。
func TestUpstream_RejectsOversizedBody(t *testing.T) {
	hit := false
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstreamServer.Close)

	up, err := NewUpstream(upstreamServer.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewUpstream: %v", err)
	}

	big := strings.Repeat("x", maxProxyBody+1)
	req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/posts", strings.NewReader(big))
	rr := httptest.NewRecorder()

	up.Handler()(rr, req)

	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413", rr.Code)
	}
	if hit {
		t.Fatal("oversized body must not reach upstream")
	}
}

// TestUpstream_EmptyBodyPost 空体 POST 也以定长发出去（不发 chunked）。
func TestUpstream_EmptyBodyPost(t *testing.T) {
	var gotTE string
	upstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(r.TransferEncoding) > 0 {
			gotTE = r.TransferEncoding[0]
		} else {
			gotTE = "identity"
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(upstreamServer.Close)

	up, err := NewUpstream(upstreamServer.URL, 2*time.Second)
	if err != nil {
		t.Fatalf("NewUpstream: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "http://gateway.local/api/v1/topics/1/follow", nil)
	rr := httptest.NewRecorder()

	up.Handler()(rr, req)

	if rr.Code != http.StatusOK || gotTE != "identity" {
		t.Fatalf("status=%d transfer=%q, want 200 + identity", rr.Code, gotTE)
	}
}
