package handler

// GP-306 开服计划与预约 + 业务通知触发集成测试：
//   - 区服开服排期（planned_open_at）管理与开服表展示（closed 对玩家隐藏）
//   - 玩家预约（幂等）/取消/我的预约；已开服 409
//   - 开服触发预约人群通知（open 过渡，community stub 收到扇出；提醒只发一次）
//   - 版本发布触发预约人群通知（game_version）
//   - S2S 预约人群查询（X-Internal-Token fail-closed）
// 通知扇出走 community /internal/notifications（站内信 + realtime-hub 远程
// 推送在 community 侧完成并有独立验收测试）。

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/tappi/tappi/services/game-catalog/internal/config"
	"github.com/tappi/tappi/services/game-catalog/internal/logic"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/utils"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

const gp306Token = "gp306-internal-token"

type communityStub struct {
	mu     sync.Mutex
	tokens []string
	reqs   [][]byte
	srv    *httptest.Server
}

func newCommunityStub() *communityStub {
	c := &communityStub{}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /internal/notifications", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.tokens = append(c.tokens, r.Header.Get("X-Internal-Token"))
		c.reqs = append(c.reqs, raw)
		c.mu.Unlock()
		if r.Header.Get("X-Internal-Token") != gp306Token {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		var body struct {
			UserIds []int64 `json:"user_ids"`
		}
		_ = json.Unmarshal(raw, &body)
		n, _ := json.Marshal(len(body.UserIds))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"created":` + string(n) + `}`))
	})
	c.srv = httptest.NewServer(mux)
	return c
}

func (c *communityStub) Calls() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.reqs)
}

func (c *communityStub) Last() ([]byte, string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.reqs) == 0 {
		return nil, ""
	}
	return c.reqs[len(c.reqs)-1], c.tokens[len(c.tokens)-1]
}

// newReservationTestServer 与 newAuthedTestServer 同款装配，额外接
// community stub（Notify）与内部令牌（Internal）。
func newReservationTestServer(t *testing.T, communityURL string) (string, func(utils.JWTClaims) string) {
	t.Helper()
	httpx.SetErrorHandlerCtx(logic.ErrorHandler)

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("release port: %v", err)
	}

	var c config.Config
	c.Port = port
	c.MySQL.DataSource = "file:" + t.TempDir() + "/games.db"
	c.Package.Dir = t.TempDir() + "/packages"
	c.Auth.JWTSecret = testJWTSecret
	c.Auth.AdminUserIds = []string{"1"}
	c.Notify.Url = communityURL
	c.Notify.Token = gp306Token
	c.Internal.Token = gp306Token

	svcCtx := svc.NewServiceContext(c)
	server := rest.MustNewServer(c.RestConf)
	RegisterHandlers(server, svcCtx)
	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/games")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("test server did not become ready in time")
		}
		time.Sleep(20 * time.Millisecond)
	}

	sign := func(claims utils.JWTClaims) string {
		token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
		signed, err := token.SignedString([]byte(testJWTSecret))
		if err != nil {
			t.Fatalf("sign token: %v", err)
		}
		return signed
	}
	return base, sign
}

func TestRoutes_ServerReservations(t *testing.T) {
	stub := newCommunityStub()
	defer stub.srv.Close()
	base, sign := newReservationTestServer(t, stub.srv.URL)
	admin := sign(claimsFor(1, "admin"))
	player := sign(claimsFor(7, "bob"))
	player2 := sign(claimsFor(8, "carol"))

	// 造合作方 + 已上架游戏（公开区服列表/通知受众前提）
	gameBody := `{"title":"预约游戏GP306","description":"gp-306","genres":["MMO"],"platforms":["Android"],"release_date":"2026-11-09","developer":"d","publisher":"p","tags":[],"score":8,"cover_image":"https://example.com/cover.jpg"}`
	code, body := doAuthed(t, http.MethodPost, base+"/games", admin, gameBody)
	if code != http.StatusOK {
		t.Fatalf("create game = %d body=%s", code, body)
	}
	var gameResp struct {
		Game struct {
			Id string `json:"id"`
		} `json:"game"`
	}
	if err := json.Unmarshal([]byte(body), &gameResp); err != nil || gameResp.Game.Id == "" {
		t.Fatalf("decode game: %v body=%s", err, body)
	}
	gameID := gameResp.Game.Id
	var partnerResp struct {
		Id int64 `json:"id"`
	}
	_, body = doAuthed(t, http.MethodPost, base+"/admin/partners", admin, `{"name":"预约厂GP306"}`)
	_ = json.Unmarshal([]byte(body), &partnerResp)
	partnerUserID := partnerResp.Id + 100
	partnerIDStr := strconv.FormatInt(partnerResp.Id, 10)
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/roles", admin,
		`{"user_id":`+strconv.FormatInt(partnerUserID, 10)+`,"role":"partner","partner_id":`+partnerIDStr+`}`); code != http.StatusOK {
		t.Fatalf("grant partner = %d", code)
	}
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/games/"+gameID+"/ownership", admin,
		`{"partner_id":`+partnerIDStr+`}`); code != http.StatusOK {
		t.Fatalf("assign ownership = %d", code)
	}
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/games/"+gameID+"/publish", admin, ""); code != http.StatusOK {
		t.Fatalf("publish game = %d", code)
	}
	partner := sign(claimsFor(partnerUserID, "partner-gp306"))

	// 1. 建未开服区服（closed + 开服排期）→ planned_open_at 落库回显
	code, body = doAuthed(t, http.MethodPost, base+"/partner/games/"+gameID+"/servers", partner,
		`{"name":"S1","region":"华东","status":"closed","planned_open_at":"2026-12-01T10:00:00+08:00"}`)
	if code != http.StatusOK || !strings.Contains(body, `"planned_open_at":"2026-12-01T10:00:00+08:00"`) {
		t.Fatalf("create server = %d body=%s", code, body)
	}
	var serverResp struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &serverResp)
	serverID := serverResp.Id
	if serverID <= 0 {
		t.Fatalf("server id missing: %s", body)
	}
	serverIDStr := strconv.FormatInt(serverID, 10)

	// 2. 公开开服表：closed 区服对玩家隐藏 → 排期随状态露出
	code, body = doAuthed(t, http.MethodGet, base+"/games/"+gameID+"/servers", "", "")
	if code != http.StatusOK || strings.Contains(body, "S1") {
		t.Fatalf("public list should hide closed server = %d body=%s", code, body)
	}

	// 3. 玩家预约：created=true；重复预约幂等 created=false；第二玩家预约
	code, body = doAuthed(t, http.MethodPost, base+"/games/"+gameID+"/servers/"+serverIDStr+"/reserve", player, "")
	if code != http.StatusOK || !strings.Contains(body, `"created":true`) {
		t.Fatalf("reserve = %d body=%s", code, body)
	}
	code, body = doAuthed(t, http.MethodPost, base+"/games/"+gameID+"/servers/"+serverIDStr+"/reserve", player, "")
	if code != http.StatusOK || !strings.Contains(body, `"created":false`) {
		t.Fatalf("reserve replay = %d body=%s", code, body)
	}
	if code, _ = doAuthed(t, http.MethodPost, base+"/games/"+gameID+"/servers/"+serverIDStr+"/reserve", player2, ""); code != http.StatusOK {
		t.Fatalf("player2 reserve = %d", code)
	}
	if stub.Calls() != 0 {
		t.Fatalf("reserve must not notify, got %d calls", stub.Calls())
	}

	// 4. 我的预约：区服名/状态/排期可见
	code, body = doAuthed(t, http.MethodGet, base+"/games/"+gameID+"/reservations/me", player, "")
	if code != http.StatusOK || !strings.Contains(body, `"server_name":"S1"`) ||
		!strings.Contains(body, `"server_status":"closed"`) ||
		!strings.Contains(body, `"planned_open_at":"2026-12-01T10:00:00+08:00"`) {
		t.Fatalf("my reservations = %d body=%s", code, body)
	}

	// 5. 开服（closed→open）→ 预约人群扇出一次（user_ids 7,8；type server_open）
	if code, _ = doAuthed(t, http.MethodPatch, base+"/partner/games/"+gameID+"/servers/"+serverIDStr, partner, `{"status":"open"}`); code != http.StatusOK {
		t.Fatalf("open server = %d", code)
	}
	if stub.Calls() != 1 {
		t.Fatalf("open should notify once, got %d calls", stub.Calls())
	}
	raw, token := stub.Last()
	var fanout struct {
		UserIds []int64 `json:"user_ids"`
		Type    string  `json:"type"`
		Target  int64   `json:"target_id"`
	}
	if err := json.Unmarshal(raw, &fanout); err != nil || token != gp306Token ||
		fanout.Type != "server_open" || fanout.Target != serverID ||
		len(fanout.UserIds) != 2 || fanout.UserIds[0] != 7 || fanout.UserIds[1] != 8 {
		t.Fatalf("fanout = %s token=%s err=%v", raw, token, err)
	}

	// 6. 再关再开（maintenance→open）：无未提醒预约 → 不重发（提醒只发一次）
	if code, _ = doAuthed(t, http.MethodPatch, base+"/partner/games/"+gameID+"/servers/"+serverIDStr, partner, `{"status":"maintenance"}`); code != http.StatusOK {
		t.Fatalf("to maintenance = %d", code)
	}
	if code, _ = doAuthed(t, http.MethodPatch, base+"/partner/games/"+gameID+"/servers/"+serverIDStr, partner, `{"status":"open"}`); code != http.StatusOK {
		t.Fatalf("reopen = %d", code)
	}
	if stub.Calls() != 1 {
		t.Fatalf("reopen must not re-notify, got %d calls", stub.Calls())
	}

	// 7. 已开服预约 → 409
	if code, _ = doAuthed(t, http.MethodPost, base+"/games/"+gameID+"/servers/"+serverIDStr+"/reserve", player, ""); code != http.StatusConflict {
		t.Fatalf("reserve open server = %d, want 409", code)
	}

	// 8. 版本发布 → game_version 扇出（预约人群 7,8）
	code, body = doAuthed(t, http.MethodPost, base+"/partner/games/"+gameID+"/versions", partner, `{"version":"1.0.1","changelog":"gp306","version_code":101}`)
	if code != http.StatusOK {
		t.Fatalf("create version = %d body=%s", code, body)
	}
	var verResp struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &verResp); err != nil || verResp.Id == "" {
		t.Fatalf("decode version: %v body=%s", err, body)
	}
	versionID := verResp.Id
	code, body = doAuthed(t, http.MethodPost, base+"/partner/games/"+gameID+"/submit", partner, `{"version_id":"`+versionID+`"}`)
	if code != http.StatusOK {
		t.Fatalf("submit version = %d body=%s", code, body)
	}
	var reviewResp struct {
		Id int64 `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &reviewResp); err != nil || reviewResp.Id == 0 {
		t.Fatalf("decode review: %v body=%s", err, body)
	}
	if code, _ = doAuthed(t, http.MethodPost, base+"/admin/reviews/"+strconv.FormatInt(reviewResp.Id, 10)+"/decision", admin, `{"decision":"approve"}`); code != http.StatusOK {
		t.Fatalf("approve review = %d", code)
	}
	before := stub.Calls()
	if code, _ = doAuthed(t, http.MethodPost, base+"/admin/games/"+gameID+"/versions/"+versionID+"/publish", admin, ""); code != http.StatusOK {
		t.Fatalf("publish version = %d", code)
	}
	if stub.Calls() != before+1 {
		t.Fatalf("version publish should notify once, got %d -> %d", before, stub.Calls())
	}
	raw, _ = stub.Last()
	var verNotify struct {
		UserIds []int64 `json:"user_ids"`
		Type    string  `json:"type"`
	}
	if err := json.Unmarshal(raw, &verNotify); err != nil || verNotify.Type != "game_version" ||
		len(verNotify.UserIds) != 2 || verNotify.UserIds[0] != 7 || verNotify.UserIds[1] != 8 {
		t.Fatalf("version notify = %s err=%v", raw, err)
	}

	// 9. S2S 预约人群：无令牌 403；带令牌返回去重人群（7,8 各一条预约）
	code, _ = doAuthed(t, http.MethodGet, base+"/internal/games/"+gameID+"/reservation-users", "", "")
	if code != http.StatusForbidden {
		t.Fatalf("internal without token = %d, want 403", code)
	}
	req, _ := http.NewRequest(http.MethodGet, base+"/internal/games/"+gameID+"/reservation-users", nil)
	req.Header.Set("X-Internal-Token", gp306Token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("internal users: %v", err)
	}
	raw2, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	var usersResp struct {
		GameId  string  `json:"game_id"`
		UserIds []int64 `json:"user_ids"`
	}
	if err := json.Unmarshal(raw2, &usersResp); err != nil || usersResp.GameId != gameID ||
		len(usersResp.UserIds) != 2 {
		t.Fatalf("reservation users = %s err=%v", raw2, err)
	}

	// 10. 取消预约（幂等）：我的预约清空
	code, _ = doAuthed(t, http.MethodDelete, base+"/games/"+gameID+"/servers/"+serverIDStr+"/reserve", player, "")
	if code != http.StatusOK {
		t.Fatalf("cancel = %d", code)
	}
	code, _ = doAuthed(t, http.MethodDelete, base+"/games/"+gameID+"/servers/"+serverIDStr+"/reserve", player, "")
	if code != http.StatusOK {
		t.Fatalf("cancel replay = %d", code)
	}
	code, body = doAuthed(t, http.MethodGet, base+"/games/"+gameID+"/reservations/me", player, "")
	if code != http.StatusOK || !strings.Contains(body, `"items":[]`) {
		t.Fatalf("my reservations after cancel = %d body=%s", code, body)
	}
}
