package analytics

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"

	"github.com/tappi/tappi/services/game-catalog/model"
)

// fakeClient 非 *client 实现：NewRelay 类型断言失败 → nil。
type fakeClient struct{}

func (fakeClient) Emit(Event)    {}
func (fakeClient) Enabled() bool { return true }

func newRelayOutbox(t *testing.T) (*model.AnalyticsOutboxModel, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "relay.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	m := model.NewAnalyticsOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("CreateTables: %v", err)
	}
	return m, db
}

func newTestClient(url string, outbox *model.AnalyticsOutboxModel) *client {
	return &client{
		url:    url + "/internal/events",
		client: &http.Client{Timeout: time.Second},
		outbox: outbox,
	}
}

// NewRelay 构造守卫：outbox 空或客户端非 *client → nil；非法周期回退默认。
func TestRelay_NewRelayGuards(t *testing.T) {
	outbox, _ := newRelayOutbox(t)
	if r := NewRelay(nil, newTestClient("http://x", nil), time.Minute, 10, time.Hour); r != nil {
		t.Fatalf("nil outbox should yield nil relay, got %+v", r)
	}
	if r := NewRelay(outbox, fakeClient{}, time.Minute, 10, time.Hour); r != nil {
		t.Fatalf("non-*client should yield nil relay, got %+v", r)
	}
	r := NewRelay(outbox, newTestClient("http://x", outbox), 0, 0, 0)
	if r == nil {
		t.Fatal("valid inputs should yield relay")
	}
	if r.interval != time.Minute || r.maxAttempts != 10 || r.retention != 7*24*time.Hour {
		t.Fatalf("defaults not applied: interval=%v max=%d retention=%v", r.interval, r.maxAttempts, r.retention)
	}
}

// RelayOnce 投递成功：post 200 → MarkDelivered。
func TestRelayOnce_Delivers(t *testing.T) {
	outbox, _ := newRelayOutbox(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	if err := outbox.Enqueue("e1", "download", `{"event_id":"e1"}`, nowUTC()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	r := NewRelay(outbox, newTestClient(ts.URL, outbox), time.Minute, 10, time.Hour)
	attempted, delivered := r.RelayOnce(context.Background())
	if attempted != 1 || delivered != 1 {
		t.Fatalf("attempted=%d delivered=%d, want 1/1", attempted, delivered)
	}
	rows, err := outbox.ListRetry(nowUTC(), 100)
	if err != nil || len(rows) != 0 {
		t.Fatalf("delivered row should not be retryable: rows=%d err=%v", len(rows), err)
	}
}

// RelayOnce 重试耗尽 → MarkDropped（行已 attempts=maxAttempts-1 且到期，
// 本次失败即达上限）。
func TestRelayOnce_RetriesThenDrops(t *testing.T) {
	outbox, db := newRelayOutbox(t)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()
	if err := outbox.Enqueue("e2", "download", `{"event_id":"e2"}`, nowUTC()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// 预置到 maxAttempts-1 次并到期，本次失败即触顶 → dropped
	if _, err := db.Exec(
		`UPDATE analytics_outbox SET attempts=1, next_retry_at='2000-01-01T00:00:00Z' WHERE event_id='e2'`); err != nil {
		t.Fatalf("preseed attempts: %v", err)
	}
	r := NewRelay(outbox, newTestClient(ts.URL, outbox), time.Minute, 2, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("relay attempted=%d delivered=%d, want 1/0", a, d)
	}
	rows, err := outbox.ListRetry(nowUTC(), 100)
	if err != nil || len(rows) != 0 {
		t.Fatalf("dropped row should not be retryable: rows=%d err=%v", len(rows), err)
	}
}

// RelayOnce 空接收者或扫描失败 → (0,0)，不 panic。
func TestRelayOnce_Noop(t *testing.T) {
	var r *Relay
	if a, d := r.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("nil relay: attempted=%d delivered=%d, want 0/0", a, d)
	}
	outbox, db := newRelayOutbox(t)
	if err := outbox.Enqueue("e3", "download", `{}`, nowUTC()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	db.Close() // 扫描失败
	r = NewRelay(outbox, newTestClient("http://x", outbox), time.Minute, 2, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("scan error: attempted=%d delivered=%d, want 0/0", a, d)
	}
}

// Run 周期循环：ctx 取消即退出，顺带清理已终态行。
func TestRelay_RunStopsOnCancel(t *testing.T) {
	outbox, _ := newRelayOutbox(t)
	r := NewRelay(outbox, newTestClient("http://127.0.0.1:1", outbox), time.Millisecond, 2, time.Hour)
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
