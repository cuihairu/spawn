package handler

// GP-303 奖励评估桥事实集成测试：桥推送给 promotions 的订单事实含首充
// （同账号同游戏无更早 PAID/DELIVERED 订单）与累充净额（PAID/DELIVERED
// 实付合计 − EXECUTED 退款）。退款执行后净额随投影聚合回落。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/model"
	"github.com/tappi/tappi/services/payments/utils"
)

func TestRoutes_RewardEvaluationGp303Facts(t *testing.T) {
	env := newDeliveryServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

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

	scanCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go env.svcCtx.PromotionBridge.Scan(scanCtx, 100*time.Millisecond)

	type facts struct {
		OrderId         string `json:"order_id"`
		GameId          string `json:"game_id"`
		AccountId       string `json:"account_id"`
		PaidCents       int64  `json:"paid_cents"`
		BaseQuantity    int64  `json:"base_quantity"`
		FirstPurchase   bool   `json:"first_purchase"`
		CumulativeCents int64  `json:"cumulative_cents"`
	}
	readFact := func(i int) facts {
		raw, _ := env.promo.Request(i)
		var f facts
		if err := json.Unmarshal(raw, &f); err != nil {
			t.Fatalf("parse facts[%d]: %v body=%s", i, err, raw)
		}
		return f
	}

	// 第一单（账号 7）：首充 true，净额 1000
	order1 := paidOrder(t, env, player)
	waitFor(t, 3*time.Second, func() bool { return env.promo.Calls() >= 1 })
	f1 := readFact(0)
	if f1.OrderId != order1 || !f1.FirstPurchase || f1.CumulativeCents != 1000 ||
		f1.PaidCents != 1000 || f1.BaseQuantity != 100 || f1.GameId != "game-a" || f1.AccountId != "7" {
		t.Fatalf("facts[0] = %+v", f1)
	}

	// 第二单（同账号）：首充 false，净额 2000
	order2 := paidOrder(t, env, player)
	waitFor(t, 3*time.Second, func() bool { return env.promo.Calls() >= 2 })
	f2 := readFact(1)
	if f2.OrderId != order2 || f2.FirstPurchase || f2.CumulativeCents != 2000 {
		t.Fatalf("facts[1] = %+v", f2)
	}

	// 退款执行后：净额回落到 1000（投影聚合口径，供撤销通道用）
	code, _ = do(t, http.MethodPost, env.base+"/admin/orders/"+order2+"/refund", admin, `{"reason":"测试退款"}`)
	if code != http.StatusOK {
		t.Fatalf("create refund = %d", code)
	}
	if err := env.svcCtx.ProjectionDB.MarkRefundExecuted(order2, model.RefundActionRefund); err != nil {
		t.Fatalf("mark refund executed: %v", err)
	}
	net, err := env.svcCtx.ProjectionDB.AccountGameNetCents("7", "game-a")
	if err != nil || net != 1000 {
		t.Fatalf("net after refund = %d err=%v, want 1000", net, err)
	}
	// 首充判定：账号 7 已有更早订单；全新账号 99 无 → 首充成立
	earlier, err := env.svcCtx.ProjectionDB.HasEarlierPaidOrder("7", "game-a", order2)
	if err != nil || !earlier {
		t.Fatalf("account 7 earlier = %v err=%v, want true", earlier, err)
	}
	fresh, err := env.svcCtx.ProjectionDB.HasEarlierPaidOrder("99", "game-a", "")
	if err != nil || fresh {
		t.Fatalf("account 99 earlier = %v err=%v, want false（即首充）", fresh, err)
	}
}
