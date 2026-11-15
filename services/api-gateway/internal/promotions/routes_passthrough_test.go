package promotions

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/tappi/tappi/services/api-gateway/internal/config"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// TestRegisterPromotionsRoutesGP301Passthrough GP-301 活动运行时消费透传：
// 玩家侧 GET /games/:gameId/activities 原样到达 promotions 上游（公开读，
// 鉴权与窗口过滤在 promotions 侧）。游戏服侧 S2S
// /internal/games/:gameId/activities 为 X-Internal-Token 专用，不经网关。
func TestRegisterPromotionsRoutesGP301Passthrough(t *testing.T) {
	type hit struct{ method, path string }
	var hits []hit
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, hit{r.Method, r.URL.Path})
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"items":[]}`)
	}))
	t.Cleanup(upstream.Close)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	var cfg config.Config
	cfg.Upstreams.Promotions.BaseURL = upstream.URL
	cfg.Upstreams.Promotions.Timeout = 2000

	var conf rest.RestConf
	conf.Port = port
	server := rest.MustNewServer(conf)
	RegisterPromotionsRoutes(server, &svc.ServiceContext{Config: cfg})
	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/admin/activities")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("promotions proxy server not ready")
		}
		time.Sleep(20 * time.Millisecond)
	}

	before := len(hits)
	resp, err := http.Get(base + "/games/game-a/activities")
	if err != nil {
		t.Fatalf("player activities via gateway: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("player activities status=%d", resp.StatusCode)
	}
	if len(hits) != before+1 || hits[len(hits)-1] != (hit{http.MethodGet, "/games/game-a/activities"}) {
		t.Fatalf("upstream hits = %+v, want one GET /games/game-a/activities", hits[before:])
	}

	// 游戏服侧快照为 S2S 专用，不经网关暴露
	resp, err = http.Get(base + "/internal/games/game-a/activities")
	if err != nil {
		t.Fatalf("internal activities via gateway: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("internal activities must not be proxied (status=200)")
	}
	if len(hits) != before+1 {
		t.Fatalf("internal activities reached upstream: hits=%+v", hits[before+1:])
	}
}
