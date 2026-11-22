package model

// couponmodel 剩余补漏分支：GetCouponByCode/GetUserCoupon/GetUserCouponByRequest
// 的查无（ErrXxxNotFound）与 Scan 失败臂，LockUserCoupon 的 UPDATE 失败与
// 非 claimed 冲突臂，ReleaseExpiredLocked 的 Scan 失败与 UPDATE 失败臂，
// UnlockUserCoupon 成功臂，ListLinksByOrder 的查行/查询失败/Scan 失败臂，
// ListUserCoupons 的 limit 钳制与 Scan 失败臂，ListCoupons 的 limit 钳制/
// status 过滤/Scan 失败臂，migrateCouponColumns 的首 ALTER 成功（return nil）
// 与返回 lastErr 臂。脏行一律写非过滤列。
//
// 登记不可达（不测）：351.4/409.3/533.4 的 MySQL 方言回退门（SQLite 语法
// 本就成功，syntax/near 门永不触发）；474.4 ReleaseExpiredLocked 的 Scan
// 失败（只扫 id=行号与字符串，脏不了）；480.3 rows.Err 中途出错。

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// couponArmsDB 建券三表并返回 (model, db, 文件路径)（路径供 ?mode=ro 只读重开）。
func couponArmsDB(t *testing.T) (*CouponModel, *sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "coupon.db")
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := NewCouponModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return m, db, path
}

// seedCoupon 落一张 published 券并返回其 id（CreateCoupon 回填 LastInsertId）。
func seedCoupon(t *testing.T, m *CouponModel, code string) int64 {
	t.Helper()
	c := &Coupon{Code: code, Name: "n", GameId: "g-a", SkuId: "s1", ThresholdCents: 100,
		DiscountType: CouponDiscountFixed, DiscountValue: 5, Stackable: false,
		PerUserLimit: 3, TotalCount: 100, StartAt: "2026-01-01T00:00:00Z",
		EndAt: "2099-01-01T00:00:00Z", Status: CouponStatusPublished, CreatedBy: 1}
	if err := m.CreateCoupon(c); err != nil {
		t.Fatalf("create coupon: %v", err)
	}
	return c.Id
}

// couponReadOnly 以 ?mode=ro 重开同一文件。
func couponReadOnly(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open readonly: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestCouponModel_GetCouponByCodeArms(t *testing.T) {
	m, db, _ := couponArmsDB(t)
	// 查无 → ErrCouponNotFound
	if _, err := m.GetCouponByCode("nope"); err != ErrCouponNotFound {
		t.Fatalf("missing code = %v, want ErrCouponNotFound", err)
	}
	// 命中成功臂（306）
	id := seedCoupon(t, m, "C-HIT")
	if got, err := m.GetCouponByCode("C-HIT"); err != nil || got.Id != id {
		t.Fatalf("hit = %v/%v, want id %d", got, err, id)
	}
	// Scan 失败：threshold_cents（非过滤列）写 text
	id = seedCoupon(t, m, "C-SCAN")
	if _, err := db.Exec(`UPDATE coupons SET threshold_cents='corrupt' WHERE id=?`, id); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := m.GetCouponByCode("C-SCAN"); err == nil {
		t.Fatal("scan fail = nil, want error")
	}
}

func TestCouponModel_GetUserCouponArms(t *testing.T) {
	m, db, _ := couponArmsDB(t)
	// 查无 → ErrUserCouponNotFound
	if _, err := m.GetUserCoupon(9999); err != ErrUserCouponNotFound {
		t.Fatalf("missing = %v, want ErrUserCouponNotFound", err)
	}
	uc := mustClaim(t, m, 1, 7, "r1")
	// Scan 失败：coupon_id 写 text（过滤列是 id）
	if _, err := db.Exec(`UPDATE user_coupons SET coupon_id='corrupt' WHERE id=?`, uc.Id); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := m.GetUserCoupon(uc.Id); err == nil {
		t.Fatal("scan fail = nil, want error")
	}
}

func TestCouponModel_GetUserCouponByRequestArms(t *testing.T) {
	m, _, _ := couponArmsDB(t)
	// 查无 → ErrUserCouponNotFound（Scan 失败臂不可达：可写脏的 int 列
	// coupon_id/user_id 都是过滤列，id 是 rowid 不可写 text——登记不测）
	if _, err := m.GetUserCouponByRequest(1, 999, "nope"); err != ErrUserCouponNotFound {
		t.Fatalf("missing = %v, want ErrUserCouponNotFound", err)
	}
	uc := mustClaim(t, m, 1, 7, "r1")
	if got, err := m.GetUserCouponByRequest(1, 7, "r1"); err != nil || got.Id != uc.Id {
		t.Fatalf("by request = %v/%v, want id %d", got, err, uc.Id)
	}
}

func TestCouponModel_LockUserCouponArms(t *testing.T) {
	m, _, path := couponArmsDB(t)
	uc := mustClaim(t, m, 1, 7, "r1")
	// 非 claimed 实例 → ErrCouponStateConflict（重复锁）
	if err := m.LockUserCoupon(uc.Id, "o1"); err != nil {
		t.Fatalf("lock: %v", err)
	}
	if err := m.LockUserCoupon(uc.Id, "o2"); err != ErrCouponStateConflict {
		t.Fatalf("relock = %v, want ErrCouponStateConflict", err)
	}
	// 只读：claimed 行在、UPDATE 失败 → err（447）
	uc2 := mustClaim(t, m, 1, 8, "r2")
	if err := NewCouponModel(couponReadOnly(t, path)).LockUserCoupon(uc2.Id, "o3"); err == nil {
		t.Fatal("lock ro = nil, want error")
	}
}

func TestCouponModel_ReleaseExpiredLockedArms(t *testing.T) {
	m, _, path := couponArmsDB(t)
	uc := mustClaim(t, m, 1, 7, "r1")
	if err := m.LockUserCoupon(uc.Id, "o1"); err != nil {
		t.Fatalf("lock: %v", err)
	}
	// 只读：锁定行在、UPDATE 失败 → return released,orderIDs,err（491）
	if _, _, err := NewCouponModel(couponReadOnly(t, path)).ReleaseExpiredLocked("2099-01-01T00:00:00Z"); err == nil {
		t.Fatal("release ro = nil, want error")
	}
	_ = m
}

func TestCouponModel_UnlockUserCouponArms(t *testing.T) {
	m, _, _ := couponArmsDB(t)
	uc := mustClaim(t, m, 1, 7, "r1")
	if err := m.LockUserCoupon(uc.Id, "o1"); err != nil {
		t.Fatalf("lock: %v", err)
	}
	// locked → claimed 回滚成功臂
	if err := m.UnlockUserCoupon(uc.Id); err != nil {
		t.Fatalf("unlock: %v", err)
	}
}

func TestCouponModel_ListLinksByOrderArms(t *testing.T) {
	m, db, _ := couponArmsDB(t)
	if err := m.LinkOrderCoupon(&CouponOrderLink{OrderId: "o1", CouponId: 1, UserCouponId: 5,
		DiscountCents: 5, Snapshot: "{}"}); err != nil {
		t.Fatalf("link: %v", err)
	}
	// 成功扫行（544/548/551/555/557）
	links, err := m.ListLinksByOrder("o1")
	if err != nil || len(links) != 1 {
		t.Fatalf("list links = %v/%v, want 1 row", links, err)
	}
	// 查询失败（546）
	if _, err := NewCouponModel(modelClosedDB(t)).ListLinksByOrder("o1"); err == nil {
		t.Fatal("list closed = nil, want error")
	}
	// Scan 失败：discount_cents（非过滤列）写 text（553）
	if _, err := db.Exec(`UPDATE coupon_order_links SET discount_cents='corrupt' WHERE order_id='o1'`); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := m.ListLinksByOrder("o1"); err == nil {
		t.Fatal("list scan = nil, want error")
	}
}

func TestCouponModel_ListUserCouponsArms(t *testing.T) {
	m, db, _ := couponArmsDB(t)
	mustClaim(t, m, 1, 7, "r1")
	// limit 钳制（563）
	if _, err := m.ListUserCoupons(7, "", 0); err != nil {
		t.Fatalf("list limit: %v", err)
	}
	// Scan 失败：coupon_id 写 text（580）
	if _, err := db.Exec(`UPDATE user_coupons SET coupon_id='corrupt' WHERE user_id=7`); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := m.ListUserCoupons(7, "", 10); err == nil {
		t.Fatal("list scan = nil, want error")
	}
}

func TestCouponModel_ListCouponsArms(t *testing.T) {
	m, db, _ := couponArmsDB(t)
	seedCoupon(t, m, "C1")
	// limit 钳制（312）
	if _, err := m.ListCoupons("", "", 0); err != nil {
		t.Fatalf("list limit: %v", err)
	}
	// status 过滤分支（320，成功）
	if _, err := m.ListCoupons("g-a", CouponStatusPublished, 5); err != nil {
		t.Fatalf("list status: %v", err)
	}
	// Scan 失败：threshold_cents 写 text（333）
	if _, err := db.Exec(`UPDATE coupons SET threshold_cents='corrupt'`); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := m.ListCoupons("g-a", "", 5); err == nil {
		t.Fatal("list scan = nil, want error")
	}
}

func TestCouponModel_MigrateColumnsArms(t *testing.T) {
	// 裸 user_coupons（无 locked_at 列）→ 首个 ALTER 成功 → return nil（219）
	bare := filepath.Join(t.TempDir(), "bare.db")
	if bdb, err := sql.Open("sqlite3", "file:"+bare); err == nil {
		if _, err := bdb.Exec(`CREATE TABLE user_coupons (id INTEGER PRIMARY KEY, coupon_id INTEGER, user_id INTEGER, request_id TEXT)`); err != nil {
			t.Fatalf("bare table: %v", err)
		}
		if err := NewCouponModel(bdb).migrateCouponColumns(); err != nil {
			t.Fatalf("migrate fresh = %v, want nil", err)
		}
		_ = bdb.Close()
	}
	// 空文件 + 只读：ALTER 失败（表不存在），两次皆败 → 记 lastErr，
	// 消息不含 duplicate/exists → 返回 lastErr（228）
	path := filepath.Join(t.TempDir(), "mig.db")
	if db, err := sql.Open("sqlite3", "file:"+path); err == nil {
		_ = db.Ping()
		_ = db.Close()
	}
	if err := NewCouponModel(couponReadOnly(t, path)).migrateCouponColumns(); err == nil {
		t.Fatal("migrate ro = nil, want error")
	}
}
