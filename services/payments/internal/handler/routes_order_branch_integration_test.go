package handler

// orderlogic.go 剩余补漏分支：未登录（user_id<=0 → 401）、商品不存在（404）、
// 充值目标区服未开放（catalog 目录无该 server → 404）、商品查询失败（500）、
// 发货目标落账失败（500）、订单投影失败（500）。
//
// 500 臂通过把 svcCtx 对应 DB 换成指向已关闭 sqlite 连接制造「数据库不可用」；
// DB 只在请求处理期被访问（NewServiceContext 不起后台 goroutine），请求之间
// 赋值无数据竞争。每个 500 场景独立起服务（换库互不影响后续步骤）。

import (
	"net/http"
	"testing"

	"github.com/tappi/tappi/services/payments/model"
	"github.com/tappi/tappi/services/payments/utils"
)

func TestRoutes_OrderUnauthorizedArms(t *testing.T) {
	env := newPaymentsServer(t)
	// user_id<=0：AuthMiddleware 放行（只验签），logic 见 user_id<=0 → 401
	anon := env.sign(utils.JWTClaims{UserId: 0, Username: "anon"})

	if code, _ := do(t, http.MethodPost, env.base+"/games/game-a/orders", anon, `{"sku_id":"sku-x"}`); code != http.StatusUnauthorized {
		t.Fatalf("create order anon = %d, want 401", code)
	}
	if code, _ := do(t, http.MethodGet, env.base+"/games/game-a/orders/o1", anon, ""); code != http.StatusUnauthorized {
		t.Fatalf("get order anon = %d, want 401", code)
	}
	if code, _ := do(t, http.MethodGet, env.base+"/games/game-a/orders", anon, ""); code != http.StatusUnauthorized {
		t.Fatalf("list my orders anon = %d, want 401", code)
	}
	if code, _ := do(t, http.MethodGet, env.base+"/games/game-a/skus", anon, ""); code != http.StatusUnauthorized {
		t.Fatalf("list skus anon = %d, want 401", code)
	}
}

func TestRoutes_OrderProductNotFound(t *testing.T) {
	env := newPaymentsServer(t)
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	// 商品不存在 → 404（CreateOrder 读商品前置）
	code, _ := do(t, http.MethodPost, env.base+"/games/game-a/orders", player, `{"sku_id":"sku-nope"}`)
	if code != http.StatusNotFound {
		t.Fatalf("order missing sku = %d, want 404", code)
	}
}

func TestRoutes_OrderServerNotOpen(t *testing.T) {
	env := newPaymentsServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	do(t, http.MethodPost, env.base+"/admin/products/sku-server", admin,
		`{"product_id":"server-sku","form":"DIRECT_PURCHASE","amount_cents":100,"currency":"CNY","game_id":"game-a"}`)
	// 游戏目录存根对 /games/*/servers 无 items → 区服不存在/未开放 → 404
	code, _ := do(t, http.MethodPost, env.base+"/games/game-a/orders", player, `{"sku_id":"sku-server","server_id":5}`)
	if code != http.StatusNotFound {
		t.Fatalf("order closed server = %d, want 404", code)
	}
}

// 商品查询失败 → 500（ProductDB 换闭库）。
func TestRoutes_OrderProductQueryError(t *testing.T) {
	env := newPaymentsServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})
	do(t, http.MethodPost, env.base+"/admin/products/sku-qerr", admin,
		`{"product_id":"p","form":"DIRECT_PURCHASE","amount_cents":100,"currency":"CNY","game_id":"game-a"}`)

	env.svcCtx.ProductDB = model.NewProductModel(closedDB(t))
	code, _ := do(t, http.MethodPost, env.base+"/games/game-a/orders", player, `{"sku_id":"sku-qerr"}`)
	if code != http.StatusInternalServerError {
		t.Fatalf("order product query err = %d, want 500", code)
	}
}

// 发货目标落账失败 → 500（DeliveryDB 换闭库；商品/Courier/投影真实）。
func TestRoutes_OrderCreateTargetError(t *testing.T) {
	env := newPaymentsServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})
	do(t, http.MethodPost, env.base+"/admin/products/sku-cterr", admin,
		`{"product_id":"p","form":"DIRECT_PURCHASE","amount_cents":1000,"currency":"CNY","game_id":"game-a"}`)

	env.svcCtx.DeliveryDB = model.NewDeliveryModel(closedDB(t))
	code, _ := do(t, http.MethodPost, env.base+"/games/game-a/orders", player, `{"sku_id":"sku-cterr"}`)
	if code != http.StatusInternalServerError {
		t.Fatalf("order create target err = %d, want 500", code)
	}
}

// 订单投影失败 → 500（ProjectionDB 换闭库；CreateTarget 成功、投影失败回滚）。
func TestRoutes_OrderProjectionError(t *testing.T) {
	env := newPaymentsServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})
	do(t, http.MethodPost, env.base+"/admin/products/sku-perr", admin,
		`{"product_id":"p","form":"DIRECT_PURCHASE","amount_cents":1000,"currency":"CNY","game_id":"game-a"}`)

	env.svcCtx.ProjectionDB = model.NewOrderProjectionModel(closedDB(t))
	code, _ := do(t, http.MethodPost, env.base+"/games/game-a/orders", player, `{"sku_id":"sku-perr"}`)
	if code != http.StatusInternalServerError {
		t.Fatalf("order projection err = %d, want 500", code)
	}
}
