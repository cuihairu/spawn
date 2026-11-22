package handler

// handler 补漏分支：AuthMiddleware 三档 401（无 Bearer/空令牌/无效令牌）、
// clientIP 三臂（XFF 逗号/单值/RemoteAddr 降级）+ authHeaderFrom 缺上下文
// 回退、管理面/玩家侧 handler 的 httpx.Parse 失败 400 臂（非法 JSON 体、
// 非数字 :id 路径、非法查询整数）、logic 层错误臂（玩家角色 403 触发；
// PlayerList/MyClaims 用闭库模型注入查询失败 500）、GetActivity/
// ListGiftPacks/GetPlayerGiftPack 成功臂、parseInt64 非数字回退。
//
// 登记不可达（不测）：ListPlayerActivities/InternalListActivities 的
// httpx.ParsePath 失败臂（gameId 为 path 必选字符串，路由恒有值且恒可解析）；
// EvaluateReward/RedeemGiftPack 的 io.ReadAll 失败臂（真实 HTTP 连接读原始体
// 不会报错，无法在协议层构造）。
//
// AuthMiddleware 空令牌臂不靠 HTTP 触发：Go 的 HTTP 解析器会剥掉头值首尾
// 空白，"Bearer " 到达服务时变成 "Bearer"（不命中 Bearer 前缀臂），故该臂
// 用合成请求直接过中间件覆盖。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/promotions/model"
	"github.com/tappi/tappi/services/promotions/utils"
)

// doRaw 发送带自定义 Authorization 头的请求（覆盖 "Bearer " 空令牌等
// do 无法表达的形态）。
func doRaw(t *testing.T, method, url, authHeader, body string) (int, string) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, _ := http.NewRequest(method, url, r)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(raw)
}

func TestRoutes_AuthMiddlewareArms(t *testing.T) {
	env := newActivityServer(t)
	url := env.base + "/admin/activities"

	// 无 Authorization 头（非 Bearer 前缀）→ 401
	if code, _ := do(t, http.MethodGet, url, "", ""); code != http.StatusUnauthorized {
		t.Fatalf("no auth = %d, want 401", code)
	}
	// "Bearer "（空令牌）→ 401
	if code, _ := doRaw(t, http.MethodGet, url, "Bearer ", ""); code != http.StatusUnauthorized {
		t.Fatalf("empty bearer = %d, want 401", code)
	}
	// 无效令牌 → 401
	if code, _ := do(t, http.MethodGet, url, "garbage.token", ""); code != http.StatusUnauthorized {
		t.Fatalf("invalid token = %d, want 401", code)
	}

	// 空令牌臂（"Bearer "）：HTTP 解析会剥尾随空白，改用合成请求直接过中间件。
	mw := AuthMiddleware(env.svcCtx)
	called := false
	h := mw(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header["Authorization"] = []string{"Bearer "}
	rec := httptest.NewRecorder()
	h(rec, req)
	if rec.Code != http.StatusUnauthorized || called {
		t.Fatalf("empty bearer middleware = %d called=%v, want 401/!called", rec.Code, called)
	}
}

func TestRoutes_ClientIPArms(t *testing.T) {
	// XFF 逗号分隔 → 取首段
	r1 := &http.Request{Header: http.Header{"X-Forwarded-For": {"1.2.3.4, 5.6.7.8"}}, RemoteAddr: "9.9.9.9:1"}
	if got := clientIP(r1); got != "1.2.3.4" {
		t.Fatalf("xff comma = %q", got)
	}
	// XFF 单值 → 去空白
	r2 := &http.Request{Header: http.Header{"X-Forwarded-For": {" 8.8.8.8"}}, RemoteAddr: "9.9.9.9:1"}
	if got := clientIP(r2); got != "8.8.8.8" {
		t.Fatalf("xff single = %q", got)
	}
	// RemoteAddr 无端口（SplitHostPort 失败）→ 原样返回
	r3 := &http.Request{RemoteAddr: "notanaddress"}
	if got := clientIP(r3); got != "notanaddress" {
		t.Fatalf("remoteaddr fallback = %q", got)
	}
	// authHeaderFrom 缺上下文键 → 空串
	if got := authHeaderFrom(&http.Request{}); got != "" {
		t.Fatalf("authHeaderFrom empty = %q", got)
	}
}

func TestRoutes_ParseErrorArms(t *testing.T) {
	env := newActivityServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	// 非法 JSON 体 / 非数字 :id 路径 / 非法查询整数触发 httpx.Parse 失败 → 400。
	cases := []struct {
		method, path, token, body string
	}{
		{http.MethodPost, "/admin/activities", admin, "{bad"},
		{http.MethodPut, "/admin/activities/1", admin, "{bad"},
		{http.MethodPost, "/admin/activities/1/publish", admin, "{bad"},
		{http.MethodPost, "/admin/activities/1/disable", admin, "{bad"},
		{http.MethodPost, "/admin/activities/1/revise", admin, "{bad"},
		{http.MethodGet, "/admin/activities", admin, "{bad"},
		{http.MethodGet, "/admin/activities/abc", admin, ""},
		{http.MethodGet, "/admin/activities/abc/versions", admin, ""},
		{http.MethodGet, "/admin/activity-audit?activity_id=abc", admin, ""},
		{http.MethodGet, "/admin/reward-grants?activity_id=abc", admin, ""},
		{http.MethodPost, "/admin/gift-packs", admin, "{bad"},
		{http.MethodPut, "/admin/gift-packs/1", admin, "{bad"},
		{http.MethodPost, "/admin/gift-packs/1/publish", admin, "{bad"},
		{http.MethodPost, "/admin/gift-packs/1/disable", admin, "{bad"},
		{http.MethodPost, "/admin/gift-packs/1/revise", admin, "{bad"},
		{http.MethodGet, "/admin/gift-packs", admin, "{bad"},
		{http.MethodGet, "/admin/gift-packs/abc", admin, ""},
		{http.MethodPost, "/admin/gift-packs/1/codes", admin, "{bad"},
		{http.MethodGet, "/admin/gift-packs/abc/codes", admin, ""},
		{http.MethodGet, "/admin/gift-claims?pack_id=abc", admin, ""},
		{http.MethodGet, "/gift-packs/abc", player, ""},
		{http.MethodPost, "/gift-packs/abc/claim", player, "{bad"},
	}
	for i, c := range cases {
		code, body := do(t, c.method, env.base+c.path, c.token, c.body)
		if code != http.StatusBadRequest {
			t.Errorf("parse case %d %s %s = %d body=%s, want 400", i, c.method, c.path, code, body)
		}
	}
}

func TestRoutes_HandlerLogicErrorArms(t *testing.T) {
	env := newActivityServer(t)
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	// 管理面逻辑错误臂：玩家角色经 requireAdminRole → 403。
	roleCases := []struct{ method, path, body string }{
		{http.MethodPut, "/admin/gift-packs/1", gpPackBody("x", false, 1)},
		{http.MethodPost, "/admin/gift-packs/1/disable", ""},
		{http.MethodPost, "/admin/gift-packs/1/revise", ""},
		{http.MethodGet, "/admin/gift-packs", ""},
		{http.MethodGet, "/admin/gift-packs/1", ""},
		{http.MethodPost, "/admin/gift-packs/1/codes", `{"codes":["X"]}`},
		{http.MethodGet, "/admin/gift-packs/1/codes", ""},
		{http.MethodGet, "/admin/gift-claims", ""},
		{http.MethodGet, "/admin/activities/1/versions", ""},
		{http.MethodGet, "/admin/activity-audit?activity_id=1", ""},
	}
	for i, c := range roleCases {
		code, body := do(t, c.method, env.base+c.path, player, c.body)
		if code != http.StatusForbidden {
			t.Errorf("role case %d %s %s = %d body=%s, want 403", i, c.method, c.path, code, body)
		}
	}

	// 活动管理面逻辑错误臂：查无活动 → 404（发布/停用/回草稿/版本列表/详情）。
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	missingCases := []struct{ method, path string }{
		{http.MethodPost, "/admin/activities/999999/publish"},
		{http.MethodPost, "/admin/activities/999999/disable"},
		{http.MethodPost, "/admin/activities/999999/revise"},
		{http.MethodGet, "/admin/activities/999999/versions"},
	}
	for i, c := range missingCases {
		code, body := do(t, c.method, env.base+c.path, admin, "")
		if code != http.StatusNotFound {
			t.Errorf("missing case %d %s %s = %d body=%s, want 404", i, c.method, c.path, code, body)
		}
	}
}

func TestRoutes_HandlerSuccessArms(t *testing.T) {
	env := newActivityServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	// 建并发布直发包，供列表/玩家侧详情成功臂。
	packID := gpCreate(t, env, admin, gpPackBody("成功臂包", false, 1))
	gpPublish(t, env, admin, packID, http.StatusOK)

	// GetActivity 成功（OkJsonCtx 臂）：先建草稿再取详情。
	code, body := do(t, http.MethodPost, env.base+"/admin/activities", admin, validActivityBody)
	if code != http.StatusOK {
		t.Fatalf("create activity = %d body=%s", code, body)
	}
	var act struct {
		Activity struct {
			Id int64 `json:"id"`
		} `json:"activity"`
	}
	_ = json.Unmarshal([]byte(body), &act)
	code, body = do(t, http.MethodGet, fmt.Sprintf("%s/admin/activities/%d", env.base, act.Activity.Id), admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"draft"`) {
		t.Fatalf("get activity success = %d body=%s", code, body)
	}

	// ListGiftPacks 成功（var+Parse/调用/OkJson 三臂）。
	code, body = do(t, http.MethodGet, env.base+"/admin/gift-packs", admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"name":"成功臂包"`) {
		t.Fatalf("list packs = %d body=%s", code, body)
	}

	// GetPlayerGiftPack 成功（var+Parse/调用/OkJson 三臂）。
	code, body = do(t, http.MethodGet, fmt.Sprintf("%s/gift-packs/%d", env.base, packID), player, "")
	if code != http.StatusOK || !strings.Contains(body, `"name":"成功臂包"`) {
		t.Fatalf("player get pack = %d body=%s", code, body)
	}

	// GetPlayerGiftPack 缺失 → 404（逻辑错误臂）。
	code, _ = do(t, http.MethodGet, env.base+"/gift-packs/999999", player, "")
	if code != http.StatusNotFound {
		t.Fatalf("player get missing = %d, want 404", code)
	}

	// parseInt64 非数字回退（server_id=abc → 0，请求仍 200）。
	code, _ = do(t, http.MethodGet, env.base+"/gift-packs?game_id=game-a&server_id=abc", player, "")
	if code != http.StatusOK {
		t.Fatalf("parseInt64 non-digit = %d, want 200", code)
	}
}

func TestRoutes_PlayerDBErrorArms(t *testing.T) {
	env := newActivityServer(t)
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	// 闭库模型：所有查询失败 → PlayerList/MyClaims 逻辑错误 500 臂。
	closed, err := model.Open("file:" + filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatalf("open closed db: %v", err)
	}
	env.svcCtx.GiftPackDB = model.NewGiftPackModel(closed)
	if err := closed.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	code, body := do(t, http.MethodGet, env.base+"/gift-packs?game_id=game-a&server_id=1", player, "")
	if code != http.StatusInternalServerError {
		t.Fatalf("player list db err = %d body=%s, want 500", code, body)
	}
	code, body = do(t, http.MethodGet, env.base+"/gift-claims/me", player, "")
	if code != http.StatusInternalServerError {
		t.Fatalf("my claims db err = %d body=%s, want 500", code, body)
	}
}
