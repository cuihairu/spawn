package handler

// 覆盖 handler 成功臂（httpx.OkJsonCtx）：列表/计数类端点 200 走通。
// 与 routes_store_errors_integration_test.go（存储失败 500）互补。

import (
	"net/http"
	"testing"
)

// TestRoutes_SuccessArms 各只读/幂等端点正常路径 → 200。
func TestRoutes_SuccessArms(t *testing.T) {
	base, sign := newAuthedTestServer(t)
	admin := sign(claimsFor(1, "admin"))
	player := sign(claimsFor(7, "bob"))

	cases := []struct {
		name   string
		method string
		url    string
		token  string
	}{
		{"list reviews", http.MethodGet, base + "/admin/reviews", admin},
		{"list audit", http.MethodGet, base + "/admin/audit", admin},
		{"list promo channels", http.MethodGet, base + "/admin/promo/channels", admin},
		{"channel stats", http.MethodGet, base + "/admin/promo/channels/c1/stats", admin},
		{"list review reports", http.MethodGet, base + "/admin/review-reports", admin},
		{"list game reviews", http.MethodGet, base + "/games/game-elden-ring/reviews", ""},
		{"unfavorite", http.MethodDelete, base + "/games/game-elden-ring/favorite", player},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			code, _ := doAuthed(t, c.method, c.url, c.token, "")
			if code != http.StatusOK {
				t.Fatalf("%s %s = %d, want 200", c.method, c.url, code)
			}
		})
	}
}
