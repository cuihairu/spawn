package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestRoutes_DownloadWithChannel GP-403 渠道包追踪：受控下载带 channel 参数
// 时按渠道计一条下载（渠道有效才计数；无效/缺失渠道不影响下载成功与否）。
// 复用 GP-105 的建版本→上传包体→提审发布链路，断言面在渠道计数上。
func TestRoutes_DownloadWithChannel(t *testing.T) {
	base, sign := newAuthedTestServer(t)
	admin := sign(claimsFor(1, "admin"))
	player := sign(claimsFor(7, "bob"))
	var body string
	code := 0

	// 登记渠道（单层）
	if code, _ := doAuthed(t, http.MethodPost, base+"/admin/promo/channels", admin,
		`{"code":"kol-li","name":"主播李"}`); code != http.StatusOK {
		t.Fatalf("create channel = %d", code)
	}

	// 合作方主体 + 授权 + 游戏归属（GP-105 同链路：partner 路由要求 partner 角色）。
	// 建合作方：该接口不回传 id，改由合作方列表回读（GP-403 测试同此口径）
	if code, body = doAuthed(t, http.MethodPost, base+"/admin/partners", admin, `{"name":"包体厂"}`); code != http.StatusOK {
		t.Fatalf("create partner = %d body=%s", code, body)
	}
	if code, body = doAuthed(t, http.MethodGet, base+"/admin/partners", admin, ""); code != http.StatusOK {
		t.Fatalf("list partners = %d body=%s", code, body)
	}
	var partnersResp struct {
		Partners []struct {
			Id int64 `json:"id"`
		} `json:"partners"`
	}
	if err := json.Unmarshal([]byte(body), &partnersResp); err != nil {
		t.Fatalf("decode partners: %v (%s)", err, body)
	}
	if len(partnersResp.Partners) == 0 {
		t.Fatalf("no partner listed: %s", body)
	}
	partnerID := partnersResp.Partners[len(partnersResp.Partners)-1].Id
	if code, _ = doAuthed(t, http.MethodPost, base+"/admin/roles", admin,
		`{"user_id":2,"role":"partner","partner_id":`+jsonNumber(partnerID)+`}`); code != http.StatusOK {
		t.Fatalf("grant partner = %d", code)
	}
	if code, _ = doAuthed(t, http.MethodPost, base+"/admin/games/game-elden-ring/ownership", admin,
		`{"partner_id":`+jsonNumber(partnerID)+`}`); code != http.StatusOK {
		t.Fatalf("assign ownership = %d", code)
	}
	partner := sign(claimsFor(2, "carol"))

	// 建版本 → 上传包体 → 提审 → 过审 → 发布（GP-105 同链路）
	code, body = doAuthed(t, http.MethodPost, base+"/partner/games/game-elden-ring/versions", partner,
		`{"version":"1.0.0","package_name":"com.ring.elden","version_code":10}`)
	if code != http.StatusOK {
		t.Fatalf("create version = %d body=%s", code, body)
	}
	var verResp struct {
		Id string `json:"id"`
	}
	if err := json.Unmarshal([]byte(body), &verResp); err != nil {
		t.Fatalf("decode version: %v (%s)", err, body)
	}
	payload := []byte("PK\x03\x04promo-package-bytes-0123456789")
	if code, body = postMultipart(t, base+"/partner/games/game-elden-ring/versions/"+verResp.Id+"/package",
		partner, "file", "demo.apk", payload); code != http.StatusOK {
		t.Fatalf("upload = %d body=%s", code, body)
	}
	if code, body = doAuthed(t, http.MethodPost, base+"/partner/games/game-elden-ring/submit", partner,
		`{"version_id":"`+verResp.Id+`"}`); code != http.StatusOK {
		t.Fatalf("submit = %d body=%s", code, body)
	}
	var submitResp struct {
		Id int64 `json:"id"`
	}
	_ = json.Unmarshal([]byte(body), &submitResp)
	if code, body = doAuthed(t, http.MethodPost, base+"/admin/reviews/"+jsonNumber(submitResp.Id)+"/decision", admin,
		`{"decision":"approve"}`); code != http.StatusOK {
		t.Fatalf("approve = %d body=%s", code, body)
	}
	if code, body = doAuthed(t, http.MethodPost,
		base+"/admin/games/game-elden-ring/versions/"+verResp.Id+"/publish", admin, ""); code != http.StatusOK {
		t.Fatalf("publish version = %d body=%s", code, body)
	}

	download := func(token, channel string) (int, string) {
		url := base + "/games/game-elden-ring/versions/" + verResp.Id + "/download"
		if channel != "" {
			url += "?channel=" + channel
		}
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("build download req: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("download %q: %v", channel, err)
		}
		return resp.StatusCode, readAll(t, resp)
	}

	statsDownloads := func() string {
		c, b := doAuthed(t, http.MethodGet, base+"/admin/promo/channels/kol-li/stats", admin, "")
		if c != http.StatusOK {
			t.Fatalf("stats = %d body=%s", c, b)
		}
		return b
	}

	// 带有效渠道下载 → 下载成功 + 渠道包计数 +1
	if code, _ := download(player, "kol-li"); code != http.StatusOK {
		t.Fatalf("download with channel = %d, want 200", code)
	}
	if b := statsDownloads(); !strings.Contains(b, `"downloads":1`) {
		t.Fatalf("stats after channel download = %s, want downloads=1", b)
	}

	// 无效渠道：下载仍成功（渠道问题不阻断下载），且不虚增计数
	if code, b := download(player, "ghost"); code != http.StatusOK {
		t.Fatalf("download ghost channel = %d body=%s", code, b)
	}
	if b := statsDownloads(); !strings.Contains(b, `"downloads":1`) {
		t.Fatalf("stats after ghost download = %s, want downloads still 1", b)
	}

	// 不带渠道：不计渠道包（自然流量）
	if code, _ := download(player, ""); code != http.StatusOK {
		t.Fatalf("download without channel = %d, want 200", code)
	}
	if b := statsDownloads(); !strings.Contains(b, `"downloads":1`) {
		t.Fatalf("stats after plain download = %s, want downloads still 1", b)
	}

	t.Logf("channel stats: %s", statsDownloads())
}
