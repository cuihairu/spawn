package model

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// newCouponReleaseModel 打开临时文件 SQLite 并建券三表（P0 修复回归：超时
// 释放 + 限领 claimed-only 计数）。
func newCouponReleaseModel(t *testing.T) *CouponModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "coupons.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	m := NewCouponModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return m
}

// mustClaim 领一张券并断言新插入（requestID 唯一即可）。
func mustClaim(t *testing.T, m *CouponModel, couponID, userID int64, requestID string) *UserCoupon {
	t.Helper()
	uc, created, err := m.ClaimCoupon(&UserCoupon{
		CouponId: couponID, UserId: userID, RequestId: requestID,
	})
	if err != nil || !created {
		t.Fatalf("claim coupon: created=%v err=%v", created, err)
	}
	return uc
}

func TestCouponModel_ReleaseExpiredLocked(t *testing.T) {
	m := newCouponReleaseModel(t)

	// 三张锁券：到期（A）、未到期（B）、locked_at 空（C，老行/异常态）。
	a := mustClaim(t, m, 1, 7, "req-a")
	b := mustClaim(t, m, 1, 7, "req-b")
	c := mustClaim(t, m, 1, 7, "req-c")
	if err := m.LockUserCoupon(a.Id, "order-expired"); err != nil {
		t.Fatalf("lock a: %v", err)
	}
	if err := m.LockUserCoupon(b.Id, "order-fresh"); err != nil {
		t.Fatalf("lock b: %v", err)
	}
	if err := m.LockUserCoupon(c.Id, "order-blank"); err != nil {
		t.Fatalf("lock c: %v", err)
	}
	// 手动把 A 的 locked_at 拨到 2h 前、C 的 locked_at 清空（模拟老行）。
	now := time.Now().UTC()
	old := now.Add(-2 * time.Hour).Format(time.RFC3339)
	if _, err := m.db.Exec(
		`UPDATE user_coupons SET locked_at=? WHERE id=?`, old, a.Id); err != nil {
		t.Fatalf("age a: %v", err)
	}
	if _, err := m.db.Exec(`UPDATE user_coupons SET locked_at='' WHERE id=?`, c.Id); err != nil {
		t.Fatalf("blank c: %v", err)
	}

	olderThan := now.Add(-30 * time.Minute).Format(time.RFC3339) // A（now-2h）早于它，B（now）晚于它
	released, orders, err := m.ReleaseExpiredLocked(olderThan)
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released != 1 || len(orders) != 1 || orders[0] != "order-expired" {
		t.Fatalf("released=%d orders=%v, want 1 [order-expired]", released, orders)
	}

	// A 回 claimed 且锁字段清空；B 仍 locked；C 不被动（locked_at 空不命中）。
	ua, err := m.GetUserCoupon(a.Id)
	if err != nil || ua.Status != UserCouponStatusClaimed || ua.LockedOrderId != "" {
		t.Fatalf("a after release: %+v err=%v", ua, err)
	}
	ub, err := m.GetUserCoupon(b.Id)
	if err != nil || ub.Status != UserCouponStatusLocked || ub.LockedOrderId != "order-fresh" {
		t.Fatalf("b after release: %+v err=%v", ub, err)
	}
	uc, err := m.GetUserCoupon(c.Id)
	if err != nil || uc.Status != UserCouponStatusLocked || uc.LockedOrderId != "order-blank" {
		t.Fatalf("c after release: %+v err=%v", uc, err)
	}

	// 幂等：再扫一轮不再命中。
	released, orders, err = m.ReleaseExpiredLocked(olderThan)
	if err != nil || released != 0 || len(orders) != 0 {
		t.Fatalf("re-scan: released=%d orders=%v err=%v, want 0", released, orders, err)
	}
}

func TestCouponModel_ReleaseExpiredLocked_UsedUntouched(t *testing.T) {
	m := newCouponReleaseModel(t)

	// used 券即使 locked_at 老也不释放（条件 status=locked）。
	u := mustClaim(t, m, 2, 9, "req-u")
	if err := m.LockUserCoupon(u.Id, "order-paid"); err != nil {
		t.Fatalf("lock u: %v", err)
	}
	if err := m.MarkCouponUsedByOrder("order-paid"); err != nil {
		t.Fatalf("mark used: %v", err)
	}
	if _, err := m.db.Exec(
		`UPDATE user_coupons SET locked_at=? WHERE id=?`, "2026-01-01T00:00:00Z", u.Id); err != nil {
		t.Fatalf("age u: %v", err)
	}

	released, _, err := m.ReleaseExpiredLocked("2026-11-12T00:00:00Z")
	if err != nil {
		t.Fatalf("release: %v", err)
	}
	if released != 0 {
		t.Fatalf("released=%d, want 0 (used not matched)", released)
	}
	got, err := m.GetUserCoupon(u.Id)
	if err != nil || got.Status != UserCouponStatusUsed {
		t.Fatalf("u after release: %+v err=%v", got, err)
	}
}

func TestCouponModel_CountUserCouponClaimed_OnlyClaimed(t *testing.T) {
	m := newCouponReleaseModel(t)

	claimed := mustClaim(t, m, 3, 11, "req-1")
	locked := mustClaim(t, m, 3, 11, "req-2")
	used := mustClaim(t, m, 3, 11, "req-3")
	if err := m.LockUserCoupon(locked.Id, "order-x"); err != nil {
		t.Fatalf("lock: %v", err)
	}
	if err := m.LockUserCoupon(used.Id, "order-y"); err != nil {
		t.Fatalf("lock: %v", err)
	}
	if err := m.MarkCouponUsedByOrder("order-y"); err != nil {
		t.Fatalf("mark used: %v", err)
	}

	// claimed-only：仅 1（claimed）；全状态计数：3（claimed+locked+used）。
	n, err := m.CountUserCouponClaimed(3, 11)
	if err != nil || n != 1 {
		t.Fatalf("claimed-only count=%d err=%v, want 1", n, err)
	}
	all, err := m.CountUserCouponClaims(3, 11)
	if err != nil || all != 3 {
		t.Fatalf("all-status count=%d err=%v, want 3", all, err)
	}

	// locked 超时释放回 claimed 后 claimed-only 计数回升（释放后仍受限领约束）。
	if err := m.LockUserCoupon(claimed.Id, "order-z"); err != nil {
		t.Fatalf("lock claimed: %v", err)
	}
	if _, err := m.db.Exec(
		`UPDATE user_coupons SET locked_at=? WHERE id=?`, "2026-01-01T00:00:00Z", claimed.Id); err != nil {
		t.Fatalf("age: %v", err)
	}
	if released, _, _ := m.ReleaseExpiredLocked("2026-11-12T00:00:00Z"); released != 2 {
		t.Fatalf("release=%d, want 2", released)
	}
	n, err = m.CountUserCouponClaimed(3, 11)
	if err != nil || n != 2 {
		t.Fatalf("claimed-only after release=%d err=%v, want 2", n, err)
	}
}
