package handler

// GP-303 回流规则集成测试：回流判定源 S2S 上报（POST /internal/player-activity，
// X-Internal-Token fail-closed，活跃事实条件推进）+ 回流人群评估（阈值按活动
// 配置 audience.returning_threshold_days，曾活跃且缺席 >= 阈值 → 命中；无历史/
// 近期活跃 → fail-closed 不命中）+ 草稿阈值校验。

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/promotions/utils"
)

const testInternalToken = "gp303-internal-token"

// reportActivity 带 X-Internal-Token 上报账号×游戏活跃事实。
func reportActivity(t *testing.T, env *testActivityEnv, token, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, env.base+"/internal/player-activity", strings.NewReader(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Internal-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("report activity: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, readAllBody(t, resp)
}

func TestRoutes_PlayerActivityReport(t *testing.T) {
	env := newActivityServer(t)

	// fail-closed：未配置令牌恒 403（即使带了令牌）
	code, body := reportActivity(t, env, testInternalToken, `{"account_id":"u1","game_id":"g1"}`)
	if code != http.StatusForbidden {
		t.Fatalf("未配置令牌 status = %d body=%s, want 403", code, body)
	}
	env.svcCtx.Config.Internal.Token = testInternalToken

	// 令牌不匹配 403
	code, body = reportActivity(t, env, "wrong-token", `{"account_id":"u1","game_id":"g1"}`)
	if code != http.StatusForbidden {
		t.Fatalf("错令牌 status = %d body=%s, want 403", code, body)
	}

	// 合法上报 200（active_at 显式给定）
	code, body = reportActivity(t, env, testInternalToken,
		`{"account_id":"u1","game_id":"g1","active_at":"2026-11-22T08:00:00Z"}`)
	if code != http.StatusOK || !strings.Contains(body, `"recorded":true`) {
		t.Fatalf("合法上报 status = %d body=%s, want 200 recorded", code, body)
	}

	// 缺字段 / 坏格式 400
	for _, bad := range []string{
		`{"game_id":"g1"}`,
		`{"account_id":"u1"}`,
		`{"account_id":"u1","game_id":"g1","active_at":"not-a-time"}`,
		`{"account_id":"u1","game_id":"g1","active_at":12345}`,
		`{`,
	} {
		code, body = reportActivity(t, env, testInternalToken, bad)
		if code != http.StatusBadRequest {
			t.Fatalf("非法体 %s status = %d body=%s, want 400", bad, code, body)
		}
	}
}

func returningActivityBody(gameID string, audience string) string {
	now := time.Now().UTC()
	return fmt.Sprintf(`{
		"name":"回流礼","game_id":%q,"timezone":"Asia/Shanghai",
		"start_at":%q,"end_at":%q,
		"server_ids":[1],"channel":"sandbox",
		"audience":%s,
		"trigger":{"type":"order","min_amount_cents":0},
		"limits":{"per_user_count":1},
		"stackable":true,
		"rewards":{"type":"multiplier","mode":"extra","rate_bp":5000}
	}`, gameID,
		now.Add(-time.Hour).Format("2006-01-02T15:04:05Z07:00"),
		now.Add(24*time.Hour).Format("2006-01-02T15:04:05Z07:00"), audience)
}

func TestRoutes_ReturningAudience(t *testing.T) {
	env := newActivityServer(t)
	env.svcCtx.Config.Internal.Token = testInternalToken
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})

	// 草稿阈值校验：越界 400（0 / 3651 / 非数字）
	for _, aud := range []string{
		`{"type":"returning","returning_threshold_days":0}`,
		`{"type":"returning","returning_threshold_days":3651}`,
		`{"type":"returning","returning_threshold_days":"soon"}`,
	} {
		code, body := do(t, http.MethodPost, env.base+"/admin/activities", admin, returningActivityBody("game-a", aud))
		if code != http.StatusBadRequest {
			t.Fatalf("非法阈值 %s status = %d body=%s, want 400", aud, code, body)
		}
	}

	// 发布回流活动：game-a 阈值 30 天（加赠 50%）；game-b 阈值 5 天（运营按活动覆盖）
	idA := publishActivity(t, env, admin, returningActivityBody("game-a",
		`{"type":"returning","returning_threshold_days":30}`))
	if idA == 0 {
		t.Fatal("publish returning activity game-a failed")
	}
	idB := publishActivity(t, env, admin, returningActivityBody("game-b",
		`{"type":"returning","returning_threshold_days":5}`))
	if idB == 0 {
		t.Fatal("publish returning activity game-b failed")
	}
	now := time.Now().UTC()
	t40 := now.Add(-40 * 24 * time.Hour).Format(time.RFC3339Nano)
	t2 := now.Add(-2 * 24 * time.Hour).Format(time.RFC3339Nano)

	// 回流账号：40 天前活跃 → 现在活跃（previous=40 天前）
	code, body := reportActivity(t, env, testInternalToken,
		fmt.Sprintf(`{"account_id":"ret-1","game_id":"game-a","active_at":%q}`, t40))
	if code != http.StatusOK {
		t.Fatalf("report t40 status = %d body=%s", code, body)
	}
	code, body = reportActivity(t, env, testInternalToken,
		`{"account_id":"ret-1","game_id":"game-a"}`)
	if code != http.StatusOK {
		t.Fatalf("report now status = %d body=%s", code, body)
	}
	// 近期账号：2 天前活跃 → 现在活跃（previous=2 天前）
	code, body = reportActivity(t, env, testInternalToken,
		fmt.Sprintf(`{"account_id":"recent-1","game_id":"game-a","active_at":%q}`, t2))
	if code != http.StatusOK {
		t.Fatalf("report t2 status = %d body=%s", code, body)
	}
	code, body = reportActivity(t, env, testInternalToken,
		`{"account_id":"recent-1","game_id":"game-a"}`)
	if code != http.StatusOK {
		t.Fatalf("report now recent status = %d body=%s", code, body)
	}

	// 回流账号在 game-a 下单（阈值 30，缺席 40 天）→ 命中并加赠 50%
	code, body = evalBodySigned(t, env, returningEvalBody("ord-ret-1", "game-a", "ret-1", 100))
	if code != http.StatusOK {
		t.Fatalf("evaluate returning status = %d body=%s", code, body)
	}
	var resp gp303Resp
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode evaluate: %v body=%s", err, body)
	}
	if len(resp.Grants) != 1 || resp.Grants[0].ActivityId != idA {
		t.Fatalf("grants = %+v, want 1 条活动 %d（缺席 40 天 >= 阈值 30 天）", resp.Grants, idA)
	}
	if resp.Grants[0].ExtraQuantity != 50 {
		t.Fatalf("extra = %d want 50（加赠 50%% of 100）", resp.Grants[0].ExtraQuantity)
	}
	if len(resp.Skipped) != 0 {
		t.Fatalf("skipped = %+v, want 空", resp.Skipped)
	}

	// 近期活跃账号（缺席 2 天 < 30 天）→ 不命中、无发放
	code, body = evalBodySigned(t, env, returningEvalBody("ord-recent-1", "game-a", "recent-1", 100))
	if code != http.StatusOK {
		t.Fatalf("evaluate recent status = %d body=%s", code, body)
	}
	resp = gp303Resp{}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode evaluate: %v body=%s", err, body)
	}
	if len(resp.Grants) != 0 {
		t.Fatalf("近期活跃 grants = %+v, want 空（缺席 2 天 < 30 天）", resp.Grants)
	}

	// 无活跃历史账号 → fail-closed 不命中
	code, body = evalBodySigned(t, env, returningEvalBody("ord-fresh-1", "game-a", "fresh-1", 100))
	if code != http.StatusOK {
		t.Fatalf("evaluate fresh status = %d body=%s", code, body)
	}
	resp = gp303Resp{}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode evaluate: %v body=%s", err, body)
	}
	if len(resp.Grants) != 0 {
		t.Fatalf("无历史 grants = %+v, want 空（fail-closed）", resp.Grants)
	}

	// 阈值按活动覆盖：game-b 阈值 5 天——缺席 40 天同样命中，缺席 2 天仍不命中
	_, _ = reportActivity(t, env, testInternalToken,
		fmt.Sprintf(`{"account_id":"ret-1","game_id":"game-b","active_at":%q}`, t40))
	code, body = reportActivity(t, env, testInternalToken,
		`{"account_id":"ret-1","game_id":"game-b"}`)
	if code != http.StatusOK {
		t.Fatalf("report game-b status = %d body=%s", code, body)
	}
	code, body = evalBodySigned(t, env, returningEvalBody("ord-ret-b", "game-b", "ret-1", 100))
	if code != http.StatusOK {
		t.Fatalf("evaluate game-b status = %d body=%s", code, body)
	}
	resp = gp303Resp{}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode evaluate: %v body=%s", err, body)
	}
	if len(resp.Grants) != 1 || resp.Grants[0].ActivityId != idB {
		t.Fatalf("game-b grants = %+v, want 活动 %d（阈值 5 天口径覆盖）", resp.Grants, idB)
	}

	// 同一活动重放不重复发放
	code, body = evalBodySigned(t, env, returningEvalBody("ord-ret-1", "game-a", "ret-1", 100))
	if code != http.StatusOK {
		t.Fatalf("replay evaluate status = %d body=%s", code, body)
	}
	resp = gp303Resp{}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode replay: %v body=%s", err, body)
	}
	if !resp.Replay || len(resp.Grants) != 1 {
		t.Fatalf("replay = %+v, want replay 且返回既有发放事实", resp)
	}
}

func returningEvalBody(orderID, gameID, account string, base int64) string {
	return fmt.Sprintf(`{
		"order_id":%q,"game_id":%q,"server_id":1,"role_id":"role-9",
		"account_id":%q,"sku_id":"sku-gem-100","product_id":"gem-100",
		"channel":"sandbox","paid_cents":1000,"currency":"CNY",
		"paid_at":%q,"base_quantity":%d
	}`, orderID, gameID, account,
		time.Now().UTC().Format("2006-01-02T15:04:05Z07:00"), base)
}

// evalBodySigned 带 HMAC 签名调用 S2S 评估（与 GP-303 其他用例同款密钥）。
func evalBodySigned(t *testing.T, env *testActivityEnv, body string) (int, string) {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(testRewardSecret))
	mac.Write([]byte(body))
	req, _ := http.NewRequest(http.MethodPost, env.base+"/internal/reward-evaluations", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Reward-Signature", hex.EncodeToString(mac.Sum(nil)))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode, readAllBody(t, resp)
}
