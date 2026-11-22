package notify

// relay.go 剩余补漏分支：nil 守卫、扫描失败、退避倍率封顶（attempts>6 →
// 6 分钟）、耗尽置 dropped，以及 MarkFailed/MarkDropped/MarkDelivered 落库
// 失败（readonly 库：读 OK、写失败）与 Run 周期轮次（attempted>0 日志臂 +
// 终态清理）。

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/model"

	_ "github.com/mattn/go-sqlite3"
)

// newRelayOutbox 建可写 outbox 库；返回 db 供预置 attempts / 关库重开。
func newRelayOutbox(t *testing.T) (*model.NotifyOutboxModel, *sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "notify-relay.db")
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	m := model.NewNotifyOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return m, db, path
}

func newPostServer(t *testing.T, status int) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(ts.Close)
	return ts
}

// roOutbox 以 mode=ro 重开库文件：ListRetry 读 OK，Mark* 写失败。
func roOutbox(t *testing.T, path string) *model.NotifyOutboxModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open ro sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return model.NewNotifyOutboxModel(db)
}

func TestRelayOnce_NilGuard(t *testing.T) {
	var r *Relay
	if a, d := r.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("nil relay = (%d,%d), want (0,0)", a, d)
	}
}

func TestRelayOnce_ScanFailed(t *testing.T) {
	m, db, _ := newRelayOutbox(t)
	ts := newPostServer(t, http.StatusOK)
	r := NewRelay(m, NewClient(ts.URL, "tok", 1000, m), time.Minute, 10, time.Hour)
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	if a, d := r.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("scan failed = (%d,%d), want (0,0)", a, d)
	}
}

// 退避封顶：attempts=6 预置、maxAttempts=10 → 本次 attempts=7 > 6 → mult 封 6。
func TestRelayOnce_BackoffCap(t *testing.T) {
	m, db, _ := newRelayOutbox(t)
	defer db.Close()
	ts := newPostServer(t, http.StatusInternalServerError)
	if err := m.Enqueue("n-cap", "statement_dispute_resolved", `{"user_ids":[7]}`, nowUTC()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := db.Exec(`UPDATE notify_outbox SET attempts=6 WHERE notify_key='n-cap'`); err != nil {
		t.Fatalf("preseed attempts: %v", err)
	}
	r := NewRelay(m, NewClient(ts.URL, "tok", 1000, m), time.Minute, 10, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0", a, d)
	}
	rows, err := m.ListRetry("2999-01-01T00:00:00Z", 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("want 1 pending row, got %d err=%v", len(rows), err)
	}
	if rows[0].Attempts != 7 {
		t.Fatalf("attempts = %d, want 7", rows[0].Attempts)
	}
}

// 重试耗尽：attempts=9 → 10 >= 10 → 置 dropped；readonly 库使 MarkDropped
// 落库失败（错误分支），投递尝试仍为 1。
func TestRelayOnce_DropWriteFailed(t *testing.T) {
	m, db, path := newRelayOutbox(t)
	if err := m.Enqueue("n-drop", "statement_dispute_resolved", `{}`, "2000-01-01T00:00:00Z"); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if _, err := db.Exec(`UPDATE notify_outbox SET attempts=9 WHERE notify_key='n-drop'`); err != nil {
		t.Fatalf("preseed attempts: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close writable: %v", err)
	}
	ro := roOutbox(t, path)
	ts := newPostServer(t, http.StatusInternalServerError)
	r := NewRelay(ro, NewClient(ts.URL, "tok", 1000, ro), time.Minute, 10, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0", a, d)
	}
}

// 失败记账落库失败（MarkFailed 写 readonly 库报错分支）。
func TestRelayOnce_MarkFailedWriteFailed(t *testing.T) {
	m, db, path := newRelayOutbox(t)
	if err := m.Enqueue("n-mf", "statement_dispute_resolved", `{}`, "2000-01-01T00:00:00Z"); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close writable: %v", err)
	}
	ro := roOutbox(t, path)
	ts := newPostServer(t, http.StatusInternalServerError)
	r := NewRelay(ro, NewClient(ts.URL, "tok", 1000, ro), time.Minute, 10, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0", a, d)
	}
}

// 投递成功但交付落库失败（MarkDelivered 写 readonly 库报错分支）。
func TestRelayOnce_MarkDeliveredWriteFailed(t *testing.T) {
	m, db, path := newRelayOutbox(t)
	if err := m.Enqueue("n-md", "statement_dispute_resolved", `{}`, "2000-01-01T00:00:00Z"); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close writable: %v", err)
	}
	ro := roOutbox(t, path)
	ts := newPostServer(t, http.StatusOK)
	r := NewRelay(ro, NewClient(ts.URL, "tok", 1000, ro), time.Minute, 10, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 1 {
		t.Fatalf("attempted=%d delivered=%d, want 1/1", a, d)
	}
}

// Run 周期轮次 attempted>0 → 记日志臂；投递成功后终态清理回收行。
func TestRelay_Run(t *testing.T) {
	m, db, _ := newRelayOutbox(t)
	defer db.Close()
	ts := newPostServer(t, http.StatusOK)
	if err := m.Enqueue("n-run", "statement_dispute_resolved", `{"user_ids":[7]}`, "2000-01-01T00:00:00Z"); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	r := NewRelay(m, NewClient(ts.URL, "tok", 1000, m), time.Millisecond, 2, time.Hour)
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
	rows, err := m.ListRetry("2999-01-01T00:00:00Z", 100)
	if err != nil || len(rows) != 0 {
		t.Fatalf("want 0 pending rows after delivery, got %d err=%v", len(rows), err)
	}
	if n, err := m.Prune("2999-01-01T00:00:00Z"); err != nil || n != 1 {
		t.Fatalf("prune delivered = %d/%v, want 1/nil", n, err)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abc", 500); got != "abc" {
		t.Fatalf("short = %q", got)
	}
	if got := truncate(strings.Repeat("a", 600), 500); len(got) != 500 {
		t.Fatalf("long len = %d, want 500", len(got))
	}
}
