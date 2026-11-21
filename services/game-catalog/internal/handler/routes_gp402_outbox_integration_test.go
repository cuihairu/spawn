package handler

// GP-402 经营分析事实投递（download）本地 outbox 补偿集成测试：data-panel 短时
// 不可达（503）→ 包体下载主流程不受影响（仍 200），事实先落 outbox（pending）；
// 下游恢复后 Relay 补投成功标记 delivered，事实事件完整到达。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tappi/tappi/services/game-catalog/internal/analytics"
	"github.com/tappi/tappi/services/game-catalog/model"
)

// TestRoutes_DownloadEventOutboxRetry download 事实投递补偿闭环：下游 503 →
// 下载 200 且落 pending；Relay 首投失败退避；下游恢复 + 退避拨回 → Relay
// 补投成功，事实事件带 event_type 与 game_id。
func TestRoutes_DownloadEventOutboxRetry(t *testing.T) {
	base, sign, svcCtx := newAuthedTestServerSvc(t, nil)
	admin := sign(claimsFor(1, "admin"))
	player := sign(claimsFor(7, "bob"))
	var body string
	code := 0

	var mu sync.Mutex
	var seen []map[string]any
	var down bool
	panel := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/events" {
			http.NotFound(w, r)
			return
		}
		mu.Lock()
		defer mu.Unlock()
		if down {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		raw, _ := io.ReadAll(r.Body)
		var payload struct {
			Events []map[string]any `json:"events"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		seen = append(seen, payload.Events...)
		w.WriteHeader(http.StatusOK)
	}))
	defer panel.Close()

	setDown := func(d bool) {
		mu.Lock()
		down = d
		mu.Unlock()
	}

	// 重装投递面：服务装配时 DATA_PANEL_URL 未配置（noop），此处接 outbox +
	// 重投循环（handler 持 svcCtx 指针，改字段即生效）。
	svcCtx.AnalyticsSvc = analytics.NewClient(panel.URL, "panel-token", 1000, svcCtx.AnalyticsOutbox)
	svcCtx.AnalyticsRelay = analytics.NewRelay(svcCtx.AnalyticsOutbox, svcCtx.AnalyticsSvc,
		time.Second, 10, 24*time.Hour)

	// 合作方主体 + 授权 + 游戏归属（GP-105 同链路：partner 路由要求 partner 角色）。
	if code, body = doAuthed(t, http.MethodPost, base+"/admin/partners", admin, `{"name":"下载厂"}`); code != http.StatusOK {
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
	payload := []byte("PK\x03\x04gp402-package-bytes-0123456789")
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

	download := func(token string) int {
		url := base + "/games/game-elden-ring/versions/" + verResp.Id + "/download"
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err != nil {
			t.Fatalf("build download req: %v", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("download: %v", err)
		}
		_ = readAll(t, resp)
		return resp.StatusCode
	}

	// 下游不可达：下载仍 200（事实投递不影响主流程），事实落 outbox pending
	setDown(true)
	if code := download(player); code != http.StatusOK {
		t.Fatalf("download down = %d, want 200", code)
	}
	mu.Lock()
	nSeen := len(seen)
	mu.Unlock()
	if nSeen != 0 {
		t.Fatalf("down: analytics events = %d, want 0", nSeen)
	}
	rows, err := svcCtx.AnalyticsOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("outbox pending = %d err=%v, want 1", len(rows), err)
	}
	row := rows[0]
	if row.EventType != analytics.EventDownload || row.Status != model.AnalyticsOutboxPending || row.Attempts != 0 {
		t.Fatalf("outbox row = %+v, want download/pending/0 attempts", row)
	}

	// 首轮 Relay：仍失败 → attempts=1 + 退避延期
	ctx := context.Background()
	attempted, delivered := svcCtx.AnalyticsRelay.RelayOnce(ctx)
	if attempted != 1 || delivered != 0 {
		t.Fatalf("relay down = %d/%d, want 1/0", attempted, delivered)
	}
	rows, _ = svcCtx.AnalyticsOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 1 || rows[0].Attempts != 1 {
		t.Fatalf("outbox after failed relay = %+v, want attempts=1", rows)
	}

	// 下游恢复 + 退避到期（把 next_retry_at 拨回过去模拟时间推进）
	setDown(false)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := svcCtx.AnalyticsOutbox.MarkFailed(row.EventId, 1, "模拟退避到期", past); err != nil {
		t.Fatalf("rewind backoff: %v", err)
	}
	attempted, delivered = svcCtx.AnalyticsRelay.RelayOnce(ctx)
	if attempted != 1 || delivered != 1 {
		t.Fatalf("relay recovered = %d/%d, want 1/1", attempted, delivered)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(seen) != 1 {
		t.Fatalf("analytics events after recovery = %d, want 1", len(seen))
	}
	if got := seen[0]["event_type"]; got != analytics.EventDownload {
		t.Fatalf("relayed event_type = %v, want %s", got, analytics.EventDownload)
	}
	if got := seen[0]["game_id"]; got != "game-elden-ring" {
		t.Fatalf("relayed game_id = %v, want game-elden-ring", got)
	}
	rows, _ = svcCtx.AnalyticsOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 0 {
		t.Fatalf("outbox pending after delivered = %d, want 0", len(rows))
	}
}
