package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tappi/tappi/services/user-service/internal/analytics"
	"github.com/tappi/tappi/services/user-service/internal/integration"
)

// gp403Stub game-catalog S2S 契约桩：POST /internal/promo/clicks/:clickId/resolve
// 返回渠道解析结果，并断言调用方带了配置的 X-Internal-Token（fail-closed 守卫）。
type gp403Stub struct {
	mu      sync.Mutex
	token   string
	seenHdr string
}

func (s *gp403Stub) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/promo/clicks/click-1/resolve" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		hdr := r.Header.Get("X-Internal-Token")
		s.mu.Lock()
		s.seenHdr = hdr
		s.mu.Unlock()
		if s.token != "" && hdr != s.token {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"channel":"kol-li","channel_id":7,"clicked_at":"2026-11-12T00:00:00Z","window_hours":72,"in_window":true}`)
	}))
}

// TestRoutes_RegisterChannelAttribution GP-403 端到端：注册携带 click_id →
// 带 X-Internal-Token 向 game-catalog 解析渠道 → 注册事实事件携带 channel。
func TestRoutes_RegisterChannelAttribution(t *testing.T) {
	stub := &gp403Stub{token: "s2s-token"}
	catalog := stub.server()
	defer catalog.Close()

	// data-panel 事实接收侧：解析事件并回读 channel 维度
	var mu sync.Mutex
	var got []map[string]interface{}
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var payload struct {
			Events []map[string]interface{} `json:"events"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		got = append(got, payload.Events...)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer panel.Close()

	base, svcCtx := newUserHandlerServer(t, gameStubHandler(t, nil))
	client := integration.NewGameCatalogClient(catalog.URL, 2*time.Second)
	client.SetInternalToken("s2s-token")
	svcCtx.GameCatalogClient = client
	svcCtx.AnalyticsSvc = analytics.NewClient(panel.URL, "panel-token", 1000)

	body := `{"username":"dave","email":"dave@example.com","password":"pw-dave","click_id":"click-1"}`
	status, respBody, raw := do(t, http.MethodPost, base+"/auth/register", "", body)
	if status != http.StatusOK || code(t, respBody) != 200 {
		t.Fatalf("register: status=%d raw=%s", status, raw)
	}

	if got := stub.seenHdr; got != "s2s-token" {
		t.Fatalf("X-Internal-Token = %q, want s2s-token", got)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 {
		t.Fatal("no analytics event delivered")
	}
	ev := got[len(got)-1]
	if ev["event_type"] != analytics.EventRegister {
		t.Fatalf("event_type = %v", ev["event_type"])
	}
	if ev["channel"] != "kol-li" {
		t.Fatalf("channel = %v, want kol-li", ev["channel"])
	}
}

// TestRoutes_RegisterChannelAbsent GP-403：无 click_id（自然流量）注册时
// 不发起渠道解析，注册事实事件不带 channel。
func TestRoutes_RegisterChannelAbsent(t *testing.T) {
	called := false
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNotFound)
	}))
	defer catalog.Close()

	var mu sync.Mutex
	var got []map[string]interface{}
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		var payload struct {
			Events []map[string]interface{} `json:"events"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		mu.Lock()
		got = append(got, payload.Events...)
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer panel.Close()

	base, svcCtx := newUserHandlerServer(t, gameStubHandler(t, nil))
	svcCtx.GameCatalogClient = integration.NewGameCatalogClient(catalog.URL, 2*time.Second)
	svcCtx.AnalyticsSvc = analytics.NewClient(panel.URL, "panel-token", 1000)

	body := `{"username":"erin","email":"erin@example.com","password":"pw-erin"}`
	status, respBody, raw := do(t, http.MethodPost, base+"/auth/register", "", body)
	if status != http.StatusOK || code(t, respBody) != 200 {
		t.Fatalf("register: status=%d raw=%s", status, raw)
	}
	if called {
		t.Fatal("must not resolve promo click without click_id")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(got) == 0 {
		t.Fatal("no analytics event delivered")
	}
	if ch, ok := got[len(got)-1]["channel"]; ok && ch != "" {
		t.Fatalf("channel = %v, want absent", ch)
	}
}

// TestRoutes_RegisterChannelResolveFailure GP-403：渠道解析 403（令牌不匹配）
// 时注册仍然成功，事件不带 channel（归因不阻断注册）。
func TestRoutes_RegisterChannelResolveFailure(t *testing.T) {
	stub := &gp403Stub{token: "s2s-token"}
	catalog := stub.server()
	defer catalog.Close()

	base, svcCtx := newUserHandlerServer(t, gameStubHandler(t, nil))
	client := integration.NewGameCatalogClient(catalog.URL, 2*time.Second)
	// 故意不设置令牌 → S2S 解析 403
	svcCtx.GameCatalogClient = client
	svcCtx.AnalyticsSvc = analytics.NewClient("http://127.0.0.1:1", "panel-token", 500)

	body := `{"username":"frank","email":"frank@example.com","password":"pw-frank","click_id":"click-1"}`
	status, respBody, raw := do(t, http.MethodPost, base+"/auth/register", "", body)
	if status != http.StatusOK || code(t, respBody) != 200 {
		t.Fatalf("register with failing attribution: status=%d raw=%s", status, raw)
	}
}
