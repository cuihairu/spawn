package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tappi/tappi/services/game-catalog/utils"
)

func claimsFor(userID int64, username string) utils.JWTClaims {
	return utils.JWTClaims{
		UserId:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
}

func doAuthed(t *testing.T, method, url, token, body string) (int, string) {
	t.Helper()
	var req *http.Request
	if body == "" {
		req, _ = http.NewRequest(method, url, nil)
	} else {
		req, _ = http.NewRequest(method, url, bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	return resp.StatusCode, readAll(t, resp)
}

// TestRoutes_AdminPartnerFlow 覆盖 GP-103 最小运营后台闭环：
// 角色守卫 → 合作方/归属 → 资料提审 → 审核 → 上下架 → 公开目录口径 + 审计。
func TestRoutes_AdminPartnerFlow(t *testing.T) {
	base, sign := newAuthedTestServer(t)
	admin := sign(claimsFor(1, "admin"))
	user2 := sign(claimsFor(2, "carol"))
	player := sign(claimsFor(7, "bob"))
	var code int
	var body string

	// 玩家访问后台 → 403（角色守卫）
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/partners", player, `{"name":"X"}`); code != http.StatusForbidden {
		t.Fatalf("player POST /admin/partners = %d, want 403", code)
	}
	if code, _ := doAuthed(t, http.MethodGet, base+"/admin/reviews", player, ""); code != http.StatusForbidden {
		t.Fatalf("player GET /admin/reviews = %d, want 403", code)
	}

	// GET /me 返回各自角色（引导 admin / 默认 player）
	if code, body = doAuthed(t, http.MethodGet, base+"/me", admin, ""); code != http.StatusOK || !strings.Contains(body, `"role":"platform_admin"`) {
		t.Fatalf("admin GET /me = %d body=%s", code, body)
	}
	if code, body = doAuthed(t, http.MethodGet, base+"/me", player, ""); code != http.StatusOK || !strings.Contains(body, `"role":"player"`) {
		t.Fatalf("player GET /me = %d body=%s", code, body)
	}

	// admin 建合作方
	code, body = doAuthed(t, http.MethodPost, base+"/admin/partners", admin, `{"name":"星辉互娱","contact_email":"star@x.com"}`)
	if code != http.StatusOK {
		t.Fatalf("create partner = %d body=%s", code, body)
	}
	var partnerResp struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &partnerResp)
	if partnerResp.Id <= 0 {
		t.Fatalf("create partner returned id=%d body=%s", partnerResp.Id, body)
	}

	// 授权 user2 为 operator
	if code, body = doAuthed(t, http.MethodPost, base+"/admin/roles", admin, `{"user_id":2,"role":"operator"}`); code != http.StatusOK {
		t.Fatalf("grant operator = %d body=%s", code, body)
	}

	// operator 可查审核队列（空）
	if code, _ = doAuthed(t, http.MethodGet, base+"/admin/reviews", user2, ""); code != http.StatusOK {
		t.Fatalf("operator GET /admin/reviews = %d, want 200", code)
	}

	// 指派游戏归属
	if code, body = doAuthed(t, http.MethodPost, base+"/admin/games/game-elden-ring/ownership", admin,
		`{"partner_id":`+jsonNumber(partnerResp.Id)+`}`); code != http.StatusOK {
		t.Fatalf("assign ownership = %d body=%s", code, body)
	}

	// 授权 user2 为 partner（绑定合作方）——覆盖自助入口
	if code, body = doAuthed(t, http.MethodPost, base+"/admin/roles", admin,
		`{"user_id":2,"role":"partner","partner_id":`+jsonNumber(partnerResp.Id)+`}`); code != http.StatusOK {
		t.Fatalf("grant partner = %d body=%s", code, body)
	}

	// partner 查看自有游戏
	if code, body = doAuthed(t, http.MethodGet, base+"/partner/games", user2, ""); code != http.StatusOK || !strings.Contains(body, "game-elden-ring") {
		t.Fatalf("partner games = %d body=%s", code, body)
	}

	// 资料提审（置 pending）
	if code, body = doAuthed(t, http.MethodPost, base+"/partner/games/game-elden-ring/submit", user2, `{}`); code != http.StatusOK || !strings.Contains(body, `"status":"pending"`) {
		t.Fatalf("submit review = %d body=%s", code, body)
	}
	// 重复提审 → 409
	if code, _ = doAuthed(t, http.MethodPost, base+"/partner/games/game-elden-ring/submit", user2, `{}`); code != http.StatusConflict {
		t.Fatalf("duplicate submit = %d, want 409", code)
	}

	// 驳回必须带原因 → 400
	code, body = doAuthed(t, http.MethodGet, base+"/admin/reviews?status=pending", admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"pending"`) {
		t.Fatalf("list pending = %d body=%s", code, body)
	}
	var reviewsResp struct {
		Reviews []struct {
			Id int64 `json:"id"`
		} `json:"reviews"`
	}
	_ = json.Unmarshal([]byte(body), &reviewsResp)
	if len(reviewsResp.Reviews) == 0 {
		t.Fatalf("pending reviews empty body=%s", body)
	}
	reviewID := reviewsResp.Reviews[0].Id
	if code, _ = doAuthed(t, http.MethodPost, base+"/admin/reviews/"+jsonNumber(reviewID)+"/decision", admin, `{"decision":"reject"}`); code != http.StatusBadRequest {
		t.Fatalf("reject without reason = %d, want 400", code)
	}
	// 审核通过
	if code, body = doAuthed(t, http.MethodPost, base+"/admin/reviews/"+jsonNumber(reviewID)+"/decision", admin, `{"decision":"approve"}`); code != http.StatusOK {
		t.Fatalf("approve review = %d body=%s", code, body)
	}

	// 上下架
	if code, body = doAuthed(t, http.MethodPost, base+"/admin/games/game-elden-ring/unpublish", admin, ""); code != http.StatusOK || !strings.Contains(body, `"status":"offline"`) {
		t.Fatalf("unpublish = %d body=%s", code, body)
	}
	// 公开目录不再出现下架游戏
	if resp, err := http.Get(base + "/games"); err == nil {
		pubBody := readAll(t, resp)
		if strings.Contains(pubBody, "game-elden-ring") {
			t.Fatalf("offline game still in public list: %s", pubBody)
		}
	}
	// 重新上架
	if code, body = doAuthed(t, http.MethodPost, base+"/admin/games/game-elden-ring/publish", admin, ""); code != http.StatusOK || !strings.Contains(body, `"status":"approved"`) {
		t.Fatalf("publish = %d body=%s", code, body)
	}

	// 审计日志记录了后台动作
	if code, body = doAuthed(t, http.MethodGet, base+"/admin/audit", admin, ""); code != http.StatusOK || !strings.Contains(body, "role_grant") {
		t.Fatalf("audit list = %d body=%s", code, body)
	}
}

// TestRoutes_PartnerVersionFlow 合作方版本提审：建版本 → 提审 → 审核通过。
func TestRoutes_PartnerVersionFlow(t *testing.T) {
	base, sign := newAuthedTestServer(t)
	admin := sign(claimsFor(1, "admin"))
	user2 := sign(claimsFor(2, "carol"))

	// 建合作方并授权 user2
	_, body := doAuthed(t, http.MethodPost, base+"/admin/partners", admin, `{"name":"版本厂"}`)
	code := 0
	var partnerResp struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &partnerResp)
	doAuthed(t, http.MethodPost, base+"/admin/roles", admin, `{"user_id":2,"role":"partner","partner_id":`+jsonNumber(partnerResp.Id)+`}`)
	doAuthed(t, http.MethodPost, base+"/admin/games/game-elden-ring/ownership", admin, `{"partner_id":`+jsonNumber(partnerResp.Id)+`}`)

	// 建版本（draft）
	if code, body = doAuthed(t, http.MethodPost, base+"/partner/games/game-elden-ring/versions", user2,
		`{"version":"1.0.0","changelog":"首个版本"}`); code != http.StatusOK || !strings.Contains(body, `"status":"draft"`) {
		t.Fatalf("create version = %d body=%s", code, body)
	}
	var verResp struct {
		Id string `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &verResp)

	// 版本提审
	if code, body = doAuthed(t, http.MethodPost, base+"/partner/games/game-elden-ring/submit", user2,
		`{"version_id":"`+verResp.Id+`"}`); code != http.StatusOK || !strings.Contains(body, "pending") {
		t.Fatalf("submit version = %d body=%s", code, body)
	}
	var submitResp struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &submitResp)

	// 审核通过
	if code, body = doAuthed(t, http.MethodPost, base+"/admin/reviews/"+jsonNumber(submitResp.Id)+"/decision", admin,
		`{"decision":"approve"}`); code != http.StatusOK {
		t.Fatalf("approve version review = %d body=%s", code, body)
	}
	// 版本列表状态 approved
	if code, body = doAuthed(t, http.MethodGet, base+"/partner/games/game-elden-ring/versions", user2, ""); code != http.StatusOK || !strings.Contains(body, `"status":"approved"`) {
		t.Fatalf("versions after approve = %d body=%s", code, body)
	}
}

func jsonNumber(v int64) string {
	return strconv.FormatInt(v, 10)
}
