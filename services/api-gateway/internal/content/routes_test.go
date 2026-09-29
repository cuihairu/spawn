package content

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

// TestContentProxyRoutesPassthrough 真实 rest server + httptest 上游：
// 验证 RegisterContentProxyRoutes 的代理透传（方法、路径、请求体、响应体）。
func TestContentProxyRoutesPassthrough(t *testing.T) {
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
	cfg.Upstreams.Content.BaseURL = upstream.URL
	cfg.Upstreams.Content.Timeout = 2000
	serverCtx := &svc.ServiceContext{Config: cfg}

	l := startProxyTestServer(t, serverCtx)

	// GET 列表
	resp, err := http.Get(l + "/api/v1/guides")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	body := readAll(t, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || body != `{"proxied":true}` {
		t.Fatalf("GET status=%d body=%q", resp.StatusCode, body)
	}
	if len(hits) != 1 || hits[0].method != http.MethodGet || hits[0].path != "/api/v1/guides" {
		t.Fatalf("hits = %+v", hits)
	}

	// PUT 更新（带请求体；PUT 无法用 http.Post 发，需 NewRequest）
	req, err := http.NewRequest(http.MethodPut, l+"/api/v1/guides/7",
		strings.NewReader(`{"title":"t"}`))
	if err != nil {
		t.Fatalf("build PUT: %v", err)
	}
	putResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT: %v", err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != 200 {
		t.Fatalf("PUT status=%d", putResp.StatusCode)
	}
	last := hits[len(hits)-1]
	if last.method != http.MethodPut || last.path != "/api/v1/guides/7" || last.body != `{"title":"t"}` {
		t.Fatalf("last hit = %+v", last)
	}
}

func startProxyTestServer(t *testing.T, serverCtx *svc.ServiceContext) string {
	t.Helper()
	lst, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := lst.Addr().(*net.TCPAddr).Port
	_ = lst.Close()

	var conf rest.RestConf
	conf.Port = port
	server := rest.MustNewServer(conf)
	RegisterContentProxyRoutes(server, serverCtx)
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

func readAll(t *testing.T, r io.Reader) string {
	t.Helper()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	return string(b)
}
