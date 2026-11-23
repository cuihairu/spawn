package courierdownload

// GP-002 download 能力查询客户端单测：成功（X-Internal-Token + 路径正确、
// download true/false 透传）、404（未映射 → false 无错）、非 200 → 错误、
// 非 JSON → 错误、传输失败 → 错误、NewClient 空 BaseURL → nil（no-op）。
// 逻辑侧（下载门 403/502/no-op）由 downloadversionlogic 白盒覆盖。

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewClient_EmptyBaseURLNil(t *testing.T) {
	if c := NewClient("", "tok", time.Second); c != nil {
		t.Fatalf("NewClient(empty) = %v, want nil（未接线 no-op）", c)
	}
	if c := NewClient("http://x", "", time.Second); c == nil {
		t.Fatal("NewClient(nonempty) = nil, want client")
	}
}

func TestDownloadEnabled_Success(t *testing.T) {
	var gotToken string
	var gotPaths []string
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotToken = r.Header.Get("X-Internal-Token")
		gotPaths = append(gotPaths, r.URL.Path)
		if r.URL.Path == "/internal/games/on/mapping" {
			_, _ = w.Write([]byte(`{"capabilities":{"download":true}}`))
			return
		}
		_, _ = w.Write([]byte(`{"capabilities":{"download":false}}`))
	}))
	defer stub.Close()

	c := NewClient(stub.URL, "s2s-tok", time.Second)
	if got, err := c.DownloadEnabled("on"); err != nil || !got {
		t.Fatalf("on = %v err=%v, want true", got, err)
	}
	if got, err := c.DownloadEnabled("off"); err != nil || got {
		t.Fatalf("off = %v err=%v, want false", got, err)
	}
	if gotToken != "s2s-tok" {
		t.Fatalf("X-Internal-Token = %q, want s2s-tok", gotToken)
	}
	if len(gotPaths) != 2 || gotPaths[0] != "/internal/games/on/mapping" ||
		gotPaths[1] != "/internal/games/off/mapping" {
		t.Fatalf("paths = %v", gotPaths)
	}
}

func TestDownloadEnabled_NotFoundIsDisabled(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"message":"游戏未接入 Courier"}`))
	}))
	defer stub.Close()

	got, err := NewClient(stub.URL, "tok", time.Second).DownloadEnabled("nomap")
	if err != nil || got {
		t.Fatalf("nomap = %v err=%v, want false（未映射=未开通）", got, err)
	}
}

func TestDownloadEnabled_Non200(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`boom`))
	}))
	defer stub.Close()

	if _, err := NewClient(stub.URL, "tok", time.Second).DownloadEnabled("g"); err == nil {
		t.Fatal("err = nil, want non-200 error")
	}
}

func TestDownloadEnabled_BadJSON(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	defer stub.Close()

	if _, err := NewClient(stub.URL, "tok", time.Second).DownloadEnabled("g"); err == nil {
		t.Fatal("err = nil, want bad-JSON error")
	}
}

func TestDownloadEnabled_TransportError(t *testing.T) {
	c := NewClient("http://127.0.0.1:1", "tok", 50*time.Millisecond)
	if _, err := c.DownloadEnabled("g"); err == nil {
		t.Fatal("err = nil, want transport error")
	}
}

// 防回归：ErrMappingNotFound 仍导出（下游可判未映射）。
func TestErrMappingNotFoundExported(t *testing.T) {
	if !errors.Is(ErrMappingNotFound, ErrMappingNotFound) {
		t.Fatal("ErrMappingNotFound must be a sentinel")
	}
}
