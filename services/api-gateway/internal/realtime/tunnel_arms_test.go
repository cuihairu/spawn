package realtime

// 隧道失败臂收口：NewTunnel 三处拒绝臂与超时默认、非 Hijacker writer、
// Hijack 失败、backend 写头失败、tunnelRequestHead 跳过客户端转发头、
// 注册期坏 URL fail-fast。 Hijack 前后各臂在 go-zero 中间件包装的
// writer 上不可复现，这里直调 handler（绕开 server 链）驱动。

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/zeromicro/go-zero/rest"
)

// TestNewTunnelRejectsBadURL 控制字符 URL → url.Parse 失败。
func TestNewTunnelRejectsBadURL(t *testing.T) {
	if _, err := NewTunnel("http://a\x01b/", time.Second); err == nil {
		t.Fatal("control-char url accepted, want parse error")
	}
}

// TestNewTunnelRejectsNonHTTP 非 http scheme / 空主机 → 拒绝。
func TestNewTunnelRejectsNonHTTP(t *testing.T) {
	if _, err := NewTunnel("https://hub.example", time.Second); err == nil {
		t.Fatal("https accepted, want error")
	}
	if _, err := NewTunnel("http:", time.Second); err == nil {
		t.Fatal("hostless url accepted, want error")
	}
}

// TestNewTunnelDefaultTimeout timeout<=0 回退 5s。
func TestNewTunnelDefaultTimeout(t *testing.T) {
	tn, err := NewTunnel("http://127.0.0.1:1", 0)
	if err != nil || tn.dialTimeout != 5*time.Second {
		t.Fatalf("dialTimeout = %v err = %v, want 5s default", tn.dialTimeout, err)
	}
}

// rawBackend 只 accept 后立即关闭的原始后端（无协议语义）。
func rawBackend(t *testing.T) string {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { _ = l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			_ = c.Close()
		}
	}()
	return l.Addr().String()
}

// TestTunnelNonHijackerWriter writer 不支持升级：关 backend 后回 500 JSON。
func TestTunnelNonHijackerWriter(t *testing.T) {
	tn, err := NewTunnel("http://"+rawBackend(t), time.Second)
	if err != nil {
		t.Fatalf("new tunnel: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/ws?token=abc", nil)
	rw := httptest.NewRecorder() // 无 Hijack 能力
	tn.Handler()(rw, req)
	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rw.Code)
	}
	if !strings.Contains(rw.Body.String(), "upgrade") {
		t.Fatalf("body = %q, want upgrade error message", rw.Body.String())
	}
}

// hijackFailWriter 声明 Hijack 但必失败：驱动 Hijack 错误臂。
type hijackFailWriter struct {
	http.ResponseWriter
}

func (w *hijackFailWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	return nil, nil, errors.New("hijack boom")
}

// TestTunnelHijackError Hijack 出错：双侧连接关闭、handler 静默返回（不写响应）。
func TestTunnelHijackError(t *testing.T) {
	tn, err := NewTunnel("http://"+rawBackend(t), time.Second)
	if err != nil {
		t.Fatalf("new tunnel: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/ws?token=abc", nil)
	rw := &hijackFailWriter{ResponseWriter: httptest.NewRecorder()}
	tn.Handler()(rw, req)
	rec := rw.ResponseWriter.(*httptest.ResponseRecorder)
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("hijack-error arm must return silently, got code=%d body=%q",
			rec.Code, rec.Body.String())
	}
}

// pipeHijacker Hijack 成功并返回内存管道：配合 accept 即关后端驱动
// backend.Write 失败臂（8MB 头必超 socket 缓冲，写阻塞至对端 RST）。
type pipeHijacker struct {
	http.ResponseWriter
}

func (w *pipeHijacker) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	c1, _ := net.Pipe()
	return c1, bufio.NewReadWriter(bufio.NewReader(c1), bufio.NewWriter(c1)), nil
}

// TestTunnelBackendWriteError 后端 accept 即关：写隧道头失败 → 双侧关、静默返回。
func TestTunnelBackendWriteError(t *testing.T) {
	tn, err := NewTunnel("http://"+rawBackend(t), time.Second)
	if err != nil {
		t.Fatalf("new tunnel: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/ws?token=abc", nil)
	req.Header.Set("X-Bulk", strings.Repeat("x", 8<<20)) // 超 wmem 缓冲 → 写必阻塞至 RST
	rw := &pipeHijacker{ResponseWriter: httptest.NewRecorder()}
	tn.Handler()(rw, req)
	rec := rw.ResponseWriter.(*httptest.ResponseRecorder)
	if rec.Code != http.StatusOK || rec.Body.Len() != 0 {
		t.Fatalf("write-error arm must return silently, got code=%d body=%q",
			rec.Code, rec.Body.String())
	}
}

// TestTunnelRequestHeadSkipsClientForwardHeaders host/XFF/X-Real-Ip 客户端
// 自带头被跳过（网关按 RemoteAddr 重写），其余头原样透传。
func TestTunnelRequestHeadSkipsClientForwardHeaders(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ws?token=abc", nil)
	req.RemoteAddr = "10.1.2.3:5555"
	req.Header.Set("X-Forwarded-For", "9.9.9.9")
	req.Header.Set("X-Real-Ip", "8.8.8.8")
	req.Header.Set("Origin", "http://localhost:4173")
	head := string(tunnelRequestHead(req, "hub.internal:7800"))

	if !strings.HasPrefix(head, "GET /ws?token=abc HTTP/1.1\r\nHost: hub.internal:7800\r\n") {
		t.Fatalf("request line/host rewritten wrong: %q", head)
	}
	if !strings.Contains(head, "X-Forwarded-For: 10.1.2.3\r\nX-Real-Ip: 10.1.2.3\r\n") {
		t.Fatalf("gateway XFF/X-Real-Ip from RemoteAddr missing: %q", head)
	}
	if strings.Contains(head, "9.9.9.9") || strings.Contains(head, "8.8.8.8") {
		t.Fatalf("client-supplied forwarding headers must be skipped: %q", head)
	}
	if !strings.Contains(head, "Origin: http://localhost:4173\r\n") {
		t.Fatalf("other headers must pass through: %q", head)
	}
}

// TestRegisterTunnelRoutesPanicsOnBadURL 注册期坏 URL → panic fail-fast
// （配置错误带崩网关启动，不带病运行）。
func TestRegisterTunnelRoutesPanicsOnBadURL(t *testing.T) {
	var conf rest.RestConf
	conf.Port = freePort(t)
	server := rest.MustNewServer(conf)
	defer server.Stop()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("register with bad url did not panic")
		}
	}()
	RegisterRealtimeTunnelRoutes(server, tunnelCtx("http://a\x01b/"))
}
