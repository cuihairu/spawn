package handler

// GP-404 合作方自助账单确认与结算差异处理（F12/F23）：
// partner 角色 + game-catalog 绑定的 partner_id 行级隔离（只读本人名下
// 单据）、账单确认、差异清单与异议处理闭环；运营复核清异议后再确认。
// 权限隔离随功能验收：角色权威在 game-catalog /me，不信任客户端声明。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/utils"
)

func TestRoutes_PartnerStatementSelfService(t *testing.T) {
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

	// 两笔已付订单（partner_id=5，与商品快照一致）
	for i := 0; i < 2; i++ {
		paidOrder(t, env, partner)
	}

	now := time.Now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).Format(time.RFC3339)
	end := time.Date(now.Year(), now.Month(), now.Day(), 23, 59, 59, 0, time.UTC).Format(time.RFC3339)

	// 合作方看不到 admin 面（角色非 partner）
	code, _ = do(t, http.MethodGet, env.base+"/partner/statements", partner, "")
	if code != http.StatusOK {
		t.Fatalf("partner list = %d body=%s", code, "")
	}
	code, _ = do(t, http.MethodGet, env.base+"/admin/statements", partner, "")
	if code != http.StatusForbidden {
		t.Fatalf("partner on admin face = %d, want 403", code)
	}

	// 运营生成两张对账单：partner 5（本合作方）+ partner 9（他人）
	gen := func(partnerID int64, rate int64) int64 {
		b := fmt.Sprintf(`{"vendor":"sandbox","partner_id":%d,"period_start":%q,"period_end":%q,"split_rate_bp":%d}`,
			partnerID, start, end, rate)
		c, bb := do(t, http.MethodPost, env.base+"/admin/statements", admin, b)
		if c != http.StatusOK {
			t.Fatalf("generate statement partner=%d = %d body=%s", partnerID, c, bb)
		}
		var v struct {
			Statement struct {
				Id   int64 `json:"id"`
				Diff int64 `json:"diff"`
			} `json:"statement"`
		}
		if err := json.Unmarshal([]byte(bb), &v); err != nil {
			t.Fatalf("parse statement: %v body=%s", err, bb)
		}
		return v.Statement.Id
	}
	ownID := gen(5, 7000)
	otherID := gen(9, 7000)

	// 列表：只含本人名下单据（他人单据 id 不出现）
	code, body = do(t, http.MethodGet, env.base+"/partner/statements", partner, "")
	if code != http.StatusOK || !strings.Contains(body, fmt.Sprintf(`"id":%d`, ownID)) ||
		strings.Contains(body, fmt.Sprintf(`"id":%d`, otherID)) {
		t.Fatalf("partner list scope = %d body=%s", code, body)
	}

	// 详情：本人 200 / 他人 404 / player 403
	code, _ = do(t, http.MethodGet, fmt.Sprintf("%s/partner/statements/%d", env.base, ownID), partner, "")
	if code != http.StatusOK {
		t.Fatalf("partner get own = %d", code)
	}
	code, _ = do(t, http.MethodGet, fmt.Sprintf("%s/partner/statements/%d", env.base, otherID), partner, "")
	if code != http.StatusNotFound {
		t.Fatalf("partner get other = %d, want 404", code)
	}
	code, _ = do(t, http.MethodGet, fmt.Sprintf("%s/partner/statements/%d/export", env.base, otherID), partner, "")
	if code != http.StatusNotFound {
		t.Fatalf("partner export other = %d, want 404", code)
	}

	// 无差异时不可提异议（settlement 未录入 → diff = 1400，先补一笔结算单使差异为 0）
	code, body = do(t, http.MethodPut, fmt.Sprintf("%s/admin/statements/%d/settlement", env.base, ownID), admin, `{"settlement":2000}`)
	if code != http.StatusOK || !strings.Contains(body, `"diff":0`) {
		t.Fatalf("set settlement 1400 = %d body=%s", code, body)
	}
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/partner/statements/%d/dispute", env.base, ownID), partner, `{"reason":"金额不符"}`)
	if code != http.StatusConflict {
		t.Fatalf("dispute without diff = %d, want 409", code)
	}

	// 差异清单：diff=0 不入清单
	code, body = do(t, http.MethodGet, env.base+"/partner/statements/differences", partner, "")
	if code != http.StatusOK || strings.Contains(body, fmt.Sprintf(`"id":%d`, ownID)) {
		t.Fatalf("differences without diff = %d body=%s", code, body)
	}

	// 运营改录渠道结算单 → 产生差异 700，进入差异清单
	code, body = do(t, http.MethodPut, fmt.Sprintf("%s/admin/statements/%d/settlement", env.base, ownID), admin, `{"settlement":1300}`)
	if code != http.StatusOK || !strings.Contains(body, `"diff":700`) {
		t.Fatalf("set settlement 700 = %d body=%s", code, body)
	}

	code, body = do(t, http.MethodGet, env.base+"/partner/statements/differences", partner, "")
	if code != http.StatusOK || !strings.Contains(body, fmt.Sprintf(`"id":%d`, ownID)) || !strings.Contains(body, `"diff":700`) {
		t.Fatalf("differences = %d body=%s", code, body)
	}

	// 合作方提异议（理由必填；空理由 400）
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/partner/statements/%d/dispute", env.base, ownID), partner, `{"reason":""}`)
	if code != http.StatusBadRequest {
		t.Fatalf("dispute empty reason = %d, want 400", code)
	}
	code, body = do(t, http.MethodPost, fmt.Sprintf("%s/partner/statements/%d/dispute", env.base, ownID), partner, `{"reason":"渠道结算单与本表不符"}`)
	if code != http.StatusOK || !strings.Contains(body, `"status":"disputed"`) || !strings.Contains(body, `"dispute_reason":"渠道结算单与本表不符"`) {
		t.Fatalf("dispute = %d body=%s", code, body)
	}

	// 异议期间：合作方确认 409；运营重生成亦 409
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/partner/statements/%d/confirm", env.base, ownID), partner, "")
	if code != http.StatusConflict {
		t.Fatalf("confirm while disputed = %d, want 409", code)
	}
	code, _ = do(t, http.MethodPost, env.base+"/admin/statements", admin,
		fmt.Sprintf(`{"vendor":"sandbox","partner_id":5,"period_start":%q,"period_end":%q,"split_rate_bp":7000}`, start, end))
	if code != http.StatusConflict {
		t.Fatalf("regenerate while disputed = %d, want 409", code)
	}

	// 运营复核处理异议 → 回草稿并清空异议；运营补平结算单 → 差异 0
	code, body = do(t, http.MethodPost, fmt.Sprintf("%s/admin/statements/%d/resolve", env.base, ownID), admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"draft"`) ||
		!strings.Contains(body, `"dispute_reason":""`) {
		t.Fatalf("resolve dispute = %d body=%s", code, body)
	}

	// GP-404 异议复核结果回流合作方（站内信）：恰好一次，类型与目标正确，
	// S2S 令牌与 community 侧同值（防跨服务伪造）。
	if env.notify.Calls() != 1 {
		t.Fatalf("notify calls = %d, want 1 (异议复核只回执一次)", env.notify.Calls())
	}
	nraw, ntok := env.notify.At(0)
	if ntok != testNotifyToken {
		t.Fatalf("notify token = %q, want %q", ntok, testNotifyToken)
	}
	var nbody struct {
		UserIds  []int64 `json:"user_ids"`
		Type     string  `json:"type"`
		TargetId int64   `json:"target_id"`
		Content  string  `json:"content"`
	}
	if err := json.Unmarshal(nraw, &nbody); err != nil {
		t.Fatalf("parse notify body: %v raw=%s", err, nraw)
	}
	if nbody.Type != "statement_dispute_resolved" || nbody.TargetId != ownID ||
		len(nbody.UserIds) != 1 || nbody.UserIds[0] != 2 ||
		!strings.Contains(nbody.Content, "异议已复核") {
		t.Fatalf("notify body = %+v, want type=statement_dispute_resolved target=%d user=2", nbody, ownID)
	}

	// GP-404 异议轮次时间线（版本化留痕）：第一轮异议已落表并结案，diff 快照
	// 为当时 700；管理面可查，partner 面 403。
	code, body = do(t, http.MethodGet, fmt.Sprintf("%s/admin/statements/%d/disputes", env.base, ownID), admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"round":1`) ||
		!strings.Contains(body, `"diff_at_dispute":700`) || !strings.Contains(body, `"dispute_by":2`) ||
		!strings.Contains(body, `"resolved_at"`) {
		t.Fatalf("dispute rounds round1 = %d body=%s", code, body)
	}
	code, _ = do(t, http.MethodGet, fmt.Sprintf("%s/admin/statements/%d/disputes", env.base, ownID), partner, "")
	if code != http.StatusForbidden {
		t.Fatalf("partner on dispute rounds = %d, want 403", code)
	}
	// 第二轮异议（复核后差异仍在，可再提）：轮次递增为 2，理由各自留痕
	code, body = do(t, http.MethodPost, fmt.Sprintf("%s/partner/statements/%d/dispute", env.base, ownID), partner, `{"reason":"结算口径需再核对"}`)
	if code != http.StatusOK || !strings.Contains(body, `"status":"disputed"`) {
		t.Fatalf("second dispute = %d body=%s", code, body)
	}
	code, body = do(t, http.MethodGet, fmt.Sprintf("%s/admin/statements/%d/disputes", env.base, ownID), admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"round":2`) ||
		!strings.Contains(body, `"dispute_reason":"结算口径需再核对"`) {
		t.Fatalf("dispute rounds round2 = %d body=%s", code, body)
	}
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/admin/statements/%d/resolve", env.base, ownID), admin, "")
	if code != http.StatusOK {
		t.Fatalf("resolve round2 = %d, want 200", code)
	}

	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/admin/statements/%d/resolve", env.base, ownID), admin, "")
	if code != http.StatusConflict {
		t.Fatalf("resolve again = %d, want 409", code)
	}
	code, body = do(t, http.MethodPut, fmt.Sprintf("%s/admin/statements/%d/settlement", env.base, ownID), admin, `{"settlement":2000}`)
	if code != http.StatusOK || !strings.Contains(body, `"diff":0`) {
		t.Fatalf("set settlement balance = %d body=%s", code, body)
	}

	// 合作方确认（先确认不自动打款）
	code, body = do(t, http.MethodPost, fmt.Sprintf("%s/partner/statements/%d/confirm", env.base, ownID), partner, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"confirmed"`) {
		t.Fatalf("partner confirm = %d body=%s", code, body)
	}
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/partner/statements/%d/confirm", env.base, ownID), partner, "")
	if code != http.StatusConflict {
		t.Fatalf("partner confirm twice = %d, want 409", code)
	}
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/partner/statements/%d/dispute", env.base, ownID), partner, `{"reason":"再提异议"}`)
	if code != http.StatusConflict {
		t.Fatalf("dispute after confirm = %d, want 409", code)
	}

	// 导出（本人单据）含两笔明细
	code, body = do(t, http.MethodGet, fmt.Sprintf("%s/partner/statements/%d/export", env.base, ownID), partner, "")
	if code != http.StatusOK || !strings.Contains(body, "order_id") || !strings.Contains(body, "summary,") {
		t.Fatalf("partner export = %d body=%s", code, body)
	}

	// 审计：合作方侧动作留痕（role=partner）
	code, body = do(t, http.MethodGet, env.base+"/admin/audit", admin, "")
	if code != http.StatusOK {
		t.Fatalf("audit = %d", code)
	}
	for _, action := range []string{"partner_statement_confirm", "partner_statement_dispute", "partner_statement_export", "partner_statement_list", "statement_dispute_resolved"} {
		if !strings.Contains(body, action) {
			t.Fatalf("audit lacks %s: %s", action, body)
		}
	}
	if !strings.Contains(body, `"actor_role":"partner"`) {
		t.Fatalf("audit lacks partner actor role: %s", body)
	}
}
