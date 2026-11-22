package model

// outbox 两表（经营分析 / 业务通知）补漏分支：CreateTables 的 MySQL 方言回退
// 与「双方言皆败」返回臂（关库）、ListRetry 非法 limit 钳到 100、列表脏行
// Scan 失败（破坏 INTEGER 列）。MySQL 回退成功路径（INSERT IGNORE / CREATE
// 成功）属登记不可构造家族（SQLite 方言不失败即不会走回退），见 coverage 台账。

import (
	"database/sql"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newOutboxModelDB 同 newOutboxModel 但返回底层连接（供关库/触发器用例）。
func newOutboxModelDB(t *testing.T) (*AnalyticsOutboxModel, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/outbox.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	m := NewAnalyticsOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create outbox tables: %v", err)
	}
	return m, db
}

// 关库下 CreateTables：SQLite 建表失败 → 尝试 MySQL 格式 → 再败 → 返回
// 「尝试了 SQLite 和 MySQL 格式」错误（覆盖双方言回退与失败出口）。
func TestOutbox_CreateTablesBothDialectsFail(t *testing.T) {
	m, db := newOutboxModelDB(t)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := m.CreateTables(); err == nil ||
		!strings.Contains(err.Error(), "尝试了 SQLite 和 MySQL 格式") {
		t.Fatalf("CreateTables on closed db = %v, want 尝试了 SQLite 和 MySQL 格式", err)
	}
}

// 业务通知 CreateTables 同出口（独立实现，须单独覆盖）。
func TestNotifyOutbox_CreateTablesBothDialectsFail(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/notify.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	m := NewNotifyOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create notify tables: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := m.CreateTables(); err == nil ||
		!strings.Contains(err.Error(), "尝试了 SQLite 和 MySQL 格式") {
		t.Fatalf("CreateTables on closed db = %v, want 尝试了 SQLite 和 MySQL 格式", err)
	}
}

// ListRetry 非法 limit：0 与 999 均钳到 100（正常查询，不报错）。
func TestOutbox_ListRetryLimitClamp(t *testing.T) {
	m := newOutboxModel(t)
	for _, limit := range []int{0, 999} {
		if _, err := m.ListRetry("2999-01-01T00:00:00Z", limit); err != nil {
			t.Fatalf("ListRetry(limit=%d) = %v, want nil", limit, err)
		}
	}

	ndb, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/notify.db")
	if err != nil {
		t.Fatalf("open notify db: %v", err)
	}
	nm := NewNotifyOutboxModel(ndb)
	if err := nm.CreateTables(); err != nil {
		t.Fatalf("create notify tables: %v", err)
	}
	for _, limit := range []int{0, 999} {
		if _, err := nm.ListRetry("2999-01-01T00:00:00Z", limit); err != nil {
			t.Fatalf("Notify ListRetry(limit=%d) = %v, want nil", limit, err)
		}
	}
}

// 列表脏行 Scan 失败：入队一行（pending 且到期）后破坏 INTEGER 列 attempts，
// ListRetry 扫描到该行时 Scan 报错。
func TestOutbox_ListRetryScanDirty(t *testing.T) {
	m, db := newOutboxModelDB(t)
	if err := m.Enqueue("e-dirty", "click", "{}", "2000-01-01T00:00:00Z"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := db.Exec(`UPDATE analytics_outbox SET attempts = 'abc' WHERE id = 1`); err != nil {
		t.Fatalf("corrupt attempts: %v", err)
	}
	if _, err := m.ListRetry("2999-01-01T00:00:00Z", 100); err == nil {
		t.Fatal("ListRetry dirty attempts = nil error, want scan error")
	}
}

func TestNotifyOutbox_ListRetryScanDirty(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/notify.db")
	if err != nil {
		t.Fatalf("open notify db: %v", err)
	}
	m := NewNotifyOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create notify tables: %v", err)
	}
	if err := m.Enqueue("k-dirty", "reserve", "{}", "2000-01-01T00:00:00Z"); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if _, err := db.Exec(`UPDATE notify_outbox SET attempts = 'abc' WHERE id = 1`); err != nil {
		t.Fatalf("corrupt attempts: %v", err)
	}
	if _, err := m.ListRetry("2999-01-01T00:00:00Z", 100); err == nil {
		t.Fatal("Notify ListRetry dirty attempts = nil error, want scan error")
	}
}
