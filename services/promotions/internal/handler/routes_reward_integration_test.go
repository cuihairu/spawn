package handler

// GP-302 倍率赠送集成测试：total/extra 双语义计算、叠加裁决（非叠加独跑 /
// 同组互斥 / 跨组叠加）、限次消费、幂等重放不重复发放、版本快照运行时
// 引用（改草稿不影响运行中版本）、S2S 验签 fail-closed、账本与审计。

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/promotions/utils"
)

// publishActivity 建草稿 → 发布（生成版本快照），返回活动 id。
func publishActivity(t *testing.T, env *testActivityEnv, admin, body string) int64 {
	t.Helper()
	code, resp := do(t, http.MethodPost, env.base+"/admin/activities", admin, body)
	if code != http.StatusOK {
		t.Fatalf("create activity = %d body=%s", code, resp)
	}
	var created struct {
		Activity struct {
			Id int64 `json:"id"`
		} `json:"activity"`
	}
	if err := json.Unmarshal([]byte(resp), &created); err != nil || created.Activity.Id == 0 {
		t.Fatalf("parse create: %v body=%s", err, resp)
	}
	code, resp = do(t, http.MethodPost, fmt.Sprintf("%s/admin/activities/%d/publish", env.base, created.Activity.Id), admin, "")
	if code != http.StatusOK {
		t.Fatalf("publish = %d body=%s", code, resp)
	}
	return created.Activity.Id
}

func evalBody(orderID string, base, paid int64) string {
	return fmt.Sprintf(`{
		"order_id":%q,"game_id":"game-a","server_id":1,"role_id":"role-9",
		"account_id":"u1","sku_id":"sku-gem-100","product_id":"gem-100",
		"channel":"sandbox","paid_cents":%d,"currency":"CNY",
		"paid_at":"2026-11-09T12:00:00+08:00","base_quantity":%d
	}`, orderID, paid, base)
}

// evaluate 带 HMAC 签名调用 S2S 评估，返回状态码与响应体。
func evaluate(t *testing.T, env *testActivityEnv, body, signature string) (int, string) {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(testRewardSecret))
	mac.Write([]byte(body))
	sig := hex.EncodeToString(mac.Sum(nil))
	if signature != "" {
		sig = signature // 覆盖为指定签名（负向用例）
	}
	req, _ := http.NewRequest(http.MethodPost, env.base+"/internal/reward-evaluations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Reward-Signature", sig)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	defer resp.Body.Close()
	raw := readAllBody(t, resp)
	return resp.StatusCode, raw
}

type evalResp struct {
	OrderId string `json:"order_id"`
	Replay  bool   `json:"replay"`
	Grants  []struct {
		ActivityId      int64           `json:"activity_id"`
		ActivityVersion int64           `json:"activity_version"`
		BaseQuantity    int64           `json:"base_quantity"`
		ExtraQuantity   int64           `json:"extra_quantity"`
		TotalQuantity   int64           `json:"total_quantity"`
		RateMode        string          `json:"rate_mode"`
		RateBp          int64           `json:"rate_bp"`
		Status          string          `json:"status"`
		Snapshot        json.RawMessage `json:"snapshot"`
	} `json:"grants"`
	Skipped []struct {
		ActivityId int64  `json:"activity_id"`
		Reason     string `json:"reason"`
	} `json:"skipped"`
}

func TestRoutes_MultiplierRewards(t *testing.T) {
	env := newActivityServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})

	// 活动布局（游戏 game-a、区服 1、渠道 sandbox、触发满 1000 分）：
	//   C：非叠加（stackable=false）总倍率 30000（=3 倍总量，额外 +200%）
	//   A：组 g1 总倍率 15000（=1.5 倍总量，额外 +50%），per_user_count=1
	//   B：组 g2 额外倍率 5000（加赠 50%），无限次
	idC := publishActivity(t, env, admin, `{
		"name":"三倍庆","game_id":"game-a","timezone":"Asia/Shanghai",
		"start_at":"2026-11-01T00:00:00+08:00","end_at":"2026-11-30T23:59:59+08:00",
		"server_ids":[1],"channel":"sandbox","audience":{"type":"all"},
		"trigger":{"type":"order","min_amount_cents":1000},
		"stackable":false,"rewards":{"type":"multiplier","mode":"total","rate_bp":30000}
	}`)
	idA := publishActivity(t, env, admin, `{
		"name":"一倍半","game_id":"game-a","timezone":"Asia/Shanghai",
		"start_at":"2026-11-01T00:00:00+08:00","end_at":"2026-11-30T23:59:59+08:00",
		"server_ids":[1],"channel":"sandbox","audience":{"type":"all"},
		"trigger":{"type":"order","min_amount_cents":1000},
		"limits":{"per_user_count":1},
		"stackable":true,"stack_group":"g1",
		"rewards":{"type":"multiplier","mode":"total","rate_bp":15000}
	}`)
	idB := publishActivity(t, env, admin, `{
		"name":"加赠半","game_id":"game-a","timezone":"Asia/Shanghai",
		"start_at":"2026-11-01T00:00:00+08:00","end_at":"2026-11-30T23:59:59+08:00",
		"server_ids":[1],"channel":"sandbox","audience":{"type":"all"},
		"trigger":{"type":"order","min_amount_cents":1000},
		"stackable":true,"stack_group":"g2",
		"rewards":{"type":"multiplier","mode":"extra","rate_bp":5000}
	}`)
	if idC == 0 || idA == 0 || idB == 0 || idA == idB || idA == idC {
		t.Fatalf("bad activity ids: C=%d A=%d B=%d", idC, idA, idB)
	}

	// 1. 非叠加独跑：C 命中 → 只发 C（额外 200，总量 300）；A/B 被排除
	code, body := evaluate(t, env, evalBody("order-1", 100, 1000), "")
	if code != http.StatusOK {
		t.Fatalf("evaluate order-1 = %d body=%s", code, body)
	}
	var r1 evalResp
	_ = json.Unmarshal([]byte(body), &r1)
	if len(r1.Grants) != 1 || r1.Grants[0].ActivityId != idC ||
		r1.Grants[0].ExtraQuantity != 200 || r1.Grants[0].TotalQuantity != 300 ||
		r1.Grants[0].RateMode != "total" || r1.Grants[0].RateBp != 30000 ||
		r1.Grants[0].ActivityVersion != 1 || r1.Grants[0].Status != "granted" {
		t.Fatalf("order-1 grants = %+v body=%s", r1.Grants, body)
	}
	if len(r1.Skipped) != 2 {
		t.Fatalf("order-1 skipped = %+v", r1.Skipped)
	}
	var snap map[string]any
	if err := json.Unmarshal(r1.Grants[0].Snapshot, &snap); err != nil || snap["rule"] == nil || snap["order"] == nil {
		t.Fatalf("order-1 snapshot = %s err=%v", r1.Grants[0].Snapshot, err)
	}

	// 2. C 停用 → A/B 跨组叠加：A 额外 50（total 1.5x）、B 额外 50（extra 50%）
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/admin/activities/%d/disable", env.base, idC), admin, "")
	if code != http.StatusOK {
		t.Fatalf("disable C = %d", code)
	}
	code, body = evaluate(t, env, evalBody("order-2", 100, 1000), "")
	if code != http.StatusOK {
		t.Fatalf("evaluate order-2 = %d body=%s", code, body)
	}
	var r2 evalResp
	_ = json.Unmarshal([]byte(body), &r2)
	if len(r2.Grants) != 2 {
		t.Fatalf("order-2 grants = %+v body=%s", r2.Grants, body)
	}

	// 3. 幂等重放：同订单再评估 → replay=true、不新增账本
	code, body = evaluate(t, env, evalBody("order-2", 100, 1000), "")
	if code != http.StatusOK {
		t.Fatalf("replay = %d body=%s", code, body)
	}
	var r3 evalResp
	_ = json.Unmarshal([]byte(body), &r3)
	if !r3.Replay || len(r3.Grants) != 2 {
		t.Fatalf("replay = %+v", r3)
	}
	code, body = do(t, http.MethodGet, env.base+"/admin/reward-grants?order_id=order-2", admin, "")
	var list struct {
		Grants []json.RawMessage `json:"grants"`
	}
	_ = json.Unmarshal([]byte(body), &list)
	if code != http.StatusOK || len(list.Grants) != 2 {
		t.Fatalf("grants list order-2 = %d count=%d body=%s", code, len(list.Grants), body)
	}

	// 4. 限次：u1 第二单 A 命中但 per_user_count=1 → skipped；B 无限次仍发
	code, body = evaluate(t, env, evalBody("order-3", 100, 1000), "")
	if code != http.StatusOK {
		t.Fatalf("evaluate order-3 = %d body=%s", code, body)
	}
	var r4 evalResp
	_ = json.Unmarshal([]byte(body), &r4)
	if len(r4.Grants) != 1 || r4.Grants[0].ActivityId != idB {
		t.Fatalf("order-3 grants = %+v body=%s", r4.Grants, body)
	}
	limitHit := false
	for _, s := range r4.Skipped {
		if s.ActivityId == idA && s.Reason == "per-user-limit-reached" {
			limitHit = true
		}
	}
	if !limitHit {
		t.Fatalf("order-3 skipped = %+v", r4.Skipped)
	}

	// 5. 版本快照运行时引用：revise A 改配置（不发布）→ 运行中版本不变
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/admin/activities/%d/revise", env.base, idA), admin, "")
	if code != http.StatusOK {
		t.Fatalf("revise A = %d", code)
	}
	edited := `{
		"name":"一倍半","game_id":"game-a","timezone":"Asia/Shanghai",
		"start_at":"2026-11-01T00:00:00+08:00","end_at":"2026-11-30T23:59:59+08:00",
		"server_ids":[1],"channel":"sandbox","audience":{"type":"all"},
		"trigger":{"type":"order","min_amount_cents":1000},
		"limits":{"per_user_count":1},
		"stackable":true,"stack_group":"g1",
		"rewards":{"type":"multiplier","mode":"total","rate_bp":40000}
	}`
	code, _ = do(t, http.MethodPut, fmt.Sprintf("%s/admin/activities/%d", env.base, idA), admin, edited)
	if code != http.StatusOK {
		t.Fatalf("edit A draft = %d", code)
	}
	// u2 未领过 A：评估应仍按 v1（15000 → 额外 50），不是草稿的 40000
	code, body = evaluate(t, env, strings.Replace(evalBody("order-4", 100, 1000), `"u1"`, `"u2"`, 1), "")
	if code != http.StatusOK {
		t.Fatalf("evaluate order-4 = %d body=%s", code, body)
	}
	var r5 evalResp
	_ = json.Unmarshal([]byte(body), &r5)
	for _, g := range r5.Grants {
		if g.ActivityId == idA && (g.ExtraQuantity != 50 || g.ActivityVersion != 1) {
			t.Fatalf("order-4 A grant used draft config: %+v", g)
		}
	}

	// 6. 发布 v2 后新订单按新版本（40000 → 额外 300），旧账本不变
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/admin/activities/%d/publish", env.base, idA), admin, "")
	if code != http.StatusOK {
		t.Fatalf("publish A v2 = %d", code)
	}
	code, body = evaluate(t, env, strings.Replace(evalBody("order-5", 100, 1000), `"u1"`, `"u3"`, 1), "")
	if code != http.StatusOK {
		t.Fatalf("evaluate order-5 = %d body=%s", code, body)
	}
	var r6 evalResp
	_ = json.Unmarshal([]byte(body), &r6)
	aGot := false
	for _, g := range r6.Grants {
		if g.ActivityId == idA {
			aGot = true
			if g.ExtraQuantity != 300 || g.ActivityVersion != 2 {
				t.Fatalf("order-5 A grant = %+v", g)
			}
		}
	}
	if !aGot {
		t.Fatalf("order-5 missing A grant: %+v", r6.Grants)
	}

	// 7. 验签 fail-closed：错签名 403
	code, _ = evaluate(t, env, evalBody("order-bad", 100, 1000), "deadbeef")
	if code != http.StatusForbidden {
		t.Fatalf("bad signature = %d, want 403", code)
	}

	// 8. 不满足触发门槛（实付 < min_amount_cents）→ 无命中
	code, body = evaluate(t, env, evalBody("order-low", 100, 500), "")
	if code != http.StatusOK {
		t.Fatalf("evaluate order-low = %d", code)
	}
	var r8 evalResp
	_ = json.Unmarshal([]byte(body), &r8)
	if len(r8.Grants) != 0 {
		t.Fatalf("order-low grants = %+v", r8.Grants)
	}

	// 9. 玩家越权（管理面 403）+ 审计含 reward_evaluate
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})
	code, _ = do(t, http.MethodGet, env.base+"/admin/reward-grants", player, "")
	if code != http.StatusForbidden {
		t.Fatalf("player grants = %d, want 403", code)
	}
	code, body = do(t, http.MethodGet, env.base+"/admin/activity-audit", admin, "")
	if code != http.StatusOK || !strings.Contains(body, "reward_evaluate") || !strings.Contains(body, "reward_grant_list") {
		t.Fatalf("audit = %d body=%s", code, body)
	}
}

func readAllBody(t *testing.T, resp *http.Response) string {
	t.Helper()
	raw, _ := io.ReadAll(resp.Body)
	return string(raw)
}
