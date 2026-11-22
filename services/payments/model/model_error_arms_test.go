package model

// model 包剩余补漏分支（覆盖扫描 82.9% → 目标 >85%）：db.Open 的空 DSN /
// 非法 DSN / Ping 失败臂，以及各模型的 CreateTables 双方言都失败臂（VIEW
// 占名使 CREATE TABLE IF NOT EXISTS 两方言皆败）、ListRetry/ListByGame 的
// limit 钳制臂与 Scan 失败臂（脏行：int 列写 text）、CreateTarget 的 UNIQUE
// 冲突臂、Enqueue 的 INSERT-OR-IGNORE 失败回退 INSERT IGNORE 臂（RAISE
// ABORT 触发器使两写皆败）、MarkFailed 成功臂、Prune 查询失败臂。
//
// 不可达（登记不入册）：各模型 Enqueue 的 MySQL 方言回退成功臂（INSERT
// IGNORE / ON CONFLICT 在 SQLite 环境要么语法错、要么本就成功，回退成功
// 路径无法在 SQLite 测试环境触发）；promotion Enqueue 的 syntax 门臂同理。

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func modelOpenDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "arms.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// modelReadOnlyDB 只读重开同一文件（写失败、读成功）。
func modelReadOnlyDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open readonly: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// modelClosedDB 已关闭的 *sql.DB（首个查询即败）。
func modelClosedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	_ = db.Close()
	return db
}

func TestOpen_Arms(t *testing.T) {
	// 空 DSN → "datasource 不能为空"
	if _, err := Open(""); err == nil {
		t.Fatal("empty dsn = nil, want error")
	}
	// 非法 MySQL DSN → sql.Open 即败
	if _, err := Open("not-a-dsn"); err == nil {
		t.Fatal("bad mysql dsn = nil, want error")
	}
	// 合法 MySQL DSN、主机不可达 → Ping 败
	if _, err := Open("root:pass@tcp(127.0.0.1:1)/db"); err == nil {
		t.Fatal("ping fail = nil, want error")
	}
}

func TestDeliveryModel_Arms(t *testing.T) {
	db := modelOpenDB(t)
	m := NewDeliveryModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create: %v", err)
	}
	o := &DeliveryOrder{OrderId: "dup", GameId: "g", ServerId: 1, RoleId: "r", AccountId: "a",
		SkuId: "s", ProductId: "p", AmountCents: 100, Currency: "CNY"}
	if err := m.CreateTarget(o); err != nil {
		t.Fatalf("create target: %v", err)
	}
	// 同 order_id 幂等命中 → ErrDeliveryOrderExists
	if err := m.CreateTarget(o); err != ErrDeliveryOrderExists {
		t.Fatalf("dup = %v, want ErrDeliveryOrderExists", err)
	}
	// ListRetry limit 钳制（0 → 100）
	if _, err := m.ListRetry("2099-01-01T00:00:00Z", 0); err != nil {
		t.Fatalf("listretry limit: %v", err)
	}
	// ListRetry Scan 失败（attempts 写 text）
	if _, err := db.Exec(`UPDATE delivery_orders SET attempts='corrupt' WHERE order_id='dup'`); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := m.ListRetry("2099-01-01T00:00:00Z", 10); err == nil {
		t.Fatal("listretry scan = nil, want error")
	}
	// ListByGame limit 钳制（0 → 100）；脏行在库 → Scan 亦败
	if _, err := m.ListByGame("", "", 0); err == nil {
		t.Fatal("listbygame limit = nil, want error")
	}
	// ListByGame status 过滤分支 + Scan 失败
	if _, err := m.ListByGame("g", "pending", 5); err == nil {
		t.Fatal("listbygame status = nil, want error")
	}
}

func TestProductModel_Arms(t *testing.T) {
	db := modelOpenDB(t)
	m := NewProductModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := m.Create(&Product{Id: "p1", ProductId: "prod", Form: FormDirectPurchase,
		AmountCents: 100, BaseQuantity: 1, Currency: "CNY", GameId: "g", PartnerId: 1}); err != nil {
		t.Fatalf("create: %v", err)
	}
	// ListByGame Scan 失败（amount_cents 写 text）
	if _, err := db.Exec(`UPDATE payment_products SET amount_cents='corrupt' WHERE id='p1'`); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := m.ListByGame("g", false); err == nil {
		t.Fatal("list scan = nil, want error")
	}
}

func TestPromotionEvalModel_Arms(t *testing.T) {
	db := modelOpenDB(t)
	m := NewPromotionEvalModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := m.Enqueue("o1", "ch", `{}`); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// MarkFailed 成功臂
	if err := m.MarkFailed("o1", 2, "boom", "2099-01-01T00:00:00Z"); err != nil {
		t.Fatalf("markfailed: %v", err)
	}
	// ListRetry 查询失败（闭库）
	if _, err := NewPromotionEvalModel(modelClosedDB(t)).ListRetry("2099-01-01T00:00:00Z", 10); err == nil {
		t.Fatal("listretry closed = nil, want error")
	}
	// ListRetry Scan 失败（attempts 写 text）
	if _, err := db.Exec(`UPDATE promotion_evaluations SET attempts='corrupt' WHERE order_id='o1'`); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := m.ListRetry("2099-01-01T00:00:00Z", 10); err == nil {
		t.Fatal("listretry scan = nil, want error")
	}
}

func TestNotifyOutboxModel_Arms(t *testing.T) {
	db := modelOpenDB(t)
	m := NewNotifyOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create: %v", err)
	}
	// ListRetry limit 钳制（0 → 100）
	if _, err := m.ListRetry("2099-01-01T00:00:00Z", 0); err != nil {
		t.Fatalf("listretry limit: %v", err)
	}
	// Enqueue 回退臂：RAISE ABORT 触发器使 INSERT OR IGNORE 败 → 回退
	// INSERT IGNORE（SQLite 语法错）→ 返回 err2
	if _, err := db.Exec(`CREATE TRIGGER trg_no BEFORE INSERT ON notify_outbox BEGIN SELECT RAISE(ABORT, 'no'); END`); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if err := m.Enqueue("k", "t", "{}", ""); err == nil {
		t.Fatal("enqueue abort = nil, want error")
	}
	// Prune 查询失败（闭库）
	if _, err := NewNotifyOutboxModel(modelClosedDB(t)).Prune("2099-01-01T00:00:00Z"); err == nil {
		t.Fatal("prune closed = nil, want error")
	}
	// ListRetry Scan 失败（干净库插行后写脏 attempts）
	db2 := modelOpenDB(t)
	m2 := NewNotifyOutboxModel(db2)
	if err := m2.CreateTables(); err != nil {
		t.Fatalf("create2: %v", err)
	}
	if err := m2.Enqueue("k2", "t", "{}", ""); err != nil {
		t.Fatalf("enqueue2: %v", err)
	}
	if _, err := db2.Exec(`UPDATE notify_outbox SET attempts='corrupt' WHERE notify_key='k2'`); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := m2.ListRetry("2099-01-01T00:00:00Z", 10); err == nil {
		t.Fatal("listretry scan = nil, want error")
	}
}

func TestAnalyticsOutboxModel_Arms(t *testing.T) {
	db := modelOpenDB(t)
	m := NewAnalyticsOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := m.ListRetry("2099-01-01T00:00:00Z", 0); err != nil {
		t.Fatalf("listretry limit: %v", err)
	}
	if _, err := db.Exec(`CREATE TRIGGER trg_no BEFORE INSERT ON analytics_outbox BEGIN SELECT RAISE(ABORT, 'no'); END`); err != nil {
		t.Fatalf("trigger: %v", err)
	}
	if err := m.Enqueue("e", "t", "{}", ""); err == nil {
		t.Fatal("enqueue abort = nil, want error")
	}
	if _, err := NewAnalyticsOutboxModel(modelClosedDB(t)).Prune("2099-01-01T00:00:00Z"); err == nil {
		t.Fatal("prune closed = nil, want error")
	}
	db2 := modelOpenDB(t)
	m2 := NewAnalyticsOutboxModel(db2)
	if err := m2.CreateTables(); err != nil {
		t.Fatalf("create2: %v", err)
	}
	if err := m2.Enqueue("e2", "t", "{}", ""); err != nil {
		t.Fatalf("enqueue2: %v", err)
	}
	if _, err := db2.Exec(`UPDATE analytics_outbox SET attempts='corrupt' WHERE event_id='e2'`); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := m2.ListRetry("2099-01-01T00:00:00Z", 10); err == nil {
		t.Fatal("listretry scan = nil, want error")
	}
}

// TestModel_CreateTablesBothFail 双方言建表皆败臂：只读连接使 SQLite 建表
// 失败，MySQL 方言回退同样失败（readonly database）→ 返回"两种格式都失败"。
func TestModel_CreateTablesBothFail(t *testing.T) {
	cases := []struct {
		name string
		mk   func(*sql.DB) error
	}{
		{"delivery", func(db *sql.DB) error { return NewDeliveryModel(db).CreateTables() }},
		{"product", func(db *sql.DB) error { return NewProductModel(db).CreateTables() }},
		{"promotion", func(db *sql.DB) error { return NewPromotionEvalModel(db).CreateTables() }},
		{"notify", func(db *sql.DB) error { return NewNotifyOutboxModel(db).CreateTables() }},
		{"analytics", func(db *sql.DB) error { return NewAnalyticsOutboxModel(db).CreateTables() }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "ro.db")
			db, err := sql.Open("sqlite3", "file:"+path)
			if err != nil {
				t.Fatalf("open: %v", err)
			}
			if err := db.Ping(); err != nil {
				t.Fatalf("ping: %v", err)
			}
			_ = db.Close()
			if err := c.mk(modelReadOnlyDB(t, path)); err == nil {
				t.Fatal("readonly create tables = nil, want error (both dialects fail)")
			}
		})
	}
}
