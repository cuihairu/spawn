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
	c.Analytics.AdminUserIds = "7"
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

// TestRoutes_PartnerAnalyticsTrend GP-404 合作方自助趋势：按日聚合事件量与
// 去重用户数、ISO 周并桶、行级隔离（他人游戏/平台级不混入）、角色闸。
// 2026-11-01 为周日：其 ISO 周起点 2026-10-26，11-02 起新周。
func TestRoutes_PartnerAnalyticsTrend(t *testing.T) {
	base := newPartnerAnalyticsTestServer(t)
	url := base + "/internal/events"
	events := []string{
		`{"event_id":"d1","event_type":"register","user_id":10,"game_id":"game-a","partner_id":5,"occurred_at":"2026-11-01T09:00:00Z"}`,
		`{"event_id":"d2","event_type":"register","user_id":11,"game_id":"game-a","partner_id":5,"occurred_at":"2026-11-02T09:00:00Z"}`,
		`{"event_id":"d3","event_type":"login","user_id":10,"game_id":"game-a","partner_id":5,"occurred_at":"2026-11-01T10:00:00Z"}`,
		`{"event_id":"d4","event_type":"login","user_id":10,"game_id":"game-a","partner_id":5,"occurred_at":"2026-11-03T09:00:00Z"}`,
		// 他人游戏（合作方 9）与平台级：行级隔离不混入
		`{"event_id":"d5","event_type":"register","user_id":20,"game_id":"game-x","partner_id":9,"occurred_at":"2026-11-01T11:00:00Z"}`,
		`{"event_id":"d6","event_type":"login","user_id":30,"occurred_at":"2026-11-01T12:00:00Z"}`,
	}
	code, body := doIngest(t, url, analyticsToken, ingestEvents(events...))
	if code != http.StatusOK || !strings.Contains(body, `"accepted":6`) {
		t.Fatalf("ingest = %d body=%s", code, body)
	}
	tok := signPartnerToken(t, 8)
	path := base + "/api/v1/analytics/partner/trend"

	// 匿名 401、名单管理员但非 partner 403
	if code, _ = doRaw(t, http.MethodGet, path+"?from=2026-11-01&to=2026-11-30", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("anon = %d, want 401", code)
	}
	if code, _ = doRaw(t, http.MethodGet, path+"?from=2026-11-01&to=2026-11-30", signPartnerToken(t, 7), ""); code != http.StatusForbidden {
		t.Fatalf("admin on partner face = %d, want 403", code)
	}
	// 非法粒度 400
	if code, _ = doRaw(t, http.MethodGet, path+"?granularity=month", tok, ""); code != http.StatusBadRequest {
		t.Fatalf("bad granularity = %d, want 400", code)
	}

	// 日粒度：三桶（11-01 双事件同用户、11-02/11-03 各一）
	code, body = doRaw(t, http.MethodGet, path+"?from=2026-11-01&to=2026-11-30&granularity=day", tok, "")
	if code != http.StatusOK {
		t.Fatalf("day trend = %d body=%s", code, body)
	}
	var day struct {
		Granularity string `json:"granularity"`
		Items       []struct {
			Bucket string `json:"bucket"`
			Events int64  `json:"events"`
			Users  int64  `json:"users"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &day); err != nil || len(day.Items) != 3 {
		t.Fatalf("day trend decode: %v body=%s", err, body)
	}
	if day.Items[0].Bucket != "2026-11-01" || day.Items[0].Events != 2 || day.Items[0].Users != 1 {
		t.Fatalf("day0 = %+v, want 11-01 2 events 1 user", day.Items[0])
	}
	if day.Items[1].Bucket != "2026-11-02" || day.Items[1].Events != 1 || day.Items[1].Users != 1 {
		t.Fatalf("day1 = %+v, want 11-02 1 event 1 user", day.Items[1])
	}
	if day.Items[2].Bucket != "2026-11-03" || day.Items[2].Events != 1 || day.Items[2].Users != 1 {
		t.Fatalf("day2 = %+v, want 11-03 1 event 1 user", day.Items[2])
	}

	// 周粒度：11-01 归 10-26 周，11-02/11-03 归 11-02 周；周用户数去重
	code, body = doRaw(t, http.MethodGet, path+"?from=2026-11-01&to=2026-11-30&granularity=week", tok, "")
	if code != http.StatusOK {
		t.Fatalf("week trend = %d body=%s", code, body)
	}
	var week struct {
		Items []struct {
			Bucket string `json:"bucket"`
			Events int64  `json:"events"`
			Users  int64  `json:"users"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(body), &week); err != nil || len(week.Items) != 2 {
		t.Fatalf("week trend decode: %v body=%s", err, body)
	}
	if week.Items[0].Bucket != "2026-10-26" || week.Items[0].Events != 2 || week.Items[0].Users != 1 {
		t.Fatalf("week0 = %+v, want 10-26 2 events 1 user", week.Items[0])
	}
	if week.Items[1].Bucket != "2026-11-02" || week.Items[1].Events != 2 || week.Items[1].Users != 2 {
		t.Fatalf("week1 = %+v, want 11-02 2 events 2 users", week.Items[1])
	}
}

// TestRoutes_PartnerAnalyticsRetention GP-404 合作方自助留存：口径与管理面
// 一致（首活跃日锚点、D0..Dwindow），按 partner 行级隔离 + 角色闸。
func TestRoutes_PartnerAnalyticsRetention(t *testing.T) {
	base := newPartnerAnalyticsTestServer(t)
	url := base + "/internal/events"
	events := []string{
		// u10 首活 11-01，续 11-02/11-03；u11 仅 11-01 → cohort 11-01 size 2
		`{"event_id":"r1","event_type":"login","user_id":10,"game_id":"game-a","partner_id":5,"occurred_at":"2026-11-01T09:00:00Z"}`,
		`{"event_id":"r2","event_type":"login","user_id":11,"game_id":"game-a","partner_id":5,"occurred_at":"2026-11-01T10:00:00Z"}`,
		`{"event_id":"r3","event_type":"login","user_id":10,"game_id":"game-a","partner_id":5,"occurred_at":"2026-11-02T09:00:00Z"}`,
		`{"event_id":"r4","event_type":"login","user_id":10,"game_id":"game-a","partner_id":5,"occurred_at":"2026-11-03T09:00:00Z"}`,
		// 他人游戏：不混入
		`{"event_id":"r5","event_type":"login","user_id":20,"game_id":"game-x","partner_id":9,"occurred_at":"2026-11-01T11:00:00Z"}`,
	}
	code, body := doIngest(t, url, analyticsToken, ingestEvents(events...))
	if code != http.StatusOK || !strings.Contains(body, `"accepted":5`) {
		t.Fatalf("ingest = %d body=%s", code, body)
	}
	tok := signPartnerToken(t, 8)
	path := base + "/api/v1/analytics/partner/retention"

	// 匿名 401、非 partner 403、非法参数 400
	if code, _ = doRaw(t, http.MethodGet, path+"?from=2026-11-01&to=2026-11-10", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("anon = %d, want 401", code)
	}
	if code, _ = doRaw(t, http.MethodGet, path+"?from=2026-11-01&to=2026-11-10", signPartnerToken(t, 7), ""); code != http.StatusForbidden {
		t.Fatalf("admin on partner face = %d, want 403", code)
	}
	if code, _ = doRaw(t, http.MethodGet, path+"?event_type=nonsense", tok, ""); code != http.StatusBadRequest {
		t.Fatalf("bad event_type = %d, want 400", code)
	}
	if code, _ = doRaw(t, http.MethodGet, path+"?window=31", tok, ""); code != http.StatusBadRequest {
		t.Fatalf("window 31 = %d, want 400", code)
	}

	// 合作方（user 8，名下 game-a）：cohort 11-01 size 2，D0=2、D1=1、D2=1
	code, body = doRaw(t, http.MethodGet, path+"?from=2026-11-01&to=2026-11-10&event_type=login&window=7", tok, "")
	if code != http.StatusOK {
		t.Fatalf("retention = %d body=%s", code, body)
	}
	var resp struct {
		PartnerId int64 `json:"partner_id"`
		Cohorts   []struct {
			Day       string                    `json:"day"`
			Size      int64                     `json:"size"`
			Retention map[string]map[string]any `json:"retention"`
		} `json:"cohorts"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil || len(resp.Cohorts) != 1 {
		t.Fatalf("retention decode: %v body=%s", err, body)
	}
	c := resp.Cohorts[0]
	if c.Day != "2026-11-01" || c.Size != 2 {
		t.Fatalf("cohort = %+v, want day 11-01 size 2", c)
	}
	d0, _ := c.Retention["0"]
	d1, _ := c.Retention["1"]
	d2, _ := c.Retention["2"]
	if int64(d0["users"].(float64)) != 2 || int64(d1["users"].(float64)) != 1 || int64(d2["users"].(float64)) != 1 {
		t.Fatalf("cohort users D0/D1/D2 = %v/%v/%v, want 2/1/1", d0["users"], d1["users"], d2["users"])
	}
	if d1["retention"].(float64) != 0.5 || d2["retention"].(float64) != 0.5 {
		t.Fatalf("cohort retention D1/D2 = %v/%v, want 0.5/0.5", d1["retention"], d2["retention"])
	}
}

// TestRoutes_PartnerAnalyticsByGame GP-404 合作方自助按游戏下钻：名下游戏
// 逐一的按事件类型明细（金额只计 order_paid），他人游戏不混入，角色闸。
func TestRoutes_PartnerAnalyticsByGame(t *testing.T) {
	base := newPartnerAnalyticsTestServer(t)
	url := base + "/internal/events"
	events := []string{
		`{"event_id":"b1","event_type":"register","user_id":10,"game_id":"game-a","partner_id":5,"occurred_at":"2026-11-01T09:00:00Z"}`,
		`{"event_id":"b2","event_type":"login","user_id":10,"game_id":"game-a","partner_id":5,"occurred_at":"2026-11-01T10:00:00Z"}`,
		`{"event_id":"b3","event_type":"order_paid","user_id":10,"game_id":"game-a","partner_id":5,"amount_cents":900,"occurred_at":"2026-11-02T09:00:00Z"}`,
		// 他人游戏（合作方 9）：不混入
		`{"event_id":"b4","event_type":"register","user_id":20,"game_id":"game-x","partner_id":9,"occurred_at":"2026-11-01T11:00:00Z"}`,
	}
	code, body := doIngest(t, url, analyticsToken, ingestEvents(events...))
	if code != http.StatusOK || !strings.Contains(body, `"accepted":4`) {
		t.Fatalf("ingest = %d body=%s", code, body)
	}
	tok := signPartnerToken(t, 8)
	path := base + "/api/v1/analytics/partner/by_game"

	// 匿名 401、非 partner 403
	if code, _ = doRaw(t, http.MethodGet, path+"?from=2026-11-01&to=2026-11-30", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("anon = %d, want 401", code)
	}
	if code, _ = doRaw(t, http.MethodGet, path+"?from=2026-11-01&to=2026-11-30", signPartnerToken(t, 7), ""); code != http.StatusForbidden {
		t.Fatalf("admin on partner face = %d, want 403", code)
	}

	// 合作方（user 8，名下 game-a）：仅 game-a 一行明细（3 类型）
	code, body = doRaw(t, http.MethodGet, path+"?from=2026-11-01&to=2026-11-30", tok, "")
	if code != http.StatusOK {
		t.Fatalf("by game = %d body=%s", code, body)
	}
	var resp struct {
		PartnerId int64 `json:"partner_id"`
		Games     []struct {
			GameId string `json:"game_id"`
			Items  []struct {
				EventType   string `json:"event_type"`
				Events      int64  `json:"events"`
				Users       int64  `json:"users"`
				AmountCents int64  `json:"amount_cents"`
			} `json:"items"`
		} `json:"games"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil || resp.PartnerId != 5 || len(resp.Games) != 1 {
		t.Fatalf("by game decode: %v body=%s", err, body)
	}
	g := resp.Games[0]
	if g.GameId != "game-a" || len(g.Items) != 3 {
		t.Fatalf("game = %+v, want game-a 3 types", g)
	}
	byType := map[string]struct{ Events, Users, Amount int64 }{}
	for _, it := range g.Items {
		byType[it.EventType] = struct{ Events, Users, Amount int64 }{it.Events, it.Users, it.AmountCents}
	}
	if r := byType["order_paid"]; r.Events != 1 || r.Users != 1 || r.Amount != 900 {
		t.Fatalf("order_paid = %+v, want 1/1/900", r)
	}
	if r := byType["register"]; r.Events != 1 || r.Users != 1 || r.Amount != 0 {
		t.Fatalf("register = %+v, want 1/1/0", r)
	}
	if r := byType["login"]; r.Events != 1 || r.Users != 1 || r.Amount != 0 {
		t.Fatalf("login = %+v, want 1/1/0", r)
	}
}
