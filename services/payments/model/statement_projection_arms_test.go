package model

// statementmodel / orderprojectionmodel 剩余补漏分支：migrateStatementColumns
// 的全成功（裸表无异议列）与返回 lastErr（只读）臂，ListStatements 的
// limit/offset 钳制、status 过滤与 Scan 失败臂，RecordDisputeRound 的查询
// 失败与 INSERT 失败（只读）臂，ResolveLatestDisputeRound 的 UPDATE 失败
// （只读）臂，ListDisputeRounds Scan 失败臂，ListDifferences 的 limit 钳制/
// vendor 过滤/Scan 失败臂，StatementLineItems Scan 失败臂；orderprojection
// 的 AccountGameNetCents 两段查询失败臂（闭库 / 删 refund_requests）、
// ListByQuery 的 offset 钳制/accountId 过滤/Scan 失败臂、ListAudit 的 limit
// 钳制/Scan 失败臂、GetRefund 查无与 Scan 失败臂、ListRefunds 的 limit 钳制
// 与 Scan 失败臂。脏行一律写非过滤列。
//
// 登记不可达（不测）：265.4 UpsertStatement / 223.4 UpsertFromOrder 的
// MySQL 方言回退门（ON CONFLICT 为 SQLite 语法且成功，syntax/near 门
// 永不触发）。

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func statementArmsDB(t *testing.T) (*StatementModel, *sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "stmt.db")
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := NewStatementModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return m, db, path
}

func projectionArmsDB(t *testing.T) (*OrderProjectionModel, *sql.DB, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "proj.db")
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := NewOrderProjectionModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return m, db, path
}

func seedStatement(t *testing.T, m *StatementModel, vendor string) int64 {
	t.Helper()
	s := &VendorStatement{Vendor: vendor, PartnerId: 1, PeriodStart: "2026-01-01",
		PeriodEnd: "2026-01-31", PaidTotal: 1000, DiscountTotal: 50, RefundTotal: 0,
		SplitBase: 950, SplitRateBp: 3000, SplitAmount: 285, Settlement: 900,
		Diff: 100, OrderCount: 2, Status: "draft", CreatedBy: 1}
	if err := m.UpsertStatement(s); err != nil {
		t.Fatalf("upsert statement: %v", err)
	}
	got, err := m.GetByKey(vendor, 1, "2026-01-01", "2026-01-31")
	if err != nil {
		t.Fatalf("get by key: %v", err)
	}
	return got.Id
}

func TestStatementModel_MigrateArms(t *testing.T) {
	// 裸表（无异议列）→ 三条 ALTER 全成功 → lastErr==nil → return nil（222）
	path := filepath.Join(t.TempDir(), "bare.db")
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE vendor_statements (
		id INTEGER PRIMARY KEY AUTOINCREMENT, vendor TEXT, partner_id INTEGER,
		period_start TEXT, period_end TEXT, created_at TEXT, updated_at TEXT)`); err != nil {
		t.Fatalf("bare table: %v", err)
	}
	if err := NewStatementModel(db).migrateStatementColumns(); err != nil {
		t.Fatalf("migrate fresh = %v, want nil", err)
	}
	// 只读空库 → ALTER 失败、消息不含 duplicate/exists → 返回 lastErr（229）
	roPath := filepath.Join(t.TempDir(), "ro.db")
	if rdb, err := sql.Open("sqlite3", "file:"+roPath); err == nil {
		_ = rdb.Ping()
		_ = rdb.Close()
	}
	ro, err := sql.Open("sqlite3", "file:"+roPath+"?mode=ro")
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	defer ro.Close()
	if err := NewStatementModel(ro).migrateStatementColumns(); err == nil {
		t.Fatal("migrate ro = nil, want error")
	}
}

func TestStatementModel_ListArms(t *testing.T) {
	m, db, _ := statementArmsDB(t)
	seedStatement(t, m, "vendor-a")
	// limit 钳制（332）+ offset 钳制（335）
	if _, err := m.ListStatements("", 0, "", 0, -1); err != nil {
		t.Fatalf("list clamp: %v", err)
	}
	// status 过滤分支（348）
	if _, err := m.ListStatements("", 0, "draft", 5, 0); err != nil {
		t.Fatalf("list status: %v", err)
	}
	// Scan 失败：paid_total（非过滤列）写 text（361）
	if _, err := db.Exec(`UPDATE vendor_statements SET paid_total='corrupt'`); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := m.ListStatements("vendor-a", 0, "", 5, 0); err == nil {
		t.Fatal("list scan = nil, want error")
	}
}

func TestStatementModel_ListDifferencesArms(t *testing.T) {
	m, db, _ := statementArmsDB(t)
	seedStatement(t, m, "vendor-a")
	// limit 钳制（482）
	if _, err := m.ListDifferences(1, "", 0); err != nil {
		t.Fatalf("diff clamp: %v", err)
	}
	// vendor 过滤分支（488）
	rows, err := m.ListDifferences(1, "vendor-a", 50)
	if err != nil || len(rows) != 1 {
		t.Fatalf("diff vendor = %d rows, %v, want 1", len(rows), err)
	}
	// Scan 失败：paid_total 写 text（502）
	if _, err := db.Exec(`UPDATE vendor_statements SET paid_total='corrupt'`); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := m.ListDifferences(1, "", 50); err == nil {
		t.Fatal("diff scan = nil, want error")
	}
}

func TestStatementModel_DisputeArms(t *testing.T) {
	m, db, path := statementArmsDB(t)
	id := seedStatement(t, m, "vendor-a")
	if _, err := m.RecordDisputeRound(id, 7, "金额不符", 100); err != nil {
		t.Fatalf("record round: %v", err)
	}
	// 查询失败（419）
	if _, err := NewStatementModel(modelClosedDB(t)).RecordDisputeRound(id, 7, "r", 1); err == nil {
		t.Fatal("record closed = nil, want error")
	}
	// 只读：SELECT ok、INSERT 失败 → return 0,err（428）
	if _, err := NewStatementModel(roDB(t, path)).RecordDisputeRound(id, 7, "r2", 100); err == nil {
		t.Fatal("record ro = nil, want error")
	}
	// 只读：有未结轮次、UPDATE 失败 → return false,err（452）
	ok, err := NewStatementModel(roDB(t, path)).ResolveLatestDisputeRound(id, 9)
	if err == nil || ok {
		t.Fatalf("resolve ro = %v/%v, want false+error", ok, err)
	}
	// Scan 失败：round_no（非过滤列）写 text（471）
	if _, err := db.Exec(`UPDATE statement_dispute_history SET round_no='corrupt' WHERE statement_id=?`, id); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := m.ListDisputeRounds(id); err == nil {
		t.Fatal("rounds scan = nil, want error")
	}
}

func TestStatementModel_LineItemsScanFail(t *testing.T) {
	m, db, _ := statementArmsDB(t)
	pm := NewOrderProjectionModel(db)
	if err := pm.CreateTables(); err != nil {
		t.Fatalf("projection tables: %v", err)
	}
	if err := pm.UpsertFromOrder(&OrderProjection{OrderId: "o1", GameId: "g", PartnerId: 1,
		AccountId: "a", SkuId: "s", ProductId: "p", Form: "DIRECT_PURCHASE",
		AmountCents: 100, BaseQuantity: 1, Currency: "CNY", Status: "PAID",
		PaidAt: "2026-01-05T00:00:00Z"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// Scan 失败：amount_cents（非过滤列）写 text（568）
	if _, err := db.Exec(`UPDATE order_projection SET amount_cents='corrupt'`); err != nil {
		t.Fatalf("corrupt: %v", err)
	}
	if _, err := m.StatementLineItems(1, "2026-01-01T00:00:00Z", "2026-02-01T00:00:00Z"); err == nil {
		t.Fatal("line items scan = nil, want error")
	}
}

func TestOrderProjection_Arms(t *testing.T) {
	m, db, _ := projectionArmsDB(t)
	if err := m.UpsertFromOrder(&OrderProjection{OrderId: "o1", GameId: "g", PartnerId: 1,
		AccountId: "acc1", SkuId: "s", ProductId: "p", Form: "DIRECT_PURCHASE",
		AmountCents: 100, BaseQuantity: 1, Currency: "CNY", Status: "PAID"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	// AccountGameNetCents 首段查询失败（闭库，282）
	if _, err := NewOrderProjectionModel(modelClosedDB(t)).AccountGameNetCents("acc1", "g"); err == nil {
		t.Fatal("net closed = nil, want error")
	}
	// ListByQuery offset 钳制（315）+ accountId 过滤分支（328）
	if _, err := m.ListByQuery("", "", "", 5, -1); err != nil {
		t.Fatalf("list clamp: %v", err)
	}
	if _, err := m.ListByQuery("g", "", "acc1", 5, 0); err != nil {
		t.Fatalf("list account: %v", err)
	}
	// ListAudit limit 钳制（360）
	if err := m.LogAudit(&OrderAuditLog{OrderId: "o1", ActorId: 1, ActorRole: "platform_admin",
		Action: "VIEW", Detail: "d", Ip: "1.2.3.4"}); err != nil {
		t.Fatalf("log audit: %v", err)
	}
	if _, err := m.ListAudit("", 0); err != nil {
		t.Fatalf("audit clamp: %v", err)
	}
	// GetRefund 查无（414）
	if _, err := m.GetRefund("nope", "REFUND"); err != ErrRefundRequestNotFound {
		t.Fatalf("refund missing = %v, want ErrRefundRequestNotFound", err)
	}
	if err := m.CreateRefund(&RefundRequest{OrderId: "o1", Action: "REFUND", AmountCents: 50,
		Reason: "r", Status: "PENDING", CreatedBy: 1}); err != nil {
		t.Fatalf("create refund: %v", err)
	}
	// ListRefunds limit 钳制（425）
	if _, err := m.ListRefunds("", 0); err != nil {
		t.Fatalf("refunds clamp: %v", err)
	}
	// 扫描失败三连：order_projection.amount_cents（341/568 走专用测试）、
	// order_audit_logs.actor_id（378）、refund_requests.amount_cents（417/443）
	if _, err := db.Exec(`UPDATE order_audit_logs SET actor_id='corrupt'`); err != nil {
		t.Fatalf("corrupt audit: %v", err)
	}
	if _, err := m.ListAudit("o1", 5); err == nil {
		t.Fatal("audit scan = nil, want error")
	}
	if _, err := db.Exec(`UPDATE refund_requests SET amount_cents='corrupt'`); err != nil {
		t.Fatalf("corrupt refund: %v", err)
	}
	if _, err := m.GetRefund("o1", "REFUND"); err == nil {
		t.Fatal("refund scan = nil, want error")
	}
	if _, err := m.ListRefunds("PENDING", 5); err == nil {
		t.Fatal("refunds scan = nil, want error")
	}
	// ListByQuery Scan 失败：amount_cents 写 text（341）
	if _, err := db.Exec(`UPDATE order_projection SET amount_cents='corrupt'`); err != nil {
		t.Fatalf("corrupt projection: %v", err)
	}
	if _, err := m.ListByQuery("g", "", "", 5, 0); err == nil {
		t.Fatal("list scan = nil, want error")
	}
}

func TestOrderProjection_NetCentsSecondQueryFail(t *testing.T) {
	// 首段（order_projection）ok、次段（refund_requests）失败 → return 0,err（291）
	m, db, _ := projectionArmsDB(t)
	if err := m.UpsertFromOrder(&OrderProjection{OrderId: "o1", GameId: "g", PartnerId: 1,
		AccountId: "acc1", SkuId: "s", ProductId: "p", Form: "DIRECT_PURCHASE",
		AmountCents: 100, BaseQuantity: 1, Currency: "CNY", Status: "PAID"}); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := db.Exec(`DROP TABLE refund_requests`); err != nil {
		t.Fatalf("drop refund_requests: %v", err)
	}
	if _, err := m.AccountGameNetCents("acc1", "g"); err == nil {
		t.Fatal("net second = nil, want error")
	}
}

// roDB 以 ?mode=ro 重开同一文件。
func roDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
