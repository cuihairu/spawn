package logic

// rewardlogic 补漏分支：Evaluate 验签/解析/字段校验 400/403 臂、三类规则
// 发放主路径（multiplier/first_purchase/cumulative 多档）、零赠送跳过、叠加
// 排除、幂等重放、限次/预算跳过（per_user/global/budget）、首充/累充已发放
// 跳过、库错误 500 臂、退款撤销通道、emitRewardGranted 启用/停用臂、
// ListGrants 管理面查询。
//
// 登记不可达（不测）：Evaluate 中 created==false 的并发重放臂（UNIQUE
// (order_id, activity_id) 冲突行必已在 ListGrants(0, orderId) 命中而被早退，
// 单线程无并发窗口）；mid-loop 的 CountGrantsByUser/ByActivity/
// SumGrantedPaidCents/CountGrantsByUserRule/ListGrantedCumulativeMilestones
// 查询错误臂（与 ListGrants 共用同一 *sql.DB，无法选择性注入单查失败）；
// buildGrantSnapshot 的 json.Marshal 失败臂（map 恒可序列化）。

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/tappi/tappi/services/promotions/internal/analytics"
	"github.com/tappi/tappi/services/promotions/internal/types"
	"github.com/tappi/tappi/services/promotions/model"
)

const rewardSecret = "sec"

// signReward 对 v 计算 HMAC-SHA256 签名，返回原始体与 hex 签名。
func signReward(t *testing.T, secret string, v any) ([]byte, string) {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(b)
	return b, hex.EncodeToString(mac.Sum(nil))
}

// evalOrder 以固定密钥评估订单并断言无错。
func evalOrder(t *testing.T, l *RewardLogic, v any) *types.RewardEvaluateResponse {
	t.Helper()
	body, sig := signReward(t, rewardSecret, v)
	resp, err := l.Evaluate(body, sig)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	return resp
}

// snapCfg 构造标准版快照（窗口 2026 全年、区服 [1]、order 触发 1000）。
// rewards 为 rewards JSON；limits 为空用全 0 缺省。
func snapCfg(gameID, rewards, limits string) string {
	if limits == "" {
		limits = `"per_user_count":0,"global_count":0,"budget_cents":0`
	}
	return `{"name":"r","game_id":"` + gameID + `","timezone":"Asia/Shanghai",` +
		`"start_at":"2026-01-01T00:00:00Z","end_at":"2026-12-31T00:00:00Z",` +
		`"server_ids":[1],"channel":"","audience":{"type":"all"},` +
		`"trigger":{"type":"order","min_amount_cents":1000},` +
		`"limits":{` + limits + `},"stackable":true,"stack_group":"","rewards":` + rewards + `}`
}

// baseOrder 标准订单（g1/区服1/实付2000/基础10000/窗口内）。
func baseOrder() types.RewardEvaluateRequest {
	return types.RewardEvaluateRequest{
		OrderId: "o1", GameId: "g1", ServerId: 1, AccountId: "7",
		PaidCents: 2000, BaseQuantity: 10000, Currency: "CNY",
		PaidAt: "2026-06-01T00:00:00Z",
	}
}

// preGrant 直接向账本插入一条 granted 行（跳过 Evaluate，构造限次/预算前提）。
func preGrant(t *testing.T, l *RewardLogic, activityID int64, orderID, account string, paid int64, ruleType string, milestone int64) {
	t.Helper()
	_, err := l.svcCtx.RewardDB.InsertGrant(&model.RewardGrant{
		OrderId: orderID, ActivityId: activityID, ActivityVersion: 1,
		GameId: "g1", AccountId: account, PaidCents: paid,
		BaseQuantity: 10000, ExtraQuantity: 100, TotalQuantity: 10100,
		RuleType: ruleType, Milestone: milestone, Status: model.RewardStatusGranted,
		Snapshot: "{}", GrantedAt: "2026-06-01T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("pre-grant: %v", err)
	}
}

func TestRewardEvaluate_ValidateArms(t *testing.T) {
	env := newLogicEnv(t, "")
	l := NewRewardLogic(context.Background(), env.sc)
	body, _ := json.Marshal(baseOrder())

	// 未配置密钥 → 403 fail-closed
	if _, err := l.Evaluate(body, ""); err == nil {
		t.Fatal("no secret = nil err")
	} else {
		mustAPIError(t, err, http.StatusForbidden)
	}

	env.withRewardSecret(t, rewardSecret)

	// 签名无效 → 403（错误签名）
	if _, err := l.Evaluate(body, "deadbeef"); err == nil {
		t.Fatal("bad sig = nil err")
	} else {
		mustAPIError(t, err, http.StatusForbidden)
	}

	// 解析失败 → 400（签名需先通过）
	badBody, badSig := signReward(t, rewardSecret, `not-json`)
	if _, err := l.Evaluate(badBody, badSig); err == nil {
		t.Fatal("bad json = nil err")
	} else {
		mustAPIError(t, err, http.StatusBadRequest)
	}

	// 空订单号 → 400
	o := baseOrder()
	o.OrderId = ""
	badBody, badSig = signReward(t, rewardSecret, o)
	if _, err := l.Evaluate(badBody, badSig); err == nil {
		t.Fatal("empty order = nil err")
	} else {
		mustAPIError(t, err, http.StatusBadRequest)
	}

	// 空游戏 → 400
	o = baseOrder()
	o.GameId = ""
	badBody, badSig = signReward(t, rewardSecret, o)
	if _, err := l.Evaluate(badBody, badSig); err == nil {
		t.Fatal("empty game = nil err")
	} else {
		mustAPIError(t, err, http.StatusBadRequest)
	}

	// 负实付 → 400
	o = baseOrder()
	o.PaidCents = -1
	badBody, badSig = signReward(t, rewardSecret, o)
	if _, err := l.Evaluate(badBody, badSig); err == nil {
		t.Fatal("negative paid = nil err")
	} else {
		mustAPIError(t, err, http.StatusBadRequest)
	}

	// 负基础量 → 400
	o = baseOrder()
	o.BaseQuantity = -1
	badBody, badSig = signReward(t, rewardSecret, o)
	if _, err := l.Evaluate(badBody, badSig); err == nil {
		t.Fatal("negative base = nil err")
	} else {
		mustAPIError(t, err, http.StatusBadRequest)
	}
}

func TestRewardEvaluate_GrantPaths(t *testing.T) {
	env := newLogicEnv(t, "")
	env.withRewardSecret(t, rewardSecret)
	l := NewRewardLogic(context.Background(), env.sc)

	// 倍率 total 15000：base 10000 → extra 5000
	seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		snapCfg("g1", `{"type":"multiplier","mode":"total","rate_bp":15000}`, ""))
	resp := evalOrder(t, l, baseOrder())
	if len(resp.Grants) != 1 || resp.Grants[0].ExtraQuantity != 5000 || resp.Grants[0].RuleType != model.RuleTypeMultiplier {
		t.Fatalf("multiplier grants = %+v", resp.Grants)
	}

	// 缺省 paid_at → 取当前时间（同窗口内仍发放）
	nopaid := baseOrder()
	nopaid.OrderId = "o-np"
	nopaid.PaidAt = ""
	if resp = evalOrder(t, l, nopaid); len(resp.Grants) != 1 {
		t.Fatalf("default paid_at grants = %+v", resp.Grants)
	}

	// 幂等重放：同订单 → Replay，返回已发放事实
	replay := evalOrder(t, l, baseOrder())
	if !replay.Replay || len(replay.Grants) != 1 {
		t.Fatalf("replay = %+v", replay)
	}

	// 首充：first_purchase=true 发放一次；非首充不参与；再首充已发放跳过
	seedPublishedActivity(t, env.sc.ActivityDB, "g2",
		snapCfg("g2", `{"type":"first_purchase","extra_quantity":500}`, ""))
	fp := baseOrder()
	fp.OrderId = "o2"
	fp.GameId = "g2"
	fp.FirstPurchase = true
	resp = evalOrder(t, l, fp)
	if len(resp.Grants) != 1 || resp.Grants[0].ExtraQuantity != 500 {
		t.Fatalf("first purchase grants = %+v", resp.Grants)
	}
	notFP := fp
	notFP.OrderId = "o3"
	notFP.FirstPurchase = false
	resp = evalOrder(t, l, notFP)
	if len(resp.Grants) != 0 || len(resp.Skipped) != 0 {
		t.Fatalf("non-first-purchase = grants %+v skipped %+v", resp.Grants, resp.Skipped)
	}
	fpAgain := fp
	fpAgain.OrderId = "o4"
	resp = evalOrder(t, l, fpAgain)
	if len(resp.Grants) != 0 {
		t.Fatalf("first purchase again = %+v", resp.Grants)
	}
	found := false
	for _, s := range resp.Skipped {
		if s.Reason == "first-purchase-already-granted" {
			found = true
		}
	}
	if !found {
		t.Fatalf("first purchase skip reason missing: %+v", resp.Skipped)
	}

	// 累充：两档（1000:10 / 2000:20），净额 2500 → 2 档发放
	seedPublishedActivity(t, env.sc.ActivityDB, "g3",
		snapCfg("g3", `{"type":"cumulative_recharge","thresholds":[{"cents":1000,"extra_quantity":10},{"cents":2000,"extra_quantity":20}]}`, ""))
	cu := baseOrder()
	cu.OrderId = "o5"
	cu.GameId = "g3"
	cu.CumulativeCents = 2500
	resp = evalOrder(t, l, cu)
	if len(resp.Grants) != 2 {
		t.Fatalf("cumulative grants = %+v", resp.Grants)
	}
	if resp.Grants[0].Milestone != 0 || resp.Grants[1].Milestone != 1 {
		t.Fatalf("cumulative milestones = %+v", resp.Grants)
	}
	// 同账号同活动同档位跨订单不重复
	cu2 := cu
	cu2.OrderId = "o6"
	resp = evalOrder(t, l, cu2)
	if len(resp.Grants) != 0 {
		t.Fatalf("cumulative repeat grants = %+v", resp.Grants)
	}
	// 零赠送（rate_bp=10000）跳过
	seedPublishedActivity(t, env.sc.ActivityDB, "g4",
		snapCfg("g4", `{"type":"multiplier","mode":"total","rate_bp":10000}`, ""))
	zero := baseOrder()
	zero.OrderId = "o7"
	zero.GameId = "g4"
	resp = evalOrder(t, l, zero)
	if len(resp.Grants) != 0 {
		t.Fatalf("zero-bonus grants = %+v", resp.Grants)
	}
	found = false
	for _, s := range resp.Skipped {
		if s.Reason == "zero-bonus" {
			found = true
		}
	}
	if !found {
		t.Fatalf("zero-bonus skip missing: %+v", resp.Skipped)
	}
}

func TestRewardEvaluate_SkipArms(t *testing.T) {
	env := newLogicEnv(t, "")
	env.withRewardSecret(t, rewardSecret)
	l := NewRewardLogic(context.Background(), env.sc)

	// per_user_count=1：先发一条 → 第二次同账号跳过
	actU := seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		snapCfg("g1", `{"type":"multiplier","mode":"total","rate_bp":15000}`, `"per_user_count":1,"global_count":0,"budget_cents":0`))
	preGrant(t, l, actU, "pre-u", "7", 2000, model.RuleTypeMultiplier, 0)
	resp := evalOrder(t, l, baseOrder())
	if len(resp.Grants) != 0 || len(resp.Skipped) != 1 || resp.Skipped[0].Reason != "per-user-limit-reached" {
		t.Fatalf("per-user skip = grants %+v skipped %+v", resp.Grants, resp.Skipped)
	}

	// global_count=1：先发一条 → 第二次跳过
	actG := seedPublishedActivity(t, env.sc.ActivityDB, "g2",
		snapCfg("g2", `{"type":"multiplier","mode":"total","rate_bp":15000}`, `"per_user_count":0,"global_count":1,"budget_cents":0`))
	preGrant(t, l, actG, "pre-g", "7", 2000, model.RuleTypeMultiplier, 0)
	gOrder := baseOrder()
	gOrder.GameId = "g2"
	gOrder.OrderId = "og"
	resp = evalOrder(t, l, gOrder)
	if len(resp.Grants) != 0 || len(resp.Skipped) != 1 || resp.Skipped[0].Reason != "global-limit-reached" {
		t.Fatalf("global skip = grants %+v skipped %+v", resp.Grants, resp.Skipped)
	}

	// budget=1000：已发 500 + 本单 2000 > 1000 → 跳过
	actB := seedPublishedActivity(t, env.sc.ActivityDB, "g3",
		snapCfg("g3", `{"type":"multiplier","mode":"total","rate_bp":15000}`, `"per_user_count":0,"global_count":0,"budget_cents":1000`))
	preGrant(t, l, actB, "pre-b", "7", 500, model.RuleTypeMultiplier, 0)
	bOrder := baseOrder()
	bOrder.GameId = "g3"
	bOrder.OrderId = "ob"
	resp = evalOrder(t, l, bOrder)
	if len(resp.Grants) != 0 || len(resp.Skipped) != 1 || resp.Skipped[0].Reason != "budget-exhausted" {
		t.Fatalf("budget skip = grants %+v skipped %+v", resp.Grants, resp.Skipped)
	}
}

func TestRewardEvaluate_StackArm(t *testing.T) {
	env := newLogicEnv(t, "")
	env.withRewardSecret(t, rewardSecret)
	l := NewRewardLogic(context.Background(), env.sc)

	// 同 stack_group 互斥：B(extra 10000) > A(extra 5000) → B 胜出，A 排除
	a := seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		`{"name":"a","game_id":"g1","timezone":"Asia/Shanghai","start_at":"2026-01-01T00:00:00Z","end_at":"2026-12-31T00:00:00Z","server_ids":[1],"channel":"","audience":{"type":"all"},"trigger":{"type":"order","min_amount_cents":1000},"limits":{"per_user_count":0,"global_count":0,"budget_cents":0},"stackable":true,"stack_group":"x","rewards":{"type":"multiplier","mode":"total","rate_bp":15000}}`)
	seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		`{"name":"b","game_id":"g1","timezone":"Asia/Shanghai","start_at":"2026-01-01T00:00:00Z","end_at":"2026-12-31T00:00:00Z","server_ids":[1],"channel":"","audience":{"type":"all"},"trigger":{"type":"order","min_amount_cents":1000},"limits":{"per_user_count":0,"global_count":0,"budget_cents":0},"stackable":true,"stack_group":"x","rewards":{"type":"multiplier","mode":"total","rate_bp":20000}}`)
	resp := evalOrder(t, l, baseOrder())
	if len(resp.Grants) != 1 || resp.Grants[0].ExtraQuantity != 10000 {
		t.Fatalf("stack winners = %+v", resp.Grants)
	}
	if len(resp.Skipped) != 1 || resp.Skipped[0].ActivityId != a || resp.Skipped[0].Reason != "stack-group-conflict" {
		t.Fatalf("stack skipped = %+v", resp.Skipped)
	}
}

func TestRewardEvaluate_WindowAndTargetArms(t *testing.T) {
	env := newLogicEnv(t, "")
	env.withRewardSecret(t, rewardSecret)
	l := NewRewardLogic(context.Background(), env.sc)

	// 快照解析失败 → 跳过
	seedPublishedActivity(t, env.sc.ActivityDB, "g1", "not-json")
	// 游戏不符（快照 g2，请求 g1）→ 跳过
	seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		snapCfg("g2", `{"type":"multiplier","mode":"total","rate_bp":15000}`, ""))
	// 窗口外（已过期）→ 跳过
	seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		`{"name":"past","game_id":"g1","start_at":"2020-01-01T00:00:00Z","end_at":"2020-06-01T00:00:00Z"}`)
	// 触发非法（type=refund）→ trigger-invalid
	seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		`{"name":"trg","game_id":"g1","start_at":"2026-01-01T00:00:00Z","end_at":"2026-12-31T00:00:00Z","server_ids":[1],"audience":{"type":"all"},"trigger":{"type":"refund"},"rewards":{"type":"multiplier","mode":"total","rate_bp":15000}}`)
	// limits 非法（负数）→ limits-invalid
	seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		snapCfg("g1", `{"type":"multiplier","mode":"total","rate_bp":15000}`, `"per_user_count":-1,"global_count":0,"budget_cents":0`))
	// 三类规格非法 → continue（multiplier rate_bp 非整数 / 首充 extra 0 / 累充空档）
	seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		snapCfg("g1", `{"type":"multiplier","mode":"total","rate_bp":"x"}`, ""))
	seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		snapCfg("g1", `{"type":"first_purchase","extra_quantity":0}`, ""))
	seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		snapCfg("g1", `{"type":"cumulative_recharge","thresholds":[]}`, ""))

	resp := evalOrder(t, l, baseOrder())
	if len(resp.Grants) != 0 {
		t.Fatalf("window/target grants = %+v", resp.Grants)
	}
	reasons := map[string]bool{}
	for _, s := range resp.Skipped {
		reasons[s.Reason] = true
	}
	if !reasons["trigger-invalid"] || !reasons["limits-invalid"] {
		t.Fatalf("window/target skipped = %+v", resp.Skipped)
	}

	// 首充订单 → first_purchase 规格非法 continue 臂
	fp := baseOrder()
	fp.OrderId = "o-fp"
	fp.FirstPurchase = true
	if resp = evalOrder(t, l, fp); len(resp.Grants) != 0 {
		t.Fatalf("fp bad-spec grants = %+v", resp.Grants)
	}

	// 实付低于触发门槛 → 直接跳过（无 skipped 明细，仅无 grant）
	below := baseOrder()
	below.PaidCents = 500
	below.OrderId = "o-below"
	resp = evalOrder(t, l, below)
	if len(resp.Grants) != 0 {
		t.Fatalf("below-min grants = %+v", resp.Grants)
	}
}

func TestRewardEvaluate_RevokeArms(t *testing.T) {
	env := newLogicEnv(t, "")
	env.withRewardSecret(t, rewardSecret)
	l := NewRewardLogic(context.Background(), env.sc)

	seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		snapCfg("g1", `{"type":"cumulative_recharge","thresholds":[{"cents":1000,"extra_quantity":10},{"cents":2000,"extra_quantity":20}]}`, ""))
	// 非法快照（撤销通道解析失败 continue）+ 高门槛单档累充（预置越界档位）
	seedPublishedActivity(t, env.sc.ActivityDB, "g1", "not-json")
	actC := seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		snapCfg("g1", `{"type":"cumulative_recharge","thresholds":[{"cents":5000,"extra_quantity":30}]}`, ""))
	preGrant(t, l, actC, "pre-c", "7", 2000, model.RuleTypeCumulative, 5)

	// 先发 milestone0（净额 1500 只达一档）
	hi := baseOrder()
	hi.OrderId = "o-hi"
	hi.CumulativeCents = 1500
	resp := evalOrder(t, l, hi)
	if len(resp.Grants) != 1 || resp.Grants[0].Milestone != 0 {
		t.Fatalf("revoke seed grants = %+v", resp.Grants)
	}
	// 退款：净额跌到 500 < 1000 → 撤销已发档位
	lo := baseOrder()
	lo.OrderId = "o-lo"
	lo.CumulativeCents = 500
	resp = evalOrder(t, l, lo)
	if len(resp.Revoked) != 1 || resp.Revoked[0].Status != model.RewardStatusRevoked {
		t.Fatalf("revoked = %+v", resp.Revoked)
	}
	// 撤销后可再补发（净额回升 1500，milestone0 无 granted 行）
	mid := baseOrder()
	mid.OrderId = "o-mid"
	mid.CumulativeCents = 1500
	resp = evalOrder(t, l, mid)
	if len(resp.Grants) != 1 || resp.Grants[0].Milestone != 0 {
		t.Fatalf("re-grant = %+v", resp.Grants)
	}
}

func TestRewardEvaluate_DBErrorArms(t *testing.T) {
	// ListPublishedSnapshots 失败 → 500（DROP activities）
	env := newLogicEnv(t, "")
	env.withRewardSecret(t, rewardSecret)
	l := NewRewardLogic(context.Background(), env.sc)
	if _, err := env.db.Exec(`DROP TABLE activities`); err != nil {
		t.Fatalf("drop activities: %v", err)
	}
	body, sig := signReward(t, rewardSecret, baseOrder())
	if _, err := l.Evaluate(body, sig); err == nil {
		t.Fatal("drop activities = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}

	// ListGrants 失败 → 500（保留 published 快照，DROP reward_grants）
	env2 := newLogicEnv(t, "")
	env2.withRewardSecret(t, rewardSecret)
	l2 := NewRewardLogic(context.Background(), env2.sc)
	seedPublishedActivity(t, env2.sc.ActivityDB, "g1",
		snapCfg("g1", `{"type":"multiplier","mode":"total","rate_bp":15000}`, ""))
	if _, err := env2.db.Exec(`DROP TABLE reward_grants`); err != nil {
		t.Fatalf("drop reward_grants: %v", err)
	}
	body, sig = signReward(t, rewardSecret, baseOrder())
	if _, err := l2.Evaluate(body, sig); err == nil {
		t.Fatal("drop reward_grants = nil err")
	} else {
		mustAPIError(t, err, http.StatusInternalServerError)
	}
}

func TestRewardEvaluate_EmitArms(t *testing.T) {
	// 数据面板启用（httptest 接收）→ 发事实
	env := newLogicEnv(t, "")
	env.withRewardSecret(t, rewardSecret)
	panel := httptestOK(t)
	env.sc.AnalyticsSvc = analytics.NewClient(panel, "tok", 1000, nil)
	l := NewRewardLogic(context.Background(), env.sc)
	seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		snapCfg("g1", `{"type":"multiplier","mode":"total","rate_bp":15000}`, ""))
	resp := evalOrder(t, l, baseOrder())
	if len(resp.Grants) != 1 {
		t.Fatalf("emit grants = %+v", resp.Grants)
	}

	// 未启用（no-op）→ 不发事实，评估不受影响
	env2 := newLogicEnv(t, "")
	env2.withRewardSecret(t, rewardSecret)
	env2.sc.AnalyticsSvc = analytics.NewClient("", "tok", 1000, nil)
	l2 := NewRewardLogic(context.Background(), env2.sc)
	seedPublishedActivity(t, env2.sc.ActivityDB, "g1",
		snapCfg("g1", `{"type":"multiplier","mode":"total","rate_bp":15000}`, ""))
	resp = evalOrder(t, l2, baseOrder())
	if len(resp.Grants) != 1 {
		t.Fatalf("noop grants = %+v", resp.Grants)
	}
}

func TestRewardListGrants_Arms(t *testing.T) {
	env := newLogicEnv(t, "")
	env.withCatalog(t, "operator", http.StatusOK)
	l := NewRewardLogic(adminCtx(7, "1.2.3.4"), env.sc)
	const auth = "Bearer t"

	// 角色 502/403 臂
	env.sc.Config.GameCatalog.BaseURL = "http://127.0.0.1:1"
	_, err := l.ListGrants(&types.RewardGrantListRequest{}, auth)
	mustAPIError(t, err, http.StatusBadGateway)
	env.withCatalog(t, "player", http.StatusOK)
	_, err = l.ListGrants(&types.RewardGrantListRequest{}, auth)
	mustAPIError(t, err, http.StatusForbidden)
	env.withCatalog(t, "operator", http.StatusOK)

	// 查无（空列表）→ 成功
	list, err := l.ListGrants(&types.RewardGrantListRequest{}, auth)
	if err != nil || len(list.Grants) != 0 {
		t.Fatalf("empty list = %+v/%v", list, err)
	}

	// 有账本行 → 过滤返回
	seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		snapCfg("g1", `{"type":"multiplier","mode":"total","rate_bp":15000}`, ""))
	if _, err := l.svcCtx.RewardDB.InsertGrant(&model.RewardGrant{
		OrderId: "o-g", ActivityId: 1, ActivityVersion: 1, GameId: "g1",
		AccountId: "7", PaidCents: 2000, BaseQuantity: 10000, ExtraQuantity: 5000,
		TotalQuantity: 15000, RuleType: model.RuleTypeMultiplier,
		Status: model.RewardStatusGranted, Snapshot: "{}", GrantedAt: "2026-06-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("insert grant: %v", err)
	}
	list, err = l.ListGrants(&types.RewardGrantListRequest{OrderId: "o-g"}, auth)
	if err != nil || len(list.Grants) != 1 {
		t.Fatalf("grant list = %+v/%v", list, err)
	}

	// 账本查询错误 → 500（DROP reward_grants）
	if _, err := env.db.Exec(`DROP TABLE reward_grants`); err != nil {
		t.Fatalf("drop reward_grants: %v", err)
	}
	_, err = l.ListGrants(&types.RewardGrantListRequest{}, auth)
	mustAPIError(t, err, http.StatusInternalServerError)
}
