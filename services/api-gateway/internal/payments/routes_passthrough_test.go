package payments

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

// TestRegisterPaymentsRoutesGP404Passthrough GP-404 合作方自助账单面透传：
// 账单列表/详情/导出/确认/异议与运营复核处理方法与路径原样到达 payments 上游
// （角色与 partner_id 行级隔离在 payments 侧）。
func TestRegisterPaymentsRoutesGP404Passthrough(t *testing.T) {
	type hit struct{ method, path string }
	var hits []hit
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, hit{r.Method, r.URL.Path})
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(upstream.Close)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	var cfg config.Config
	cfg.Upstreams.Payments.BaseURL = upstream.URL
	cfg.Upstreams.Payments.Timeout = 2000

	var conf rest.RestConf
	conf.Port = port
	server := rest.MustNewServer(conf)
	RegisterPaymentsRoutes(server, &svc.ServiceContext{Config: cfg})
	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("payments proxy server not ready")
		}
		time.Sleep(20 * time.Millisecond)
	}

	routes := []struct{ method, path string }{
		{http.MethodGet, "/partner/statements"},
		{http.MethodGet, "/partner/statements/differences"},
		{http.MethodGet, "/partner/statements/7"},
		{http.MethodGet, "/partner/statements/7/export"},
		{http.MethodPost, "/partner/statements/7/confirm"},
		{http.MethodPost, "/partner/statements/7/dispute"},
		{http.MethodPost, "/admin/statements/7/resolve"},
	}
	for _, rc := range routes {
		req, err := http.NewRequest(rc.method, base+rc.path, nil)
		if err != nil {
			t.Fatalf("%s %s: %v", rc.method, rc.path, err)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("%s %s: %v", rc.method, rc.path, err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s %s status=%d", rc.method, rc.path, resp.StatusCode)
		}
		last := hits[len(hits)-1]
		if last.method != rc.method || last.path != rc.path {
			t.Fatalf("%s %s hit = %+v", rc.method, rc.path, last)
		}
	}

	// 发货分发 webhook 为 S2S 专用，不经网关暴露
	before := len(hits)
	resp, err := http.Post(base+"/internal/deliveries", "application/json",
		strings.NewReader(`{"orderId":"x"}`))
	if err != nil {
		t.Fatalf("internal deliveries via gateway: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("internal deliveries must not be proxied (status=200)")
	}
	if len(hits) != before {
		t.Fatalf("internal deliveries reached upstream: hits=%+v", hits[before:])
	}
}
