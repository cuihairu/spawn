package handler

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tappi/tappi/services/api-gateway/internal/adapter"
	"github.com/tappi/tappi/services/api-gateway/internal/community"
	"github.com/tappi/tappi/services/api-gateway/internal/config"
	"github.com/tappi/tappi/services/api-gateway/internal/content"
	"github.com/tappi/tappi/services/api-gateway/internal/datapanel"
	"github.com/tappi/tappi/services/api-gateway/internal/games"
	"github.com/tappi/tappi/services/api-gateway/internal/payments"
	"github.com/tappi/tappi/services/api-gateway/internal/promotions"
	"github.com/tappi/tappi/services/api-gateway/internal/realtime"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/tappi/tappi/services/api-gateway/internal/users"

	"github.com/zeromicro/go-zero/rest"
)

// pathLog 记录上游收到的 (method, path)；三个审计域各一个独立 stub。
type pathLog struct {
	mu    sync.Mutex
	calls []string
}

func (l *pathLog) add(method, path string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, method+" "+path)
}

func (l *pathLog) last(t *testing.T) string {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.calls) == 0 {
		t.Fatal("上游没有收到任何调用")
	}
	return l.calls[len(l.calls)-1]
}

func (l *pathLog) count(t *testing.T) int {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.calls)
}

// TestGateway_AllRouteGroupsBoot 全上游组装网：gateway.go 生产装配按同一
// 顺序注册全部九个路由组（含默认配置启用的 adapter/payments/promotions/
// realtime 可选组），断言网关可启动。2026-11-09 起 adapter 与 payments 组
// 各自代理上游的 GET /admin/audit，与 games 组（game-catalog）同名——三组
// 同时注册让 rest server 启动即 panic，而当时单组集成测试各自只装配本组、
// 门禁全绿拦不住。本测试就是那张缺失的全量装配网：新路由组进入生产装配
// 时必须同步加进这里（或复用本测试的注册序列）。
func TestGateway_AllRouteGroupsBoot(t *testing.T) {
	gameLog := &pathLog{}
	courierLog := &pathLog{}
	paymentsLog := &pathLog{}
	gameStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gameLog.add(r.Method, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(gameStub.Close)
	courierStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		courierLog.add(r.Method, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(courierStub.Close)
	paymentsStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paymentsLog.add(r.Method, r.URL.Path)
		w.WriteHeader(http.StatusOK)
		_, _ = io.WriteString(w, `{"ok":true}`)
	}))
	t.Cleanup(paymentsStub.Close)

	var c config.Config
	c.Name = "api-gateway-fullboot"
	c.Host = "127.0.0.1"
	c.Port = freePort(t)
	c.Auth.JWTSecret = "fullboot-test-secret"
	c.Upload.Dir = t.TempDir()
	c.Upload.MaxBytes = 10485760
	c.Upstreams.UserService.BaseURL = "http://127.0.0.1:1"
	c.Upstreams.UserService.Timeout = 1000
	c.Upstreams.GameCatalog.BaseURL = gameStub.URL
	c.Upstreams.GameCatalog.Timeout = 2000
	c.Upstreams.Content.BaseURL = "http://127.0.0.1:1"
	c.Upstreams.Content.Timeout = 1000
	c.Upstreams.Community.BaseURL = "http://127.0.0.1:1"
	c.Upstreams.Community.Timeout = 1000
	c.Upstreams.DataPanel.BaseURL = "http://127.0.0.1:1"
	c.Upstreams.DataPanel.Timeout = 1000
	c.Upstreams.RealtimeHub.BaseURL = "http://127.0.0.1:1"
	c.Upstreams.RealtimeHub.Timeout = 1000
	c.Upstreams.CourierAdapter.BaseURL = courierStub.URL
	c.Upstreams.CourierAdapter.Timeout = 2000
	c.Upstreams.Payments.BaseURL = paymentsStub.URL
	c.Upstreams.Payments.Timeout = 2000
	c.Upstreams.Promotions.BaseURL = "http://127.0.0.1:1"
	c.Upstreams.Promotions.Timeout = 1000

	svcCtx := svc.NewServiceContext(c)
	server := rest.MustNewServer(c.RestConf)
	// 与 gateway.go run() 的注册序列逐行对应；新增路由组两侧同步。
	RegisterHandlers(server, svcCtx)
	content.RegisterContentProxyRoutes(server, svcCtx)
	community.RegisterCommunityProxyRoutes(server, svcCtx)
	users.RegisterUserProxyRoutes(server, svcCtx)
	games.RegisterGameProxyRoutes(server, svcCtx)
	datapanel.RegisterDataPanelProxyRoutes(server, svcCtx)
	adapter.RegisterCourierAdapterRoutes(server, svcCtx)
	payments.RegisterPaymentsRoutes(server, svcCtx)
	promotions.RegisterPromotionsRoutes(server, svcCtx)
	realtime.RegisterRealtimeTunnelRoutes(server, svcCtx)
	go server.Start()
	t.Cleanup(server.Stop)

	// 启动即 panic 的场景（重复路由）表现为端口永不就绪：探测 /games/featured
	// 可达（上游 stub 返回 200）即证明装配完成。
	base := "http://127.0.0.1:" + strconv.Itoa(c.Port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/games/featured")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("gateway did not become ready (装配 panic 会表现为端口永不就绪)")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 三条审计路径各回各家：games 组保持裸路径（game-catalog 审计，UI 在用），
	// courier/payments 组挂域内路径、转发还原成上游路径。基线先记录：就绪探测
	// /games/featured 会打到 game-catalog stub，用增量断言避免把它算进去。
	gameBefore := gameLog.count(t)
	courierBefore := courierLog.count(t)
	paymentsBefore := paymentsLog.count(t)

	doGet(t, base+"/admin/audit")
	if got := gameLog.last(t); got != "GET /admin/audit" {
		t.Fatalf("game-catalog stub got %q, want GET /admin/audit", got)
	}
	if n := gameLog.count(t) - gameBefore; n != 1 {
		t.Fatalf("game-catalog stub new calls = %d, want 1", n)
	}

	doGet(t, base+"/admin/courier/audit")
	if got := courierLog.last(t); got != "GET /admin/audit" {
		t.Fatalf("courier-adapter stub got %q, want GET /admin/audit (路径还原)", got)
	}
	if n := courierLog.count(t) - courierBefore; n != 1 {
		t.Fatalf("courier-adapter stub new calls = %d, want 1", n)
	}

	doGet(t, base+"/admin/payments/audit")
	if got := paymentsLog.last(t); got != "GET /admin/audit" {
		t.Fatalf("payments stub got %q, want GET /admin/audit (路径还原)", got)
	}
	if n := paymentsLog.count(t) - paymentsBefore; n != 1 {
		t.Fatalf("payments stub new calls = %d, want 1", n)
	}
}

func doGet(t *testing.T, url string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET %s = %d body=%s, want 200", url, resp.StatusCode, strings.TrimSpace(string(body)))
	}
}

func freePort(t *testing.T) int {
	t.Helper()
	// net.Listen("tcp", "127.0.0.1:0") + Close 的端口归还窗口在此可接受：
	// 与其他测试并行时由 go-zero 自身 bind 失败兜底（概率极低，重跑即绿）。
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}
