package handler

// GP-204 运营订单面集成测试：玩家我的订单列表（Courier 只读转发）、
// 订单投影（CREATED→PAID→DELIVERED 状态来自 Courier 上报）、运营检索 +
// 审计、退款/冲正管理适配（登记 PENDING 意图，执行待 Courier 端点，不
// 伪造成功）。复用 newDeliveryServer 的存根与 do/dispatchRaw 骨架。

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/payments/model"
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

	// 4. 退款/冲正管理适配：登记 PENDING 意图 + 状态映射。本测试未配
	// COURIER_PAYMENTS_ADMIN_TOKEN → 执行器为 nil（未接线），保持 PENDING 且
	// execution=pending，**不伪造 executed**；接线的执行路径见
	// TestRoutes_RefundExecutor。
	code, body = do(t, http.MethodPost, env.base+"/admin/orders/"+orderID+"/refund", admin, `{"reason":"重复扣款"}`)
	if code != http.StatusOK || !strings.Contains(body, `"status":"PENDING"`) || !strings.Contains(body, `"execution":"pending"`) {
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

// TestRoutes_RefundExecutor GP-204 退款执行器接线：配了
// COURIER_PAYMENTS_ADMIN_TOKEN 时 CreateRefund 同步首跑（PENDING → Courier
// 管理面退款端点 /v1/payments/admin/orders/{id}/refund → EXECUTED/REJECTED）；
// scope 取 GP-002 映射后的 courier_game_id+env（绝不发 spawn game id），管理
// 凭证走独立 Bearer 头；确定性拒绝（409 PAYMENT_ORDER_STATE）置 REJECTED 不
// 重试、不伪造 executed。
func TestRoutes_RefundExecutor(t *testing.T) {
	env := newDeliveryServerCfg(t, func(e *testDeliveryEnv) {
		e.courierAdminToken = "courier-admin-tok"
	})
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	code, body := do(t, http.MethodPost, env.base+"/admin/products/sku-re-1", admin,
		`{"product_id":"re-1","form":"DIRECT_PURCHASE","amount_cents":1000,"currency":"CNY","game_id":"game-a","partner_id":2}`)
	if code != http.StatusOK {
		t.Fatalf("create product = %d body=%s", code, body)
	}

	// payOrder 下单 + 渠道回调推 PAID（投影按上报落 PAID，可退款）。
	payOrder := func() string {
		id := createOrder(t, env, player, `{"sku_id":"sku-re-1","server_id":1}`)
		cb := fmt.Sprintf(`{"orderId":%q,"paidAt":"2026-11-23T12:00:00.000Z"}`, id)
		req, _ := http.NewRequest(http.MethodPost, env.base+"/callback/sandbox", strings.NewReader(cb))
		req.Header.Set("X-Payment-Signature", deliverySig(testSandboxSecret, cb))
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("sandbox callback: %v", err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("sandbox callback = %d", resp.StatusCode)
		}
		return id
	}
	refund := func(orderID, reason string) (int, string) {
		return do(t, http.MethodPost, env.base+"/admin/orders/"+orderID+"/refund", admin,
			fmt.Sprintf(`{"reason":%q}`, reason))
	}

	// 1. 已接线：PENDING → Courier 管理面接受登记 → EXECUTED
	ok := payOrder()
	code, body = refund(ok, "重复扣款")
	if code != http.StatusOK || !strings.Contains(body, `"execution":"executed"`) || !strings.Contains(body, `"status":"EXECUTED"`) {
		t.Fatalf("refund executed = %d body=%s", code, body)
	}
	if env.refundAdminCalls != 1 {
		t.Fatalf("courier admin refund calls = %d, want 1", env.refundAdminCalls)
	}
	// scope = 映射后的 courier_game_id+env；管理凭证独立 Bearer（非玩家令牌）
	if env.refundAdminScope["gameId"] != "courier-game-a" || env.refundAdminScope["env"] != "dev" ||
		env.refundAdminScope["auth"] != "Bearer courier-admin-tok" {
		t.Fatalf("refund admin scope/auth = %v", env.refundAdminScope)
	}

	// 2. 确定性拒绝：Courier 409 PAYMENT_ORDER_STATE → REJECTED
	env.refundAdminStatus, env.refundAdminCode = http.StatusConflict, "PAYMENT_ORDER_STATE"
	rej := payOrder()
	code, body = refund(rej, "重复扣款")
	if code != http.StatusOK || !strings.Contains(body, `"execution":"rejected"`) || !strings.Contains(body, `"status":"REJECTED"`) {
		t.Fatalf("refund rejected = %d body=%s", code, body)
	}

	// 3. 列表按状态筛（EXECUTED/REJECTED 均终态，PENDING 无残留）
	code, body = do(t, http.MethodGet, env.base+"/admin/refunds?status=EXECUTED", admin, "")
	if code != http.StatusOK || !strings.Contains(body, ok) || strings.Contains(body, rej) {
		t.Fatalf("list refunds executed = %d body=%s", code, body)
	}
	code, body = do(t, http.MethodGet, env.base+"/admin/refunds?status=REJECTED", admin, "")
	if code != http.StatusOK || !strings.Contains(body, rej) || strings.Contains(body, ok) {
		t.Fatalf("list refunds rejected = %d body=%s", code, body)
	}
	code, body = do(t, http.MethodGet, env.base+"/admin/refunds?status=PENDING", admin, "")
	if code != http.StatusOK || strings.Contains(body, ok) || strings.Contains(body, rej) {
		t.Fatalf("list refunds pending = %d body=%s", code, body)
	}
}

// TestRoutes_CourierRedeliver GP-204 断单对账重发管理适配：courier 管理面
// /v1/payments/admin/orders/{id}/redeliver 同步执行。scope 取 GP-002 映射后的
// courier_game_id+env（绝不发 spawn game id），管理凭证走独立 Bearer 头；管理面
// 未接线（AdminToken 未配置）fail-closed 503 不伪造重发成功；确定性拒绝
// （409 已发货、404 订单不存在）如实映射。
func TestRoutes_CourierRedeliver(t *testing.T) {
	setup := func(t *testing.T, withToken bool) (*testDeliveryEnv, string, string) {
		var env *testDeliveryEnv
		if withToken {
			env = newDeliveryServerCfg(t, func(e *testDeliveryEnv) { e.courierAdminToken = "courier-admin-tok" })
		} else {
			env = newDeliveryServer(t)
		}
		admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
		code, body := do(t, http.MethodPost, env.base+"/admin/products/sku-rd-1", admin,
			`{"product_id":"rd-1","form":"DIRECT_PURCHASE","amount_cents":1000,"currency":"CNY","game_id":"game-a","partner_id":2}`)
		if code != http.StatusOK {
			t.Fatalf("create product = %d body=%s", code, body)
		}
		player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})
		orderID := createOrder(t, env, player, `{"sku_id":"sku-rd-1","server_id":1}`)
		return env, orderID, admin
	}

	// 1. 未接线（AdminToken 未配置）：fail-closed 503，不伪造重发成功
	env, orderID, admin := setup(t, false)
	code, body := do(t, http.MethodPost, env.base+"/admin/orders/"+orderID+"/redeliver", admin, "")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("redeliver unwired = %d body=%s, want 503", code, body)
	}

	// 2. 已接线：200 redelivered=true + courier_status + scope/凭证断言 + 审计
	env2, orderID2, admin2 := setup(t, true)
	code, body = do(t, http.MethodPost, env2.base+"/admin/orders/"+orderID2+"/redeliver", admin2, "")
	if code != http.StatusOK || !strings.Contains(body, `"redelivered":true`) || !strings.Contains(body, `"courier_status":"DELIVERED"`) {
		t.Fatalf("redeliver ok = %d body=%s", code, body)
	}
	if env2.redeliverAdminCalls != 1 {
		t.Fatalf("courier redeliver calls = %d, want 1", env2.redeliverAdminCalls)
	}
	if env2.redeliverAdminScope["gameId"] != "courier-game-a" || env2.redeliverAdminScope["env"] != "dev" ||
		env2.redeliverAdminScope["auth"] != "Bearer courier-admin-tok" {
		t.Fatalf("redeliver scope/auth = %v", env2.redeliverAdminScope)
	}
	code, body = do(t, http.MethodGet, env2.base+"/admin/audit?order_id="+orderID2, admin2, "")
	if code != http.StatusOK || !strings.Contains(body, "courier_redeliver") {
		t.Fatalf("audit lacks courier_redeliver: %s", body)
	}

	// 3. 确定性拒绝映射：409 PAYMENT_ORDER_STATE（已发货，不二次发货）→ 409
	env3, orderID3, admin3 := setup(t, true)
	env3.redeliverAdminStatus, env3.redeliverAdminCode = http.StatusConflict, "PAYMENT_ORDER_STATE"
	code, _ = do(t, http.MethodPost, env3.base+"/admin/orders/"+orderID3+"/redeliver", admin3, "")
	if code != http.StatusConflict {
		t.Fatalf("redeliver 409 = %d, want 409", code)
	}

	// 4. courier 侧订单不存在 → 404
	env4, orderID4, admin4 := setup(t, true)
	env4.redeliverAdminStatus, env4.redeliverAdminCode = http.StatusNotFound, "PAYMENT_ORDER_NOT_FOUND"
	code, _ = do(t, http.MethodPost, env4.base+"/admin/orders/"+orderID4+"/redeliver", admin4, "")
	if code != http.StatusNotFound {
		t.Fatalf("redeliver 404 = %d, want 404", code)
	}

	// 4b. 管理凭证无效（courier 401）→ 503（fail-closed，不裸奔）
	env4b, orderID4b, admin4b := setup(t, true)
	env4b.redeliverAdminStatus, env4b.redeliverAdminCode = http.StatusUnauthorized, "COMMON_UNAUTHENTICATED"
	code, _ = do(t, http.MethodPost, env4b.base+"/admin/orders/"+orderID4b+"/redeliver", admin4b, "")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("redeliver auth 401 = %d, want 503", code)
	}

	// 4c. 未识别 courier 错误码 → 502（不透传内部细节）
	env4c, orderID4c, admin4c := setup(t, true)
	env4c.redeliverAdminStatus, env4c.redeliverAdminCode = http.StatusInternalServerError, "INTERNAL"
	code, _ = do(t, http.MethodPost, env4c.base+"/admin/orders/"+orderID4c+"/redeliver", admin4c, "")
	if code != http.StatusBadGateway {
		t.Fatalf("redeliver internal = %d, want 502", code)
	}

	// 5. 本侧投影无此订单 → 404
	env5 := newDeliveryServerCfg(t, func(e *testDeliveryEnv) { e.courierAdminToken = "courier-admin-tok" })
	admin5 := env5.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	code, _ = do(t, http.MethodPost, env5.base+"/admin/orders/order_never_redeliver/redeliver", admin5, "")
	if code != http.StatusNotFound {
		t.Fatalf("redeliver unknown order = %d, want 404", code)
	}

	// 5b. scope 解析失败：订单所属游戏无 Courier 映射（404）→ 502（GP-002 映射
	// 权威，绝不把 spawn 游戏 id 当 Courier scope）。投影行直接植入以模拟
	// 「已下单但游戏后撤映射」的既有事实。
	seedNomap := func(env *testDeliveryEnv, orderID, gameID string) {
		err := env.svcCtx.ProjectionDB.UpsertFromOrder(&model.OrderProjection{
			OrderId: orderID, GameId: gameID, AccountId: "7", SkuId: "sku-rd-1",
			ProductId: "rd-1", Form: "DIRECT_PURCHASE", AmountCents: 1000,
			Currency: "CNY", Status: "PAID",
		})
		if err != nil {
			t.Fatalf("seed projection %s: %v", orderID, err)
		}
	}
	env5b := newDeliveryServerCfg(t, func(e *testDeliveryEnv) { e.courierAdminToken = "courier-admin-tok" })
	admin5b := env5b.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	seedNomap(env5b, "order_nomap_rd", "nomap-game")
	code, body = do(t, http.MethodPost, env5b.base+"/admin/orders/order_nomap_rd/redeliver", admin5b, "")
	if code != http.StatusBadGateway {
		t.Fatalf("redeliver nomap game = %d body=%s, want 502", code, body)
	}
	if env5b.redeliverAdminCalls != 0 {
		t.Fatalf("nomap scope failure still called courier: calls=%d", env5b.redeliverAdminCalls)
	}

	// 5c. 映射服务故障（500，非 404）→ 通用兜底 502（不透传内部细节）
	env5c := newDeliveryServerCfg(t, func(e *testDeliveryEnv) { e.courierAdminToken = "courier-admin-tok" })
	admin5c := env5c.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	seedNomap(env5c, "order_err500_rd", "err500-game")
	code, body = do(t, http.MethodPost, env5c.base+"/admin/orders/order_err500_rd/redeliver", admin5c, "")
	if code != http.StatusBadGateway {
		t.Fatalf("redeliver mapping 500 = %d body=%s, want 502", code, body)
	}
	if env5c.redeliverAdminCalls != 0 {
		t.Fatalf("mapping failure still called courier: calls=%d", env5c.redeliverAdminCalls)
	}

	// 6. 玩家越权 403
	code, _ = do(t, http.MethodPost, env2.base+"/admin/orders/"+orderID2+"/redeliver",
		env2.sign(utils.JWTClaims{UserId: 7, Username: "bob"}), "")
	if code != http.StatusForbidden {
		t.Fatalf("player redeliver = %d, want 403", code)
	}
}

// TestRoutes_InternalOrderQuery GP-307 S2S 订单事实查询：courier-adapter
// 售后工单 orderId 归属强校验的事实源。X-Internal-Token 先于解析严格比对，
// 未配置恒 403（fail-closed）；带令牌返回投影事实（game_id/account_id 供
// 归属判定）；未知订单 404。S2S 专用不进网关。
func TestRoutes_InternalOrderQuery(t *testing.T) {
	env := newDeliveryServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	code, body := do(t, http.MethodPost, env.base+"/admin/products/sku-iq-1", admin,
		`{"product_id":"iq-1","form":"DIRECT_PURCHASE","amount_cents":1000,"currency":"CNY","game_id":"game-a","partner_id":2}`)
	if code != http.StatusOK {
		t.Fatalf("create product = %d body=%s", code, body)
	}
	orderID := createOrder(t, env, player, `{"sku_id":"sku-iq-1","server_id":1}`)

	get := func(token string) (int, string) {
		req, _ := http.NewRequest(http.MethodGet, env.base+"/internal/orders/"+orderID, nil)
		if token != "" {
			req.Header.Set("X-Internal-Token", token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("internal order query: %v", err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, string(raw)
	}

	// 1. 令牌未配置：任何令牌都 403（fail-closed）
	if code, _ := get("whatever"); code != http.StatusForbidden {
		t.Fatalf("unconfigured token = %d, want 403", code)
	}

	// 2. 配置令牌：缺失 403 / 错误 403 / 正确 200 且事实齐全
	env.svcCtx.Config.Internal.Token = "gp307-orders-token"
	if code, _ := get(""); code != http.StatusForbidden {
		t.Fatalf("missing token = %d, want 403", code)
	}
	if code, _ := get("wrong-token"); code != http.StatusForbidden {
		t.Fatalf("wrong token = %d, want 403", code)
	}
	code, body = get("gp307-orders-token")
	if code != http.StatusOK {
		t.Fatalf("valid token = %d body=%s", code, body)
	}
	var respBody struct {
		Order struct {
			OrderId     string `json:"order_id"`
			GameId      string `json:"game_id"`
			AccountId   string `json:"account_id"`
			Status      string `json:"status"`
			AmountCents int64  `json:"amount_cents"`
		} `json:"order"`
	}
	if err := json.Unmarshal([]byte(body), &respBody); err != nil ||
		respBody.Order.OrderId != orderID || respBody.Order.GameId != "game-a" ||
		respBody.Order.AccountId != "7" || respBody.Order.Status != "CREATED" ||
		respBody.Order.AmountCents != 1000 {
		t.Fatalf("order facts = %s err=%v", body, err)
	}

	// 3. 未知订单 404
	req, _ := http.NewRequest(http.MethodGet, env.base+"/internal/orders/order_missing", nil)
	req.Header.Set("X-Internal-Token", "gp307-orders-token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("missing order query: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing order = %d, want 404", resp.StatusCode)
	}
}

// TestRoutes_CreateOrderPaymentDisabled GP-002 支付能力门：映射存在但 payment
// 能力未开（存根 nopay 分支缺 capabilities → fail-closed false）→ 下单 403，
// 不触达 Courier。
func TestRoutes_CreateOrderPaymentDisabled(t *testing.T) {
	env := newDeliveryServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	// 商品属于 nopay-game（映射 stub nopay 分支：有 courierGameId/env，无能力位）
	code, body := do(t, http.MethodPost, env.base+"/admin/products/sku-nopay", admin,
		`{"product_id":"nopay","form":"DIRECT_PURCHASE","amount_cents":1000,"currency":"CNY","game_id":"nopay-game","partner_id":2}`)
	if code != http.StatusOK {
		t.Fatalf("create product = %d body=%s", code, body)
	}
	code, body = do(t, http.MethodPost, env.base+"/games/nopay-game/orders", player,
		`{"sku_id":"sku-nopay"}`)
	if code != http.StatusForbidden || !strings.Contains(body, "未开通支付") {
		t.Fatalf("payment disabled order = %d body=%s, want 403", code, body)
	}
}

// TestRoutes_SkuSync GP-202 部署面 SKU 登记管理适配：payment_products 服务端
// 定价经 Courier 管理面 v1.2 /v1/payments/admin/skus 登记/改价到 teller 价格表。
// 显式动作不挂商品创建钩子；管理面未接线（AdminToken 未配置）fail-closed 503
// 不伪造登记成功；scope 取 GP-002 映射后 courier_game_id+env（绝不透传 spawn
// 游戏 id），管理凭证走独立 Bearer 头；部分失败逐条留痕且审计落档。
func TestRoutes_SkuSync(t *testing.T) {
	setup := func(t *testing.T, withToken bool) (*testDeliveryEnv, string) {
		var env *testDeliveryEnv
		if withToken {
			env = newDeliveryServerCfg(t, func(e *testDeliveryEnv) { e.courierAdminToken = "courier-admin-tok" })
		} else {
			env = newDeliveryServer(t)
		}
		admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
		for _, sku := range []string{"sku-sync-a", "sku-sync-b"} {
			code, body := do(t, http.MethodPost, env.base+"/admin/products/"+sku, admin,
				fmt.Sprintf(`{"product_id":"%s","form":"DIRECT_PURCHASE","amount_cents":1000,"currency":"CNY","game_id":"game-a","partner_id":2}`, sku))
			if code != http.StatusOK {
				t.Fatalf("create product %s = %d body=%s", sku, code, body)
			}
		}
		return env, admin
	}

	// 1. 未接线（AdminToken 未配置）：503 且不触达 courier
	env, admin := setup(t, false)
	code, body := do(t, http.MethodPost, env.base+"/admin/games/game-a/products/sync", admin, "")
	if code != http.StatusServiceUnavailable || env.skuAdminCalls != 0 {
		t.Fatalf("sku sync unwired = %d body=%s calls=%d, want 503/0", code, body, env.skuAdminCalls)
	}

	// 2. 已接线：逐条登记 + scope/凭证断言 + 服务端定价载荷 + 成功计数 + 审计
	env2, admin2 := setup(t, true)
	code, body = do(t, http.MethodPost, env2.base+"/admin/games/game-a/products/sync", admin2, "")
	if code != http.StatusOK || !strings.Contains(body, `"synced":2`) || !strings.Contains(body, `"failed":0`) {
		t.Fatalf("sku sync ok = %d body=%s", code, body)
	}
	if env2.skuAdminCalls != 2 {
		t.Fatalf("courier admin sku calls = %d, want 2", env2.skuAdminCalls)
	}
	for i, scope := range env2.skuAdminScope {
		if scope["gameId"] != "courier-game-a" || scope["env"] != "dev" ||
			scope["auth"] != "Bearer courier-admin-tok" {
			t.Fatalf("sku sync scope/auth[%d] = %v", i, scope)
		}
		loaded := env2.skuAdminBody[i]
		if loaded["amountCents"] != int64(1000) || loaded["currency"] != "CNY" ||
			loaded["form"] != "DIRECT_PURCHASE" {
			t.Fatalf("sku sync payload[%d] = %v", i, loaded)
		}
	}
	code, body = do(t, http.MethodGet, env2.base+"/admin/audit", admin2, "")
	if code != http.StatusOK || !strings.Contains(body, "courier_sku_sync") {
		t.Fatalf("audit lacks courier_sku_sync: %s", body)
	}

	// 3. 部分失败：指定 sku 按 courier 400 COMMON_INVALID_ARGUMENT 拒绝 →
	//    failed=1 逐条留痕错误码，其余仍登记成功（不整体回滚）。
	env3, admin3 := setup(t, true)
	env3.skuAdminFailSku = "sku-sync-b"
	env3.skuAdminStatus, env3.skuAdminCode = http.StatusBadRequest, "COMMON_INVALID_ARGUMENT"
	code, body = do(t, http.MethodPost, env3.base+"/admin/games/game-a/products/sync", admin3, "")
	if code != http.StatusOK || !strings.Contains(body, `"synced":1`) || !strings.Contains(body, `"failed":1`) ||
		!strings.Contains(body, "COMMON_INVALID_ARGUMENT") {
		t.Fatalf("sku sync partial fail = %d body=%s", code, body)
	}

	// 4. 未映射游戏 → 502 且不触达 courier（GP-002 映射权威）
	env4, admin4 := setup(t, true)
	code, _ = do(t, http.MethodPost, env4.base+"/admin/games/nomap-game/products/sync", admin4, "")
	if code != http.StatusBadGateway || env4.skuAdminCalls != 0 {
		t.Fatalf("sku sync nomap = %d calls=%d, want 502/0", code, env4.skuAdminCalls)
	}

	// 5. 玩家越权 403
	code, _ = do(t, http.MethodPost, env2.base+"/admin/games/game-a/products/sync",
		env2.sign(utils.JWTClaims{UserId: 7, Username: "bob"}), "")
	if code != http.StatusForbidden {
		t.Fatalf("player sku sync = %d, want 403", code)
	}

	// 6. disabled 商品仍登记（价格表=定价事实，含 disabled；可购门仍由本侧
	//    enabled 位控制），避免将来误改成 enabledOnly=true 静默漏价。
	env6, admin6 := setup(t, true)
	code, _ = do(t, http.MethodPatch, env6.base+"/admin/products/sku-sync-b/enabled", admin6, `{"enabled":false}`)
	if code != http.StatusOK {
		t.Fatalf("disable product = %d", code)
	}
	code, body = do(t, http.MethodPost, env6.base+"/admin/games/game-a/products/sync", admin6, "")
	if code != http.StatusOK || !strings.Contains(body, `"synced":2`) || env6.skuAdminCalls != 2 {
		t.Fatalf("sku sync disabled product = %d body=%s calls=%d, want 200/2/2", code, body, env6.skuAdminCalls)
	}
}
