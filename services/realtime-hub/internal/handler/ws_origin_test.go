package handler

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestWSCrossOrigin 浏览器跨源握手：准入闸只有 JWT query，Origin 不设限
// （gorilla 默认同源校验会把 web :4173 跨端口连接 403 掉，浏览器路径
// 一旦走网关隧道 Origin 与 hub Host 必然不同源）。
func TestWSCrossOrigin(t *testing.T) {
	srv, h := newTestServer(t)
	d := websocket.Dialer{HandshakeTimeout: 3 * time.Second}
	header := http.Header{"Origin": []string{"http://localhost:4173"}}
	conn, resp, err := d.Dial("ws"+strings.TrimPrefix(srv.URL, "http")+"/ws?token="+signToken(t, 7, time.Hour), header)
	if err != nil {
		t.Fatalf("cross origin dial: %v (resp %+v)", err, resp)
	}
	defer conn.Close()
	waitConnections(t, h, map[int64]int{7: 1})
}
