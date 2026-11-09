package proxy

import (
	"bytes"
	"crypto/rand"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// GP-105 包体通道：流式透传不受 JSON 反代的 1 MiB 缓冲与 5s 整体超时约束。
func TestStreamingHandler_LargeBodyPassthrough(t *testing.T) {
	// 上游回显请求体大小与 Range 头
	var gotLength int64
	var gotRange string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotLength = int64(len(b))
		gotRange = r.Header.Get("Range")
		w.Header().Set("Content-Type", "application/vnd.android.package-archive")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(b)
	}))
	defer upstream.Close()

	u, err := NewUpstream(upstream.URL, 200*time.Millisecond)
	if err != nil {
		t.Fatalf("new upstream: %v", err)
	}

	// 1 MiB + 1 字节：JSON 反代会 413，流式通道必须原样通过
	payload := make([]byte, maxProxyBody+1)
	if _, err := rand.Read(payload); err != nil {
		t.Fatalf("rand: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/partner/games/g1/versions/ver-1/package", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=x")
	req.Header.Set("Range", "bytes=100-") // 下载方向的续传头同样要透传
	rec := httptest.NewRecorder()

	u.StreamingHandler()(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if gotLength != int64(len(payload)) {
		t.Fatalf("upstream got %d bytes, want %d (1MiB cap leaked into streaming route)", gotLength, len(payload))
	}
	if gotRange != "bytes=100-" {
		t.Fatalf("Range header = %q, want forwarded", gotRange)
	}
	body := rec.Body.Bytes()
	if int64(len(body)) != int64(len(payload)) || !bytes.Equal(body, payload) {
		t.Fatalf("response truncated/mismatch: got %d bytes", len(body))
	}
}

func TestStreamingHandler_UpstreamUnavailable(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	base := upstream.URL
	upstream.Close() // 立即关闭制造不可达

	u, err := NewUpstream(base, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("new upstream: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/games/g1/versions/ver-1/download", nil)
	rec := httptest.NewRecorder()
	u.StreamingHandler()(rec, req)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "upstream unavailable") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}
