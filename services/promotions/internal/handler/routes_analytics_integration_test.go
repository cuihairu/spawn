package handler

// GP-402 经营分析事实投递（reward_granted）集成测试：data-panel 短时不可达
// （503）→ 发放主流程不受影响（评估仍 200），事实先落本地 outbox（pending）；
// 下游恢复后 Relay 补投成功标记 delivered，事实批次完整到达（含币种）。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/tappi/tappi/services/promotions/model"
	"github.com/tappi/tappi/services/promotions/utils"
)

type analyticsEvent struct {
	EventId     string `json:"event_id"`
	EventType   string `json:"event_type"`
	UserId      int64  `json:"user_id"`
	GameId      string `json:"game_id"`
	ActivityId  int64  `json:"activity_id"`
	OrderId     string `json:"order_id"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
}

// newAnalyticsActivityServer 活动服务 + data-panel /internal/events 存根：
// panelDown 置真时存根返 503（模拟数据面板短时不可达）。
func newAnalyticsActivityServer(t *testing.T) (*testActivityEnv, func(bool)) {
	t.Helper()

	var mu sync.Mutex
	var seen []string
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
		body, _ := io.ReadAll(r.Body)
		seen = append(seen, string(body))
		_, _ = w.Write([]byte(`{"accepted":1}`))
	}))
	t.Cleanup(panel.Close)

	env := newActivityServerAnalytics(t, panel.URL, "gp402-internal-token")
	env.analyticsSeen = &seen
	return env, func(d bool) {
		mu.Lock()
		down = d
		mu.Unlock()
	}
}

// TestRoutes_RewardGrantedOutboxRetry reward_granted 投递补偿闭环：
// 下游 503 → 评估 200 主流程不受影响 + 落 pending；Relay 首投失败退避；
// 下游恢复 + 退避拨回 → Relay 补投成功，事实事件带币种与活动 id。
func TestRoutes_RewardGrantedOutboxRetry(t *testing.T) {
	env, setDown := newAnalyticsActivityServer(t)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})
	ctx := context.Background()

	// 发布一个满 1000 分触发、总倍率 15000 的活动
	id := publishActivity(t, env, admin, `{
		"name":"发奖投递","game_id":"game-a","timezone":"Asia/Shanghai",
		"start_at":"2026-11-01T00:00:00+08:00","end_at":"2026-11-30T23:59:59+08:00",
		"server_ids":[1],"channel":"sandbox","audience":{"type":"all"},
		"trigger":{"type":"order","min_amount_cents":1000},
		"stackable":false,"rewards":{"type":"multiplier","mode":"total","rate_bp":15000}
	}`)
	if id == 0 {
		t.Fatal("publish activity failed")
	}
	eventID := "reward_granted:order-aw:" + strconv.FormatInt(id, 10)

	// 下游短时不可达：发放主流程不受影响（评估仍 200），事实先落 outbox
	setDown(true)
	code, body := evaluate(t, env, evalBody("order-aw", 100, 2000), "")
	if code != http.StatusOK {
		t.Fatalf("evaluate = %d body=%s, want 200", code, body)
	}
	if len(*env.analyticsSeen) != 0 {
		t.Fatalf("down: analytics batches = %d, want 0", len(*env.analyticsSeen))
	}
	rows, err := env.svcCtx.AnalyticsOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("outbox pending = %d err=%v, want 1", len(rows), err)
	}
	row := rows[0]
	if row.EventId != eventID || row.EventType != "reward_granted" ||
		row.Status != model.AnalyticsOutboxPending || row.Attempts != 0 {
		t.Fatalf("outbox row = %+v, want pending/0 attempts/derived event_id", row)
	}

	// Relay 首次重投：下游仍不可达 → 记失败 + 指数退避延期
	attempted, delivered := env.svcCtx.AnalyticsRelay.RelayOnce(ctx)
	if attempted != 1 || delivered != 0 {
		t.Fatalf("relay down = %d/%d, want 1/0", attempted, delivered)
	}
	rows, _ = env.svcCtx.AnalyticsOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 1 || rows[0].Attempts != 1 || rows[0].Status != model.AnalyticsOutboxPending {
		t.Fatalf("outbox after failed relay = %+v, want attempts=1 pending", rows)
	}

	// 下游恢复 + 退避到期（把 next_retry_at 拨回过去模拟时间推进）
	setDown(false)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := env.svcCtx.AnalyticsOutbox.MarkFailed(row.EventId, 1, "模拟退避到期", past); err != nil {
		t.Fatalf("rewind backoff: %v", err)
	}
	attempted, delivered = env.svcCtx.AnalyticsRelay.RelayOnce(ctx)
	if attempted != 1 || delivered != 1 {
		t.Fatalf("relay recovered = %d/%d, want 1/1", attempted, delivered)
	}
	if len(*env.analyticsSeen) != 1 {
		t.Fatalf("analytics batches after recovery = %d, want 1", len(*env.analyticsSeen))
	}
	ev := decodeAnalyticsEvents(t, (*env.analyticsSeen)[0])
	if len(ev) != 1 {
		t.Fatalf("relayed events = %+v, want 1", ev)
	}
	got := ev[0]
	if got.EventType != "reward_granted" || got.EventId != eventID ||
		got.ActivityId != id || got.OrderId != "order-aw" || got.AmountCents != 2000 || got.Currency != "CNY" {
		t.Fatalf("relayed event = %+v, want reward_granted/%d/2000/CNY", got, id)
	}
	rows, _ = env.svcCtx.AnalyticsOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 0 {
		t.Fatalf("outbox pending after delivered = %d, want 0", len(rows))
	}
}

func decodeAnalyticsEvents(t *testing.T, body string) []analyticsEvent {
	t.Helper()
	var req struct {
		Events []analyticsEvent `json:"events"`
	}
	if err := json.Unmarshal([]byte(body), &req); err != nil {
		t.Fatalf("decode analytics body %q: %v", body, err)
	}
	return req.Events
}
