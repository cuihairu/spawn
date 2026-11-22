package handler

// productlogic.go 剩余补漏分支：Create 校验臂（400 空 id/product_id、非法形态、
// 非正金额、币种非三字母、409 重复 id、500 落库失败）、SetEnabled 的 404/500、
// ListByGame 的 403 角色守卫/500 查询失败。
//
// 500 臂通过把 svcCtx.ProductDB 换成指向已关闭 sqlite 的连接制造「数据库
// 不可用」；ProductDB 只在请求处理期被访问（NewServiceContext 不起后台
// goroutine），请求之间赋值无数据竞争。

import (
	"database/sql"
	"net/http"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/payments/model"
	"github.com/tappi/tappi/services/payments/utils"

	_ "github.com/mattn/go-sqlite3"
)

func closedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close sqlite: %v", err)
	}
	return db
}

func TestRoutes_ProductValidationArms(t *testing.T) {
	env := newPaymentsServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	// product_id 为空 → 400
	code, _ := do(t, http.MethodPost, env.base+"/admin/products/sku-empty", admin,
		`{"product_id":"","form":"DIRECT_PURCHASE","amount_cents":1,"currency":"CNY","game_id":"game-a"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("empty product_id = %d, want 400", code)
	}
	// 非法商品形态 → 400
	code, _ = do(t, http.MethodPost, env.base+"/admin/products/sku-badform", admin,
		`{"product_id":"x","form":"BOGUS","amount_cents":1,"currency":"CNY","game_id":"game-a"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("bad form = %d, want 400", code)
	}
	// 金额必须为正 → 400
	code, _ = do(t, http.MethodPost, env.base+"/admin/products/sku-zero", admin,
		`{"product_id":"x","form":"DIRECT_PURCHASE","amount_cents":0,"currency":"CNY","game_id":"game-a"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("zero amount = %d, want 400", code)
	}
	// 币种非三字母 → 400
	code, _ = do(t, http.MethodPost, env.base+"/admin/products/sku-cnybad", admin,
		`{"product_id":"x","form":"DIRECT_PURCHASE","amount_cents":1,"currency":"CN","game_id":"game-a"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("bad currency = %d, want 400", code)
	}
	// 重复商品 id → 409
	do(t, http.MethodPost, env.base+"/admin/products/sku-dup", admin,
		`{"product_id":"p1","form":"DIRECT_PURCHASE","amount_cents":1,"currency":"CNY","game_id":"game-a"}`)
	code, _ = do(t, http.MethodPost, env.base+"/admin/products/sku-dup", admin,
		`{"product_id":"p1","form":"DIRECT_PURCHASE","amount_cents":1,"currency":"CNY","game_id":"game-a"}`)
	if code != http.StatusConflict {
		t.Fatalf("duplicate id = %d, want 409", code)
	}
	// SetEnabled 商品不存在 → 404
	code, _ = do(t, http.MethodPatch, env.base+"/admin/products/nope/enabled", admin, `{"enabled":true}`)
	if code != http.StatusNotFound {
		t.Fatalf("set enabled missing = %d, want 404", code)
	}
	// ListByGame 角色守卫：player → 403
	code, _ = do(t, http.MethodGet, env.base+"/admin/games/game-a/products", player, "")
	if code != http.StatusForbidden {
		t.Fatalf("player list products = %d, want 403", code)
	}
}

func TestRoutes_ProductDBErrorArms(t *testing.T) {
	env := newPaymentsServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})

	env.svcCtx.ProductDB = model.NewProductModel(closedDB(t))

	// Create 落库失败 → 500
	code, _ := do(t, http.MethodPost, env.base+"/admin/products/sku-500", admin,
		`{"product_id":"x","form":"DIRECT_PURCHASE","amount_cents":1,"currency":"CNY","game_id":"game-a"}`)
	if code != http.StatusInternalServerError {
		t.Fatalf("create db error = %d, want 500", code)
	}
	// SetEnabled 落库失败 → 500
	code, _ = do(t, http.MethodPatch, env.base+"/admin/products/sku-500/enabled", admin, `{"enabled":true}`)
	if code != http.StatusInternalServerError {
		t.Fatalf("set enabled db error = %d, want 500", code)
	}
	// ListByGame 查询失败 → 500
	code, _ = do(t, http.MethodGet, env.base+"/admin/games/game-a/products", admin, "")
	if code != http.StatusInternalServerError {
		t.Fatalf("list products db error = %d, want 500", code)
	}
}
