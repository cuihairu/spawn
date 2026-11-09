package handler

// GP-204 运营订单面集成测试：玩家我的订单列表（Courier 只读转发）、
// 订单投影（CREATED→PAID→DELIVERED 状态来自 Courier 上报）、运营检索 +
// 审计、退款/冲正管理适配（登记 PENDING 意图，执行待 Courier 端点，不
// 伪造成功）。复用 newDeliveryServer 的存根与 do/dispatchRaw 骨架。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/payments/utils"
)

func createOrder(t *testing.T, env *testDeliveryEnv, player, body string) string {
	t.Helper()
	code, resp := do(t, http.MethodPost, env.base+"/games/game-a/orders", player, body)
	if code != http.StatusOK {
		t.Fatalf("create order = %d body=%s", code, resp)
	}
	var o struct {
		Order struct {
			Id string `json:"id"`
		} `json:"order"`
	}
	if err := json.Unmarshal([]byte(resp), &o); err != nil || o.Order.Id == "" {
		t.Fatalf("parse order: %v body=%s", err, resp)
	}
	return o.Order.Id
}

func TestRoutes_OrdersProjectionAndRefund(t *testing.T) {
	env := newDeliveryServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	// 前置：商品 + 游戏服发货接口
	code, body := do(t, http.MethodPost, env.base+"/admin/products/sku-gem-100", admin,
		`{"product_id":"gem-100","form":"DIRECT_PURCHASE","amount_cents":1000,"currency":"CNY","game_id":"game-a","partner_id":2}`)
	if code != http.StatusOK {
		t.Fatalf("create product = %d body=%s", code, body)
	}
	code, _ = do(t, http.MethodPut, env.base+"/admin/games/game-a/delivery", admin,
		fmt.Sprintf(`{"endpoint":%q,"secret":"gs-secret"}`, env.gameServer.srv.URL))
	if code != http.StatusOK {
		t.Fatalf("configure endpoint = %d", code)
	}

	// 1. 玩家我的订单列表（Courier 只读转发）
	orderID := createOrder(t, env, player, `{"sku_id":"sku-gem-100","server_id":1}`)
	code, body = do(t, http.MethodGet, env.base+"/games/game-a/orders", player, "")
	if code != http.StatusOK || !strings.Contains(body, orderID) {
		t.Fatalf("list my orders = %d body=%s", code, body)
	}

	// 2. 投影 CREATED → 渠道回调 PAID → 发货投递 DELIVERED（状态来自 Courier 上报）
	code, body = do(t, http.MethodGet, env.base+"/admin/orders/"+orderID, admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"CREATED"`) {
		t.Fatalf("projection created = %d body=%s", code, body)
	}
	cbBody := fmt.Sprintf(`{"orderId":%q,"paidAt":"2026-11-09T12:01:00.000Z"}`, orderID)
	req, _ := http.NewRequest(http.MethodPost, env.base+"/callback/sandbox", strings.NewReader(cbBody))
	req.Header.Set("X-Payment-Signature", deliverySig(testSandboxSecret, cbBody))
	resp, _ := http.DefaultClient.Do(req)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sandbox callback = %d", resp.StatusCode)
	}
	code, body = do(t, http.MethodGet, env.base+"/admin/orders/"+orderID, admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"PAID"`) {
		t.Fatalf("projection paid = %d body=%s", code, body)
	}
	dispatchBody := fmt.Sprintf(`{"orderId":%q,"accountId":"7","skuId":"sku-gem-100","productId":"gem-100","form":"DIRECT_PURCHASE","amountCents":1000,"currency":"CNY"}`, orderID)
	code, _ = dispatchRaw(t, env.base+"/internal/deliveries", dispatchBody, deliverySig(testDeliverySecret, dispatchBody))
	if code != http.StatusOK {
		t.Fatalf("dispatch = %d", code)
	}
	code, body = do(t, http.MethodGet, env.base+"/admin/orders/"+orderID, admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"DELIVERED"`) {
		t.Fatalf("projection delivered = %d body=%s", code, body)
	}

	// 3. 运营检索：按游戏/状态筛 + 审计已记（order_query/order_view）
	code, body = do(t, http.MethodGet, env.base+"/admin/orders?game_id=game-a&status=DELIVERED", admin, "")
	if code != http.StatusOK || !strings.Contains(body, orderID) {
		t.Fatalf("admin list orders = %d body=%s", code, body)
	}
	// order_view 按订单记审计；order_query 是全局查询动作（无 order_id），
	// 只出现在不过滤的审计列表里。
	code, body = do(t, http.MethodGet, env.base+"/admin/audit?order_id="+orderID, admin, "")
	if code != http.StatusOK || !strings.Contains(body, "order_view") {
		t.Fatalf("audit by order = %d body=%s", code, body)
	}
	code, body = do(t, http.MethodGet, env.base+"/admin/audit", admin, "")
	if code != http.StatusOK || !strings.Contains(body, "order_query") {
		t.Fatalf("audit unfiltered = %d body=%s", code, body)
	}
	// 玩家越权
	code, _ = do(t, http.MethodGet, env.base+"/admin/orders?game_id=game-a", player, "")
	if code != http.StatusForbidden {
		t.Fatalf("player admin orders = %d, want 403", code)
	}

	// 4. 退款/冲正管理适配：登记 PENDING 意图 + 状态映射，执行待 Courier 端点
	code, body = do(t, http.MethodPost, env.base+"/admin/orders/"+orderID+"/refund", admin, `{"reason":"重复扣款"}`)
	if code != http.StatusOK || !strings.Contains(body, `"status":"PENDING"`) || !strings.Contains(body, "pending-courier-refund-endpoint") {
		t.Fatalf("create refund = %d body=%s", code, body)
	}
	// 同订单同动作重复 → 409
	code, _ = do(t, http.MethodPost, env.base+"/admin/orders/"+orderID+"/refund", admin, `{"reason":"x"}`)
	if code != http.StatusConflict {
		t.Fatalf("duplicate refund = %d, want 409", code)
	}
	// 未付款订单不可退款：不存在的订单 404，CREATED 状态 409
	code, _ = do(t, http.MethodPost, env.base+"/admin/orders/order_never_paid/refund", admin, `{"reason":"x"}`)
	if code != http.StatusNotFound {
		t.Fatalf("refund unknown order = %d, want 404", code)
	}
	created := createOrder(t, env, player, `{"sku_id":"sku-gem-100","server_id":1}`)
	code, _ = do(t, http.MethodPost, env.base+"/admin/orders/"+created+"/refund", admin, `{"reason":"x"}`)
	if code != http.StatusConflict {
		t.Fatalf("refund created order = %d, want 409", code)
	}
	code, body = do(t, http.MethodGet, env.base+"/admin/refunds?status=PENDING", admin, "")
	if code != http.StatusOK || !strings.Contains(body, orderID) {
		t.Fatalf("list refunds = %d body=%s", code, body)
	}
	// 审计含 refund_request
	code, body = do(t, http.MethodGet, env.base+"/admin/audit?order_id="+orderID, admin, "")
	if code != http.StatusOK || !strings.Contains(body, "refund_request") {
		t.Fatalf("audit lacks refund_request: %s", body)
	}
}
