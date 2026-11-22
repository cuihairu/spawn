package logic

// platform.go 剩余补漏分支：platformRole / catalogServerOpen 的 http.NewRequest
// 失败臂（base 含非法字符 → url.Parse 失败）、响应体读取失败臂（服务端声明
// Content-Length 后提前断连 → unexpected EOF），以及 requirePartnerRole 的
// 角色服务不可用 502 臂。

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// badURLBase 构造必然 url.Parse 失败的 base（主机名含空格）。
const badURLBase = "http://exa mple.com"

func TestPlatformRole_NewRequestArms(t *testing.T) {
	if _, _, err := platformRole(t.Context(), testSvc(badURLBase), "Bearer t"); err == nil {
		t.Fatal("bad url base = nil error, want error")
	}
	if _, err := catalogServerOpen(t.Context(), testSvc(badURLBase), "game-a", 1); err == nil {
		t.Fatal("servers bad url base = nil error, want error")
	}
}

// truncatingCatalog 返回 200 且声明 Content-Length=100，但只写几个字节就断连，
// 使客户端 io.ReadAll 返回 unexpected EOF。
func truncatingCatalog(t *testing.T) *httptest.Server {
	t.Helper()
	st := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("hijacker unavailable")
			return
		}
		conn, buf, _ := hj.Hijack()
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\nshort")
		_ = buf.Flush() // 先把半截响应冲给客户端，再断连
		_ = conn.Close()
	}))
	t.Cleanup(st.Close)
	return st
}

func TestPlatformRole_ReadErrorArms(t *testing.T) {
	st := truncatingCatalog(t)
	if _, _, err := platformRole(t.Context(), testSvc(st.URL), "Bearer t"); err == nil {
		t.Fatal("truncated /me = nil error, want error")
	}
	st2 := truncatingCatalog(t)
	if _, err := catalogServerOpen(t.Context(), testSvc(st2.URL), "game-a", 1); err == nil {
		t.Fatal("truncated servers = nil error, want error")
	}
}

func TestRequirePartnerRole_ServiceDown(t *testing.T) {
	// 目录不可达 → 502（角色服务不可用）
	if _, err := requirePartnerRole(t.Context(), testSvc("http://127.0.0.1:1"), "Bearer t"); err == nil {
		t.Fatal("catalog down = nil error, want 502")
	}
}
