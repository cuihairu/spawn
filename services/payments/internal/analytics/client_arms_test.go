package analytics

// client.go 剩余补漏分支：Emit 的 Enqueue 失败（readonly outbox：写败仍继续
// 直投）与 MarkDelivered 失败两个 fire-and-forget 记日志臂、noop 与真客户端
// 的 Enabled() 两臂、UserIDOf 解析失败返回 0 臂；relay.Run 的 Prune 失败臂
// （readonly）与 Prune 回收行数 >0 日志臂（delivered_at 拨到远古的已投递行）。
//
// 登记不可达（不测）：json.Marshal（纯结构体）失败臂、http.NewRequest 失败
// 臂（url 由 NewClient 拼接恒合法）。

import (
	"context"
	"testing"
	"time"
)

func TestEmit_OutboxWriteFailedArms(t *testing.T) {
	// readonly outbox：Enqueue 败（记日志仍直投）→ post 成功 → MarkDelivered 败
	_, db, path := newRelayOutbox(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	ro := roOutbox(t, path)
	ts := newPostServer(t, 200)
	c := NewClient(ts.URL, "tok", 1000, ro)
	if !c.Enabled() {
		t.Fatal("real client Enabled = false, want true")
	}
	c.Emit(Event{EventId: "e1", EventType: EventOrderPaid, UserId: 7, OrderId: "o1", AmountCents: 100})
}

func TestEmit_NoopArms(t *testing.T) {
	// baseURL 空 → noop：Emit 零动作、Enabled=false
	c := NewClient("", "tok", 1000, nil)
	if c.Enabled() {
		t.Fatal("noop Enabled = true, want false")
	}
	c.Emit(Event{EventId: "e2", EventType: EventOrderPaid})
}

func TestUserIDOf_Arms(t *testing.T) {
	// 非数字 → 0；负数 → 0；合法 → 原值
	if got := UserIDOf("abc"); got != 0 {
		t.Fatalf("abc = %d, want 0", got)
	}
	if got := UserIDOf("-5"); got != 0 {
		t.Fatalf("-5 = %d, want 0", got)
	}
	if got := UserIDOf("42"); got != 42 {
		t.Fatalf("42 = %d, want 42", got)
	}
}

func TestRelay_RunPruneArms(t *testing.T) {
	// 回收 >0 日志臂：delivered_at 拨到远古 → 已过保留窗口
	m, db, _ := newRelayOutbox(t)
	if err := m.Enqueue("e-prune", EventOrderPaid, `{}`, nowUTC()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := m.MarkDelivered("e-prune", "2000-01-01T00:00:00Z"); err != nil {
		t.Fatalf("MarkDelivered: %v", err)
	}
	defer db.Close()
	ts := newPostServer(t, 200)
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
	// Prune 失败臂：readonly（每轮 ListRetry 读 0 行后 DELETE 报错）
	_, db2, path := newRelayOutbox(t)
	if err := db2.Close(); err != nil {
		t.Fatalf("close2: %v", err)
	}
	ro := roOutbox(t, path)
	rr := NewRelay(ro, NewClient(ts.URL, "tok", 1000, ro), time.Millisecond, 2, time.Hour)
	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan struct{})
	go func() {
		rr.Run(ctx2)
		close(done2)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel2()
	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("Run(prune fail) did not stop after cancel")
	}
}
