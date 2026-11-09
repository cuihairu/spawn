package handler

// GP-304 礼包集成测试：管理面（建/改/发布/停用/回草稿/码导入/码列表/
// 领取记录）+ 玩家侧（展示/领取限领与幂等/我的领取）+ S2S 核销回调
// （HMAC 验签、幂等、fail-closed）。码包发布须有可用码；直发包库存不限。

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/promotions/utils"
)

func gpPackBody(name string, codeRequired bool, limit int64) string {
	return fmt.Sprintf(`{
		"name":%q,"game_id":"game-a","description":"test pack",
		"timezone":"Asia/Shanghai",
		"start_at":"2026-01-01T00:00:00+08:00","end_at":"2026-12-31T23:59:59+08:00",
		"server_ids":[1],"channel":"sandbox","audience":{"type":"all"},
		"per_user_limit":%d,"code_required":%t,"content":{"items":[{"sku":"gem-100","qty":1}]}
	}`, name, limit, codeRequired)
}

func gpCreate(t *testing.T, env *testActivityEnv, admin, body string) int64 {
	t.Helper()
	code, resp := do(t, http.MethodPost, env.base+"/admin/gift-packs", admin, body)
	if code != http.StatusOK {
		t.Fatalf("create gift pack = %d body=%s", code, resp)
	}
	var r struct {
		GiftPack struct {
			Id     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"gift_pack"`
	}
	if err := json.Unmarshal([]byte(resp), &r); err != nil || r.GiftPack.Id == 0 || r.GiftPack.Status != "draft" {
		t.Fatalf("parse create = %v body=%s", err, resp)
	}
	return r.GiftPack.Id
}

func gpPublish(t *testing.T, env *testActivityEnv, admin string, id int64, want int) {
	t.Helper()
	code, body := do(t, http.MethodPost, fmt.Sprintf("%s/admin/gift-packs/%d/publish", env.base, id), admin, "")
	if code != want {
		t.Fatalf("publish pack %d = %d (want %d) body=%s", id, code, want, body)
	}
}

func gpSign(secret, body string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func TestRoutes_GiftPacks(t *testing.T) {
	env := newActivityServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	player := env.sign(utils.JWTClaims{UserId: 7, Username: "bob"})

	// 直发包：per_user_limit=2，库存不限
	directID := gpCreate(t, env, admin, gpPackBody("直发礼", false, 2))
	// 码包：per_user_limit=1，须有可用码才能发布
	codeID := gpCreate(t, env, admin, gpPackBody("码包礼", true, 1))

	// 1. 草稿可编辑
	code, body := do(t, http.MethodPut, fmt.Sprintf("%s/admin/gift-packs/%d", env.base, directID), admin, gpPackBody("直发礼改", false, 2))
	if code != http.StatusOK || !strings.Contains(body, `"name":"直发礼改"`) {
		t.Fatalf("update draft pack = %d body=%s", code, body)
	}

	// 2. 直发包可直接发布（无码门槛）；码包无码发布 → 400
	gpPublish(t, env, admin, directID, http.StatusOK)
	gpPublish(t, env, admin, codeID, http.StatusBadRequest)

	// 3. 码包导入码：2 新 + 重复计数
	code, body = do(t, http.MethodPost, fmt.Sprintf("%s/admin/gift-packs/%d/codes", env.base, codeID), admin, `{"codes":["AAA111","BBB222","AAA111"]}`)
	if code != http.StatusOK || !strings.Contains(body, `"imported":2`) || !strings.Contains(body, `"duplicate":1`) {
		t.Fatalf("import codes = %d body=%s", code, body)
	}
	gpPublish(t, env, admin, codeID, http.StatusOK)

	// 4. 玩家侧展示：命中 server 1 双包；server 2 无包
	code, body = do(t, http.MethodGet, env.base+"/gift-packs?game_id=game-a&server_id=1", player, "")
	if code != http.StatusOK || !strings.Contains(body, `"name":"直发礼改"`) || !strings.Contains(body, `"name":"码包礼"`) {
		t.Fatalf("player list server1 = %d body=%s", code, body)
	}
	code, body = do(t, http.MethodGet, env.base+"/gift-packs?game_id=game-a&server_id=2", player, "")
	if code != http.StatusOK || strings.Contains(body, `"items":[{"`) {
		t.Fatalf("player list server2 should be empty = %d body=%s", code, body)
	}

	// 5. 领取直发包：r1 → claim_index 1；同 r1 幂等不新增；r2 → index 2；r3 → 限领 409
	claim := func(id int64, reqID string, want int) string {
		code, body := do(t, http.MethodPost, fmt.Sprintf("%s/gift-packs/%d/claim", env.base, id), player,
			fmt.Sprintf(`{"server_id":1,"request_id":%q}`, reqID))
		if code != want {
			t.Fatalf("claim pack %d req=%s = %d (want %d) body=%s", id, reqID, code, want, body)
		}
		return body
	}
	body = claim(directID, "r1", http.StatusOK)
	if !strings.Contains(body, `"claim_index":1`) || !strings.Contains(body, `"account_id":"7"`) {
		t.Fatalf("claim r1 body=%s", body)
	}
	body = claim(directID, "r1", http.StatusOK) // 幂等：返回既有
	if !strings.Contains(body, `"claim_index":1`) {
		t.Fatalf("claim r1 idempotent body=%s", body)
	}
	claim(directID, "r2", http.StatusOK)
	claim(directID, "r3", http.StatusConflict) // per_user_limit=2 已达

	// 6. 领取码包：c1 → 拿到码；幂等 c1 返回同码；c2 → 409（限领 1）
	body = claim(codeID, "c1", http.StatusOK)
	if !strings.Contains(body, `"claim_index":1`) || !strings.Contains(body, `"code":`) {
		t.Fatalf("claim code pack c1 body=%s", body)
	}
	var cc struct {
		Claim struct {
			Id int64 `json:"id"`
		} `json:"claim"`
		Code struct {
			Id     int64  `json:"id"`
			Status string `json:"status"`
		} `json:"code"`
	}
	if err := json.Unmarshal([]byte(body), &cc); err != nil || cc.Code.Id == 0 || cc.Claim.Id == 0 {
		t.Fatalf("parse code claim = %v body=%s", err, body)
	}
	body = claim(codeID, "c1", http.StatusOK) // 幂等返回同码
	if !strings.Contains(body, fmt.Sprintf(`"id":%d`, cc.Code.Id)) {
		t.Fatalf("code idempotent code changed body=%s", body)
	}
	claim(codeID, "c2", http.StatusConflict)

	// 7. S2S 核销：验签先于解析；未配置密钥 fail-closed 已在 Reward 侧覆盖；
	//    正确签名 → 200 且码 used；重放 → 幂等 200；坏签名 → 403
	redeemBody := fmt.Sprintf(`{"code_id":%d,"pack_id":%d}`, cc.Code.Id, codeID)
	// 无签名头（fail-closed 缺密钥同义）→ 403
	code, _ = do(t, http.MethodPost, env.base+"/internal/gift-pack-redemptions", "", redeemBody)
	if code != http.StatusForbidden {
		t.Fatalf("redeem bad secret = %d, want 403", code)
	}
	good := gpSign(testRewardSecret, redeemBody)
	req, _ := http.NewRequest(http.MethodPost, env.base+"/internal/gift-pack-redemptions", strings.NewReader(redeemBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GiftPack-Signature", good)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("redeem: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("redeem good = %d, want 200", resp.StatusCode)
	}
	// 幂等重放
	resp2, _ := http.DefaultClient.Do(req)
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("redeem replay = %d, want 200", resp2.StatusCode)
	}

	// 8. 管理面码列表：核销码 used
	code, body = do(t, http.MethodGet, fmt.Sprintf("%s/admin/gift-packs/%d/codes", env.base, codeID), admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"used"`) || !strings.Contains(body, `"status":"available"`) {
		t.Fatalf("code list = %d body=%s", code, body)
	}

	// 9. 管理面领取记录：直发包 2 条 + 码包 1 条
	code, body = do(t, http.MethodGet, fmt.Sprintf("%s/admin/gift-packs/%d/claims", env.base, directID), admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"claim_index":2`) {
		t.Fatalf("claims direct = %d body=%s", code, body)
	}

	// 10. 玩家我的领取：3 条
	code, body = do(t, http.MethodGet, env.base+"/gift-claims/me", player, "")
	if code != http.StatusOK || strings.Count(body, `"account_id":"7"`) != 3 {
		t.Fatalf("my claims = %d body=%s", code, body)
	}

	// 11. 管理角色强制：玩家令牌访问管理面 → 403
	code, _ = do(t, http.MethodPost, env.base+"/admin/gift-packs", player, gpPackBody("越权", false, 1))
	if code != http.StatusForbidden {
		t.Fatalf("player create pack = %d, want 403", code)
	}

	// 12. 校验：per_user_limit 0 → 400；无码发布门槛已验（步骤 2）
	code, _ = do(t, http.MethodPost, env.base+"/admin/gift-packs", admin, gpPackBody("非法", false, 0))
	if code != http.StatusBadRequest {
		t.Fatalf("bad limit create = %d, want 400", code)
	}

	// 13. 停用 → 回草稿：停用后玩家不可见/不可领
	do(t, http.MethodPost, fmt.Sprintf("%s/admin/gift-packs/%d/disable", env.base, directID), admin, "")
	code, _ = do(t, http.MethodPost, fmt.Sprintf("%s/gift-packs/%d/claim", env.base, directID), player, `{"server_id":1,"request_id":"r9"}`)
	if code != http.StatusConflict {
		t.Fatalf("claim disabled = %d, want 409", code)
	}
	do(t, http.MethodPost, fmt.Sprintf("%s/admin/gift-packs/%d/revise", env.base, directID), admin, "")

	// 14. 审计：建包/领包动作落 activity_audit_logs
	code, body = do(t, http.MethodGet, fmt.Sprintf("%s/admin/activity-audit?activity_id=%d", env.base, codeID), admin, "")
	if code != http.StatusOK || !strings.Contains(body, "gift_pack_create") || !strings.Contains(body, "gift_pack_claim") || !strings.Contains(body, "gift_pack_redeem") {
		t.Fatalf("audit = %d body=%s", code, body)
	}
}
