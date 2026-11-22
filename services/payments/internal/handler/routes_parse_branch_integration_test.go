package handler

// routes.go 剩余补漏分支：AuthMiddleware 的 401 三臂（缺 Bearer 前缀 / 空令牌 /
// 无效令牌）、clientIP 的 X-Forwarded-For 两臂，以及各 handler 的 httpx.Parse
// 失败臂（畸形 JSON body → 400 返回，绕过角色守卫与逻辑层）。

import (
	"net/http"
	"testing"

	"github.com/tappi/tappi/services/payments/utils"
)

func TestRoutes_AuthMiddlewareArms(t *testing.T) {
	env := newPaymentsServer(t)

	// 缺 Bearer 前缀 → 401
	if code, _ := do(t, http.MethodGet, env.base+"/games/game-a/skus", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("no auth header = %d, want 401", code)
	}
	// Bearer 空令牌 → 401
	if code, _ := do(t, http.MethodGet, env.base+"/games/game-a/skus", "Bearer ", ""); code != http.StatusUnauthorized {
		t.Fatalf("empty token = %d, want 401", code)
	}
	// 无效令牌（签名错）→ 401
	if code, _ := do(t, http.MethodGet, env.base+"/games/game-a/skus", "Bearer garbage.token.here", ""); code != http.StatusUnauthorized {
		t.Fatalf("bad token = %d, want 401", code)
	}
}

func TestRoutes_ClientIPArms(t *testing.T) {
	env := newPaymentsServer(t)
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	// X-Forwarded-For 多级（取首个）与单级
	req, _ := http.NewRequest(http.MethodGet, env.base+"/games/game-a/skus", nil)
	req.Header.Set("Authorization", "Bearer "+player)
	req.Header.Set("X-Forwarded-For", "203.0.113.5, 198.51.100.7")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("xff multi: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("xff multi = %d, want 200", resp.StatusCode)
	}

	req2, _ := http.NewRequest(http.MethodGet, env.base+"/games/game-a/skus", nil)
	req2.Header.Set("Authorization", "Bearer "+player)
	req2.Header.Set("X-Forwarded-For", "203.0.113.5")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("xff single: %v", err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("xff single = %d, want 200", resp2.StatusCode)
	}
}

// 各 handler 的 httpx.Parse 失败臂：畸形 JSON body → 400。
func TestRoutes_ParseErrorArms(t *testing.T) {
	env := newPaymentsServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})

	endpoints := []struct {
		method, url string
	}{
		{http.MethodPost, "/games/game-a/orders"},
		{http.MethodGet, "/games/game-a/orders"},
		{http.MethodGet, "/games/game-a/orders/o1"},
		{http.MethodGet, "/games/game-a/skus"},
		{http.MethodPost, "/admin/products/x"},
		{http.MethodPatch, "/admin/products/x/enabled"},
		{http.MethodGet, "/admin/games/game-a/products"},
		{http.MethodPut, "/admin/games/game-a/delivery"},
		{http.MethodGet, "/admin/games/game-a/delivery"},
		{http.MethodGet, "/admin/deliveries"},
		{http.MethodPost, "/admin/deliveries/o/redeliver"},
		{http.MethodPatch, "/admin/deliveries/o/target"},
		{http.MethodGet, "/admin/orders"},
		{http.MethodGet, "/admin/orders/o"},
		{http.MethodPost, "/admin/orders/o/refund"},
		{http.MethodGet, "/admin/refunds"},
		{http.MethodGet, "/admin/audit"},
		{http.MethodPost, "/admin/statements"},
		{http.MethodGet, "/admin/statements"},
		{http.MethodGet, "/admin/statements/1"},
		{http.MethodPut, "/admin/statements/1/settlement"},
		{http.MethodPost, "/admin/statements/1/confirm"},
		{http.MethodGet, "/admin/statements/1/export"},
		{http.MethodPost, "/admin/statements/1/resolve"},
		{http.MethodGet, "/admin/statements/1/disputes"},
		{http.MethodGet, "/partner/statements"},
		{http.MethodGet, "/partner/statements/1"},
		{http.MethodGet, "/partner/statements/1/export"},
		{http.MethodPost, "/partner/statements/1/confirm"},
		{http.MethodPost, "/partner/statements/1/dispute"},
		{http.MethodGet, "/partner/statements/differences"},
	}
	for _, ep := range endpoints {
		code, body := do(t, ep.method, env.base+ep.url, admin, `{`)
		if code != http.StatusBadRequest {
			t.Fatalf("%s %s parse err = %d body=%s, want 400", ep.method, ep.url, code, body)
		}
	}
}
