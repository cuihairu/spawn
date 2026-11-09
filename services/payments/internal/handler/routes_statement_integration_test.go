package handler

// GP-205 厂商对账单集成测试：从只读投影聚合实付/优惠承担/退款冲减/合同
// 分成基数，差异与人工确认、CSV 导出；先人工确认不自动打款（F23）。
// 事实源仍是 Courier（投影为事实副本），不旁路契约。
//
// 红先行：本文件先于实现提交——运行时应红（路由 404 / MarkRefundExecuted
// 未定义），实现 model/types/logic/routes 后转绿。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/model"
	"github.com/tappi/tappi/services/payments/utils"
)

func paidOrder(t *testing.T, env *testDeliveryEnv, player string) string {
	t.Helper()
	orderID := createOrder(t, env, player, `{"sku_id":"sku-gem-100","server_id":1}`)
	cbBody := fmt.Sprintf(`{"orderId":%q,"paidAt":%q}`, orderID, time.Now().UTC().Format(time.RFC3339))
	req, _ := http.NewRequest(http.MethodPost, env.base+"/callback/sandbox", strings.NewReader(cbBody))
	req.Header.Set("X-Payment-Signature", deliverySig(testSandboxSecret, cbBody))
	resp, _ := http.DefaultClient.Do(req)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("sandbox callback = %d", resp.StatusCode)
	}
	dispatchBody := fmt.Sprintf(`{"orderId":%q,"accountId":"7","skuId":"sku-gem-100","productId":"gem-100","form":"DIRECT_PURCHASE","amountCents":1000,"currency":"CNY"}`, orderID)
	code, _ := dispatchRaw(t, env.base+"/internal/deliveries", dispatchBody, deliverySig(testDeliverySecret, dispatchBody))
	if code != http.StatusOK {
		t.Fatalf("dispatch = %d", code)
	}
	return orderID
}

func TestRoutes_VendorStatement(t *testing.T) {
	env := newDeliveryServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

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

	// 两笔已付已发订单：A 全额留仓（无退款），B 执行退款 1000（冲减）
	orderA := paidOrder(t, env, player)
	orderB := paidOrder(t, env, player)

	// B 登记退款并标记执行（executor 接入前的测试路径；Courier 退款端点接入后由执行器置 EXECUTED）
	code, _ = do(t, http.MethodPost, env.base+"/admin/orders/"+orderB+"/refund", admin, `{"reason":"测试冲减"}`)
	if code != http.StatusOK {
		t.Fatalf("create refund B = %d", code)
	}
	if err := env.svcCtx.ProjectionDB.MarkRefundExecuted(orderB, model.RefundActionRefund); err != nil {
		t.Fatalf("mark refund executed: %v", err)
	}

	// 生成对账单：周期 = 今天（UTC），vendor=sandbox，partner=2，分成 70%
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
			Id            int64  `json:"id"`
			PaidTotal     int64  `json:"paid_total"`
			DiscountTotal int64  `json:"discount_total"`
			RefundTotal   int64  `json:"refund_total"`
			SplitBase     int64  `json:"split_base"`
			SplitRateBp   int64  `json:"split_rate_bp"`
			SplitAmount   int64  `json:"split_amount"`
			Settlement    int64  `json:"settlement"`
			Diff          int64  `json:"diff"`
			OrderCount    int    `json:"order_count"`
			Status        string `json:"status"`
		} `json:"statement"`
	}
	if err := json.Unmarshal([]byte(body), &stmt); err != nil {
		t.Fatalf("parse statement: %v body=%s", err, body)
	}
	s := stmt.Statement
	if s.PaidTotal != 2000 || s.DiscountTotal != 0 || s.RefundTotal != 1000 || s.SplitBase != 1000 ||
		s.SplitRateBp != 7000 || s.SplitAmount != 700 || s.Settlement != 0 || s.Diff != 1000 ||
		s.OrderCount != 2 || s.Status != "draft" {
		t.Fatalf("statement summary = %+v, want paid=2000 refund=1000 base=1000 split=700 diff=1000 count=2 draft", s)
	}
	stmtID := s.Id

	// 结算单金额人工录入：700 → 差异 300；再 1000 → 差异 0
	code, body = do(t, http.MethodPut, fmt.Sprintf("%s/admin/statements/%d/settlement", env.base, stmtID), admin, `{"settlement":700}`)
	if code != http.StatusOK || !strings.Contains(body, `"diff":300`) {
		t.Fatalf("set settlement 700 = %d body=%s", code, body)
	}
	code, body = do(t, http.MethodPut, fmt.Sprintf("%s/admin/statements/%d/settlement", env.base, stmtID), admin, `{"settlement":1000}`)
	if code != http.StatusOK || !strings.Contains(body, `"diff":0`) {
		t.Fatalf("set settlement 1000 = %d body=%s", code, body)
	}

	// 人工确认（不自动打款）
	code, body = do(t, http.MethodPost, fmt.Sprintf("%s/admin/statements/%d/confirm", env.base, stmtID), admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"confirmed"`) {
		t.Fatalf("confirm statement = %d body=%s", code, body)
	}
	// 确认后不可再改结算单 / 重生成
	code, _ = do(t, http.MethodPut, fmt.Sprintf("%s/admin/statements/%d/settlement", env.base, stmtID), admin, `{"settlement":500}`)
	if code != http.StatusConflict {
		t.Fatalf("settlement after confirm = %d, want 409", code)
	}
	code, _ = do(t, http.MethodPost, env.base+"/admin/statements", admin, genBody)
	if code != http.StatusConflict {
		t.Fatalf("regenerate confirmed = %d, want 409", code)
	}

	// CSV 导出：含表头与两笔订单明细
	code, body = do(t, http.MethodGet, fmt.Sprintf("%s/admin/statements/%d/export", env.base, stmtID), admin, "")
	if code != http.StatusOK || !strings.Contains(body, "order_id") || !strings.Contains(body, orderA) || !strings.Contains(body, orderB) {
		t.Fatalf("export csv = %d body=%s", code, body)
	}

	// 列表 + 权限
	code, body = do(t, http.MethodGet, env.base+"/admin/statements?vendor=sandbox", admin, "")
	if code != http.StatusOK || !strings.Contains(body, fmt.Sprintf(`"id":%d`, stmtID)) {
		t.Fatalf("list statements = %d body=%s", code, body)
	}
	code, _ = do(t, http.MethodPost, env.base+"/admin/statements", player, genBody)
	if code != http.StatusForbidden {
		t.Fatalf("player generate = %d, want 403", code)
	}
	code, _ = do(t, http.MethodGet, fmt.Sprintf("%s/admin/statements/%d/export", env.base, stmtID), player, "")
	if code != http.StatusForbidden {
		t.Fatalf("player export = %d, want 403", code)
	}

	// 审计含 statement_generate/confirm/settlement/export
	code, body = do(t, http.MethodGet, env.base+"/admin/audit", admin, "")
	if code != http.StatusOK {
		t.Fatalf("audit = %d", code)
	}
	for _, action := range []string{"statement_generate", "statement_confirm", "statement_settlement", "statement_export"} {
		if !strings.Contains(body, action) {
			t.Fatalf("audit lacks %s: %s", action, body)
		}
	}
}
