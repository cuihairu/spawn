package users

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

// TestUserProxyRoutesPassthrough 真实 rest server + httptest 上游：
// 验证注册路由、方法、路径、Bearer/请求体透传与响应体原样返回。
func TestUserProxyRoutesPassthrough(t *testing.T) {
	type hit struct{ method, path, auth, body string }
	var hits []hit
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		hits = append(hits, hit{r.Method, r.URL.Path, r.Header.Get("Authorization"), string(b)})
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/users/7" && r.Header.Get("Authorization") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			io.WriteString(w, `{"code":401,"message":"missing authorization token"}`)
			return
		}
		io.WriteString(w, `{"proxied":true}`)
	}))
	t.Cleanup(upstream.Close)

	var cfg config.Config
	cfg.Upstreams.UserService.BaseURL = upstream.URL
	cfg.Upstreams.UserService.Timeout = 2000
	serverCtx := &svc.ServiceContext{Config: cfg}

	l := startTestServer(t, serverCtx)

	// 注册：信封响应原样透传（业务错误走 HTTP 200 + code != 200，客户端语义不变）
	resp, err := http.Post(l+"/auth/register", "application/json",
		strings.NewReader(`{"username":"alice","email":"a@x.dev","password":"pw"}`))
	if err != nil {
		t.Fatalf("POST: %v", err)
	}
	body := readAll(t, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || body != `{"proxied":true}` {
		t.Fatalf("register status=%d body=%q", resp.StatusCode, body)
	}
	last := hits[len(hits)-1]
	if last.method != http.MethodPost || last.path != "/auth/register" ||
		!strings.Contains(last.body, `"username":"alice"`) {
		t.Fatalf("register hit = %+v", last)
	}

	// 资料：Bearer 透传
	req, _ := http.NewRequest(http.MethodGet, l+"/users/7", nil)
	req.Header.Set("Authorization", "Bearer gw-token")
	profResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /users/7: %v", err)
	}
	body = readAll(t, profResp.Body)
	profResp.Body.Close()
	if profResp.StatusCode != 200 || body != `{"proxied":true}` {
		t.Fatalf("profile status=%d body=%q", profResp.StatusCode, body)
	}
	last = hits[len(hits)-1]
	if last.method != http.MethodGet || last.path != "/users/7" || last.auth != "Bearer gw-token" {
		t.Fatalf("profile hit = %+v", last)
	}

	// 匿名资料 → 上游 401 原样透传（App 的 401 统一跳转依赖状态码）
	anonResp, err := http.Get(l + "/users/7")
	if err != nil {
		t.Fatalf("GET anon: %v", err)
	}
	anonResp.Body.Close()
	if anonResp.StatusCode != 401 {
		t.Fatalf("anon profile status=%d, want 401", anonResp.StatusCode)
	}
}

// TestRegisterUserProxyRoutesPanics 非法上游地址 → NewUpstream 失败 panic。
func TestRegisterUserProxyRoutesPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("invalid upstream base url must panic at registration")
		}
	}()

	var cfg config.Config
	cfg.Upstreams.UserService.BaseURL = "/relative/only"
	cfg.Upstreams.UserService.Timeout = 1000
	RegisterUserProxyRoutes(rest.MustNewServer(rest.RestConf{}), &svc.ServiceContext{Config: cfg})
}

func startTestServer(t *testing.T, serverCtx *svc.ServiceContext) string {
	t.Helper()
	port := freePort(t)
	var conf rest.RestConf
	conf.Port = port
	server := rest.MustNewServer(conf)
	RegisterUserProxyRoutes(server, serverCtx)
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
