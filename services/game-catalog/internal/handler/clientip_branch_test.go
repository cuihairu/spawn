package handler

// clientIP 的无端口回退臂：RemoteAddr 无 host:port 形式（SplitHostPort 失败）
// → 原样返回远端地址。

import (
	"net/http"
	"testing"
)

func TestClientIP_NoPortFallback(t *testing.T) {
	req := &http.Request{RemoteAddr: "10.0.0.7"} // 无端口
	if got := clientIP(req); got != "10.0.0.7" {
		t.Fatalf("clientIP no port = %q, want 10.0.0.7", got)
	}
	req = &http.Request{RemoteAddr: "10.0.0.7:8080"}
	if got := clientIP(req); got != "10.0.0.7" {
		t.Fatalf("clientIP with port = %q, want 10.0.0.7", got)
	}
}
