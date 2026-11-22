package handler

// GP-401/GP-403 handler 逻辑错误分支：Parse 通过但 logic 返回错误
// （400 校验 / 404 不存在）→ httpx.ErrorCtx 覆盖。
// 与 routes_parse_errors_integration_test.go（Parse 400）互补。

import (
	"net/http"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
)

// 默认服务端：各 /admin 与玩家/公开路由触达 logic 错误分支。
func TestRoutes_LogicErrors(t *testing.T) {
	base, sign := newAuthedTestServer(t)
	admin := sign(claimsFor(1, "admin"))
	player := sign(claimsFor(7, "bob"))

	cases := []struct {
		name     string
		method   string
		url      string
		token    string
		body     string
		wantCode int
	}{
		// admin 面：logic 400 校验
		{"grant role user_id 0", http.MethodPost, base + "/admin/roles", admin, `{"user_id":0,"role":"operator"}`, http.StatusBadRequest},
		{"create partner empty name", http.MethodPost, base + "/admin/partners", admin, `{"name":"  "}`, http.StatusBadRequest},
		{"channel stats empty code", http.MethodGet, base + "/admin/promo/channels/%20/stats", admin, "", http.StatusBadRequest},
		// admin 面：logic 404 不存在（游戏不存在 → AssignOwnership/Publish/Unpublish）
		{"assign ownership unknown game", http.MethodPost, base + "/admin/games/g-ghost/ownership", admin, `{"partner_id":1}`, http.StatusNotFound},
		{"publish unknown game", http.MethodPost, base + "/admin/games/g-ghost/publish", admin, "", http.StatusNotFound},
		{"unpublish unknown game", http.MethodPost, base + "/admin/games/g-ghost/unpublish", admin, "", http.StatusNotFound},
		// 公开读：游戏不存在 → 404（visibleGame）
		{"list reviews unknown game", http.MethodGet, base + "/games/g-ghost/reviews", "", "", http.StatusNotFound},
		// 玩家写：游戏不存在 → 404（visibleGame）
		{"unvote unknown game", http.MethodDelete, base + "/games/g-ghost/reviews/1/vote", player, "", http.StatusNotFound},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _ := doAuthed(t, c.method, c.url, c.token, c.body)
			if code != c.wantCode {
				t.Fatalf("%s %s = %d, want %d", c.method, c.url, code, c.wantCode)
			}
		})
	}
}

// InternalResolveClick：配置 Internal.Token 后，S2S 守卫通过与两类 400：
// 坏 JSON（Parse 臂）与空白 click_id（logic 校验臂）。
func TestRoutes_InternalResolveLogicError(t *testing.T) {
	base, _ := newAuthedTestServerConfig(t, func(c *config.Config) {
		c.Internal.Token = "s2s-token"
	})
	cases := []struct {
		name string
		body string
	}{
		{"parse error", `{`},
		{"empty click id", `{"click_id":"  ","registered_at":"2026-01-01T00:00:00Z"}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodPost, base+"/internal/promo/clicks/x/resolve",
				strings.NewReader(c.body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Internal-Token", "s2s-token")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("internal resolve: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusBadRequest {
				t.Fatalf("internal resolve %s = %d, want 400", c.name, resp.StatusCode)
			}
		})
	}
}
