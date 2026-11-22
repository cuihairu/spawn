package notify

// notify.go 剩余补漏分支：Publish 的 Enqueue 失败（readonly outbox：写败仍
// 继续直投）与 MarkDelivered 失败两个 fire-and-forget 记日志臂、noop 与
// 真客户端的 Enabled() 两臂；relay.Run 的 Prune 失败臂（readonly）与
// Prune 回收行数 >0 日志臂（delivered_at 拨到远古的已投递行）。
//
// 登记不可达（不测）：json.Marshal（纯结构体）失败臂、http.NewRequest
// 失败臂（url 由 NewClient 拼接恒合法）、crypto/rand.Read 失败臂。

import (
	"context"
	"testing"
	"time"
)

func TestPublish_OutboxWriteFailedArms(t *testing.T) {
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
	c.Publish(Notification{UserIds: []int64{7}, Type: "statement_dispute_resolved", TargetId: 1, Content: "c"})
}

func TestPublish_NoopArms(t *testing.T) {
	// baseURL 空 → noop：Publish 零动作、Enabled=false
	c := NewClient("", "tok", 1000, nil)
	if c.Enabled() {
		t.Fatal("noop Enabled = true, want false")
	}
	c.Publish(Notification{UserIds: []int64{7}, Type: "t"})
}

func TestRelay_RunPruneArms(t *testing.T) {
	// 回收 >0 日志臂：delivered_at 拨到远古 → 已过保留窗口
	m, db, _ := newRelayOutbox(t)
	if err := m.Enqueue("n-prune", "statement_dispute_resolved", `{}`, nowUTC()); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	if err := m.MarkDelivered("n-prune", "2000-01-01T00:00:00Z"); err != nil {
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
