package couponrec

// release.go 剩余补漏分支：NewRelease 默认值（ttl/interval<=0）、扫描失败
// （关库 → ReleaseExpiredLocked 报错 → 记日志返回 0）与 Run 周期轮次
// （released>0 日志臂）。

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/model"

	_ "github.com/mattn/go-sqlite3"
)

func newReleaseModel(t *testing.T) (*model.CouponModel, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "coupon-release.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	m := model.NewCouponModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return m, db
}

// NewRelease 取默认（ttl=1h / interval=5min）且空扫描返回 0；关库后扫描
// 失败走记日志分支。
func TestRelease_DefaultsAndScanFailed(t *testing.T) {
	m, db := newReleaseModel(t)
	r := NewRelease(m, 0, 0) // ttl/interval<=0 → 默认
	if n, orders := r.ReleaseOnce(context.Background(), time.Now().UTC()); n != 0 || orders != nil {
		t.Fatalf("empty scan = (%d,%v), want (0,nil)", n, orders)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	if n, _ := r.ReleaseOnce(context.Background(), time.Now().UTC()); n != 0 {
		t.Fatalf("scan after close = %d, want 0", n)
	}
}

// Run 周期轮次：过期锁券被释放（released>0 → 日志臂），ctx 取消即退出。
func TestRelease_RunReleasesExpiredLock(t *testing.T) {
	m, db := newReleaseModel(t)
	defer db.Close()
	uc, created, err := m.ClaimCoupon(&model.UserCoupon{CouponId: 1, UserId: 7, RequestId: "req-r1"})
	if err != nil || !created {
		t.Fatalf("claim: created=%v err=%v", created, err)
	}
	if err := m.LockUserCoupon(uc.Id, "order-rel"); err != nil {
		t.Fatalf("lock: %v", err)
	}
	old := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	if _, err := db.Exec(`UPDATE user_coupons SET locked_at=? WHERE id=?`, old, uc.Id); err != nil {
		t.Fatalf("age lock: %v", err)
	}
	r := NewRelease(m, time.Hour, time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.Run(ctx)
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := m.GetUserCoupon(uc.Id)
		if err == nil && got.Status == model.UserCouponStatusClaimed {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("release loop never released the expired lock")
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}
