package model

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newClosedReportModel 建表后立即关闭底层库，专打各方法的错误返回路径。
func newClosedReportModel(t *testing.T) *ReportModel {
	t.Helper()
	m := newReportTestDB(t)
	if err := m.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	return m
}

func TestCreateReportsTable_ClosedDBErrors(t *testing.T) {
	m := newClosedReportModel(t)
	if err := m.CreateReportsTable(); err == nil {
		t.Fatal("CreateReportsTable on closed db must error")
	}
}

func TestReportCreate_ClosedDBErrors(t *testing.T) {
	m := newClosedReportModel(t)
	if err := m.Create(report(7, "post", 100)); err == nil {
		t.Fatal("Create on closed db must error")
	}
}

func TestReportGetByID_ClosedDBErrors(t *testing.T) {
	m := newClosedReportModel(t)
	if _, err := m.GetByID(1); err == nil {
		t.Fatal("GetByID on closed db must error")
	}
}

func TestReportListByStatus_ClosedDBErrors(t *testing.T) {
	m := newClosedReportModel(t)
	if _, _, err := m.ListByStatus("", 20, 0); err == nil {
		t.Fatal("ListByStatus on closed db must error")
	}
}

func TestReportUpdateStatus_ClosedDBErrors(t *testing.T) {
	m := newClosedReportModel(t)
	if err := m.UpdateStatus(1, "resolved", 99); err == nil {
		t.Fatal("UpdateStatus on closed db must error")
	}
}

// newRawReportModel 建临时库并按给定 schema 建表（不走 CreateReportsTable），
// 供缺列故障注入用例使用——关库会让先执行的 COUNT 先失败，够不到列表查询分支。
func newRawReportModel(t *testing.T, schema string, inserts ...string) *ReportModel {
	t.Helper()

	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "community.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("exec schema: %v", err)
	}
	for _, q := range inserts {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	return NewReportModel(db)
}

// TestReportListByStatus_MissingColumnQueryFails 仅有 status 的表：
// COUNT(*) 正常、具名列 SELECT 报 no such column → 列表查询错误分支。
func TestReportListByStatus_MissingColumnQueryFails(t *testing.T) {
	m := newRawReportModel(t,
		`CREATE TABLE reports (id INTEGER PRIMARY KEY, status VARCHAR(32) NOT NULL)`,
		`INSERT INTO reports (id, status) VALUES (1, 'pending')`)

	if _, _, err := m.ListByStatus("pending", 20, 0); err == nil {
		t.Fatal("ListByStatus on column-missing table must error")
	}
}

// TestReportListByStatus_DirtyRowScanFails 列齐全但 handled_by 存文本：
// COUNT 与 SELECT 均成功，Scan 到 int64 失败 → 行扫描错误分支。
func TestReportListByStatus_DirtyRowScanFails(t *testing.T) {
	m := newReportTestDB(t)
	if _, err := m.db.Exec(`INSERT INTO reports
		(id, reporter_id, target_type, target_id, reason, status, handled_by, created_at, handled_at)
		VALUES (1, 7, 'post', 100, '违规内容', 'pending', 'x', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert dirty row: %v", err)
	}

	if _, _, err := m.ListByStatus("pending", 20, 0); err == nil {
		t.Fatal("ListByStatus with dirty row must error")
	}
}

func TestReportUpdateStatusIfPending_ClosedDBErrors(t *testing.T) {
	m := newClosedReportModel(t)
	if ok, err := m.UpdateStatusIfPending(1, "resolved", 99); err == nil || ok {
		t.Fatalf("UpdateStatusIfPending on closed db: ok=%v err=%v, want error", ok, err)
	}
}
