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
		{http.MethodGet, "/admin/statements/7/disputes"},
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

// TestRegisterPaymentsRoutesCourierRedeliverPassthrough GP-204 断单对账重发
// 管理适配：POST /admin/orders/:orderId/redeliver 原样穿网关到达 payments
// 上游（鉴权/角色/fail-closed 均在 payments 侧）。
func TestRegisterPaymentsRoutesCourierRedeliverPassthrough(t *testing.T) {
	type hit struct{ method, path string }
	var hits []hit
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, hit{r.Method, r.URL.Path})
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"redelivered":true,"courier_status":"DELIVERED"}`)
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

	req, err := http.NewRequest(http.MethodPost, base+"/admin/orders/order_rd_1/redeliver", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("redeliver via gateway: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("redeliver status=%d", resp.StatusCode)
	}
	last := hits[len(hits)-1]
	if last.method != http.MethodPost || last.path != "/admin/orders/order_rd_1/redeliver" {
		t.Fatalf("redeliver hit = %+v", last)
	}
}

// TestRegisterPaymentsRoutesSkuSyncPassthrough GP-202 部署面 SKU 登记：
// POST /admin/games/:gameId/products/sync 原样穿网关到达 payments 上游
// （鉴权/角色/fail-closed 均在 payments 侧）。
func TestRegisterPaymentsRoutesSkuSyncPassthrough(t *testing.T) {
	type hit struct{ method, path string }
	var hits []hit
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits = append(hits, hit{r.Method, r.URL.Path})
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"gameId":"game-a","items":[],"synced":0,"failed":0}`)
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

	req, err := http.NewRequest(http.MethodPost, base+"/admin/games/game-a/products/sync", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("sku sync via gateway: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sku sync status=%d", resp.StatusCode)
	}
	last := hits[len(hits)-1]
	if last.method != http.MethodPost || last.path != "/admin/games/game-a/products/sync" {
		t.Fatalf("sku sync hit = %+v", last)
	}
}

// TestRegisterPaymentsRoutesCourierAuthHeaderPassthrough GP-202 客户端带
// Courier token 决策：玩家下单请求的 X-Courier-Authorization（客户端经
// GP-201 代登录/票据兑换取得的 Courier 会话令牌）必须原样穿网关到达
// payments 上游——payments 以它为转发 Courier 的 Bearer，绝不用平台 JWT。
func TestRegisterPaymentsRoutesCourierAuthHeaderPassthrough(t *testing.T) {
	var gotAuth, gotCourier string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotCourier = r.Header.Get("X-Courier-Authorization")
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

	req, err := http.NewRequest(http.MethodPost, base+"/games/game-a/orders", strings.NewReader(`{"sku_id":"sku-1"}`))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer platform-jwt")
	req.Header.Set("X-Courier-Authorization", "Bearer courier-session-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("post order: %v", err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("post order status=%d", resp.StatusCode)
	}
	if gotCourier != "Bearer courier-session-token" {
		t.Fatalf("X-Courier-Authorization passthrough = %q", gotCourier)
	}
	if gotAuth != "Bearer platform-jwt" {
		t.Fatalf("Authorization passthrough = %q", gotAuth)
	}
}
