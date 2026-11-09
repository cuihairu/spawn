package handler

// GP-303 首充/累充/回流集成测试：首充一次性发放 + 账本兜底（迟到事件双
// 首充跳过）、累充档位按净额达档逐档发放（幂等键含档位序号、跨订单不重复）、
// 退款净额跌破档位置 revoked（重放也跑撤销通道）、净额回升后补发、奖励
// 类型并列不折叠进基础倍率、草稿校验（档位升序/正额）、审计。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/promotions/utils"
)

// gp303Body 构造 GP-303 评估体（account 可换）。
func gp303Body(orderID, account string, first bool, cumulative int64) string {
	return fmt.Sprintf(`{
		"order_id":%q,"game_id":"game-a","server_id":1,"role_id":"role-9",
		"account_id":%q,"sku_id":"sku-gem-100","product_id":"gem-100",
		"channel":"sandbox","paid_cents":1000,"currency":"CNY",
		"paid_at":"2026-11-09T12:00:00+08:00","base_quantity":100,
		"first_purchase":%t,"cumulative_cents":%d
	}`, orderID, account, first, cumulative)
}

type gp303Grant struct {
	ActivityId    int64           `json:"activity_id"`
	RuleType      string          `json:"rule_type"`
	Milestone     int64           `json:"milestone"`
	ExtraQuantity int64           `json:"extra_quantity"`
	Status        string          `json:"status"`
	Snapshot      json.RawMessage `json:"snapshot"`
}

type gp303Resp struct {
	OrderId string       `json:"order_id"`
	Replay  bool         `json:"replay"`
	Grants  []gp303Grant `json:"grants"`
	Skipped []struct {
		ActivityId int64  `json:"activity_id"`
		Reason     string `json:"reason"`
	} `json:"skipped"`
	Revoked []gp303Grant `json:"revoked"`
}

func TestRoutes_FirstAndCumulativeRewards(t *testing.T) {
	env := newActivityServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})

	// 首充活动：固定加赠 50（非叠入基础倍率，与 multiplier 并列）
	idF := publishActivity(t, env, admin, `{
		"name":"首充礼","game_id":"game-a","timezone":"Asia/Shanghai",
		"start_at":"2026-11-01T00:00:00+08:00","end_at":"2026-11-30T23:59:59+08:00",
		"server_ids":[1],"channel":"sandbox","audience":{"type":"all"},
		"trigger":{"type":"order","min_amount_cents":0},
		"stackable":true,
		"rewards":{"type":"first_purchase","extra_quantity":50}
	}`)
	// 累充活动：两档 500/1000，各自固定加赠 20/50
	idC := publishActivity(t, env, admin, `{
		"name":"累充档","game_id":"game-a","timezone":"Asia/Shanghai",
		"start_at":"2026-11-01T00:00:00+08:00","end_at":"2026-11-30T23:59:59+08:00",
		"server_ids":[1],"channel":"sandbox","audience":{"type":"all"},
		"trigger":{"type":"order","min_amount_cents":0},
		"stackable":true,
		"rewards":{"type":"cumulative_recharge","thresholds":[
			{"cents":500,"extra_quantity":20},
			{"cents":1000,"extra_quantity":50}
		]}
	}`)
	if idF == 0 || idC == 0 || idF == idC {
		t.Fatalf("bad activity ids: F=%d C=%d", idF, idC)
	}

	// 1. 首充首单 + 累充达 600（档0）：两活动各发一档
	code, body := evaluate(t, env, gp303Body("o-1", "u1", true, 600), "")
	if code != http.StatusOK {
		t.Fatalf("evaluate o-1 = %d body=%s", code, body)
	}
	var r1 gp303Resp
	_ = json.Unmarshal([]byte(body), &r1)
	if len(r1.Grants) != 2 {
		t.Fatalf("o-1 grants = %+v body=%s", r1.Grants, body)
	}
	fGot, cGot := false, false
	for _, g := range r1.Grants {
		if g.Status != "granted" {
			t.Fatalf("o-1 grant status = %s", g.Status)
		}
		switch g.RuleType {
		case "first_purchase":
			fGot = true
			if g.ActivityId != idF || g.ExtraQuantity != 50 {
				t.Fatalf("o-1 first grant = %+v", g)
			}
		case "cumulative_recharge":
			cGot = true
			if g.ActivityId != idC || g.Milestone != 0 || g.ExtraQuantity != 20 {
				t.Fatalf("o-1 cum grant = %+v", g)
			}
			var snap map[string]any
			if err := json.Unmarshal(g.Snapshot, &snap); err != nil {
				t.Fatalf("o-1 cum snapshot: %v", err)
			}
			rule, _ := snap["rule"].(map[string]any)
			if rule["threshold_cents"] != float64(500) || rule["milestone"] != float64(0) {
				t.Fatalf("o-1 cum snapshot rule = %+v", rule)
			}
		}
	}
	if !fGot || !cGot {
		t.Fatalf("o-1 missing rule types: first=%v cum=%v", fGot, cGot)
	}

	// 2. 重放 o-1：replay=true，账本不重复
	code, body = evaluate(t, env, gp303Body("o-1", "u1", true, 600), "")
	if code != http.StatusOK {
		t.Fatalf("replay o-1 = %d", code)
	}
	var r2 gp303Resp
	_ = json.Unmarshal([]byte(body), &r2)
	if !r2.Replay || len(r2.Grants) != 2 {
		t.Fatalf("replay o-1 = %+v", r2)
	}
	code, body = do(t, http.MethodGet, env.base+"/admin/reward-grants?order_id=o-1", admin, "")
	var list struct {
		Grants []gp303Grant `json:"grants"`
	}
	_ = json.Unmarshal([]byte(body), &list)
	if code != http.StatusOK || len(list.Grants) != 2 {
		t.Fatalf("grants list o-1 = %d count=%d body=%s", code, len(list.Grants), body)
	}

	// 3. 第二单非首充、累充 1100：档0 已被 o-1 发（milestone-already-granted）、
	//    档1 新发（o-2 键下）
	code, body = evaluate(t, env, gp303Body("o-2", "u1", false, 1100), "")
	if code != http.StatusOK {
		t.Fatalf("evaluate o-2 = %d body=%s", code, body)
	}
	var r3 gp303Resp
	_ = json.Unmarshal([]byte(body), &r3)
	if len(r3.Grants) != 1 || r3.Grants[0].RuleType != "cumulative_recharge" ||
		r3.Grants[0].Milestone != 1 || r3.Grants[0].ExtraQuantity != 50 {
		t.Fatalf("o-2 grants = %+v body=%s", r3.Grants, body)
	}
	m0Skipped := false
	for _, s := range r3.Skipped {
		if s.ActivityId == idC && s.Reason == "milestone-already-granted" {
			m0Skipped = true
		}
	}
	if !m0Skipped {
		t.Fatalf("o-2 skipped = %+v", r3.Skipped)
	}

	// 4. 首充兜底（迟到事件双首充）：o-3 又带 first_purchase=true，该账号
	//    该活动首充已发 → 跳过；累充两档均已发 → 全部跳过，零发放
	code, body = evaluate(t, env, gp303Body("o-3", "u1", true, 1500), "")
	if code != http.StatusOK {
		t.Fatalf("evaluate o-3 = %d body=%s", code, body)
	}
	var r4 gp303Resp
	_ = json.Unmarshal([]byte(body), &r4)
	if len(r4.Grants) != 0 {
		t.Fatalf("o-3 grants = %+v body=%s", r4.Grants, body)
	}
	fpSkipped := false
	for _, s := range r4.Skipped {
		if s.ActivityId == idF && s.Reason == "first-purchase-already-granted" {
			fpSkipped = true
		}
	}
	if !fpSkipped {
		t.Fatalf("o-3 skipped = %+v", r4.Skipped)
	}

	// 5. 退款重放撤销：o-2 已有账本行（发过档1），退款后重入队（净额 300，
	//    跌破两档）→ 重放路径仍跑撤销通道：档0（o-1）与档1（o-2）均置 revoked
	code, body = evaluate(t, env, gp303Body("o-2", "u1", false, 300), "")
	if code != http.StatusOK {
		t.Fatalf("refund replay o-2 = %d body=%s", code, body)
	}
	var r5 gp303Resp
	_ = json.Unmarshal([]byte(body), &r5)
	if !r5.Replay || len(r5.Grants) != 1 || len(r5.Revoked) != 2 {
		t.Fatalf("refund replay o-2 grants=%+v revoked=%+v body=%s", r5.Grants, r5.Revoked, body)
	}
	revokedStatus := map[int64]string{}
	for _, g := range r5.Revoked {
		if g.ActivityId != idC || g.Status != "revoked" {
			t.Fatalf("refund replay revoked = %+v", g)
		}
		revokedStatus[g.Milestone] = g.Status
	}
	if revokedStatus[0] != "revoked" || revokedStatus[1] != "revoked" {
		t.Fatalf("refund replay revoked milestones = %+v", revokedStatus)
	}

	// 6. 账本持久化 revoked（管理面可查；o-1 的累充档0 已被撤销，首充行仍 granted）
	code, body = do(t, http.MethodGet, env.base+"/admin/reward-grants?order_id=o-1", admin, "")
	_ = json.Unmarshal([]byte(body), &list)
	if code != http.StatusOK || len(list.Grants) != 2 {
		t.Fatalf("o-1 after revoke = %d grants=%+v body=%s", code, list.Grants, body)
	}
	cumRevoked, fpGranted := false, false
	for _, g := range list.Grants {
		if g.RuleType == "cumulative_recharge" && g.Status == "revoked" {
			cumRevoked = true
		}
		if g.RuleType == "first_purchase" && g.Status == "granted" {
			fpGranted = true
		}
	}
	if !cumRevoked || !fpGranted {
		t.Fatalf("o-1 after revoke = %+v", list.Grants)
	}

	// 7. 净额回升补发：o-5 净额 1200 → 档0/档1 重新发放（o-5 键下），无撤销
	code, body = evaluate(t, env, gp303Body("o-5", "u1", false, 1200), "")
	if code != http.StatusOK {
		t.Fatalf("evaluate o-5 = %d body=%s", code, body)
	}
	var r7 gp303Resp
	_ = json.Unmarshal([]byte(body), &r7)
	if len(r7.Grants) != 2 || len(r7.Revoked) != 0 {
		t.Fatalf("o-5 grants=%+v revoked=%+v body=%s", r7.Grants, r7.Revoked, body)
	}
	for _, g := range r7.Grants {
		if g.Status != "granted" {
			t.Fatalf("o-5 grant status = %s", g.Status)
		}
	}

	// 8. 草稿校验：档位非升序 / 首充非正额 / 未知类型 → 400
	badCases := []string{
		`{"name":"x","game_id":"game-a","timezone":"Asia/Shanghai","start_at":"2026-11-01T00:00:00+08:00","end_at":"2026-11-30T23:59:59+08:00","rewards":{"type":"cumulative_recharge","thresholds":[{"cents":1000,"extra_quantity":50},{"cents":500,"extra_quantity":20}]}}`,
		`{"name":"x","game_id":"game-a","timezone":"Asia/Shanghai","start_at":"2026-11-01T00:00:00+08:00","end_at":"2026-11-30T23:59:59+08:00","rewards":{"type":"first_purchase","extra_quantity":0}}`,
		`{"name":"x","game_id":"game-a","timezone":"Asia/Shanghai","start_at":"2026-11-01T00:00:00+08:00","end_at":"2026-11-30T23:59:59+08:00","rewards":{"type":"cashback","rate_bp":5000}}`,
	}
	for i, bc := range badCases {
		code, _ = do(t, http.MethodPost, env.base+"/admin/activities", admin, bc)
		if code != http.StatusBadRequest {
			t.Fatalf("bad case %d = %d, want 400", i, code)
		}
	}

	// 9. 审计含首充/累充评估（grant/revoke 明细可查）
	code, body = do(t, http.MethodGet, env.base+"/admin/activity-audit", admin, "")
	if code != http.StatusOK || !strings.Contains(body, "cumulative_cents") ||
		!strings.Contains(body, "first_purchase") || !strings.Contains(body, "revoked") {
		t.Fatalf("audit = %d body=%s", code, body)
	}
}
