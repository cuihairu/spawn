package handler

// GP-301 活动快照运行时消费集成测试：玩家侧 GET /games/:gameId/activities
// （公开读，仅已发布+窗口内+全体人群，预算不下发）；游戏服侧 S2S
// GET /internal/games/:gameId/activities（X-Internal-Token fail-closed，
// 完整快照含人群/触发明细）。草稿/停用/窗口外/定向人群均不出现。

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/promotions/utils"
)

// runtimePublish 创建并发布一个活动，返回活动 id。
func runtimePublish(t *testing.T, env *testActivityEnv, body string) int64 {
	t.Helper()
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
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
		t.Fatalf("decode create: %v body=%s", err, resp)
	}
	code, resp = do(t, http.MethodPost, fmt.Sprintf(env.base+"/admin/activities/%d/publish", created.Activity.Id), admin, "")
	if code != http.StatusOK {
		t.Fatalf("publish activity = %d body=%s", code, resp)
	}
	return created.Activity.Id
}

func runtimeActivityBody(name, start, end, audience string) string {
	return fmt.Sprintf(`{
		"name":%q,"game_id":"game-a","timezone":"Asia/Shanghai",
		"start_at":%q,"end_at":%q,
		"server_ids":[1,2],"channel":"",
		"audience":{"type":%q},
		"trigger":{"type":"order","min_amount_cents":1000},
		"limits":{"per_user_count":3,"budget_cents":100000},
		"stackable":false,"stack_group":"g1",
		"rewards":{"type":"multiplier","mode":"total","rate_bp":15000}
	}`, name, start, end, audience)
}

func TestRoutes_RuntimeActivities(t *testing.T) {
	env := newActivityServer(t)
	now := time.Now()
	inStart := now.Add(-1 * time.Hour).Format("2006-01-02T15:04:05Z07:00")
	inEnd := now.Add(24 * time.Hour).Format("2006-01-02T15:04:05Z07:00")
	pastEnd := now.Add(-1 * time.Hour).Format("2006-01-02T15:04:05Z07:00")
	pastStart := now.Add(-48 * time.Hour).Format("2006-01-02T15:04:05Z07:00")

	// 1. 窗口内全体人群活动 + 窗口外活动 + 定向人群活动
	inID := runtimePublish(t, env, runtimeActivityBody("进行中活动", inStart, inEnd, "all"))
	runtimePublish(t, env, runtimeActivityBody("已结束活动", pastStart, pastEnd, "all"))
	runtimePublish(t, env, runtimeActivityBody("定向活动", inStart, inEnd, "new_user"))

	// 2. 草稿不出现（建而不发）
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	code, _ := do(t, http.MethodPost, env.base+"/admin/activities", admin, runtimeActivityBody("草稿活动", inStart, inEnd, "all"))
	if code != http.StatusOK {
		t.Fatalf("create draft = %d", code)
	}

	// 3. 玩家侧列表（公开读）：只见窗口内全体人群活动，预算不下发
	code, resp := do(t, http.MethodGet, env.base+"/games/game-a/activities", "", "")
	if code != http.StatusOK {
		t.Fatalf("player list = %d body=%s", code, resp)
	}
	var list struct {
		Items []struct {
			Id        int64          `json:"id"`
			Version   int64          `json:"version"`
			Name      string         `json:"name"`
			Rewards   map[string]any `json:"rewards"`
			Limits    map[string]any `json:"limits"`
			Stackable bool           `json:"stackable"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(resp), &list); err != nil {
		t.Fatalf("decode list: %v body=%s", err, resp)
	}
	if len(list.Items) != 1 || list.Items[0].Id != inID || list.Items[0].Name != "进行中活动" {
		t.Fatalf("player items = %+v, want only activity %d", list.Items, inID)
	}
	if list.Items[0].Version != 1 || !strings.Contains(resp, `"rate_bp":15000`) {
		t.Fatalf("snapshot facts missing: %s", resp)
	}
	if strings.Contains(resp, "budget_cents") || strings.Contains(resp, "100000") {
		t.Fatalf("budget leaked to player view: %s", resp)
	}
	if strings.Contains(resp, "定向活动") {
		t.Fatalf("targeted activity leaked: %s", resp)
	}

	// 4. 其他游戏不可见
	code, resp = do(t, http.MethodGet, env.base+"/games/game-zzz/activities", "", "")
	if code != http.StatusOK || !strings.Contains(resp, `"items":[]`) {
		t.Fatalf("other game list = %d body=%s", code, resp)
	}

	// 5. 游戏服侧 S2S：未配置恒 403；缺/错令牌 403
	code, resp = do(t, http.MethodGet, env.base+"/internal/games/game-a/activities", "", "")
	if code != http.StatusForbidden {
		t.Fatalf("internal unconfigured = %d, want 403", code)
	}
	env.svcCtx.Config.Internal.Token = "gp301-internal-token"
	code, _ = do(t, http.MethodGet, env.base+"/internal/games/game-a/activities", "", "")
	if code != http.StatusForbidden {
		t.Fatalf("internal missing token = %d, want 403", code)
	}
	req, _ := http.NewRequest(http.MethodGet, env.base+"/internal/games/game-a/activities", nil)
	req.Header.Set("X-Internal-Token", "wrong-token")
	httpResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("internal query: %v", err)
	}
	_ = httpResp.Body.Close()
	if httpResp.StatusCode != http.StatusForbidden {
		t.Fatalf("internal wrong token = %d, want 403", httpResp.StatusCode)
	}

	// 6. 带令牌：完整快照（窗口内全体+定向都在；草稿/窗口外不在；预算保留）
	req, _ = http.NewRequest(http.MethodGet, env.base+"/internal/games/game-a/activities", nil)
	req.Header.Set("X-Internal-Token", "gp301-internal-token")
	httpResp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("internal query: %v", err)
	}
	raw := readAllBody(t, httpResp)
	_ = httpResp.Body.Close()
	var snaps struct {
		Items []struct {
			ActivityId int64           `json:"activity_id"`
			Version    int64           `json:"version"`
			Snapshot   json.RawMessage `json:"snapshot"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(raw), &snaps); err != nil {
		t.Fatalf("decode snapshots: %v body=%s", err, raw)
	}
	if len(snaps.Items) != 2 {
		t.Fatalf("internal items = %d, want 2 (in-window all + targeted)", len(snaps.Items))
	}
	body := string(raw)
	if !strings.Contains(body, `"type":"new_user"`) || !strings.Contains(body, "budget_cents") {
		t.Fatalf("internal snapshot lacks full config: %s", body)
	}
	if strings.Contains(body, "已结束活动") || strings.Contains(body, "草稿活动") {
		t.Fatalf("out-of-window or draft leaked: %s", body)
	}
	for _, it := range snaps.Items {
		if it.ActivityId == inID && it.Version != 1 {
			t.Fatalf("snapshot version = %d, want 1", it.Version)
		}
	}
}
