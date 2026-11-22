package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
)

// TestRoutes_PromoChannelAdminFlow GP-403 管理面闭环：角色守卫 → 登记 →
// 列表 → 变更（PATCH）→ 启停 → 效果计数 → 变更历史（audit_logs）。
func TestRoutes_PromoChannelAdminFlow(t *testing.T) {
	base, sign := newAuthedTestServer(t)
	admin := sign(claimsFor(1, "admin"))
	player := sign(claimsFor(7, "bob"))

	// 玩家访问管理面 → 403（单层渠道登记收 platform_admin/operator）
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/promo/channels", player, `{"code":"x","name":"X"}`); code != http.StatusForbidden {
		t.Fatalf("player POST /admin/promo/channels = %d, want 403", code)
	}

	// 登记：坏 code（字符白名单）→ 400
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/promo/channels", admin,
		`{"code":"空格 不行","name":"X"}`); code != http.StatusBadRequest {
		t.Fatalf("bad code = %d, want 400", code)
	}
	// 归因窗口越界 → 400
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/promo/channels", admin,
		`{"code":"w","name":"窗口越界","window_hours":9999}`); code != http.StatusBadRequest {
		t.Fatalf("window overflow = %d, want 400", code)
	}

	// 正常登记（window_hours 缺省 0 → 默认 72）
	code, body := doAuthed(t, http.MethodPost, base+"/admin/promo/channels", admin,
		`{"code":"kol-li","name":"主播李"}`)
	if code != http.StatusOK {
		t.Fatalf("create = %d body=%s", code, body)
	}
	var ch struct {
		Id     int64  `json:"id"`
		Code   string `json:"code"`
		Name   string `json:"name"`
		Window int64  `json:"attribution_window_hours"`
		Status string `json:"status"`
	}
	if err := json.Unmarshal([]byte(body), &ch); err != nil {
		t.Fatalf("unmarshal create: %v body=%s", err, body)
	}
	if ch.Id <= 0 || ch.Code != "kol-li" || ch.Window != 72 || ch.Status != "active" {
		t.Fatalf("created = %+v body=%s", ch, body)
	}

	// 重复 code → 409
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/promo/channels", admin,
		`{"code":"kol-li","name":"重复"}`); code != http.StatusConflict {
		t.Fatalf("duplicate code = %d, want 409", code)
	}

	// 列表
	code, body = doAuthed(t, http.MethodGet, base+"/admin/promo/channels?limit=10", admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"code":"kol-li"`) {
		t.Fatalf("list = %d body=%s", code, body)
	}

	// 变更：改名 + 改窗口
	code, body = doAuthed(t, http.MethodPatch,
		fmt.Sprintf("%s/admin/promo/channels/%d", base, ch.Id), admin,
		`{"name":"主播李（新）","window_hours":120}`)
	if code != http.StatusOK || !strings.Contains(body, `"attribution_window_hours":120`) {
		t.Fatalf("update = %d body=%s", code, body)
	}
	// 不存在的渠道 → 404
	if code, _ := doAuthed(t, http.MethodPatch, base+"/admin/promo/channels/999", admin,
		`{"name":"幽灵"}`); code != http.StatusNotFound {
		t.Fatalf("update missing = %d, want 404", code)
	}

	// 变更历史：audit_logs 里能看到 create 与 update（resource_type=promo_channel）
	code, body = doAuthed(t, http.MethodGet, base+"/admin/audit?resource_type=promo_channel&limit=10", admin, "")
	if code != http.StatusOK ||
		!strings.Contains(body, "promo_channel_create") ||
		!strings.Contains(body, "promo_channel_update") {
		t.Fatalf("audit = %d body=%s", code, body)
	}

	// 推广链接落地：记录点击（admin 代打，公开端点本来免登录，用裸请求覆盖）
	clickURL := base + "/promo/resolve?channel=kol-li&click_id=click-a"
	resp, err := http.Get(clickURL)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	raw := readAll(t, resp)
	// 上一步已把窗口 PATCH 到 120 小时，落地解析随之返回 120*3600 秒
	if resp.StatusCode != http.StatusOK || !strings.Contains(raw, `"click_id":"click-a"`) ||
		!strings.Contains(raw, `"window_secs":432000`) {
		t.Fatalf("resolve = %d body=%s", resp.StatusCode, raw)
	}
	// 幂等重放：同 click_id 再落地一次仍 200（窗口秒数一致，不重复计数）
	resp, err = http.Get(clickURL)
	if err != nil {
		t.Fatalf("resolve replay: %v", err)
	}
	raw = readAll(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(raw, `"click_id":"click-a"`) {
		t.Fatalf("replay = %d body=%s", resp.StatusCode, raw)
	}

	// 无效渠道 → 404；缺 channel → 400
	if resp, err := http.Get(base + "/promo/resolve?channel=ghost&click_id=c2"); err != nil || resp.StatusCode != http.StatusNotFound {
		if err == nil {
			_ = resp.Body.Close()
		}
		t.Fatalf("unknown channel = %v/%v, want 404", err, resp)
	}
	if resp, err := http.Get(base + "/promo/resolve"); err != nil || resp.StatusCode != http.StatusBadRequest {
		if err == nil {
			_ = resp.Body.Close()
		}
		t.Fatalf("missing channel = %v/%v, want 400", err, resp)
	}

	// 渠道包下载登记（恒 200，渠道无效不阻断）
	req, _ := http.NewRequest(http.MethodPost, base+"/promo/download-click?channel=kol-li&game_id=g-1", nil)
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusOK {
		if err == nil {
			_ = resp.Body.Close()
		}
		t.Fatalf("download-click = %v/%v, want 200", err, resp)
	}

	// 效果计数：点击 1（幂等重放不增）、下载 1、归因 0
	code, body = doAuthed(t, http.MethodGet, base+"/admin/promo/channels/kol-li/stats", admin, "")
	if code != http.StatusOK ||
		!strings.Contains(body, `"clicks":1`) ||
		!strings.Contains(body, `"downloads":1`) ||
		!strings.Contains(body, `"attributed_users":0`) {
		t.Fatalf("stats = %d body=%s", code, body)
	}

	// 停用渠道：链接不再计点击（resolve 404），存量点击计数不受影响
	if code, _ := doAuthed(t, http.MethodPost,
		fmt.Sprintf("%s/admin/promo/channels/%d/status", base, ch.Id), admin,
		`{"status":"disabled"}`); code != http.StatusOK {
		t.Fatalf("disable = %d, want 200", code)
	}
	if resp, err := http.Get(base + "/promo/resolve?channel=kol-li&click_id=click-b"); err != nil || resp.StatusCode != http.StatusNotFound {
		if err == nil {
			_ = resp.Body.Close()
		}
		t.Fatalf("resolve disabled = %v/%v, want 404", err, resp)
	}
	code, body = doAuthed(t, http.MethodGet, base+"/admin/promo/channels/kol-li/stats", admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"clicks":1`) {
		t.Fatalf("stats after disable = %d body=%s", code, body)
	}
	// 坏状态值 → 400
	if code, _ := doAuthed(t, http.MethodPost,
		fmt.Sprintf("%s/admin/promo/channels/%d/status", base, ch.Id), admin,
		`{"status":"paused"}`); code != http.StatusBadRequest {
		t.Fatalf("bad status = %d, want 400", code)
	}
}

// TestRoutes_InternalPromoResolve GP-403 S2S 归因解析：X-Internal-Token 守卫
// （无/错令牌 403，fail-closed）；令牌匹配时按窗口判定归因并回写 user_id。
func TestRoutes_InternalPromoResolve(t *testing.T) {
	const s2sToken = "promo-s2s-token"
	base, sign := newAuthedTestServerConfig(t, func(c *config.Config) {
		c.Internal.Token = s2sToken
	})
	admin := sign(claimsFor(1, "admin"))

	// 准备渠道与点击
	if code, body := doAuthed(t, http.MethodPost, base+"/admin/promo/channels", admin,
		`{"code":"kol-li","name":"主播李"}`); code != http.StatusOK {
		t.Fatalf("create channel = %d body=%s", code, body)
	}
	linkResp, err := http.Get(base + "/promo/resolve?channel=kol-li&click_id=click-a")
	if err != nil {
		t.Fatalf("resolve link: %v", err)
	}
	if linkResp.StatusCode != http.StatusOK {
		t.Fatalf("resolve link = %d body=%s", linkResp.StatusCode, readAll(t, linkResp))
	}
	_ = linkResp.Body.Close()

	resolve := func(token, clickID, registeredAt string, userID int64) (int, string) {
		payload := map[string]any{
			"click_id":      clickID,
			"registered_at": registeredAt,
			"user_id":       userID,
		}
		buf, _ := json.Marshal(payload)
		req, _ := http.NewRequest(http.MethodPost,
			base+"/internal/promo/clicks/"+clickID+"/resolve", bytes.NewReader(buf))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("X-Internal-Token", token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("internal resolve: %v", err)
		}
		return resp.StatusCode, readAll(t, resp)
	}

	// 无令牌 → 403；错令牌 → 403（fail-closed）
	if code, body := resolve("", "click-a", time.Now().UTC().Format(time.RFC3339), 42); code != http.StatusForbidden {
		t.Fatalf("no token = %d body=%s, want 403", code, body)
	}
	if code, body := resolve("wrong", "click-a", time.Now().UTC().Format(time.RFC3339), 42); code != http.StatusForbidden {
		t.Fatalf("wrong token = %d body=%s, want 403", code, body)
	}

	// 正确令牌 + 窗口内注册 → 归因命中（in_window=true，channel 返回）
	now := time.Now().UTC().Format(time.RFC3339)
	code, body := resolve(s2sToken, "click-a", now, 42)
	if code != http.StatusOK || !strings.Contains(body, `"channel":"kol-li"`) ||
		!strings.Contains(body, `"in_window":true`) {
		t.Fatalf("resolve in window = %d body=%s", code, body)
	}

	// 归因已回写：效果计数 attributed_users=1
	code, body = doAuthed(t, http.MethodGet, base+"/admin/promo/channels/kol-li/stats", admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"attributed_users":1`) {
		t.Fatalf("stats after attribution = %d body=%s", code, body)
	}

	// 窗口外注册（点击在当下，注册时刻推到远未来）→ in_window=false，
	// 且不抢注（点击已归因给 42，他人不再覆盖）
	far := time.Now().UTC().Add(90 * 24 * time.Hour).Format(time.RFC3339)
	code, body = resolve(s2sToken, "click-a", far, 43)
	if code != http.StatusOK || !strings.Contains(body, `"in_window":false`) {
		t.Fatalf("resolve out of window = %d body=%s", code, body)
	}
	// 未抢注：仍只有 1 个归因用户
	code, body = doAuthed(t, http.MethodGet, base+"/admin/promo/channels/kol-li/stats", admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"attributed_users":1`) {
		t.Fatalf("stats after out-of-window = %d body=%s", code, body)
	}

	// 未知 click_id → 200 + in_window=false（自然流量，非错误）
	if code, body := resolve(s2sToken, "click-none", now, 42); code != http.StatusOK ||
		!strings.Contains(body, `"in_window":false`) {
		t.Fatalf("unknown click = %d body=%s", code, body)
	}
}
