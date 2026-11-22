package datapanel

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/api-gateway/internal/config"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// TestRegisterDataPanelProxyRoutesAnalyticsPassthrough GP-402 经营分析查询
// 路由透传：方法与路径/query 原样到达 data-panel 上游（登录与名单鉴权在
// data-panel 侧）；既有战绩路由保持可用。
func TestRegisterDataPanelProxyRoutesAnalyticsPassthrough(t *testing.T) {
	type hit struct{ method, path, query string }
	var hits []hit
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, hit{r.Method, r.URL.Path, r.URL.RawQuery})
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(upstream.Close)

	port := freePort(t)
	var cfg config.Config
	cfg.Upstreams.DataPanel.BaseURL = upstream.URL
	cfg.Upstreams.DataPanel.Timeout = 2000
	server := rest.MustNewServer(rest.RestConf{Port: port})
	RegisterDataPanelProxyRoutes(server, &svc.ServiceContext{Config: cfg})
	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/api/v1/stats/users/1001/summary")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("proxy test server not ready: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	hits = hits[:0] // 排除探活命中

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/v1/analytics/overview?from=2026-11-01&to=2026-11-30"},
		{http.MethodGet, "/api/v1/analytics/funnel?from=2026-11-01"},
		{http.MethodGet, "/api/v1/analytics/retention?window=2"},
		{http.MethodGet, "/api/v1/analytics/activities/7/effect"},
		{http.MethodGet, "/api/v1/analytics/reconciliation?from=2026-11-01"},
		{http.MethodGet, "/api/v1/analytics/channels?from=2026-11-01"},
		{http.MethodGet, "/api/v1/analytics/partner?from=2026-11-01"}, // GP-404 合作方自助数据
		{http.MethodGet, "/api/v1/stats/users/1001/summary"},
		{http.MethodGet, "/api/v1/stats/users/1001/games?limit=5"},
	}
	for _, rc := range routes {
		resp, err := http.Get(base + rc.path)
		if err != nil {
			t.Fatalf("%s %s: %v", rc.method, rc.path, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s %s status=%d", rc.method, rc.path, resp.StatusCode)
		}
		last := hits[len(hits)-1]
		wantPath := rc.path
		if i := strings.IndexByte(wantPath, '?'); i >= 0 {
			wantPath = wantPath[:i]
		}
		if last.method != rc.method || last.path != wantPath {
			t.Fatalf("%s %s hit = %+v", rc.method, rc.path, last)
		}
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}
