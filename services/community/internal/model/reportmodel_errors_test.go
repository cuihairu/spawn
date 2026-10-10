package model

import (
	"testing"
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
