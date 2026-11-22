package handler

// GP-402 经营分析事实投递（F24）：payments 侧 order_paid / refund_requested
// 事实事件经 data-panel /internal/events 上报。
//   - 回调 PAID 后投 order_paid：金额/游戏/账号取自订单投影（交易事实）
//   - event_id 由订单号派生（"order_paid:<orderId>"），回调重放幂等
//   - 退款登记后投 refund_requested：金额取本次登记金额（部分退款按登记值）
//   - 事实事件只记请求事实，不伪造执行成功（退款执行器未接入）

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/model"
	"github.com/tappi/tappi/services/payments/utils"
)

type analyticsEvent struct {
	EventId     string `json:"event_id"`
	EventType   string `json:"event_type"`
	UserId      int64  `json:"user_id"`
	GameId      string `json:"game_id"`
	OrderId     string `json:"order_id"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
}

func adminClaims() utils.JWTClaims {
	return utils.JWTClaims{UserId: 1, Username: "admin"}
}

// seedProjection 直接装配一条 PAID 订单投影（退款登记要求订单已付款）；
// 不经下单链路，测试只关心事实投递口径。
func seedProjection(t *testing.T, env *testEnv, orderID, gameID, accountID string, amountCents int64) {
	t.Helper()
	err := env.svcCtx.ProjectionDB.UpsertFromOrder(&model.OrderProjection{
		OrderId: orderID, GameId: gameID, AccountId: accountID,
		SkuId: "sku-1", ProductId: "prod-1", Form: "direct",
		AmountCents: amountCents, BaseQuantity: 1, Currency: "CNY",
		Status: "PAID", PaidAt: "2026-11-12T08:00:00Z",
	})
	if err != nil {
		t.Fatalf("seed projection: %v", err)
	}
}

// TestPaymentsAnalyticsOrderPaidEvent 回调 PAID → order_paid 事实事件。
func TestPaymentsAnalyticsOrderPaidEvent(t *testing.T) {
	env := newPaymentsServer(t)

	orderID := "order_analytics-402"
	seedProjection(t, env, orderID, "game-analytics", "7", 1800)

	payload := []byte(`{"orderId":"` + orderID + `","paidAt":"2026-11-12T09:00:00Z"}`)
	code := postCallback(t, env, payload)
	if code != http.StatusOK {
		t.Fatalf("callback status = %d, want 200", code)
	}
	if len(env.analyticsSeen) != 1 {
		t.Fatalf("analytics batches = %d body=%v, want 1", len(env.analyticsSeen), env.analyticsSeen)
	}
	if env.analyticsTok != "gp402-payments-token" {
		t.Fatalf("X-Internal-Token = %q, want gp402-payments-token", env.analyticsTok)
	}
	ev := decodeEvents(t, env.analyticsSeen[0])
	if len(ev) != 1 {
		t.Fatalf("events = %+v, want 1", ev)
	}
	got := ev[0]
	if got.EventType != "order_paid" || got.EventId != "order_paid:"+orderID ||
		got.GameId != "game-analytics" || got.OrderId != orderID || got.AmountCents != 1800 || got.UserId != 7 ||
		got.Currency != "CNY" {
		t.Fatalf("event = %+v, want order_paid/game-analytics/1800/user 7/CNY", got)
	}

	// 回调重放：event_id 派生键不变 → 数据面板侧幂等只算一次（此处只验证
	// 派生键稳定，摄入端 UNIQUE 忽略重复）
	code = postCallback(t, env, payload)
	if code != http.StatusOK {
		t.Fatalf("callback replay status = %d, want 200", code)
	}
	if len(env.analyticsSeen) != 2 {
		t.Fatalf("analytics batches after replay = %d, want 2", len(env.analyticsSeen))
	}
	replay := decodeEvents(t, env.analyticsSeen[1])
	if len(replay) != 1 || replay[0].EventId != "order_paid:"+orderID {
		t.Fatalf("replay event = %+v, want same derived event_id", replay)
	}
}

// TestPaymentsAnalyticsRefundRequestedEvent 退款登记 → refund_requested 事实事件
// （部分退款按登记金额；不执行、不伪造执行成功）。
func TestPaymentsAnalyticsRefundRequestedEvent(t *testing.T) {
	env := newPaymentsServer(t)

	token := env.sign(adminClaims())
	orderID := "order_analytics-refund"
	seedProjection(t, env, orderID, "game-analytics", "7", 2000)

	code := refundRequest(t, env, token, orderID, 800, "用户申请退款")
	if code != http.StatusOK {
		t.Fatalf("refund status = %d, want 200", code)
	}
	if len(env.analyticsSeen) != 1 {
		t.Fatalf("analytics batches = %d body=%v, want 1", len(env.analyticsSeen), env.analyticsSeen)
	}
	ev := decodeEvents(t, env.analyticsSeen[0])
	if len(ev) != 1 {
		t.Fatalf("events = %+v, want 1", ev)
	}
	got := ev[0]
	if got.EventType != "refund_requested" || got.EventId != "refund_requested:"+orderID+":REFUND" ||
		got.AmountCents != 800 || got.OrderId != orderID || got.UserId != 7 || got.Currency != "CNY" {
		t.Fatalf("event = %+v, want refund_requested/800/user 7/CNY", got)
	}
}

// TestPaymentsAnalyticsOutboxRetry GP-402 本地 outbox 补偿：data-panel 短时
// 不可达（503）→ order_paid 落 pending；Relay 重投失败按退避延期（立即再
// 扫 0 尝试）；下游恢复后 Relay 补投成功标记 delivered，事实批次完整到达。
func TestPaymentsAnalyticsOutboxRetry(t *testing.T) {
	env := newPaymentsServer(t)
	ctx := context.Background()

	// 下游短时不可达：主流程不受影响（回调仍 200），事实先落 outbox
	env.analyticsDown.Store(true)
	orderID := "order_analytics-outbox"
	seedProjection(t, env, orderID, "game-analytics", "7", 1500)
	if code := postCallback(t, env, []byte(`{"orderId":"`+orderID+`","paidAt":"2026-11-12T09:00:00Z"}`)); code != http.StatusOK {
		t.Fatalf("callback status = %d, want 200", code)
	}
	if len(env.analyticsSeen) != 0 {
		t.Fatalf("down: analytics batches = %d, want 0", len(env.analyticsSeen))
	}
	rows, err := env.svcCtx.AnalyticsOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("outbox pending = %d err=%v, want 1", len(rows), err)
	}
	row := rows[0]
	if row.EventId != "order_paid:"+orderID || row.EventType != "order_paid" ||
		row.Status != model.AnalyticsOutboxPending || row.Attempts != 0 {
		t.Fatalf("outbox row = %+v, want pending/0 attempts/derived event_id", row)
	}

	// Relay 首次重投：下游仍不可达 → 记失败 + 指数退避延期
	attempted, delivered := env.svcCtx.AnalyticsRelay.RelayOnce(ctx)
	if attempted != 1 || delivered != 0 {
		t.Fatalf("relay down = %d/%d, want 1/0", attempted, delivered)
	}
	rows, _ = env.svcCtx.AnalyticsOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 1 || rows[0].Attempts != 1 || rows[0].Status != model.AnalyticsOutboxPending {
		t.Fatalf("outbox after failed relay = %+v, want attempts=1 pending", rows)
	}
	// 退避期内再扫：未到期不重投（backoff 生效）
	attempted, delivered = env.svcCtx.AnalyticsRelay.RelayOnce(ctx)
	if attempted != 0 || delivered != 0 {
		t.Fatalf("relay within backoff = %d/%d, want 0/0", attempted, delivered)
	}

	// 下游恢复 + 退避到期（把 next_retry_at 拨回过去模拟时间推进）
	env.analyticsDown.Store(false)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := env.svcCtx.AnalyticsOutbox.MarkFailed(row.EventId, 1, "模拟退避到期", past); err != nil {
		t.Fatalf("rewind backoff: %v", err)
	}
	attempted, delivered = env.svcCtx.AnalyticsRelay.RelayOnce(ctx)
	if attempted != 1 || delivered != 1 {
		t.Fatalf("relay recovered = %d/%d, want 1/1", attempted, delivered)
	}
	if len(env.analyticsSeen) != 1 {
		t.Fatalf("analytics batches after recovery = %d, want 1", len(env.analyticsSeen))
	}
	ev := decodeEvents(t, env.analyticsSeen[0])
	if len(ev) != 1 || ev[0].EventId != "order_paid:"+orderID || ev[0].AmountCents != 1500 || ev[0].UserId != 7 {
		t.Fatalf("relayed event = %+v, want order_paid/1500/user 7", ev)
	}
	rows, _ = env.svcCtx.AnalyticsOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 0 {
		t.Fatalf("outbox pending after delivered = %d, want 0", len(rows))
	}
}

func decodeEvents(t *testing.T, body string) []analyticsEvent {
	t.Helper()
	var req struct {
		Events []analyticsEvent `json:"events"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode analytics body %q: %v", body, err)
	}
	return req.Events
}

// postCallback 以沙箱 HMAC 签名投递回调，返回状态码。
func postCallback(t *testing.T, env *testEnv, payload []byte) int {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(testSandboxSecret))
	mac.Write(payload)
	req, err := http.NewRequest(http.MethodPost, env.base+"/callback/sandbox", bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("build callback: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Payment-Signature", hex.EncodeToString(mac.Sum(nil)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST /callback/sandbox: %v", err)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.StatusCode
}

// refundRequest 管理面登记退款/冲正意图，返回状态码。
func refundRequest(t *testing.T, env *testEnv, token, orderID string, amount int64, reason string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"action": "REFUND", "amount_cents": amount, "reason": reason,
	})
	req, err := http.NewRequest(http.MethodPost, env.base+"/admin/orders/"+orderID+"/refund", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("build refund: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST refund: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Logf("refund body=%s", raw)
	}
	return resp.StatusCode
}
