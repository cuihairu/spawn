package handler

// GP-306 业务通知 outbox 补偿集成测试：community 短时不可达（503）→ 活动发布
// 主流程不受影响（仍 200 published），通知意图先落 notify_outbox（pending）；
// 下游恢复后 Relay 补投成功标记 delivered，game_activity 扇出完整到达（受众/
// 类型/目标原样）。重投安全由 community 侧 UNIQUE(user,actor,type,target)
// 去重保证。

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/promotions/utils"
)

func TestRoutes_ActivityPublishNotifyOutboxRetry(t *testing.T) {
	community := newCommunityRecorder()
	defer community.srv.Close()
	community.down.Store(true)
	env := newActivityNotifyServer(t, community.srv.URL)
	admin := env.sign(utils.JWTClaims{UserId: 1, Username: "admin"})

	// community 不可达：发布仍 200 published，通知意图落 outbox pending
	code, body := do(t, http.MethodPost, env.base+"/admin/activities", admin, validActivityBody)
	if code != http.StatusOK {
		t.Fatalf("create activity = %d body=%s", code, body)
	}
	var resp struct {
		Activity struct {
			Id int64 `json:"id"`
		} `json:"activity"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil || resp.Activity.Id == 0 {
		t.Fatalf("parse create = %v body=%s", err, body)
	}
	id := resp.Activity.Id
	code, body = do(t, http.MethodPost, fmt.Sprintf("%s/admin/activities/%d/publish", env.base, id), admin, "")
	if code != http.StatusOK || !strings.Contains(body, `"status":"published"`) {
		t.Fatalf("publish while community down = %d body=%s", code, body)
	}
	if community.Calls() != 0 {
		t.Fatalf("community calls = %d, want 0 (未恢复不应有成功投递)", community.Calls())
	}
	outbox := env.svcCtx.NotifyOutbox
	if outbox == nil {
		t.Fatal("notify outbox not wired")
	}
	rows, err := outbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("notify outbox pending = %d err=%v, want 1", len(rows), err)
	}
	row := rows[0]
	if row.NotifyType != "game_activity" || row.Status != "pending" || row.Attempts != 0 {
		t.Fatalf("outbox row = %+v, want game_activity/pending/0 attempts", row)
	}

	// 首轮 Relay：仍失败 → attempts=1 + 退避延期
	relay := env.svcCtx.NotifyRelay
	if relay == nil {
		t.Fatal("notify relay not wired")
	}
	ctx := context.Background()
	attempted, delivered := relay.RelayOnce(ctx)
	if attempted != 1 || delivered != 0 {
		t.Fatalf("relay down = %d/%d, want 1/0", attempted, delivered)
	}
	rows, _ = outbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 1 || rows[0].Attempts != 1 {
		t.Fatalf("outbox after failed relay = %+v, want attempts=1", rows)
	}

	// 下游恢复 + 退避到期（把 next_retry_at 拨回过去模拟时间推进）
	community.down.Store(false)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := outbox.MarkFailed(row.NotifyKey, 1, "模拟退避到期", past); err != nil {
		t.Fatalf("rewind backoff: %v", err)
	}
	attempted, delivered = relay.RelayOnce(ctx)
	if attempted != 1 || delivered != 1 {
		t.Fatalf("relay recovered = %d/%d, want 1/1", attempted, delivered)
	}
	if community.Calls() != 1 {
		t.Fatalf("community calls after recovery = %d, want 1", community.Calls())
	}
	raw := community.Last()
	var fanout struct {
		UserIds  []int64 `json:"user_ids"`
		Type     string  `json:"type"`
		TargetId int64   `json:"target_id"`
	}
	if err := json.Unmarshal(raw, &fanout); err != nil {
		t.Fatalf("parse relayed body: %v raw=%s", err, raw)
	}
	if fanout.Type != "game_activity" || fanout.TargetId != id ||
		len(fanout.UserIds) != 2 || fanout.UserIds[0] != 7 || fanout.UserIds[1] != 8 {
		t.Fatalf("relayed body = %+v, want game_activity target=%d users=[7 8]", fanout, id)
	}
	rows, _ = outbox.ListRetry("9999-01-01T00:00:00Z", 100)
	if len(rows) != 0 {
		t.Fatalf("outbox pending after delivered = %d, want 0", len(rows))
	}
}
