package community

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

// TestCommunityProxyRoutesPassthrough 真实 rest server + httptest 上游：
// 验证 RegisterCommunityProxyRoutes 的代理透传（方法、路径、请求体、响应体）。
func TestCommunityProxyRoutesPassthrough(t *testing.T) {
	type hit struct{ method, path, body string }
	var hits []hit
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		hits = append(hits, hit{r.Method, r.URL.Path, string(b)})
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"proxied":true}`)
	}))
	t.Cleanup(upstream.Close)

	var cfg config.Config
	cfg.Upstreams.Community.BaseURL = upstream.URL
	cfg.Upstreams.Community.Timeout = 2000
	serverCtx := &svc.ServiceContext{Config: cfg}

	l := startTestServer(t, serverCtx)

	// GET 列表
	resp, err := http.Get(l + "/api/v1/topics")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body := readAll(t, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || body != `{"proxied":true}` {
		t.Fatalf("GET status=%d body=%q", resp.StatusCode, body)
	}
	if len(hits) != 1 || hits[0].method != http.MethodGet || hits[0].path != "/api/v1/topics" {
		t.Fatalf("hits = %+v", hits)
	}

	// POST 创建（带请求体）
	postResp, err := http.Post(l+"/api/v1/topics", "application/json",
		strings.NewReader(`{"name":"n"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	postResp.Body.Close()
	if postResp.StatusCode != 200 {
		t.Fatalf("POST status=%d", postResp.StatusCode)
	}
	last := hits[len(hits)-1]
	if last.method != http.MethodPost || last.path != "/api/v1/topics" || last.body != `{"name":"n"}` {
		t.Fatalf("last hit = %+v", last)
	}
}

func startTestServer(t *testing.T, serverCtx *svc.ServiceContext) string {
	t.Helper()
	port := freePort(t)
	var conf rest.RestConf
	conf.Port = port
	server := rest.MustNewServer(conf)
	RegisterCommunityProxyRoutes(server, serverCtx)
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
