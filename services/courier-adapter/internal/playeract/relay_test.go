package playeract

// Relay 重投覆盖（GP-303 活跃上报 outbox 续投）：nil 装配/默认回退、扫描失败、
// 各 Mark 写失败（best-effort 仅日志）、退避乘数封顶、Run 周期 tick 与 ctx 取消。
// DB 失败臂用关库/触发器（RAISE(ABORT)）驱动，均诚实可达，不伪造。

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewRelay_NilAndDefaults(t *testing.T) {
	env := newAOEnv(t)
	c := NewClient("http://127.0.0.1:1", "tok", 50)

	if r := NewRelay(nil, c, time.Minute, 2, time.Hour); r != nil {
		t.Fatalf("NewRelay(nil outbox) = %v, want nil", r)
	}
	if r := NewRelay(env.m, nil, time.Minute, 2, time.Hour); r != nil {
		t.Fatalf("NewRelay(nil client) = %v, want nil", r)
	}
	// 非正 interval/maxAttempts/retention 各自回退默认（不 panic，仍装配）。
	if r := NewRelay(env.m, c, 0, 0, 0); r == nil {
		t.Fatal("NewRelay(0,0,0) = nil, want 已装配（默认回退）")
	}
	if r := NewRelay(env.m, c, -1, -2, -3); r == nil {
		t.Fatal("NewRelay(负值) = nil, want 已装配")
	}
}

func TestRelayOnce_NilReceiver(t *testing.T) {
	// nil 重投器（未装配）：RelayOnce 直接 0/0，不 panic。
	var r *Relay
	if a, d := r.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("RelayOnce = %d/%d, want 0/0", a, d)
	}
}

func TestRelayOnce_ListRetryDBError(t *testing.T) {
	// ListRetry 扫描失败（关库）→ 0/0（不误报、不 panic）。
	env := newAOEnv(t)
	env.closeDB()
	r := NewRelay(env.m, NewClient("http://127.0.0.1:1", "tok", 50), time.Minute, 2, time.Hour)
	if r == nil {
		t.Fatal("NewRelay = nil")
	}
	if a, d := r.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("RelayOnce = %d/%d, want 0/0（扫描失败）", a, d)
	}
}

func TestRelayOnce_MarkDroppedDBError(t *testing.T) {
	// 重试耗尽置 dropped 但写失败：仅记日志（best-effort），继续处理其余行。
	stub := &promotionsStub{}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	setFail(stub, true)
	env := newAOEnv(t)
	env.addTrigger(t, `CREATE TRIGGER trg_drop BEFORE UPDATE ON player_activity_outbox
		WHEN NEW.status='dropped' BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := env.m.Enqueue("act:drop:g1:t", "u9", "g1", "t9", `{"account_id":"u9"}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// maxAttempts=1 → 首次失败即耗尽
	r := NewRelay(env.m, NewClient(srv.URL, "tok", 2000), time.Minute, 1, time.Hour)
	if r == nil {
		t.Fatal("NewRelay = nil")
	}
	attempted, delivered := r.RelayOnce(context.Background())
	if attempted != 1 || delivered != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0", attempted, delivered)
	}
}

func TestRelayOnce_MarkFailedDBError(t *testing.T) {
	// 未耗尽但 MarkFailed 写失败：仅记日志（best-effort），不 panic。
	stub := &promotionsStub{}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	setFail(stub, true)
	env := newAOEnv(t)
	env.addTrigger(t, `CREATE TRIGGER trg_mfr BEFORE UPDATE ON player_activity_outbox
		WHEN NEW.status='pending' BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := env.m.Enqueue("act:mf:g1:t", "u8", "g1", "t8", `{"account_id":"u8"}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	r := NewRelay(env.m, NewClient(srv.URL, "tok", 2000), time.Minute, 10, time.Hour)
	if r == nil {
		t.Fatal("NewRelay = nil")
	}
	attempted, delivered := r.RelayOnce(context.Background())
	if attempted != 1 || delivered != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0", attempted, delivered)
	}
}

func TestRelayOnce_BackoffCap(t *testing.T) {
	// 退避乘数封顶 6：attempts>=7 时 mult 固定 6（不无限增长），MarkFailed 正常写。
	stub := &promotionsStub{}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	setFail(stub, true)
	env := newAOEnv(t)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := env.m.Enqueue("act:cap:g1:t", "u7", "g1", "t7", `{"account_id":"u7"}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// 预置 attempts=7（已 7 次失败仍 pending，模拟长退避中途）
	if err := env.m.MarkFailed("act:cap:g1:t", 7, "prev", past); err != nil {
		t.Fatalf("seed attempts: %v", err)
	}
	r := NewRelay(env.m, NewClient(srv.URL, "tok", 2000), time.Minute, 10, time.Hour)
	if r == nil {
		t.Fatal("NewRelay = nil")
	}
	attempted, delivered := r.RelayOnce(context.Background())
	if attempted != 1 || delivered != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0", attempted, delivered)
	}
	rows, err := env.m.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %+v err=%v, want 1 条仍 pending", rows, err)
	}
	if rows[0].Attempts != 8 {
		t.Fatalf("attempts = %d, want 8（第 8 次失败）", rows[0].Attempts)
	}
}

func TestRelayOnce_MarkDeliveredDBError(t *testing.T) {
	// 重投成功但 MarkDelivered 写失败：受理判定已完成（delivered=1），写失败仅
	// 记日志（best-effort）——不因写失败把已受理当失败。
	stub := &promotionsStub{}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	env := newAOEnv(t)
	env.addTrigger(t, `CREATE TRIGGER trg_mdr BEFORE UPDATE ON player_activity_outbox
		WHEN NEW.status='delivered' BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if err := env.m.Enqueue("act:md:g1:t", "u6", "g1", "t6", `{"account_id":"u6"}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	r := NewRelay(env.m, NewClient(srv.URL, "tok", 2000), time.Minute, 10, time.Hour)
	if r == nil {
		t.Fatal("NewRelay = nil")
	}
	attempted, delivered := r.RelayOnce(context.Background())
	if attempted != 1 || delivered != 1 {
		t.Fatalf("attempted=%d delivered=%d, want 1/1（受理判定完成，写失败仅日志）", attempted, delivered)
	}
}

func TestRun_PruneDBError(t *testing.T) {
	// Prune 清理失败：仅记日志（best-effort），Run 继续下一轮，不 panic、不退出。
	stub := &promotionsStub{}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	env := newAOEnv(t)
	env.addTrigger(t, `CREATE TRIGGER trg_del BEFORE DELETE ON player_activity_outbox
		BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	oldDelivered := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	if err := env.m.Enqueue("act:prune:g1:t", "u3", "g1", "t3", `{"account_id":"u3"}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := env.m.MarkDelivered("act:prune:g1:t", oldDelivered); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	relay := NewRelay(env.m, NewClient(srv.URL, "tok", 2000), 10*time.Millisecond, 10, time.Hour)
	if relay == nil {
		t.Fatal("NewRelay = nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { relay.Run(ctx); close(done) }()
	time.Sleep(100 * time.Millisecond) // 数轮 tick：Prune 失败日志被驱动
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on ctx cancel")
	}
}

func TestRun_NilReceiver(t *testing.T) {
	// nil 重投器（未装配）：Run 直接返回，不 panic（与 refundexec Run 同款守卫）。
	var r *Relay
	r.Run(context.Background())
}

func TestRun_TickAndCancel(t *testing.T) {
	// 周期 tick：到期行被重投 delivered（attempted>0 日志）+ 已终态旧行被清理
	// （prune n>0 日志）；ctx 取消即退出。
	stub := &promotionsStub{}
	srv := httptest.NewServer(stub.handler())
	t.Cleanup(srv.Close)
	env := newAOEnv(t)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	// rowA：到期 pending → 首 tick 重投成功
	if err := env.m.Enqueue("act:run:g1:ta", "u1", "g1", "ta", `{"account_id":"u1"}`, past); err != nil {
		t.Fatalf("enqueue A: %v", err)
	}
	// rowB：已 delivered 且 delivered_at 很旧 → 首 tick 被 Prune 清理
	oldDelivered := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339Nano)
	if err := env.m.Enqueue("act:run:g1:tb", "u2", "g1", "tb", `{"account_id":"u2"}`, past); err != nil {
		t.Fatalf("enqueue B: %v", err)
	}
	if err := env.m.MarkDelivered("act:run:g1:tb", oldDelivered); err != nil {
		t.Fatalf("mark delivered B: %v", err)
	}

	relay := NewRelay(env.m, NewClient(srv.URL, "tok", 2000), 10*time.Millisecond, 10, time.Hour)
	if relay == nil {
		t.Fatal("NewRelay = nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { relay.Run(ctx); close(done) }()

	// 轮询：rowA 已 delivered + rowB 已 Prune → 重投队列空。
	deadline := time.Now().Add(3 * time.Second)
	for {
		rows, err := env.m.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339Nano), 100)
		if err == nil && len(rows) == 0 {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("tick 未达成（rowA 重投/rowB 清理），rows=%+v", rows)
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
