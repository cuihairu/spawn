package handler

// GP-404 业务通知 outbox 补偿集成测试：community 短时不可达（503）→ 异议复核
// 主流程不受影响（仍 200 回草稿），通知意图先落 notify_outbox（pending）；下游
// 恢复后 Relay 补投成功标记 delivered，通知完整到达（类型/目标/受众原样）。
// 重投安全由 community 侧 UNIQUE(user,actor,type,target) 去重保证。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/utils"
)

func TestRoutes_StatementDisputeNotifyOutboxRetry(t *testing.T) {
	env := newDeliveryServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	partner := env.sign(utils.JWTClaims{UserId: 2, Username: "studio-b"})

	code, body := do(t, http.MethodPost, env.base+"/admin/products/sku-gem-100", admin,
		`{"product_id":"gem-100","form":"DIRECT_PURCHASE","amount_cents":1000,"currency":"CNY","game_id":"game-a","partner_id":5}`)
	if code != http.StatusOK {
		t.Fatalf("create product = %d body=%s", code, body)
	}
	code, _ = do(t, http.MethodPut, env.base+"/admin/games/game-a/delivery", admin,
		fmt.Sprintf(`{"endpoint":%q,"secret":"gs-secret"}`, env.gameServer.srv.URL))
	if code != http.StatusOK {
		t.Fatalf("configure endpoint = %d", code)
	}
	for i := 0; i < 2; i++ {
		paidOrder(t, env, partner)
	}

	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.UTC).Format(time.RFC3339)
	code, body = do(t, http.MethodPost, env.base+"/admin/statements", admin,
		fmt.Sprintf(`{"vendor":"sandbox","partner_id":5,"period_start":%q,"period_end":%q,"split_rate_bp":7000}`, start, end))
	if code != http.StatusOK {
		t.Fatalf("generate statement = %d body=%s", code, body)
	}
	var v struct {
		Statement struct {
			Id int64 `json:"id"`
		} `json:"statement"`
	}
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		t.Fatalf("parse statement: %v body=%s", err, body)
	}
	ownID := v.Statement.Id

	// 运营改录渠道结算单 → 产生差异 → 合作方提异议
	code, body = do(t, http.MethodPut, fmt.Sprintf("%s/admin/statements/%d/settlement", env.base, ownID), admin, `{"settlement":1300}`)
	if code != http.StatusOK || !strings.Contains(body, `"diff":700`) {
		t.Fatalf("set settlement = %d body=%s", code, body)
	}
	code, body = do(t, http.MethodPost, fmt.Sprintf("%s/partner/statements/%d/dispute", env.base, ownID), partner, `{"reason":"渠道结算单与本表不符"}`)
	if code != http.StatusOK || !strings.Contains(body, `"status":"disputed"`) {
		t.Fatalf("dispute = %d body=%s", code, body)
	}

	// community 不可达：复核主流程仍 200 回草稿，通知意图落 outbox pending
	env.notify.down.Store(true)
	code, body = do(t, http.MethodPost, fmt.Sprintf("%s/admin/statements/%d/resolve", env.base, ownID), admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"draft"`) {
		t.Fatalf("resolve while community down = %d body=%s", code, body)
	}
	if env.notify.Calls() != 0 {
		t.Fatalf("notify calls = %d, want 0 (community 未恢复不应有成功投递)", env.notify.Calls())
	}
	outbox := env.svcCtx.NotifyOutbox
	if outbox == nil {
		t.Fatal("notify outbox not wired")
	}
	rows, err := outbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("notify outbox pending = %d err=%v, want 1", len(rows), err)
	}
	row := rows[0]
	if row.NotifyType != "statement_dispute_resolved" || row.Status != "pending" || row.Attempts != 0 {
		t.Fatalf("outbox row = %+v, want statement_dispute_resolved/pending/0 attempts", row)
	}

	// 首轮 Relay：仍失败 → attempts=1 + 退避延期
	relay := env.svcCtx.NotifyRelay
	if relay == nil {
		t.Fatal("notify relay not wired")
	}
	ctx := context.Background()
	attempted, delivered := relay.RelayOnce(ctx)
	if attempted != 1 || delivered != 0 {
		t.Fatalf("relay down = %d/%d, want 1/0", attempted, delivered)
	}
	rows, _ = outbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 1 || rows[0].Attempts != 1 {
		t.Fatalf("outbox after failed relay = %+v, want attempts=1", rows)
	}

	// 下游恢复 + 退避到期（把 next_retry_at 拨回过去模拟时间推进）
	env.notify.down.Store(false)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := outbox.MarkFailed(row.NotifyKey, 1, "模拟退避到期", past); err != nil {
		t.Fatalf("rewind backoff: %v", err)
	}
	attempted, delivered = relay.RelayOnce(ctx)
	if attempted != 1 || delivered != 1 {
		t.Fatalf("relay recovered = %d/%d, want 1/1", attempted, delivered)
	}
	if env.notify.Calls() != 1 {
		t.Fatalf("notify calls after recovery = %d, want 1", env.notify.Calls())
	}
	raw, ntok := env.notify.At(0)
	if ntok != testNotifyToken {
		t.Fatalf("relayed token = %q, want %q", ntok, testNotifyToken)
	}
	var nbody struct {
		UserIds  []int64 `json:"user_ids"`
		Type     string  `json:"type"`
		TargetId int64   `json:"target_id"`
	}
	if err := json.Unmarshal(raw, &nbody); err != nil {
		t.Fatalf("parse relayed body: %v raw=%s", err, raw)
	}
	if nbody.Type != "statement_dispute_resolved" || nbody.TargetId != ownID ||
		len(nbody.UserIds) != 1 || nbody.UserIds[0] != 2 {
		t.Fatalf("relayed body = %+v, want type=statement_dispute_resolved target=%d user=2", nbody, ownID)
	}
	rows, _ = outbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 0 {
		t.Fatalf("outbox pending after delivered = %d, want 0", len(rows))
	}
}
