package handler

// routes.go 补漏分支：认证中间件空令牌 401（直调中间件 handler，绕开 HTTP
// 走线对头值尾随空格的剥离）与无效令牌 401；clientIP 的 XFF 多跳/单跳/
// 无端口 RemoteAddr 臂与 authHeaderFrom 兜底空串；各 handler 的解析失败臂
// （application/json + 非 JSON 体 → httpx.Parse 报错）与逻辑层错误透传臂
// （映射缺失 404 / 撤销令牌空 400 / 非 operator 403）。
//
// 登记不可达（不测）：无（全部臂经上述场景触达）。

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/courier-adapter/internal/svc"
	"github.com/tappi/tappi/services/courier-adapter/utils"
)

// TestAuthMiddleware_EmptyTokenArm 直调中间件：Bearer 前缀 + 空令牌 → 401
// 「令牌不能为空」（走线后头值尾随空格被剥离，只能直调触达）。
func TestAuthMiddleware_EmptyTokenArm(t *testing.T) {
	mw := AuthMiddleware(&svc.ServiceContext{Auth: utils.NewAuth("mw-test-secret")})
	called := false
	next := func(w http.ResponseWriter, r *http.Request) { called = true }

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/games/g1/session", nil)
	req.Header.Set("Authorization", "Bearer ")
	mw(next)(rec, req)
	if called || rec.Code != http.StatusUnauthorized ||
		!strings.Contains(rec.Body.String(), "令牌不能为空") {
		t.Fatalf("empty token = %d called=%v body=%s", rec.Code, called, rec.Body.String())
	}
}

// TestClientIP_Arms clientIP 的 XFF 多跳（取首跳）、XFF 单跳（原样）、
// 无端口 RemoteAddr（SplitHostPort 失败回退原样）与正常有端口臂。
func TestClientIP_Arms(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		remote  string
		want    string
	}{
		{"xff multi", map[string]string{"X-Forwarded-For": "1.2.3.4, 5.6.7.8"}, "", "1.2.3.4"},
		{"xff single", map[string]string{"X-Forwarded-For": "9.9.9.9"}, "", "9.9.9.9"},
		{"no port remote", nil, "1.2.3.4", "1.2.3.4"},
		{"port remote", nil, "1.2.3.4:5678", "1.2.3.4"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
			for k, v := range c.headers {
				req.Header.Set(k, v)
			}
			req.RemoteAddr = c.remote
			if got := clientIP(req); got != c.want {
				t.Fatalf("clientIP = %q, want %q", got, c.want)
			}
		})
	}
}

// TestAuthHeaderFrom_DefaultArm 无注入值 → 兜底空串。
func TestAuthHeaderFrom_DefaultArm(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	if got := authHeaderFrom(req); got != "" {
		t.Fatalf("authHeaderFrom = %q, want empty", got)
	}
}

// TestRoutes_ParseErrorArms 每个 handler 的解析失败臂：application/json +
// 非 JSON 体使 httpx.Parse 报错 → 400。player 路由用 player 令牌、
// admin 路由用 operator 令牌。
func TestRoutes_ParseErrorArms(t *testing.T) {
	env := newTestEnv(t)
	base := env.base
	player := signToken(t, 77, "player")
	operator := signToken(t, 42, "ops")
	bad := "not-json"

	cases := []struct {
		name, method, path, token, body string
	}{
		{"game session", http.MethodPost, "/games/g1/session", player, bad},
		{"redeem", http.MethodPost, "/session/redeem", "", bad},
		{"delete account", http.MethodPost, "/games/g1/account/delete", player, bad},
		{"cancel delete", http.MethodPost, "/games/g1/account/delete/cancel", player, bad},
		{"get mapping", http.MethodGet, "/admin/games/g1/mapping", operator, bad},
		{"upsert mapping", http.MethodPut, "/admin/games/g1/mapping", operator, bad},
		{"create support", http.MethodPost, "/games/g1/support/tickets", player, bad},
		{"list support", http.MethodGet, "/games/g1/support/tickets", player, bad},
		{"support detail", http.MethodGet, "/games/g1/support/tickets/tkt_1", player, bad},
		{"append support", http.MethodPost, "/games/g1/support/tickets/tkt_1/messages", player, bad},
		{"support faq", http.MethodGet, "/games/g1/support/faq", player, bad},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, body := doJSON(t, c.method, base+c.path, c.token, c.body)
			if code != http.StatusBadRequest {
				t.Fatalf("%s parse fail = %d body=%s, want 400", c.name, code, body)
			}
		})
	}
}

// TestRoutes_LogicErrorArms 各 handler 逻辑层错误透传臂（httpx.ErrorCtx 回
// 吐 logic 状态码）：注销映射缺失 404、撤销令牌空 400、映射读缺失 404、
// 审计非 operator 403、工单列表/详情/FAQ 映射缺失 404。无效令牌 401 一并
// 在此触达（走线可达）。
func TestRoutes_LogicErrorArms(t *testing.T) {
	env := newTestEnv(t)
	base := env.base
	player := signToken(t, 77, "player")
	operator := signToken(t, 42, "ops")

	cases := []struct {
		name, method, path, token, body string
		want                            int
	}{
		{"invalid token", http.MethodPost, "/games/g1/session", "garbage", "", http.StatusUnauthorized},
		{"delete missing mapping", http.MethodPost, "/games/missing/account/delete", player, "", http.StatusNotFound},
		{"cancel empty token", http.MethodPost, "/games/g1/account/delete/cancel", player, `{"cancelToken":""}`, http.StatusBadRequest},
		{"get mapping missing", http.MethodGet, "/admin/games/missing/mapping", operator, "", http.StatusNotFound},
		{"audit as player", http.MethodGet, "/admin/audit", player, "", http.StatusForbidden},
		{"list missing mapping", http.MethodGet, "/games/missing/support/tickets", player, "", http.StatusNotFound},
		{"detail missing mapping", http.MethodGet, "/games/missing/support/tickets/tkt_1", player, "", http.StatusNotFound},
		{"faq missing mapping", http.MethodGet, "/games/missing/support/faq", player, "", http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, body := doJSON(t, c.method, base+c.path, c.token, c.body)
			if code != c.want {
				t.Fatalf("%s = %d body=%s, want %d", c.name, code, body, c.want)
			}
		})
	}
}
