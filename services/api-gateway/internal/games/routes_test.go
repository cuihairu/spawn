package games

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
