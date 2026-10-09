package model

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

func newReportTestDB(t *testing.T) *ReportModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "community.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	m := NewReportModel(db)
	if err := m.CreateReportsTable(); err != nil {
		t.Fatalf("create reports table: %v", err)
	}
	return m
}

func report(reporter int64, targetType string, targetId int64) *types.Report {
	return &types.Report{
		ReporterId: reporter, TargetType: targetType, TargetId: targetId,
		Reason: "违规内容", Status: "pending", CreatedAt: "2026-10-10T00:00:00Z",
	}
}

func TestCreateReportsTable_Twice(t *testing.T) {
	m := newReportTestDB(t)
	if err := m.CreateReportsTable(); err != nil {
		t.Fatalf("second create should be no-op: %v", err)
	}
}

func TestReportCreate_DedupsSameReporter(t *testing.T) {
	m := newReportTestDB(t)

	for i := 0; i < 2; i++ {
		if err := m.Create(report(8, "post", 100)); err != nil {
			t.Fatalf("create #%d: %v", i+1, err)
		}
	}
	list, total, err := m.ListByStatus("", 20, 0)
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("total=%d len=%d err=%v, want 1", total, len(list), err)
	}
}

func TestReportList_FilterByStatusAndPaging(t *testing.T) {
	m := newReportTestDB(t)

	for i := 0; i < 3; i++ {
		if err := m.Create(report(int64(8+i), "post", int64(100+i))); err != nil {
			t.Fatalf("seed %d: %v", i, err)
		}
	}
	if err := m.UpdateStatus(1, "resolved", 1); err != nil {
		t.Fatalf("update status: %v", err)
	}

	pending, total, err := m.ListByStatus("pending", 20, 0)
	if err != nil || total != 2 || len(pending) != 2 {
		t.Fatalf("pending total=%d len=%d err=%v, want 2", total, len(pending), err)
	}
	all, totalAll, err := m.ListByStatus("", 20, 0)
	if err != nil || totalAll != 3 || len(all) != 3 {
		t.Fatalf("all total=%d len=%d err=%v, want 3", totalAll, len(all), err)
	}
	if all[0].TargetId != 102 || all[2].TargetId != 100 {
		t.Fatalf("order mismatch: %+v", all)
	}

	page, _, err := m.ListByStatus("", 2, 2)
	if err != nil || len(page) != 1 || page[0].TargetId != 100 {
		t.Fatalf("page2 mismatch: %+v", page)
	}
}

func TestReportUpdateStatus_Missing(t *testing.T) {
	m := newReportTestDB(t)

	if err := m.UpdateStatus(999, "resolved", 1); err != ErrReportNotFound {
		t.Fatalf("update missing: err=%v, want ErrReportNotFound", err)
	}
	if _, err := m.GetByID(999); err != ErrReportNotFound {
		t.Fatalf("get missing: err=%v, want ErrReportNotFound", err)
	}
}
