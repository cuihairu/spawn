// Package realtime 网关 WS 隧道：/ws 原始字节透传至 realtime-hub。
// 缓冲式反代（internal/proxy）带不动 WS upgrade，这里握手前后都不
// 解析不缓冲：Hijack 客户端连接 → 原样转发请求头 → 双向 splice，
// JWT 校验仍由 hub 在升级时完成（信任边界不变，网关只补 X-Forwarded-For）。
package realtime

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Tunnel realtime-hub 的原始透传隧道。
type Tunnel struct {
	hostPort    string
	dialTimeout time.Duration
}

// NewTunnel base URL 形如 http://host:port（内网明文口径，仅支持 http）。
func NewTunnel(baseURL string, timeout time.Duration) (*Tunnel, error) {
	u, err := url.Parse(strings.TrimRight(strings.TrimSpace(baseURL), "/"))
	if err != nil {
		return nil, fmt.Errorf("parse realtime hub base url: %w", err)
	}
	if u.Scheme != "http" || u.Host == "" {
		return nil, fmt.Errorf("invalid realtime hub base url: %s", baseURL)
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Tunnel{hostPort: u.Host, dialTimeout: timeout}, nil
}

// Handler GET /ws 原始透传。hub 不可达在 Hijack 前回 502（保留正常
// 错误语义）；升级失败/非 WS 请求由 hub 的响应原样流回客户端。
func (t *Tunnel) Handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		backend, err := net.DialTimeout("tcp", t.hostPort, t.dialTimeout)
		if err != nil {
			writeTunnelError(w, http.StatusBadGateway, "realtime hub unavailable")
			return
		}
		hijacker, ok := w.(http.Hijacker)
		if !ok {
			_ = backend.Close()
			writeTunnelError(w, http.StatusInternalServerError, "connection does not support upgrade")
			return
		}
		clientConn, clientRW, err := hijacker.Hijack()
		if err != nil {
			_ = backend.Close()
			return
		}
		if _, err := backend.Write(tunnelRequestHead(r, t.hostPort)); err != nil {
			_ = clientConn.Close()
			_ = backend.Close()
			return
		}
		// 隧道在后台搬运，handler 立即返回（长连接不能挂在 go-zero
		// 超时看门狗的账上）；连接已 Hijack，net/http 不再接管。
		go splice(backend, clientConn, clientRW.Reader)
	}
}

// splice 双向搬运直至任一侧关闭；请求头已写出，之后是纯字节流。
func splice(backend net.Conn, clientConn net.Conn, clientSide io.Reader) {
	defer backend.Close()
	defer clientConn.Close()
	go func() {
		_, _ = io.Copy(backend, clientSide)
		_ = backend.Close() // 让对向 copy 解除阻塞
	}()
	_, _ = io.Copy(clientConn, backend)
}

// tunnelRequestHead 重写请求行与 Host，其余客户端头原样透传——
// Connection/Upgrade/Sec-WebSocket-* 正是升级握手本体，不得剥离
// （与缓冲反代的 hop-by-hop 剥离语义不同：本连接即最终一跳）。
func tunnelRequestHead(r *http.Request, hostPort string) []byte {
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s HTTP/1.1\r\n", r.Method, r.URL.RequestURI())
	fmt.Fprintf(&b, "Host: %s\r\n", hostPort)
	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil && ip != "" {
		fmt.Fprintf(&b, "X-Forwarded-For: %s\r\nX-Real-Ip: %s\r\n", ip, ip)
	}
	for key, values := range r.Header {
		switch strings.ToLower(key) {
		case "host", "x-forwarded-for", "x-real-ip":
			continue
		}
		for _, v := range values {
			fmt.Fprintf(&b, "%s: %s\r\n", key, v)
		}
	}
	b.WriteString("\r\n")
	return []byte(b.String())
}

func writeTunnelError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, fmt.Sprintf(`{"code":%d,"message":%q}`, status, message))
}
