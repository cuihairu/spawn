package games

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

// TestGameProxyRoutesPassthrough 真实 rest server + httptest 上游：
// 验证列表 query（keyword/limit/offset）与详情路径参数的透传。
func TestGameProxyRoutesPassthrough(t *testing.T) {
	type hit struct{ method, path, query string }
	var hits []hit
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, hit{r.Method, r.URL.Path, r.URL.RawQuery})
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/games/nope" {
			w.WriteHeader(http.StatusNotFound)
			io.WriteString(w, `{"code":404,"message":"游戏不存在"}`)
			return
		}
		io.WriteString(w, `{"proxied":true}`)
	}))
	t.Cleanup(upstream.Close)

	var cfg config.Config
	cfg.Upstreams.GameCatalog.BaseURL = upstream.URL
	cfg.Upstreams.GameCatalog.Timeout = 2000
	serverCtx := &svc.ServiceContext{Config: cfg}

	l := startTestServer(t, serverCtx)

	// 列表 + 搜索/分页 query 原样透传
	resp, err := http.Get(l + "/games?keyword=rpg&limit=20&offset=0")
	if err != nil {
		t.Fatalf("GET /games: %v", err)
	}
	body := readAll(t, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || body != `{"proxied":true}` {
		t.Fatalf("list status=%d body=%q", resp.StatusCode, body)
	}
	last := hits[len(hits)-1]
	if last.path != "/games" || last.query != "keyword=rpg&limit=20&offset=0" {
		t.Fatalf("list hit = %+v", last)
	}

	// 详情：路径参数透传
	resp, err = http.Get(l + "/games/g-1")
	if err != nil {
		t.Fatalf("GET /games/g-1: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("detail status=%d", resp.StatusCode)
	}
	last = hits[len(hits)-1]
	if last.path != "/games/g-1" {
		t.Fatalf("detail hit = %+v", last)
	}

	// 缺失 → 上游 404 原样透传
	resp, err = http.Get(l + "/games/nope")
	if err != nil {
		t.Fatalf("GET /games/nope: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 404 {
		t.Fatalf("missing detail status=%d, want 404", resp.StatusCode)
	}

	// 运营标签维护：PATCH 方法 + 路径 + Authorization 透传到上游
	req, _ := http.NewRequest(http.MethodPatch, l+"/games/g-1/tags", nil)
	req.Header.Set("Authorization", "Bearer tkn")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH /games/g-1/tags: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("PATCH tags status=%d", resp.StatusCode)
	}
	last = hits[len(hits)-1]
	if last.method != http.MethodPatch || last.path != "/games/g-1/tags" {
		t.Fatalf("PATCH tags hit = %+v", last)
	}
}

// TestGameProxyRoutesGP401Passthrough GP-401 评价/收藏路由透传：
// 评分/长评/举报/开发者回复/举报处置/收藏与我的收藏，方法与路径原样到达上游。
func TestGameProxyRoutesGP401Passthrough(t *testing.T) {
	type hit struct{ method, path string }
	var hits []hit
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, hit{r.Method, r.URL.Path})
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(upstream.Close)

	var cfg config.Config
	cfg.Upstreams.GameCatalog.BaseURL = upstream.URL
	cfg.Upstreams.GameCatalog.Timeout = 2000
	serverCtx := &svc.ServiceContext{Config: cfg}
	l := startTestServer(t, serverCtx)

	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/games/g-1/reviews?limit=10&offset=0", ""},
		{http.MethodPost, "/games/g-1/rating", `{"rating":5}`},
		{http.MethodPost, "/games/g-1/reviews", `{"content":"内容不少于十个字，验证透传。"}`},
		{http.MethodPut, "/games/g-1/reviews/7", `{"content":"作者改评的正文内容。"}`},
		{http.MethodPost, "/games/g-1/reviews/7/report", `{"reason":"疑似抄袭"}`},
		{http.MethodPost, "/partner/games/g-1/reviews/7/reply", `{"content":"感谢反馈，下一版本优化。"}`},
		{http.MethodGet, "/admin/review-reports?status=pending", ""},
		{http.MethodPost, "/admin/review-reports/9/handle", `{"action":"dismiss"}`},
		{http.MethodPost, "/games/g-1/favorite", ""},
		{http.MethodDelete, "/games/g-1/favorite", ""},
		{http.MethodGet, "/me/favorites", ""},
	}
	for _, rc := range routes {
		var req *http.Request
		if rc.body == "" {
			req, _ = http.NewRequest(rc.method, l+rc.path, nil)
		} else {
			req, _ = http.NewRequest(rc.method, l+rc.path, strings.NewReader(rc.body))
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", rc.method, rc.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
			t.Fatalf("%s %s status=%d", rc.method, rc.path, resp.StatusCode)
		}
		last := hits[len(hits)-1]
		// r.URL.Path 不含 query：期望路径在 ? 处截断后对比
		wantPath := rc.path
		if i := strings.IndexByte(wantPath, '?'); i >= 0 {
			wantPath = wantPath[:i]
		}
		if last.method != rc.method || last.path != wantPath {
			t.Fatalf("%s %s hit = %+v", rc.method, rc.path, last)
		}
	}
}

// TestGameProxyRoutesGP403Passthrough GP-403 单层渠道推广路由透传：
// 推广链接落地/渠道包下载（公开）与渠道登记/变更/启停/效果计数（管理面）
// 方法与路径原样到达 game-catalog；S2S 归因解析故意不进网关。
func TestGameProxyRoutesGP403Passthrough(t *testing.T) {
	type hit struct{ method, path string }
	var hits []hit
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, hit{r.Method, r.URL.Path})
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(upstream.Close)

	var cfg config.Config
	cfg.Upstreams.GameCatalog.BaseURL = upstream.URL
	cfg.Upstreams.GameCatalog.Timeout = 2000
	serverCtx := &svc.ServiceContext{Config: cfg}
	l := startTestServer(t, serverCtx)

	routes := []struct{ method, path, body string }{
		{http.MethodGet, "/promo/resolve?channel=kol-li&click_id=c-1", ""},
		{http.MethodPost, "/promo/download-click?channel=kol-li&game_id=g-1", ""},
		{http.MethodPost, "/admin/promo/channels", `{"code":"kol-li","name":"主播李"}`},
		{http.MethodGet, "/admin/promo/channels?limit=10", ""},
		{http.MethodPatch, "/admin/promo/channels/3", `{"name":"新名字","window_hours":120}`},
		{http.MethodPost, "/admin/promo/channels/3/status", `{"status":"disabled"}`},
		{http.MethodGet, "/admin/promo/channels/kol-li/stats", ""},
	}
	for _, rc := range routes {
		var req *http.Request
		if rc.body == "" {
			req, _ = http.NewRequest(rc.method, l+rc.path, nil)
		} else {
			req, _ = http.NewRequest(rc.method, l+rc.path, strings.NewReader(rc.body))
			req.Header.Set("Content-Type", "application/json")
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", rc.method, rc.path, err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 {
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

	// S2S 归因解析走内部令牌守卫，不经网关暴露：404 且上游未收到
	before := len(hits)
	resp, err := http.Post(l+"/internal/promo/clicks/c-1/resolve", "application/json",
		strings.NewReader(`{"registered_at":"2026-11-12T10:00:00Z","user_id":7}`))
	if err != nil {
		t.Fatalf("internal resolve via gateway: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("internal promo resolve must not be proxied (status=200)")
	}
	if len(hits) != before {
		t.Fatalf("internal promo resolve reached upstream: hits=%+v", hits[before:])
	}
}

// TestRegisterGameProxyRoutesPanics 非法上游地址 → NewUpstream 失败 panic。
func TestRegisterGameProxyRoutesPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("invalid upstream base url must panic at registration")
		}
	}()

	var cfg config.Config
	cfg.Upstreams.GameCatalog.BaseURL = "/relative/only"
	cfg.Upstreams.GameCatalog.Timeout = 1000
	RegisterGameProxyRoutes(rest.MustNewServer(rest.RestConf{}), &svc.ServiceContext{Config: cfg})
}

func startTestServer(t *testing.T, serverCtx *svc.ServiceContext) string {
	t.Helper()
	port := freePort(t)
	var conf rest.RestConf
	conf.Port = port
	server := rest.MustNewServer(conf)
	RegisterGameProxyRoutes(server, serverCtx)
	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/readyz") // 未注册路径：探活但不经代理
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return base
		}
		if time.Now().After(deadline) {
			t.Fatal("proxy test server did not become ready")
		}
		time.Sleep(20 * time.Millisecond)
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

func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(b)
}
