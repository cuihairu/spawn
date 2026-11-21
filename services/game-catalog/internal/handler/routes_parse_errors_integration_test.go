package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// 坏 JSON 请求体：各带 body 的 handler 在 httpx.Parse 阶段即失败 → 400
// （覆盖 handler Parse 错误分支，不进入 logic）。
func TestRoutes_ParseErrors(t *testing.T) {
	base, sign := newAuthedTestServer(t)
	admin := sign(claimsFor(1, "admin"))

	// 建合作方并授权 user2 为 partner，供合作方路由组 Parse 错误测试。
	_, body := doAuthed(t, http.MethodPost, base+"/admin/partners", admin, `{"name":"解析厂"}`)
	var partnerResp struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &partnerResp)
	doAuthed(t, http.MethodPost, base+"/admin/roles", admin, `{"user_id":2,"role":"partner","partner_id":`+jsonNumber(partnerResp.Id)+`}`)
	partner := sign(claimsFor(2, "carol"))
	player := sign(claimsFor(7, "bob"))

	cases := []struct {
		name   string
		method string
		url    string
		token  string
	}{
		// 运营后台
		{"admin roles", http.MethodPost, "/admin/roles", admin},
		{"admin create partner", http.MethodPost, "/admin/partners", admin},
		{"admin set partner status", http.MethodPatch, "/admin/partners/1", admin},
		{"admin assign ownership", http.MethodPost, "/admin/games/game-elden-ring/ownership", admin},
		{"admin decide review", http.MethodPost, "/admin/reviews/1/decision", admin},
		{"admin restore review", http.MethodPost, "/admin/reviews/1/restore", admin},
		{"admin handle report", http.MethodPost, "/admin/review-reports/1/handle", admin},
		{"admin promo create", http.MethodPost, "/admin/promo/channels", admin},
		{"admin promo update", http.MethodPatch, "/admin/promo/channels/1", admin},
		{"admin promo status", http.MethodPost, "/admin/promo/channels/1/status", admin},
		{"admin publish game", http.MethodPost, "/admin/games/game-elden-ring/publish", admin},
		{"admin unpublish game", http.MethodPost, "/admin/games/game-elden-ring/unpublish", admin},
		{"admin publish version", http.MethodPost, "/admin/games/game-elden-ring/versions/v1/publish", admin},
		{"admin revoke version", http.MethodPost, "/admin/games/game-elden-ring/versions/v1/revoke", admin},
		// 合作方自助
		{"partner submit", http.MethodPost, "/partner/games/game-elden-ring/submit", partner},
		{"partner create version", http.MethodPost, "/partner/games/game-elden-ring/versions", partner},
		{"partner upload package", http.MethodPost, "/partner/games/game-elden-ring/versions/v1/package", partner},
		{"partner create server", http.MethodPost, "/partner/games/game-elden-ring/servers", partner},
		{"partner update server", http.MethodPatch, "/partner/games/game-elden-ring/servers/1", partner},
		{"partner reply review", http.MethodPost, "/partner/games/game-elden-ring/reviews/1/reply", partner},
		// 玩家写接口
		{"player rate", http.MethodPost, "/games/game-elden-ring/rating", player},
		{"player create review", http.MethodPost, "/games/game-elden-ring/reviews", player},
		{"player update review", http.MethodPut, "/games/game-elden-ring/reviews/1", player},
		{"player vote review", http.MethodPost, "/games/game-elden-ring/reviews/1/vote", player},
		{"player report review", http.MethodPost, "/games/game-elden-ring/reviews/1/report", player},
		{"player favorite", http.MethodPost, "/games/game-elden-ring/favorite", player},
		{"player reserve", http.MethodPost, "/games/game-elden-ring/servers/1/reserve", player},
		{"player create game", http.MethodPost, "/games", admin},
		{"player update tags", http.MethodPatch, "/games/game-elden-ring/tags", admin},
		{"promo download click", http.MethodPost, "/promo/download-click", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _ := doAuthed(t, c.method, base+c.url, c.token, `{`)
			if code != http.StatusBadRequest {
				t.Fatalf("%s %s = %d, want 400", c.method, c.url, code)
			}
		})
	}
}

// PATCH /admin/partners/:id 与 GET /admin/games/:id/versions 两条此前未走通
// 的后台路由：成功路径 + 非法状态/未知合作方错误面。
func TestRoutes_PartnerStatusAndAdminVersions(t *testing.T) {
	base, sign := newAuthedTestServer(t)
	admin := sign(claimsFor(1, "admin"))

	_, body := doAuthed(t, http.MethodPost, base+"/admin/partners", admin, `{"name":"状态厂"}`)
	var partnerResp struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &partnerResp)

	// 挂起 → suspended
	code, body := doAuthed(t, http.MethodPatch, base+"/admin/partners/"+jsonNumber(partnerResp.Id), admin, `{"status":"suspended"}`)
	if code != http.StatusOK || !strings.Contains(body, `"status":"suspended"`) {
		t.Fatalf("suspend partner = %d body=%s", code, body)
	}
	// 恢复 → active
	if code, body = doAuthed(t, http.MethodPatch, base+"/admin/partners/"+jsonNumber(partnerResp.Id), admin, `{"status":"active"}`); code != http.StatusOK || !strings.Contains(body, `"status":"active"`) {
		t.Fatalf("activate partner = %d body=%s", code, body)
	}
	// 非法状态 → 400
	if code, _ = doAuthed(t, http.MethodPatch, base+"/admin/partners/"+jsonNumber(partnerResp.Id), admin, `{"status":"bogus"}`); code != http.StatusBadRequest {
		t.Fatalf("invalid partner status = %d, want 400", code)
	}
	// 未知合作方 → 404
	if code, _ = doAuthed(t, http.MethodPatch, base+"/admin/partners/99999", admin, `{"status":"active"}`); code != http.StatusNotFound {
		t.Fatalf("unknown partner status = %d, want 404", code)
	}

	// 版本列表：无版本游戏 → 200 空列表；建版本后 → 200 含版本。
	if code, body = doAuthed(t, http.MethodGet, base+"/admin/games/game-elden-ring/versions", admin, ""); code != http.StatusOK || !strings.Contains(body, `"versions":`) {
		t.Fatalf("list versions empty = %d body=%s", code, body)
	}
	// 经合作方提审建一条已过审版本，管理面应能列出。
	_, body = doAuthed(t, http.MethodPost, base+"/admin/partners", admin, `{"name":"版本厂2"}`)
	_ = json.Unmarshal([]byte(body), &partnerResp)
	doAuthed(t, http.MethodPost, base+"/admin/roles", admin, `{"user_id":2,"role":"partner","partner_id":`+jsonNumber(partnerResp.Id)+`}`)
	doAuthed(t, http.MethodPost, base+"/admin/games/game-elden-ring/ownership", admin, `{"partner_id":`+jsonNumber(partnerResp.Id)+`}`)
	partner := sign(claimsFor(2, "carol"))
	_, body = doAuthed(t, http.MethodPost, base+"/partner/games/game-elden-ring/versions", partner, `{"version":"1.0.0","version_code":1}`)
	if code, body = doAuthed(t, http.MethodGet, base+"/admin/games/game-elden-ring/versions", admin, ""); code != http.StatusOK || !strings.Contains(body, `"version":"1.0.0"`) {
		t.Fatalf("list versions populated = %d body=%s", code, body)
	}
}
