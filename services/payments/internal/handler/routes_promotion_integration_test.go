package handler

// GP-302 奖励评估桥集成测试：订单 PAID 后按投影+发货账本事实入队（基础量
// 来自服务端商品配置），分发桥以 HMAC 签名推送 promotions；同订单幂等
// （重复回调不重复入队、不重复推送）。

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/utils"
)

func TestRoutes_RewardEvaluationBridge(t *testing.T) {
	env := newDeliveryServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})
	secret := "promotions-reward-test-secret"

	// 商品带基础购买量（GP-302 倍率基础量，服务端配置）
	code, body := do(t, http.MethodPost, env.base+"/admin/products/sku-gem-100", admin,
		`{"product_id":"gem-100","form":"DIRECT_PURCHASE","amount_cents":1000,"currency":"CNY","game_id":"game-a","partner_id":2,"base_quantity":100}`)
	if code != http.StatusOK || !strings.Contains(body, `"base_quantity":100`) {
		t.Fatalf("create product = %d body=%s", code, body)
	}
	code, _ = do(t, http.MethodPut, env.base+"/admin/games/game-a/delivery", admin,
		fmt.Sprintf(`{"endpoint":%q,"secret":"gs-secret"}`, env.gameServer.srv.URL))
	if code != http.StatusOK {
		t.Fatalf("configure endpoint = %d", code)
	}

	// 启动分发扫描（100ms 快扫；进程内由 main 启动，测试自启）
	scanCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go env.svcCtx.PromotionBridge.Scan(scanCtx, 100*time.Millisecond)

	// 下单 → 回调 PAID（入队）→ 投递
	orderID := paidOrder(t, env, player)

	// 等待桥推送并断言签名与订单事实
	waitFor(t, 3*time.Second, func() bool { return env.promo.Calls() >= 1 })
	raw, sig := env.promo.Request(0)
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(raw)
	if sig != hex.EncodeToString(mac.Sum(nil)) {
		t.Fatalf("reward eval signature mismatch: %s", sig)
	}
	var facts struct {
		OrderId      string `json:"order_id"`
		GameId       string `json:"game_id"`
		ServerId     int64  `json:"server_id"`
		AccountId    string `json:"account_id"`
		SkuId        string `json:"sku_id"`
		Channel      string `json:"channel"`
		PaidCents    int64  `json:"paid_cents"`
		BaseQuantity int64  `json:"base_quantity"`
	}
	if err := json.Unmarshal(raw, &facts); err != nil {
		t.Fatalf("parse facts: %v body=%s", err, raw)
	}
	if facts.OrderId != orderID || facts.GameId != "game-a" || facts.ServerId != 1 ||
		facts.AccountId != "7" || facts.SkuId != "sku-gem-100" ||
		facts.Channel != "sandbox" || facts.PaidCents != 1000 || facts.BaseQuantity != 100 {
		t.Fatalf("facts = %+v raw=%s", facts, raw)
	}

	// 幂等：同订单重复回调（渠道重发场景）不重复入队/推送
	cbBody := fmt.Sprintf(`{"orderId":%q,"paidAt":%q}`, orderID, time.Now().UTC().Format(time.RFC3339))
	req, _ := http.NewRequest(http.MethodPost, env.base+"/callback/sandbox", strings.NewReader(cbBody))
	req.Header.Set("X-Payment-Signature", deliverySig(testSandboxSecret, cbBody))
	resp, _ := http.DefaultClient.Do(req)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("repeat callback = %d", resp.StatusCode)
	}
	time.Sleep(400 * time.Millisecond)
	if calls := env.promo.Calls(); calls != 1 {
		t.Fatalf("promo calls after replay = %d, want 1（重复事件不重复发放）", calls)
	}

	// 管理面账本查询路由在 promotions 侧（此处只断言桥配置注入）
	if !env.svcCtx.PromotionBridge.Enabled() {
		t.Fatal("promotion bridge should be enabled with stub upstream")
	}
}

// waitFor 轮询直到条件成立或超时。
func waitFor(t *testing.T, d time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("waitFor timeout")
}
