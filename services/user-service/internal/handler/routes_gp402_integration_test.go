package handler

// GP-402 经营分析事实投递（register）本地 outbox 补偿集成测试：data-panel 短时
// 不可达（503）→ 注册主流程不受影响（仍 200），事实先落 outbox（pending）；
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

	"github.com/tappi/tappi/services/user-service/internal/analytics"
	"github.com/tappi/tappi/services/user-service/model"
)

// TestRoutes_RegisterEventOutboxRetry register 事实投递补偿闭环：下游 503 →
// 注册 200 且落 pending；Relay 首投失败退避；下游恢复 + 退避拨回 → Relay
// 补投成功，事实事件带 user_id 与 event_type。
func TestRoutes_RegisterEventOutboxRetry(t *testing.T) {
	base, svcCtx := newUserHandlerServer(t, gameStubHandler(t, nil))

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

	// 助手只装核心字段，outbox 需在此落表并接线（测试库 = 助手开的临时库）。
	svcCtx.AnalyticsOutbox = model.NewAnalyticsOutboxModel(svcCtx.DB)
	if err := svcCtx.AnalyticsOutbox.CreateTables(); err != nil {
		t.Fatalf("create analytics outbox: %v", err)
	}
	svcCtx.AnalyticsSvc = analytics.NewClient(panel.URL, "panel-token", 1000, svcCtx.AnalyticsOutbox)
	svcCtx.AnalyticsRelay = analytics.NewRelay(svcCtx.AnalyticsOutbox, svcCtx.AnalyticsSvc,
		time.Second, 10, 24*time.Hour)

	setDown(true)
	body := `{"username":"olaf","email":"olaf@example.com","password":"pw-olaf"}`
	status, respBody, raw := do(t, http.MethodPost, base+"/auth/register", "", body)
	if status != http.StatusOK || code(t, respBody) != 200 {
		t.Fatalf("register: status=%d raw=%s, want 200", status, raw)
	}
	if len(seen) != 0 {
		t.Fatalf("down: analytics events = %d, want 0", len(seen))
	}
	rows, err := svcCtx.AnalyticsOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("outbox pending = %d err=%v, want 1", len(rows), err)
	}
	row := rows[0]
	if row.EventType != analytics.EventRegister || row.Status != model.AnalyticsOutboxPending || row.Attempts != 0 {
		t.Fatalf("outbox row = %+v, want register/pending/0 attempts", row)
	}

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
	if got := seen[0]["event_type"]; got != analytics.EventRegister {
		t.Fatalf("relayed event_type = %v, want %s", got, analytics.EventRegister)
	}
	rows, _ = svcCtx.AnalyticsOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 0 {
		t.Fatalf("outbox pending after delivered = %d, want 0", len(rows))
	}
}
