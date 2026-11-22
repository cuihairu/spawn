package model

// model 补漏分支：两个 outbox 的 CreateTables 双方言均失败（只读文件）、
// Enqueue INSERT-OR-IGNORE→INSERT-IGNORE 回退闸（RAISE 触发器拦主语句与
// 回退语句）、ListRetry 的 limit 钳位/查询失败/脏行扫描失败、Prune 写失败；
// usermodel 的 UpdatePromoChannel 写失败与「更新成功但 FindOne 查无」失效回
// 退、migratePromoChannel 的 ALTER 补列、columnPresent 无匹配收尾。
//
// 登记不可达（不测）：Create 的 LastInsertId 失败臂（Exec 成功后
// LastInsertId 不再报错）；CreateUsersTable/两个 outbox 的 MySQL 方言成功
// 路径（CREATE TABLE/INSERT IGNORE 是 SQLite 语法错误，方言成功臂只能在
// MySQL 库上跑）；migratePromoChannel 的 PRAGMA 失败→SHOW COLUMNS 与 SHOW
// COLUMNS 成功体（同为 MySQL 方言）；columnPresent 的 rows.Columns 报错/
// idx 越界（PRAGMA 恒 6 列、活结果集 Columns 不报错）与 Scan 报错（扫入
// interface{} 恒成功）。

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// roDB 打开临时文件 SQLite，先写一张最小 _seed 表后重开只读：0 字节文件
// 只读打开会失败，且「只读 + 表已存在」才让 CREATE TABLE IF NOT EXISTS 只读
// no-op——两者让 CreateTables 的 SQLite 与 MySQL 双方言都失败。
func roDB(t *testing.T, name string) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	seed, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open seed: %v", err)
	}
	if _, err := seed.Exec(`CREATE TABLE _seed (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("seed table: %v", err)
	}
	if err := seed.Close(); err != nil {
		t.Fatalf("close seed: %v", err)
	}
	ro, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open readonly: %v", err)
	}
	t.Cleanup(func() { ro.Close() })
	return ro
}

// TestOutbox_CreateTablesBothDialectFail 只读文件上两个 outbox 的 CreateTables：
// SQLite 方言失败后 MySQL 方言亦失败 → 返回双尝试错误。
func TestOutbox_CreateTablesBothDialectFail(t *testing.T) {
	if err := NewAnalyticsOutboxModel(roDB(t, "ro-analytics.db")).CreateTables(); err == nil || !strings.Contains(err.Error(), "尝试了") {
		t.Fatalf("analytics create-readonly = %v, want 双方言失败", err)
	}
	if err := NewPromoAttrOutboxModel(roDB(t, "ro-promo.db")).CreateTables(); err == nil || !strings.Contains(err.Error(), "尝试了") {
		t.Fatalf("promo create-readonly = %v, want 双方言失败", err)
	}
}

// TestOutbox_EnqueueFallbackGate RAISE 触发器拦下 INSERT OR IGNORE 与回退
// INSERT IGNORE（OR IGNORE 不豁免触发器）→ Enqueue 返回回退失败。
func TestOutbox_EnqueueFallbackGate(t *testing.T) {
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)

	a := newOutboxModel(t)
	if _, err := a.db.Exec(`CREATE TRIGGER block_analytics_insert BEFORE INSERT ON analytics_outbox BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
		t.Fatalf("analytics trigger: %v", err)
	}
	if err := a.Enqueue("ev-blocked", "register", `{"events":[]}`, past); err == nil {
		t.Fatal("blocked analytics enqueue = nil err, want error")
	}

	p := newPromoAttrOutboxModel(t)
	if _, err := p.db.Exec(`CREATE TRIGGER block_promo_insert BEFORE INSERT ON promo_attribution_outbox BEGIN SELECT RAISE(ABORT, 'blocked'); END`); err != nil {
		t.Fatalf("promo trigger: %v", err)
	}
	if err := p.Enqueue("attr:blocked:1", "click-1", 1, "2026-11-20T00:00:00Z", `{}`, past); err == nil {
		t.Fatal("blocked promo enqueue = nil err, want error")
	}
}

// TestAnalyticsOutbox_LimitClampAndDBError 钳位：limit<=0 → 按 100 兜底；
// 查询失败：闭库直接返回错误。
func TestAnalyticsOutbox_LimitClampAndDBError(t *testing.T) {
	m := newOutboxModel(t)
	if _, err := m.ListRetry(nowUTC(), 0); err != nil {
		t.Fatalf("clamped list: %v", err)
	}
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatalf("open closed: %v", err)
	}
	closed := NewAnalyticsOutboxModel(db)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := closed.ListRetry(nowUTC(), 100); err == nil {
		t.Fatal("closed list = nil err, want error")
	}
	if _, err := closed.Prune(nowUTC()); err == nil {
		t.Fatal("closed prune = nil err, want error")
	}
}

// TestAnalyticsOutbox_ScanFailDirtyRow 脏行：attempts（int 列）被写成文本
// 且不是 WHERE 过滤列 → Scan 到 *int 失败。
func TestAnalyticsOutbox_ScanFailDirtyRow(t *testing.T) {
	m := newOutboxModel(t)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if _, err := m.db.Exec(
		`INSERT INTO analytics_outbox (event_id, event_type, payload, status, attempts, last_error, next_retry_at, delivered_at, created_at)
		 VALUES ('ev-dirty', 'register', '{}', 'pending', 'corrupt', '', ?, '', ?)`,
		past, past,
	); err != nil {
		t.Fatalf("dirty insert: %v", err)
	}
	if _, err := m.ListRetry(past, 100); err == nil {
		t.Fatal("dirty list = nil err, want scan error")
	}
}

// TestPromoAttrOutbox_LimitClampAndDBError 镜像 analytics：钳位 + 闭库查询/
// 清理失败。
func TestPromoAttrOutbox_LimitClampAndDBError(t *testing.T) {
	m := newPromoAttrOutboxModel(t)
	if _, err := m.ListRetry(nowUTC(), 0); err != nil {
		t.Fatalf("clamped list: %v", err)
	}
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatalf("open closed: %v", err)
	}
	closed := NewPromoAttrOutboxModel(db)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := closed.ListRetry(nowUTC(), 100); err == nil {
		t.Fatal("closed list = nil err, want error")
	}
	if _, err := closed.Prune(nowUTC()); err == nil {
		t.Fatal("closed prune = nil err, want error")
	}
}

// TestPromoAttrOutbox_ScanFailDirtyRow 脏行：attempts 写成文本 → Scan 失败。
func TestPromoAttrOutbox_ScanFailDirtyRow(t *testing.T) {
	m := newPromoAttrOutboxModel(t)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339Nano)
	if _, err := m.db.Exec(
		`INSERT INTO promo_attribution_outbox (attr_key, click_id, user_id, registered_at, payload, status, attempts, last_error, next_retry_at, delivered_at, created_at)
		 VALUES ('attr:dirty:1', 'click-1', 1, '2026-11-20T00:00:00Z', '{}', 'pending', 'corrupt', '', ?, '', ?)`,
		past, past,
	); err != nil {
		t.Fatalf("dirty insert: %v", err)
	}
	if _, err := m.ListRetry(past, 100); err == nil {
		t.Fatal("dirty list = nil err, want scan error")
	}
}

// TestUserModel_PromoChannelArms UpdatePromoChannel 的写失败臂（闭库）与
// 「UPDATE 成功但 FindOne 查无」失效回退臂（不存在 id：UPDATE 零行无错，
// FindOne 报错 → 只失效 id 键）。
func TestUserModel_PromoChannelArms(t *testing.T) {
	closed, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	m := NewUserModel(closed)
	if err := closed.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := m.UpdatePromoChannel(1, "ch"); err == nil || !strings.Contains(err.Error(), "更新归因渠道失败") {
		t.Fatalf("closed update channel = %v, want 更新归因渠道失败", err)
	}

	// 存活库上不存在 id：UPDATE 无错、FindOne 报错 → 回退只失效 id 键。
	db := setupTestDB(t)
	alive := NewUserModel(db)
	if err := alive.UpdatePromoChannel(999999, "ch"); err != nil {
		t.Fatalf("update missing id: %v", err)
	}
}

// TestUserModel_MigratePromoChannelAlter 补列：建一张没有 promo_channel 的
// users 表 → migratePromoChannel 走 ALTER 路径补列（列已存在则幂等跳过）。
func TestUserModel_MigratePromoChannelAlter(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE users (id INTEGER PRIMARY KEY, username TEXT)`); err != nil {
		t.Fatalf("create bare table: %v", err)
	}
	m := NewUserModel(db)
	if err := m.migratePromoChannel(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	rows, err := db.Query(`PRAGMA table_info(users)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()
	if !columnPresent(rows, 1, "promo_channel") {
		t.Fatal("promo_channel column not added")
	}
	// 幂等：再跑一次（列已存在 → has=true → 直接返回，ALTER 被跳过）。
	if err := m.migratePromoChannel(); err != nil {
		t.Fatalf("migrate idempotent: %v", err)
	}
}

// TestUserModel_CheckUsernameExistsDBError 闭库上 CheckUsernameExists 的
// COUNT 查询失败臂（与已覆盖的 CheckEmailExists 同形）。
func TestUserModel_CheckUsernameExistsDBError(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	m := NewUserModel(db)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := m.CheckUsernameExists("alice"); err == nil || !strings.Contains(err.Error(), "检查用户名失败") {
		t.Fatalf("closed check username = %v, want 检查用户名失败", err)
	}
}

// TestColumnPresent_NoMatch columnPresent 遍历完仍未命中 → 收尾返回 false。
func TestColumnPresent_NoMatch(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER PRIMARY KEY, name TEXT)`); err != nil {
		t.Fatalf("create: %v", err)
	}
	rows, err := db.Query(`PRAGMA table_info(t)`)
	if err != nil {
		t.Fatalf("pragma: %v", err)
	}
	defer rows.Close()
	if columnPresent(rows, 1, "no_such_column") {
		t.Fatal("no-match column present = true")
	}
}
