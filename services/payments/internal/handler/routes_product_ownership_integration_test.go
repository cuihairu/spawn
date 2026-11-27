package handler

// GP-202 F15 商户归属核验回归：POST /admin/products/:id 的 partner_id 不再
// 信管理面入参——以 game-catalog /internal/games/:id/ownership 权威核验。
// 声明与归属不一致 403、无归属/吊销 404、目录服务不可达 502，全部拒绝落库；
// 匹配时按归属落库（对账分成账本的归属键由此闭环）。

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/payments/internal/config"
	"github.com/tappi/tappi/services/payments/utils"
)

func createOwnershipProduct(t *testing.T, env *testEnv, sku, body string) (int, string) {
	t.Helper()
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	return do(t, http.MethodPost, env.base+"/admin/products/"+sku, admin, body)
}

func TestRoutes_ProductPartnerOwnership(t *testing.T) {
	env := newPaymentsServer(t)

	// 匹配：game-a 存根归属 partner 2 → 200，落库 partner 与权威一致
	code, body := createOwnershipProduct(t, env, "sku-own-ok",
		`{"product_id":"own-ok","form":"DIRECT_PURCHASE","amount_cents":100,"currency":"CNY","game_id":"game-a","partner_id":2}`)
	if code != http.StatusOK {
		t.Fatalf("matched ownership = %d body=%s", code, body)
	}
	var created struct {
		PartnerId int64 `json:"partner_id"`
	}
	if err := json.Unmarshal([]byte(body), &created); err != nil || created.PartnerId != 2 {
		t.Fatalf("stored partner = %s err=%v, want partner_id 2", body, err)
	}

	// 不一致：声明 partner 9 与归属 2 冲突 → 403，且商品未落库
	code, _ = createOwnershipProduct(t, env, "sku-own-mismatch",
		`{"product_id":"own-mismatch","form":"DIRECT_PURCHASE","amount_cents":100,"currency":"CNY","game_id":"game-a","partner_id":9}`)
	if code != http.StatusForbidden {
		t.Fatalf("mismatched ownership = %d body=%s, want 403", code, body)
	}
	code, body = do(t, http.MethodGet, env.base+"/admin/games/game-a/products", env.sign(utils.JWTClaims{UserId: 1}), "")
	if code != http.StatusOK || strings.Contains(body, "own-mismatch") || !strings.Contains(body, "own-ok") {
		t.Fatalf("product list after mismatch should only contain own-ok, got %d body=%s", code, body)
	}

	// 无归属记录（noowner*）→ 404；吊销（revoked*）同样 404
	code, _ = createOwnershipProduct(t, env, "sku-own-missing",
		`{"product_id":"own-missing","form":"DIRECT_PURCHASE","amount_cents":100,"currency":"CNY","game_id":"noowner-game","partner_id":2}`)
	if code != http.StatusNotFound {
		t.Fatalf("missing ownership = %d, want 404", code)
	}
	code, _ = createOwnershipProduct(t, env, "sku-own-revoked",
		`{"product_id":"own-revoked","form":"DIRECT_PURCHASE","amount_cents":100,"currency":"CNY","game_id":"revoked-game","partner_id":2}`)
	if code != http.StatusNotFound {
		t.Fatalf("revoked ownership = %d, want 404", code)
	}

	// 目录归属服务 500 → 502 fail-closed（无法核验就拒绝创建）
	code, _ = createOwnershipProduct(t, env, "sku-own-down",
		`{"product_id":"own-down","form":"DIRECT_PURCHASE","amount_cents":100,"currency":"CNY","game_id":"err500-game","partner_id":2}`)
	if code != http.StatusBadGateway {
		t.Fatalf("ownership service down = %d, want 502", code)
	}

	// 令牌不匹配（S2S 令牌错配）→ 目录 403 → 502 fail-closed
	env2 := newPaymentsServer(t, func(c *config.Config) { c.Internal.Token = "mismatched-token" })
	code, _ = createOwnershipProduct(t, env2, "sku-own-token",
		`{"product_id":"own-token","form":"DIRECT_PURCHASE","amount_cents":100,"currency":"CNY","game_id":"game-a","partner_id":2}`)
	if code != http.StatusBadGateway {
		t.Fatalf("token mismatched = %d, want 502", code)
	}
}
