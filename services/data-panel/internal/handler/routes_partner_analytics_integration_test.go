package handler

// GP-404 合作方自助数据端到端（F12）：/api/v1/analytics/partner 按
// game-catalog /me 判 partner 角色 + partner_id 归属，数据按 partner_id
// 行级隔离（只统计本人名下事件）；player/匿名不可见；game-catalog 不可达
// fail-closed（502）。

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tappi/tappi/services/data-panel/internal/config"
	"github.com/tappi/tappi/services/data-panel/internal/httperr"
	"github.com/tappi/tappi/services/data-panel/internal/svc"
	"github.com/tappi/tappi/services/data-panel/utils"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// partnerMe 解析 /me 请求里的平台 JWT，返回 (userId, ok)。
func partnerMe(r *http.Request) (int64, bool) {
	auth := utils.NewAuth(testSecret)
	token := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	claims, err := auth.ParseToken(token)
	if err != nil {
		return 0, false
	}
	return claims.UserId, true
}

// newPartnerAnalyticsTestServer 真实 rest 服务器 + game-catalog /me 存根：
// user 8 = partner（绑定合作方 5），其余 = player。目录不可达时传空串
// （fail-closed 路径）。
func newPartnerAnalyticsTestServer(t *testing.T) string {
	t.Helper()
	// user 8 = partner（绑定合作方 5），其余 = player
	catalog := newPartnerCatalogStub(t)
	return newPartnerAnalyticsTestServerCatalog(t, catalog.URL)
}

// newPartnerCatalogStub game-catalog /me 存根：user 8 → partner/partner_id=5。
func newPartnerCatalogStub(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/partner/games" {
			// game-a 属合作方 5（game→partner 归属权威）
			_ = json.NewEncoder(w).Encode(map[string]any{"games": []map[string]any{
				{"id": "game-a"},
			}, "total": 1})
			return
		}
		if r.URL.Path != "/me" {
			http.NotFound(w, r)
			return
		}
		role, partnerID := "player", int64(0)
		if id, ok := partnerMe(r); ok && id == 8 {
			role, partnerID = "partner", 5
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"role": role, "partner_id": partnerID})
	}))
}

func newPartnerAnalyticsTestServerCatalog(t *testing.T, catalogURL string) string {
	t.Helper()
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	if catalogURL == "" {
		// 目录不可达：指向未监听的端口（fail-closed 路径）
		catalogURL = "http://127.0.0.1:1"
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	var conf rest.RestConf
	conf.Port = port

	var c config.Config
	c.RestConf = conf
	c.Auth.JWTSecret = testSecret
	c.MySQL.DataSource = "file:" + t.TempDir() + "/stats.db"
	c.Internal.Token = analyticsToken
	c.Analytics.AdminUserIds = []string{"7"}
	c.GameCatalog.BaseURL = catalogURL

	svcCtx := svc.NewServiceContext(c)
	server := rest.MustNewServer(conf)
	RegisterHandlers(server, svcCtx)
	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/api/v1/stats/users/1001/summary")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("server not ready: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	return base
}

// signPartnerToken 签平台 JWT（与 newAnalyticsTestServer 同款口径）。
func signPartnerToken(t *testing.T, userID int64) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, utils.JWTClaims{
		UserId:   userID,
		Username: "tester",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

// TestRoutes_PartnerAnalyticsRowIsolation 合作方自助数据：行级隔离 + 角色闸。
func TestRoutes_PartnerAnalyticsRowIsolation(t *testing.T) {
	base := newPartnerAnalyticsTestServer(t)
	url := base + "/internal/events"

	// 摄入：合作方 5 名下游戏 game-a（注册×2 + 付费 900）+ 他人游戏 game-x
	// （注册×1 + 付费 500）+ 无人认领 game-c（注册×1）——行级隔离按名下游戏
	// 收窄，三类互不混入
	events := []string{
		`{"event_id":"p1","event_type":"register","user_id":10,"game_id":"game-a","partner_id":5,"occurred_at":"2026-11-01T09:00:00Z"}`,
		`{"event_id":"p2","event_type":"register","user_id":11,"game_id":"game-a","partner_id":5,"occurred_at":"2026-11-02T09:00:00Z"}`,
		`{"event_id":"p3","event_type":"order_paid","user_id":10,"game_id":"game-a","partner_id":5,"amount_cents":900,"occurred_at":"2026-11-01T10:00:00Z"}`,
		`{"event_id":"p4","event_type":"register","user_id":20,"game_id":"game-x","partner_id":9,"occurred_at":"2026-11-01T11:00:00Z"}`,
		`{"event_id":"p5","event_type":"order_paid","user_id":20,"game_id":"game-x","partner_id":9,"amount_cents":500,"occurred_at":"2026-11-01T12:00:00Z"}`,
		`{"event_id":"p6","event_type":"register","user_id":30,"game_id":"game-c","occurred_at":"2026-11-01T13:00:00Z"}`,
	}
	code, body := doIngest(t, url, analyticsToken, ingestEvents(events...))
	if code != http.StatusOK || !strings.Contains(body, `"accepted":6`) {
		t.Fatalf("ingest = %d body=%s", code, body)
	}

	path := base + "/api/v1/analytics/partner"

	// 匿名 401
	if code, _ = doRaw(t, http.MethodGet, path+"?from=2026-11-01&to=2026-11-30", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous = %d, want 401", code)
	}
	// 名单管理员但角色非 partner（user 7）→ 403
	adminTok := signPartnerToken(t, 7)
	if code, _ = doRaw(t, http.MethodGet, path+"?from=2026-11-01&to=2026-11-30", adminTok, ""); code != http.StatusForbidden {
		t.Fatalf("admin user on partner face = %d, want 403", code)
	}

	// partner（user 8，绑定合作方 5，名下游戏 game-a）：只统计 game-a 事件
	tok := signPartnerToken(t, 8)
	code, body = doRaw(t, http.MethodGet, path+"?from=2026-11-01&to=2026-11-30", tok, "")
	if code != http.StatusOK {
		t.Fatalf("partner query = %d body=%s", code, body)
	}
	var resp struct {
		PartnerId int64 `json:"partner_id"`
		Items     []struct {
			EventType   string `json:"event_type"`
			Events      int64  `json:"events"`
			Users       int64  `json:"users"`
			AmountCents int64  `json:"amount_cents"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("parse partner resp: %v body=%s", err, body)
	}
	if resp.PartnerId != 5 {
		t.Fatalf("partner_id = %d, want 5", resp.PartnerId)
	}
	byType := map[string]struct {
		Events int64
		Users  int64
		Amount int64
	}{}
	for _, it := range resp.Items {
		byType[it.EventType] = struct {
			Events int64
			Users  int64
			Amount int64
		}{it.Events, it.Users, it.AmountCents}
	}
	if r := byType["register"]; r.Events != 2 || r.Users != 2 {
		t.Fatalf("register row = %+v, want 2 events 2 users（合作方 9 与平台级不混入）", r)
	}
	if r := byType["order_paid"]; r.Events != 1 || r.Users != 1 || r.Amount != 900 {
		t.Fatalf("order_paid row = %+v, want 1 event 1 user 900", r)
	}
	if _, ok := byType["login"]; ok {
		t.Fatalf("unexpected login row: %s", body)
	}

	// 区间收窄：11-02 起只余 1 例注册
	code, body = doRaw(t, http.MethodGet, path+"?from=2026-11-02&to=2026-11-30", tok, "")
	if code != http.StatusOK || !strings.Contains(body, `"events":1`) {
		t.Fatalf("narrow window = %d body=%s", code, body)
	}

	// 负 partner_id 摄入拒绝（行级隔离键不可为负）
	if code, _ = doIngest(t, url, analyticsToken, ingestEvents(
		`{"event_id":"pn","event_type":"login","user_id":1,"partner_id":-3}`)); code != http.StatusBadRequest {
		t.Fatalf("negative partner_id = %d, want 400", code)
	}
}

// TestRoutes_PartnerAnalyticsCatalogDown 角色服务不可达 → fail-closed（502）。
func TestRoutes_PartnerAnalyticsCatalogDown(t *testing.T) {
	base := newPartnerAnalyticsTestServerCatalog(t, "")
	tok := signPartnerToken(t, 8)
	url := base + "/api/v1/analytics/partner?from=2026-11-01&to=2026-11-30"

	if code, body := doRaw(t, http.MethodGet, url, tok, ""); code != http.StatusBadGateway {
		t.Fatalf("catalog down = %d body=%s, want 502", code, body)
	}
}
