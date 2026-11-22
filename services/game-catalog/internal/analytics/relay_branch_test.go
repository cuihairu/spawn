package analytics

// relay.go 补漏分支：RelayOnce 的 Mark* 失败臂（UPDATE 触发器强制中止，覆盖
// "SELECT 成功但 UPDATE 失败"）、MarkFailed 成功转移、Run 的 prune 成功/失败
// 与 truncate 纯函数。成功重投与耗尽置 dropped 已由 relay_test.go 覆盖。

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/game-catalog/model"
)

// failUpdateTrigger 在 analytics_outbox 上挂 BEFORE UPDATE 触发器：任何 UPDATE
// 一律中止（用于制造 "ListRetry SELECT 成功、Mark* UPDATE 失败"）。
func failUpdateTrigger(t *testing.T, db *sql.DB) {
	t.Helper()
	if _, err := db.Exec(`CREATE TRIGGER fail_update BEFORE UPDATE ON analytics_outbox
		BEGIN SELECT RAISE(ABORT, 'forced update failure'); END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
}

func newPostServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(ts.Close)
	return ts
}

// truncate 纯函数：短串原样返回，长串按 n 截断。
func TestTruncate(t *testing.T) {
	if got := truncate("short", 10); got != "short" {
		t.Fatalf("truncate(short) = %q, want short", got)
	}
	long := strings.Repeat("x", 600)
	if got := truncate(long, 500); len(got) != 500 {
		t.Fatalf("truncate(long) len = %d, want 500", len(got))
	}
}

// RelayOnce 首投失败且 attempts < maxAttempts → MarkFailed（行留 pending，
// attempts 递增、记 last_error，下次重投时间已退避）。
func TestRelayOnce_MarkFailedBranch(t *testing.T) {
	outbox, _ := newRelayOutbox(t)
	ts := newPostServer(t, http.StatusInternalServerError)
	if err := outbox.Enqueue("a-mf", "download", `{"event_id":"a-mf"}`, nowUTC()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	r := NewRelay(outbox, newTestClient(ts.URL, outbox), time.Minute, 3, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0", a, d)
	}
	// next_retry_at 已退避到未来，用远期 now 扫回全部 pending 行。
	rows, err := outbox.ListRetry("2999-01-01T00:00:00Z", 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("want 1 pending row, got %d err=%v", len(rows), err)
	}
	if rows[0].Attempts != 1 || rows[0].Status != model.AnalyticsOutboxPending {
		t.Fatalf("row = attempts=%d status=%q, want 1/pending", rows[0].Attempts, rows[0].Status)
	}
	if rows[0].NextRetryAt == "" {
		t.Fatal("next_retry_at should be set after backoff")
	}
}

// RelayOnce MarkFailed 失败臂：UPDATE 被触发器中止 → 记日志，不 panic。
func TestRelayOnce_MarkFailedError(t *testing.T) {
	outbox, db := newRelayOutbox(t)
	ts := newPostServer(t, http.StatusInternalServerError)
	if err := outbox.Enqueue("a-mfe", "download", `{"event_id":"a-mfe"}`, nowUTC()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	failUpdateTrigger(t, db)
	r := NewRelay(outbox, newTestClient(ts.URL, outbox), time.Minute, 3, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0", a, d)
	}
}

// RelayOnce MarkDelivered 失败臂：post 成功但标记 delivered 被触发器中止。
func TestRelayOnce_MarkDeliveredError(t *testing.T) {
	outbox, db := newRelayOutbox(t)
	ts := newPostServer(t, http.StatusOK)
	if err := outbox.Enqueue("a-mde", "download", `{"event_id":"a-mde"}`, nowUTC()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	failUpdateTrigger(t, db)
	r := NewRelay(outbox, newTestClient(ts.URL, outbox), time.Minute, 3, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 1 {
		t.Fatalf("attempted=%d delivered=%d, want 1/1", a, d)
	}
}

// RelayOnce MarkDropped 失败臂：触顶置 dropped 被触发器中止 → 记日志，不 panic。
func TestRelayOnce_MarkDroppedError(t *testing.T) {
	outbox, db := newRelayOutbox(t)
	ts := newPostServer(t, http.StatusInternalServerError)
	if err := outbox.Enqueue("a-dre", "download", `{"event_id":"a-dre"}`, nowUTC()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE analytics_outbox SET attempts=1 WHERE event_id='a-dre'`); err != nil {
		t.Fatalf("preseed attempts: %v", err)
	}
	failUpdateTrigger(t, db)
	r := NewRelay(outbox, newTestClient(ts.URL, outbox), time.Minute, 2, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0", a, d)
	}
}

// Run 周期循环：prune 成功（清理已终态旧行，n>0 记日志）与 ctx 取消即退出。
func TestRelay_RunPruneSuccess(t *testing.T) {
	outbox, db := newRelayOutbox(t)
	ts := newPostServer(t, http.StatusOK)
	if err := outbox.Enqueue("a-prune", "download", `{"event_id":"a-prune"}`, nowUTC()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := db.Exec(
		`UPDATE analytics_outbox SET status='delivered', delivered_at='2000-01-01T00:00:00Z' WHERE event_id='a-prune'`); err != nil {
		t.Fatalf("preseed delivered: %v", err)
	}
	r := NewRelay(outbox, newTestClient(ts.URL, outbox), time.Millisecond, 2, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}

// Run 周期循环：prune 失败（关库）→ 记日志，不 panic，ctx 取消即退出。
func TestRelay_RunPruneError(t *testing.T) {
	outbox, db := newRelayOutbox(t)
	ts := newPostServer(t, http.StatusOK)
	db.Close()
	r := NewRelay(outbox, newTestClient(ts.URL, outbox), time.Millisecond, 2, time.Hour)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}
