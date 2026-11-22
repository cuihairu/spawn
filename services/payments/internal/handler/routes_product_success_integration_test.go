package handler

// productlogic.go 剩余补漏分支：ListByGame 成功臂（≥1 商品 → items 组装）、
// SetEnabled 成功臂与玩家 403 守卫。经 HTTP 层打到 ListByGame/SetEnabled 的
// 成功路径与角色守卫。

import (
	"net/http"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/payments/utils"
)

func TestRoutes_ProductSuccessArms(t *testing.T) {
	env := newPaymentsServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	// 建一个商品（game-a），供 ListByGame 成功臂有 ≥1 行。
	code, body := do(t, http.MethodPost, env.base+"/admin/products/sku-ok", admin,
		`{"product_id":"ok","form":"DIRECT_PURCHASE","amount_cents":100,"currency":"CNY","game_id":"game-a"}`)
	if code != http.StatusOK {
		t.Fatalf("create product = %d body=%s", code, body)
	}

	// ListByGame 成功：items 含新建商品
	code, body = do(t, http.MethodGet, env.base+"/admin/games/game-a/products", admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"product_id":"ok"`) {
		t.Fatalf("list products success = %d body=%s", code, body)
	}

	// SetEnabled 成功（上架）→ 200
	code, body = do(t, http.MethodPatch, env.base+"/admin/products/sku-ok/enabled", admin, `{"enabled":true}`)
	if code != http.StatusOK || !strings.Contains(body, `"enabled":true`) {
		t.Fatalf("set enabled success = %d body=%s", code, body)
	}
	// SetEnabled 玩家 403 守卫
	code, _ = do(t, http.MethodPatch, env.base+"/admin/products/sku-ok/enabled", player, `{"enabled":true}`)
	if code != http.StatusForbidden {
		t.Fatalf("player set enabled = %d, want 403", code)
	}
}
