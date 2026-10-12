package realtime

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/tappi/tappi/services/api-gateway/internal/config"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// hubSpy 隧道透传观测点：hub 实际收到的 Origin 与 X-Forwarded-For。
type hubSpy struct {
	origins []string
	xffs    []string
}

// fakeEchoHub gorilla echo 上游：token 缺失回 401，升级成功后原样回显帧。
func fakeEchoHub(t *testing.T) (*httptest.Server, *hubSpy) {
	t.Helper()
	spy := &hubSpy{}
	// 与真实 hub 同口径：准入闸在 JWT，Origin 不设限（跨源透传直测）。
	up := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") == "" {
			http.Error(w, "missing token", http.StatusUnauthorized)
			return
		}
		spy.origins = append(spy.origins, r.Header.Get("Origin"))
		spy.xffs = append(spy.xffs, r.Header.Get("X-Forwarded-For"))
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			mt, data, err := conn.ReadMessage()
			if err != nil {
				return
			}
			if err := conn.WriteMessage(mt, data); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv, spy
}

// startTunnelTestServer 真实 go-zero rest server；productionLike=true 时
// 打开与 conf.Load 默认一致的全量中间件（验证隧道穿过 WithCode/Timeout
// 等包装链后 Hijack 仍可用）。
func startTunnelTestServer(t *testing.T, serverCtx *svc.ServiceContext, productionLike bool) string {
	t.Helper()
	var conf rest.RestConf
	conf.Port = freePort(t)
	if productionLike {
		conf.Middlewares = rest.MiddlewaresConf{
			Trace: true, Log: true, Prometheus: true, MaxConns: true, Breaker: true,
			Shedding: true, Timeout: true, Recover: true, Metrics: true, MaxBytes: true, Gunzip: true,
		}
	}
	server := rest.MustNewServer(conf)
	RegisterRealtimeTunnelRoutes(server, serverCtx)
	go server.Start()
	t.Cleanup(server.Stop)
	return waitReady(t, conf.Port)
}

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port
}

func waitReady(t *testing.T, port int) string {
	t.Helper()
	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/not-registered-probe")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return base
		}
		if time.Now().After(deadline) {
			t.Fatal("tunnel test server did not become ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func dialEcho(t *testing.T, base, origin string, roundTrips int) {
	t.Helper()
	d := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	var header http.Header
	if origin != "" {
		header = http.Header{"Origin": []string{origin}}
	}
	conn, _, err := d.Dial("ws"+base[len("http"):]+"/ws?token=abc", header)
	if err != nil {
		t.Fatalf("ws dial: %v", err)
	}
	defer conn.Close()
	for i := 0; i < roundTrips; i++ {
		payload := "hello-" + strconv.Itoa(i)
		if err := conn.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
			t.Fatalf("write: %v", err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read: %v", err)
		}
		if string(data) != payload {
			t.Fatalf("echo = %q, want %q", data, payload)
		}
	}
}

// TestTunnelEchoThroughRestServer 全量中间件链上握手 + 双向帧往返；
// query token 与跨源 Origin 头须原样到达 hub。
func TestTunnelEchoThroughRestServer(t *testing.T) {
	hub, spy := fakeEchoHub(t)
	base := startTunnelTestServer(t, tunnelCtx(hub.URL), true)
	dialEcho(t, base, "http://localhost:4173", 3)
	if len(spy.origins) != 1 || spy.origins[0] != "http://localhost:4173" {
		t.Fatalf("origins = %v", spy.origins)
	}
	// 客户端真实 IP 经 X-Forwarded-For/X-Real-Ip 到达 hub
	if len(spy.xffs) != 1 || spy.xffs[0] != "127.0.0.1" {
		t.Fatalf("xffs = %v", spy.xffs)
	}
}

// TestTunnelEchoPlainWriter 无中间件（裸 net/http writer）同样透传。
func TestTunnelEchoPlainWriter(t *testing.T) {
	hub, _ := fakeEchoHub(t)
	base := startTunnelTestServer(t, tunnelCtx(hub.URL), false)
	dialEcho(t, base, "", 2)
}

// TestTunnelHubDown hub 不可达：Hijack 前回 502 JSON。
func TestTunnelHubDown(t *testing.T) {
	base := startTunnelTestServer(t, tunnelCtx("http://127.0.0.1:1"), false)
	resp, err := http.Get(base + "/ws?token=abc")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusBadGateway || len(body) == 0 {
		t.Fatalf("status=%d body=%q", resp.StatusCode, body)
	}
}

// TestTunnelUpstreamReject hub 升级前拒绝（401）：状态码原样流回客户端。
func TestTunnelUpstreamReject(t *testing.T) {
	hub, _ := fakeEchoHub(t)
	base := startTunnelTestServer(t, tunnelCtx(hub.URL), false)
	d := websocket.Dialer{HandshakeTimeout: 5 * time.Second}
	conn, resp, err := d.Dial("ws"+base[len("http"):]+"/ws", nil) // 无 token
	if err == nil {
		conn.Close()
		t.Fatal("dial should fail")
	}
	if resp == nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("resp = %+v err = %v", resp, err)
	}
}

// TestRegisterSkipsWhenBaseURLEmpty 未配置 hub：不注册 /ws（404），
// 网关其余功能不受影响。
func TestRegisterSkipsWhenBaseURLEmpty(t *testing.T) {
	base := startTunnelTestServer(t, tunnelCtx(""), false)
	resp, err := http.Get(base + "/ws")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", resp.StatusCode)
	}
}

func tunnelCtx(baseURL string) *svc.ServiceContext {
	var cfg config.Config
	cfg.Upstreams.RealtimeHub.BaseURL = baseURL
	cfg.Upstreams.RealtimeHub.Timeout = 5000
	return &svc.ServiceContext{Config: cfg}
}
