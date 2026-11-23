package analytics

// GP-402 事实事件 outbox 重投 + 客户端覆盖：nil 装配/默认回退、扫描失败、
// 各 Mark 写失败（best-effort 仅日志）、退避乘数封顶、Run 周期与 ctx 取消；
// 客户端 Emit 直投/失败/outbox 补偿、EmitAsync、Enabled、noop 语义。
// DB 失败臂用关库/触发器（RAISE(ABORT)）驱动，均诚实可达，不伪造。

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tappi/tappi/services/user-service/model"

	_ "github.com/mattn/go-sqlite3"
)

// anEnv analytics outbox 测试环境：模型 + 原始连接。
type anEnv struct {
	m   *model.AnalyticsOutboxModel
	raw *sql.DB
}

func newANEnv(t *testing.T) *anEnv {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/analytics.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := model.NewAnalyticsOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create outbox tables: %v", err)
	}
	return &anEnv{m: m, raw: db}
}

func (e *anEnv) closeDB() { _ = e.raw.Close() }

func (e *anEnv) addTrigger(t *testing.T, ddl string) {
	t.Helper()
	if _, err := e.raw.Exec(ddl); err != nil {
		t.Fatalf("add trigger: %v", err)
	}
}

// stubPanel data-panel 摄入存根：fail 时返 500（驱动重投失败臂）。
// handler 与测试 goroutine 并发读写，须持锁（-race）。
type stubPanel struct {
	mu    sync.Mutex
	fail  bool
	calls int
}

func (s *stubPanel) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		s.calls++
		fail := s.fail
		s.mu.Unlock()
		if fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
}

// calls 原子读调用次数（-race 安全）。
func (s *stubPanel) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// setFail 原子切换成败（-race 安全）。
func (s *stubPanel) setFail(fail bool) {
	s.mu.Lock()
	s.fail = fail
	s.mu.Unlock()
}

// newTestClient 启用态客户端（非 noop，可被 NewRelay 接受）。
func newTestClient(url string, outbox *model.AnalyticsOutboxModel) *client {
	return NewClient(url, "tok", 2000, outbox).(*client)
}

func TestNewClient_NoopWhenUnset(t *testing.T) {
	// baseURL 空 → noop：Emit/EmitAsync 均无动作，Enabled=false，
	// 且 NewRelay 不接受 noop（类型断言 !ok → nil）。
	c := NewClient("", "tok", 2000, nil)
	if c.Enabled() {
		t.Fatal("NewClient(\"\").Enabled() = true, want false（noop）")
	}
	c.Emit(Event{EventId: "e1", EventType: EventRegister})
	c.EmitAsync(Event{EventId: "e2", EventType: EventLogin})
	time.Sleep(20 * time.Millisecond) // EmitAsync 的 goroutine 收敛
	if r := NewRelay(newANEnv(t).m, c, time.Minute, 2, time.Hour); r != nil {
		t.Fatal("NewRelay(noop) = non-nil, want nil（noop 不启动扫描）")
	}
}

func TestClient_Enabled(t *testing.T) {
	env := newANEnv(t)
	if !newTestClient("http://127.0.0.1:1", env.m).Enabled() {
		t.Fatal("enabled client.Enabled() = false, want true")
	}
}

func TestClient_Emit_DeliveredAndOutboxMarked(t *testing.T) {
	// 直投成功 → outbox 行置 delivered（不再重投）。
	env := newANEnv(t)
	stub := &stubPanel{}
	srv := stub.server()
	t.Cleanup(srv.Close)
	c := newTestClient(srv.URL, env.m)
	c.Emit(Event{EventId: "ev-ok", EventType: EventRegister, UserId: 1})

	if stub.callCount() != 1 {
		t.Fatalf("panel calls = %d, want 1", stub.callCount())
	}
	rows, err := env.m.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339), 100)
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows=%+v err=%v, want 已 delivered 清空重投队列", rows, err)
	}
}

func TestClient_Emit_PostFailLeavesPending(t *testing.T) {
	// 直投失败 → 行留 pending（attempts=0）待 Relay 重投。
	env := newANEnv(t)
	stub := &stubPanel{fail: true}
	srv := stub.server()
	t.Cleanup(srv.Close)
	c := newTestClient(srv.URL, env.m)
	c.Emit(Event{EventId: "ev-fail", EventType: EventLogin, UserId: 2})

	rows, err := env.m.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339), 100)
	if err != nil || len(rows) != 1 || rows[0].Status != model.AnalyticsOutboxPending {
		t.Fatalf("rows=%+v err=%v, want 1 条 pending（直投失败留痕）", rows, err)
	}
}

func TestClient_Emit_NoOutbox(t *testing.T) {
	// outbox 为 nil：仅直投，不触碰队列（无本地补偿模式）。
	stub := &stubPanel{}
	srv := stub.server()
	t.Cleanup(srv.Close)
	c := newTestClient(srv.URL, nil)
	c.Emit(Event{EventId: "ev-nq", EventType: EventRegister})
	if stub.callCount() != 1 {
		t.Fatalf("panel calls = %d, want 1（无 outbox 仍直投）", stub.callCount())
	}
}

func TestClient_Emit_EnqueueAndMarkDeliveredDBErrors(t *testing.T) {
	// outbox 写失败（关库）：Emit 仍尝试直投（写失败仅记日志），直投成功不 panic。
	env := newANEnv(t)
	stub := &stubPanel{}
	srv := stub.server()
	t.Cleanup(srv.Close)
	c := newTestClient(srv.URL, env.m)
	env.closeDB()
	c.Emit(Event{EventId: "ev-dberr", EventType: EventRegister})
	if stub.callCount() != 1 {
		t.Fatalf("panel calls = %d, want 1（队列写失败仍直投）", stub.callCount())
	}
}

func TestClient_Post_TransportError(t *testing.T) {
	// 连接拒绝：post 传输失败（直投与 Relay 重投共用该路径），失败仅记日志。
	stub := &stubPanel{}
	c := newTestClient("http://127.0.0.1:1", nil) // 端口未监听
	c.Emit(Event{EventId: "ev-tx", EventType: EventRegister})
	if stub.callCount() != 0 {
		t.Fatalf("panel calls = %d, want 0（不可达未触达）", stub.callCount())
	}
}

func TestClient_Emit_BadURLNewRequest(t *testing.T) {
	// post 的 NewRequest 失败臂：URL 含非法控制字符（NewClient 不校验 URL）。
	c := &client{url: "http://a\x01b/internal/events", token: "tok",
		client: &http.Client{Timeout: time.Second}}
	c.Emit(Event{EventId: "ev-bad", EventType: EventRegister})
}

func TestClient_EmitAsync_Delivers(t *testing.T) {
	// EmitAsync fire-and-forget：异步最终投递成功。
	env := newANEnv(t)
	stub := &stubPanel{}
	srv := stub.server()
	t.Cleanup(srv.Close)
	c := newTestClient(srv.URL, env.m)
	c.EmitAsync(Event{EventId: "ev-async", EventType: EventLogin})
	deadline := time.Now().Add(2 * time.Second)
	for stub.callCount() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if stub.callCount() != 1 {
		t.Fatalf("panel calls = %d, want 1（异步投递）", stub.callCount())
	}
}

// ---- Relay ----

func TestNewRelay_NilAndDefaults(t *testing.T) {
	env := newANEnv(t)
	c := newTestClient("http://127.0.0.1:1", env.m)

	if r := NewRelay(nil, c, time.Minute, 2, time.Hour); r != nil {
		t.Fatalf("NewRelay(nil outbox) = %v, want nil", r)
	}
	if r := NewRelay(env.m, noop{}, time.Minute, 2, time.Hour); r != nil {
		t.Fatalf("NewRelay(noop client) = %v, want nil", r)
	}
	if r := NewRelay(env.m, c, 0, 0, 0); r == nil {
		t.Fatal("NewRelay(0,0,0) = nil, want 已装配（默认回退）")
	}
	if r := NewRelay(env.m, c, -1, -2, -3); r == nil {
		t.Fatal("NewRelay(负值) = nil, want 已装配")
	}
}

func TestRelayOnce_NilReceiver(t *testing.T) {
	var r *Relay
	if a, d := r.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("RelayOnce = %d/%d, want 0/0", a, d)
	}
}

func TestRelayOnce_ListRetryDBError(t *testing.T) {
	env := newANEnv(t)
	env.closeDB()
	r := NewRelay(env.m, newTestClient("http://127.0.0.1:1", env.m), time.Minute, 2, time.Hour)
	if r == nil {
		t.Fatal("NewRelay = nil")
	}
	if a, d := r.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("RelayOnce = %d/%d, want 0/0（扫描失败）", a, d)
	}
}

func TestRelayOnce_ExhaustsAndDelivers(t *testing.T) {
	env := newANEnv(t)
	stub := &stubPanel{}
	srv := stub.server()
	t.Cleanup(srv.Close)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := env.m.Enqueue("ev-1", EventRegister, `{"events":[]}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	r := NewRelay(env.m, newTestClient(srv.URL, env.m), time.Minute, 10, time.Hour)
	if r == nil {
		t.Fatal("NewRelay = nil")
	}

	stub.setFail(false)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 1 {
		t.Fatalf("attempted=%d delivered=%d, want 1/1", a, d)
	}
	rows, err := env.m.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339), 100)
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows=%+v err=%v, want 已 delivered 清空", rows, err)
	}

	// 失败 + maxAttempts=1 → 首次失败即耗尽 dropped
	stub.setFail(true)
	if err := env.m.Enqueue("ev-2", EventLogin, `{"events":[]}`, past); err != nil {
		t.Fatalf("enqueue 2: %v", err)
	}
	r1 := NewRelay(env.m, newTestClient(srv.URL, env.m), time.Minute, 1, time.Hour)
	if a, d := r1.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0", a, d)
	}
	if a, d := r1.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 0/0（已 dropped）", a, d)
	}
}

func TestRelayOnce_MarkDroppedDBError(t *testing.T) {
	env := newANEnv(t)
	stub := &stubPanel{fail: true}
	srv := stub.server()
	t.Cleanup(srv.Close)
	env.addTrigger(t, `CREATE TRIGGER trg_drop BEFORE UPDATE ON analytics_outbox
		WHEN NEW.status='dropped' BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := env.m.Enqueue("ev-d", EventRegister, `{"events":[]}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	r := NewRelay(env.m, newTestClient(srv.URL, env.m), time.Minute, 1, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0", a, d)
	}
}

func TestRelayOnce_MarkFailedDBErrorAndBackoffCap(t *testing.T) {
	env := newANEnv(t)
	stub := &stubPanel{fail: true}
	srv := stub.server()
	t.Cleanup(srv.Close)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := env.m.Enqueue("ev-mf", EventLogin, `{"events":[]}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// 先播种 attempts=7（无触发器时不中止），再挂触发器驱动 MarkFailed 失败臂。
	if err := env.m.MarkFailed("ev-mf", 7, "prev", past); err != nil {
		t.Fatalf("seed attempts: %v", err)
	}
	env.addTrigger(t, `CREATE TRIGGER trg_mf BEFORE UPDATE ON analytics_outbox
		WHEN NEW.status='pending' BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	r := NewRelay(env.m, newTestClient(srv.URL, env.m), time.Minute, 10, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0", a, d)
	}
	rows, err := env.m.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339), 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%+v err=%v, want 1 条仍 pending", rows, err)
	}
	if rows[0].Attempts != 7 || rows[0].Status != model.AnalyticsOutboxPending {
		t.Fatalf("attempts=%d status=%s, want 7/pending（写失败原样保留、仍可重试）",
			rows[0].Attempts, rows[0].Status)
	}
}

func TestRelayOnce_MarkDeliveredDBError(t *testing.T) {
	env := newANEnv(t)
	stub := &stubPanel{}
	srv := stub.server()
	t.Cleanup(srv.Close)
	env.addTrigger(t, `CREATE TRIGGER trg_md BEFORE UPDATE ON analytics_outbox
		WHEN NEW.status='delivered' BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := env.m.Enqueue("ev-md", EventRegister, `{"events":[]}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	r := NewRelay(env.m, newTestClient(srv.URL, env.m), time.Minute, 10, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 1 {
		t.Fatalf("attempted=%d delivered=%d, want 1/1（受理判定完成，写失败仅日志）", a, d)
	}
}

func TestRun_NilReceiver(t *testing.T) {
	var r *Relay
	r.Run(context.Background())
}

func TestRun_TickAndCancel(t *testing.T) {
	env := newANEnv(t)
	stub := &stubPanel{}
	srv := stub.server()
	t.Cleanup(srv.Close)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := env.m.Enqueue("ev-run-a", EventRegister, `{"events":[]}`, past); err != nil {
		t.Fatalf("enqueue A: %v", err)
	}
	oldDelivered := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	if err := env.m.Enqueue("ev-run-b", EventLogin, `{"events":[]}`, past); err != nil {
		t.Fatalf("enqueue B: %v", err)
	}
	if err := env.m.MarkDelivered("ev-run-b", oldDelivered); err != nil {
		t.Fatalf("mark delivered B: %v", err)
	}

	relay := NewRelay(env.m, newTestClient(srv.URL, env.m), 10*time.Millisecond, 10, time.Hour)
	if relay == nil {
		t.Fatal("NewRelay = nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { relay.Run(ctx); close(done) }()

	deadline := time.Now().Add(3 * time.Second)
	for {
		rows, err := env.m.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339), 100)
		if err == nil && len(rows) == 0 {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("tick 未达成，rows=%+v", rows)
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on ctx cancel")
	}
}

func TestRun_PruneDBError(t *testing.T) {
	env := newANEnv(t)
	stub := &stubPanel{}
	srv := stub.server()
	t.Cleanup(srv.Close)
	env.addTrigger(t, `CREATE TRIGGER trg_del BEFORE DELETE ON analytics_outbox
		BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	oldDelivered := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	if err := env.m.Enqueue("ev-prune", EventRegister, `{"events":[]}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := env.m.MarkDelivered("ev-prune", oldDelivered); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	relay := NewRelay(env.m, newTestClient(srv.URL, env.m), 10*time.Millisecond, 10, time.Hour)
	if relay == nil {
		t.Fatal("NewRelay = nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { relay.Run(ctx); close(done) }()
	time.Sleep(100 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on ctx cancel")
	}
}

func TestTruncate_Long(t *testing.T) {
	long := strings.Repeat("x", 600)
	if got := truncate(long, 500); len(got) != 500 {
		t.Fatalf("truncate(len=%d) = len %d, want 500", len(long), len(got))
	}
	if got := truncate("short", 500); got != "short" {
		t.Fatalf("truncate(short) = %q, want 原样", got)
	}
}
