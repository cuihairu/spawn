package realtime

// 发布器失败臂收口：hub 返非 200 / 传输失败 / NewRequest 失败均仅记日志
// （fire-and-forget 契约），不上抛、不 panic。

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestPublishHubError500 hub 返 500：仅记日志（fire-and-forget）。
func TestPublishHubError500(t *testing.T) {
	hit := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit <- struct{}{}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	NewPublisher(srv.URL, "tok").Publish("evt-500", []int64{7}, map[string]any{"k": 1})
	select {
	case <-hit:
	case <-time.After(2 * time.Second):
		t.Fatal("hub stub not hit")
	}
	time.Sleep(100 * time.Millisecond) // 投递 goroutine 收敛后再退出（覆盖计数落盘）
}

// TestPublishTransportError 连接拒绝：传输失败仅记日志。
func TestPublishTransportError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := l.Addr().String()
	_ = l.Close() // 释放端口 → connection refused

	NewPublisher("http://"+addr, "tok").Publish("evt-refused", []int64{7}, nil)
	time.Sleep(100 * time.Millisecond) // 投递 goroutine 收敛后再退出
}

// TestPublishBadURLNewRequest 白盒：URL 含控制字符使 NewRequest 失败臂。
func TestPublishBadURLNewRequest(t *testing.T) {
	p := &publisher{url: "http://a\x01b/internal/publish", token: "tok",
		client: &http.Client{Timeout: 3 * time.Second}}
	p.Publish("evt-badurl", []int64{7}, map[string]any{"k": 1})
	time.Sleep(100 * time.Millisecond) // 投递 goroutine 收敛后再退出
}
