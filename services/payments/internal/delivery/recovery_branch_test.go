package delivery

// recovery.go 剩余补漏分支：NewRecovery 默认 interval（<=0 → 1 分钟）、
// 扫描失败（关库 → ListRetry 报错 → 记日志返回 0）、发货失败（游戏服 500 →
// Dispatch 失败 → 记日志）与 Run 周期轮次（attempted>0 日志臂）。

import (
	"context"
	"testing"
	"time"
)

func TestRecovery_DefaultIntervalAndScanFailed(t *testing.T) {
	g := newGameStub(t, "gs-secret")
	db, deliveries, _, d := newStack(t, "file:"+t.TempDir()+"/rec.db", g.srv.URL, time.Minute)
	rec := NewRecovery(deliveries, d, 0) // interval<=0 → 默认 1 分钟
	if a, dl := rec.RecoverOnce(context.Background()); a != 0 || dl != 0 {
		t.Fatalf("empty scan = (%d,%d), want (0,0)", a, dl)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	if a, dl := rec.RecoverOnce(context.Background()); a != 0 || dl != 0 {
		t.Fatalf("scan after close = (%d,%d), want (0,0)", a, dl)
	}
}

// Run 周期轮次：游戏服 500 → 发货失败记日志臂（attempted>0 日志臂）。
func TestRecovery_RunDispatchFailureLog(t *testing.T) {
	g := newGameStub(t, "gs-secret")
	_, deliveries, _, d := newStack(t, "file:"+t.TempDir()+"/rec2.db", g.srv.URL, time.Minute)
	seedTarget(t, deliveries, "order_rec")
	g.fail.Store(true) // 游戏服 500 → Dispatch 失败 → 恢复日志臂
	rec := NewRecovery(deliveries, d, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		rec.Run(ctx)
		close(done)
	}()
	deadline := time.Now().Add(3 * time.Second)
	for g.Calls() == 0 {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("recovery loop never attempted dispatch")
		}
		time.Sleep(10 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}
