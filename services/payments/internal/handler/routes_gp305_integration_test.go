package handler

// GP-305 优惠券集成测试：管理面（券定义/状态机/列表）+ 玩家侧（领取幂等/
// 限领/我的券）+ 下单服务端报价（门槛/叠加/占用，客户端仅提交 sku_id +
// coupon_ids，不提交可信金额；他人持有券不可见 400，不泄露存在性）+
// PAID 核销（locked→used）+ 退款返还路径（ReturnCouponsByOrder，退款执行
// 器接线见 todo）+ 厂商对账单优惠承担聚合（GP-205 分成基数冲减）。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/utils"
)

func gpCouponBody(code, name string, threshold, discount int64, stackable bool, perUserLimit int64) string {
	return fmt.Sprintf(`{
		"code":%q,"name":%q,"game_id":"game-a","sku_id":"","threshold_cents":%d,
		"discount_type":"fixed","discount_value":%d,"stackable":%t,
		"per_user_limit":%d,"total_count":0,
		"start_at":"2026-01-01T00:00:00+08:00","end_at":"2026-12-31T23:59:59+08:00"
	}`, code, name, threshold, discount, stackable, perUserLimit)
}

func gpCreateCoupon(t *testing.T, env *testDeliveryEnv, admin, body string) int64 {
	t.Helper()
	code, resp := do(t, http.MethodPost, env.base+"/admin/coupons", admin, body)
	if code != http.StatusOK {
		t.Fatalf("create coupon = %d body=%s", code, resp)
	}
	var r struct {
		Coupon struct {
			Id     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"coupon"`
	}
	if err := json.Unmarshal([]byte(resp), &r); err != nil || r.Coupon.Id == 0 || r.Coupon.Status != "draft" {
		t.Fatalf("parse coupon = %v body=%s", err, resp)
	}
	return r.Coupon.Id
}

func gpPublishCoupon(t *testing.T, env *testDeliveryEnv, admin string, id int64) {
	t.Helper()
	code, body := do(t, http.MethodPost, fmt.Sprintf("%s/admin/coupons/%d/publish", env.base, id), admin, "")
	if code != http.StatusOK {
		t.Fatalf("publish coupon %d = %d body=%s", id, code, body)
	}
}

func gpOrderReq(t *testing.T, env *testDeliveryEnv, player string, couponIDs []int64) (int, string) {
	t.Helper()
	ids := make([]string, 0, len(couponIDs))
	for _, id := range couponIDs {
		ids = append(ids, fmt.Sprintf("%d", id))
	}
	body := `{"sku_id":"sku-gem-100","server_id":1}`
	if len(ids) > 0 {
		body = fmt.Sprintf(`{"sku_id":"sku-gem-100","coupon_ids":[%s],"server_id":1}`, strings.Join(ids, ","))
	}
	return do(t, http.MethodPost, env.base+"/games/game-a/orders", player, body)
}

func TestRoutes_Coupons(t *testing.T) {
	env := newDeliveryServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})
	player2 := env.sign(utils.JWTClaims{UserId: 8, Username: "carol"})

	// 前置：商品（1000 分） + 游戏服发货接口（复用交付骨架）
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

	// 券 A：fixed 200 / 门槛 500 / 可叠 / 限领 2；B：fixed 100 / 不可叠 / 限领 1；
	// C：门槛 5000（高于列表价 1000 → 未达门槛）；D：fixed 50 / 可叠
	idA := gpCreateCoupon(t, env, admin, gpCouponBody("COUP-A", "A券", 500, 200, true, 2))
	idB := gpCreateCoupon(t, env, admin, gpCouponBody("COUP-B", "B券", 500, 100, false, 1))
	idC := gpCreateCoupon(t, env, admin, gpCouponBody("COUP-C", "C券", 5000, 100, true, 1))
	idD := gpCreateCoupon(t, env, admin, gpCouponBody("COUP-D", "D券", 500, 50, true, 1))
	gpPublishCoupon(t, env, admin, idA)
	gpPublishCoupon(t, env, admin, idB)
	gpPublishCoupon(t, env, admin, idC)
	gpPublishCoupon(t, env, admin, idD)

	// 1. 玩家领取：a1 → claimed；a1 重放幂等；a2 第二张；a3 限领 409
	claim := func(id int64, reqID string, want int) string {
		code, body := do(t, http.MethodPost, fmt.Sprintf("%s/coupons/%d/claim", env.base, id), player,
			fmt.Sprintf(`{"request_id":%q}`, reqID))
		if code != want {
			t.Fatalf("claim coupon %d req=%s = %d (want %d) body=%s", id, reqID, code, want, body)
		}
		return body
	}
	if !strings.Contains(claim(idA, "a1", http.StatusOK), `"status":"claimed"`) {
		t.Fatalf("claim a1 body=%s", claim(idA, "a1", http.StatusOK))
	}
	claim(idA, "a1", http.StatusOK) // 幂等重放
	claim(idA, "a2", http.StatusOK)
	claim(idA, "a3", http.StatusConflict) // per_user_limit=2
	claim(idB, "b1", http.StatusOK)
	claim(idC, "c1", http.StatusOK)
	claim(idD, "d1", http.StatusOK)
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/coupons/%d/claim", env.base, idB), player2, `{"request_id":"b8"}`)
	if code != http.StatusOK {
		t.Fatalf("player2 claim B = %d", code)
	}

	// 2. 未达门槛（C 门槛 5000 > 1000）→ 400
	if code, _ = gpOrderReq(t, env, player, []int64{idC}); code != http.StatusBadRequest {
		t.Fatalf("below-threshold order = %d, want 400", code)
	}

	// 3. 他人持有的券不可见：D 仅 player7 领取，player2 下单带 D → 400 未领取
	//（按 user_id 收敛查询，不泄露他人持有）
	if code, _ = gpOrderReq(t, env, player2, []int64{idD}); code != http.StatusBadRequest {
		t.Fatalf("other user's coupon = %d, want 400", code)
	}

	// 4. 有效下单用 A → 200：折让 200 / 应付 800 / 券应用；我的券 locked 带订单号
	code, body = gpOrderReq(t, env, player, []int64{idA})
	if code != http.StatusOK || !strings.Contains(body, `"discount_cents":200`) ||
		!strings.Contains(body, `"pay_amount_cents":800`) || !strings.Contains(body, `"code":"COUP-A"`) {
		t.Fatalf("order with A = %d body=%s", code, body)
	}
	var orderResp struct {
		Order struct {
			Id string `json:"id"`
		} `json:"order"`
	}
	if err := json.Unmarshal([]byte(body), &orderResp); err != nil || orderResp.Order.Id == "" {
		t.Fatalf("parse order: %v body=%s", err, body)
	}
	orderA := orderResp.Order.Id
	code, body = do(t, http.MethodGet, env.base+"/coupons/my?status=locked", player, "")
	if code != http.StatusOK || !strings.Contains(body, `"code":"COUP-A"`) || !strings.Contains(body, orderA) {
		t.Fatalf("my locked coupons = %d body=%s", code, body)
	}

	// 5. 叠加拒绝：D(可叠) + B(不可叠) → 400，且不落锁（两券仍 claimed）
	if code, _ = gpOrderReq(t, env, player, []int64{idD, idB}); code != http.StatusBadRequest {
		t.Fatalf("non-stackable combine = %d, want 400", code)
	}

	// 6. B 单独下单 → 200（折让 100 / 应付 900）；再用 B → 409（限领 1 且已锁，
	// 本人无 claimed 实例但有持有记录）
	code, body = gpOrderReq(t, env, player, []int64{idB})
	if code != http.StatusOK || !strings.Contains(body, `"discount_cents":100`) ||
		!strings.Contains(body, `"pay_amount_cents":900`) {
		t.Fatalf("order with B = %d body=%s", code, body)
	}
	if code, _ = gpOrderReq(t, env, player, []int64{idB}); code != http.StatusConflict {
		t.Fatalf("reuse locked B = %d, want 409", code)
	}

	// 7. PAID 回调 → A used（locked→used 核销）
	cbBody := fmt.Sprintf(`{"orderId":%q,"paidAt":%q}`, orderA, time.Now().UTC().Format(time.RFC3339))
	req, _ := http.NewRequest(http.MethodPost, env.base+"/callback/sandbox", strings.NewReader(cbBody))
	req.Header.Set("X-Payment-Signature", deliverySig(testSandboxSecret, cbBody))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("callback: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("paid callback = %d", resp.StatusCode)
	}
	code, body = do(t, http.MethodGet, env.base+"/coupons/my?status=used", player, "")
	if code != http.StatusOK || !strings.Contains(body, `"code":"COUP-A"`) {
		t.Fatalf("my used coupons = %d body=%s", code, body)
	}

	// 8. 退款返还路径（执行器接线见 todo）：直接调用 ReturnCouponsByOrder 模拟
	//    退款 EXECUTED 钩子 → A 回到 claimed 可复用
	if err := env.svcCtx.CouponDB.ReturnCouponsByOrder(orderA); err != nil {
		t.Fatalf("return coupons: %v", err)
	}
	code, body = do(t, http.MethodGet, env.base+"/coupons/my?status=claimed", player, "")
	if code != http.StatusOK || !strings.Contains(body, `"code":"COUP-A"`) {
		t.Fatalf("my returned coupons = %d body=%s", code, body)
	}

	// 9. 厂商对账单（GP-205）：仅 A 单已付（1000）→ 折让承担 200 冲减分成基数
	//    split_base = 1000 − 200 − 0 = 800，split = 800×70% = 560
	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.UTC).Format(time.RFC3339)
	genBody := fmt.Sprintf(`{"vendor":"sandbox","partner_id":2,"period_start":%q,"period_end":%q,"split_rate_bp":7000}`, start, end)
	code, body = do(t, http.MethodPost, env.base+"/admin/statements", admin, genBody)
	if code != http.StatusOK {
		t.Fatalf("generate statement = %d body=%s", code, body)
	}
	var stmt struct {
		Statement struct {
			PaidTotal     int64 `json:"paid_total"`
			DiscountTotal int64 `json:"discount_total"`
			RefundTotal   int64 `json:"refund_total"`
			SplitBase     int64 `json:"split_base"`
			SplitAmount   int64 `json:"split_amount"`
			OrderCount    int   `json:"order_count"`
		} `json:"statement"`
	}
	if err := json.Unmarshal([]byte(body), &stmt); err != nil {
		t.Fatalf("parse statement: %v body=%s", err, body)
	}
	s := stmt.Statement
	if s.PaidTotal != 1000 || s.DiscountTotal != 200 || s.RefundTotal != 0 ||
		s.SplitBase != 800 || s.SplitAmount != 560 || s.OrderCount != 1 {
		t.Fatalf("statement summary = %+v, want paid=1000 discount=200 base=800 split=560 count=1", s)
	}

	// 10. 管理面列表可见券定义；玩家越权建券 403；非法折扣（percent>100%）400
	code, body = do(t, http.MethodGet, env.base+"/admin/coupons?game_id=game-a", admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"code":"COUP-A"`) || !strings.Contains(body, `"status":"published"`) {
		t.Fatalf("admin list coupons = %d body=%s", code, body)
	}
	code, _ = do(t, http.MethodPost, env.base+"/admin/coupons", player, gpCouponBody("X", "越权", 0, 100, true, 1))
	if code != http.StatusForbidden {
		t.Fatalf("player create coupon = %d, want 403", code)
	}
	code, _ = do(t, http.MethodPost, env.base+"/admin/coupons", admin,
		`{"code":"BAD","name":"坏券","game_id":"game-a","threshold_cents":0,"discount_type":"percent_bp","discount_value":20000,"per_user_limit":1,"start_at":"2026-01-01T00:00:00+08:00","end_at":"2026-12-31T23:59:59+08:00"}`)
	if code != http.StatusBadRequest {
		t.Fatalf("bad percent coupon = %d, want 400", code)
	}

	// 11. 停用 D → 领取拒 409
	do(t, http.MethodPost, fmt.Sprintf("%s/admin/coupons/%d/disable", env.base, idD), admin, "")
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/coupons/%d/claim", env.base, idD), player, `{"request_id":"d9"}`)
	if code != http.StatusConflict {
		t.Fatalf("claim disabled coupon = %d, want 409", code)
	}
}
