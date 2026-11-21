package handler

// GP-403 渠道归因（register 携带 click_id）本地 outbox 补偿集成测试：
// game-catalog 短时不可达（503）→ 注册主流程不受影响（仍 200），归因意图先落
// outbox（pending）；下游恢复后 Relay 补投成功标记 delivered，归因解析完整
// 到达（重投锚定原始 registered_at、user_id）。

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/tappi/tappi/services/user-service/internal/integration"
	"github.com/tappi/tappi/services/user-service/internal/promoattr"
	"github.com/tappi/tappi/services/user-service/model"
)

// TestRoutes_RegisterPromoAttrOutboxRetry 归因补偿闭环：下游 503 → 注册 200 且
// 落 pending；Relay 首投失败退避；下游恢复 + 退避拨回 → Relay 补投成功，解析
// 请求带原始 click_id 与 user_id，队列清空。
func TestRoutes_RegisterPromoAttrOutboxRetry(t *testing.T) {
	var mu sync.Mutex
	var bodies []map[string]any
	var down bool
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/promo/clicks/click-9/resolve" {
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
		var body map[string]any
		if err := json.Unmarshal(raw, &body); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		bodies = append(bodies, body)
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"channel":"kol-li","channel_id":7,"clicked_at":"2026-11-20T00:00:00Z","window_hours":72,"in_window":true}`)
	}))
	defer catalog.Close()
	setDown := func(d bool) {
		mu.Lock()
		down = d
		mu.Unlock()
	}

	base, svcCtx := newUserHandlerServer(t, gameStubHandler(t, nil))
	client := integration.NewGameCatalogClient(catalog.URL, 2*time.Second)
	svcCtx.GameCatalogClient = client
	// 助手只装核心字段，outbox 需在此落表并接线（测试库 = 助手开的临时库）。
	svcCtx.PromoAttrOutbox = model.NewPromoAttrOutboxModel(svcCtx.DB)
	if err := svcCtx.PromoAttrOutbox.CreateTables(); err != nil {
		t.Fatalf("create promo attr outbox: %v", err)
	}
	svcCtx.PromoAttrRelay = promoattr.NewRelay(svcCtx.PromoAttrOutbox, client,
		time.Second, 10, 24*time.Hour)

	// 下游不可达：注册仍 200（归因旁路不影响注册），意图落 outbox pending
	setDown(true)
	body := `{"username":"nora","email":"nora@example.com","password":"pw-nora","click_id":"click-9"}`
	status, respBody, raw := do(t, http.MethodPost, base+"/auth/register", "", body)
	if status != http.StatusOK || code(t, respBody) != 200 {
		t.Fatalf("register: status=%d raw=%s, want 200", status, raw)
	}
	rows, err := svcCtx.PromoAttrOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("outbox pending = %d err=%v, want 1", len(rows), err)
	}
	data, ok := respBody["data"].(map[string]interface{})
	if !ok {
		t.Fatalf("register body = %v, want data envelope", respBody)
	}
	newUserID, _ := data["user_id"].(float64)
	if newUserID <= 0 {
		t.Fatalf("register data = %v, want user_id > 0", data)
	}
	// user_id 在入队时即已知（点击归属本次注册的用户），随 payload 一并落库
	row := rows[0]
	if row.ClickId != "click-9" || row.UserId != int64(newUserID) ||
		row.Status != model.PromoAttrOutboxPending || row.Attempts != 0 {
		t.Fatalf("outbox row = %+v, want click-9/user %d/pending/0 attempts", row, int64(newUserID))
	}

	// 首轮 Relay：仍失败 → attempts=1 + 退避延期
	ctx := context.Background()
	attempted, delivered := svcCtx.PromoAttrRelay.RelayOnce(ctx)
	if attempted != 1 || delivered != 0 {
		t.Fatalf("relay down = %d/%d, want 1/0", attempted, delivered)
	}
	rows, _ = svcCtx.PromoAttrOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 1 || rows[0].Attempts != 1 {
		t.Fatalf("outbox after failed relay = %+v, want attempts=1", rows)
	}

	// 下游恢复 + 退避到期（把 next_retry_at 拨回过去模拟时间推进）
	setDown(false)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := svcCtx.PromoAttrOutbox.MarkFailed(row.AttrKey, 1, "模拟退避到期", past); err != nil {
		t.Fatalf("rewind backoff: %v", err)
	}
	attempted, delivered = svcCtx.PromoAttrRelay.RelayOnce(ctx)
	if attempted != 1 || delivered != 1 {
		t.Fatalf("relay recovered = %d/%d, want 1/1", attempted, delivered)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(bodies) != 1 {
		t.Fatalf("resolve attempts after recovery = %d, want 1", len(bodies))
	}
	if bodies[0]["click_id"] != "click-9" {
		t.Fatalf("relayed click_id = %v, want click-9", bodies[0]["click_id"])
	}
	if uid, _ := bodies[0]["user_id"].(float64); uid != newUserID {
		t.Fatalf("relayed user_id = %v, want %d", bodies[0]["user_id"], int64(newUserID))
	}
	rows, _ = svcCtx.PromoAttrOutbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 0 {
		t.Fatalf("outbox pending after delivered = %d, want 0", len(rows))
	}
}
