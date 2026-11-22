package analytics

// relay.go 剩余补漏分支：退避倍率封顶（attempts>6 → 6 分钟）与 Run 周期
// 轮次内 attempted>0 的日志臂。

import (
	"context"
	"net/http"
	"testing"
	"time"
)

// 退避封顶：attempts=6 预置、maxAttempts=10 → 本次 attempts=7 > 6 → mult 封 6。
func TestRelayOnce_BackoffCap(t *testing.T) {
	outbox, db := newRelayOutbox(t)
	ts := newPostServer(t, http.StatusInternalServerError)
	if err := outbox.Enqueue("a-cap", "download", `{"event_id":"a-cap"}`, nowUTC()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := db.Exec(`UPDATE analytics_outbox SET attempts=6 WHERE event_id='a-cap'`); err != nil {
		t.Fatalf("preseed attempts: %v", err)
	}
	r := NewRelay(outbox, newTestClient(ts.URL, outbox), time.Minute, 10, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0", a, d)
	}
	rows, err := outbox.ListRetry("2999-01-01T00:00:00Z", 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("want 1 pending row, got %d err=%v", len(rows), err)
	}
	if rows[0].Attempts != 7 {
		t.Fatalf("attempts = %d, want 7", rows[0].Attempts)
	}
}

// Run 周期轮次 attempted>0 → 记日志臂：入队到期行，tick 后重投成功。
func TestRelay_RunAttemptedLogf(t *testing.T) {
	outbox, _ := newRelayOutbox(t)
	ts := newPostServer(t, http.StatusOK)
	if err := outbox.Enqueue("a-log", "download", `{"event_id":"a-log"}`, "2000-01-01T00:00:00Z"); err != nil {
		t.Fatalf("Enqueue: %v", err)
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
	// 行应已被投递转移（attempted>0 路径生效）。
	rows, err := outbox.ListRetry("2999-01-01T00:00:00Z", 100)
	if err != nil || len(rows) != 0 {
		t.Fatalf("want 0 pending rows after delivery, got %d err=%v", len(rows), err)
	}
	if n, err := outbox.Prune("2999-01-01T00:00:00Z"); err != nil || n != 1 {
		t.Fatalf("prune delivered = %d/%v, want 1/nil", n, err)
	}
}
