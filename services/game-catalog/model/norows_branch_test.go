package model

// 各哨兵的 ErrNoRows 分支：单查未命中 → 对应哨兵上抛（与关库通用错误臂区分）。
// 其余模型层未覆盖块全属登记不可构造家族（MySQL SHOW COLUMNS 成功路径、
// LastInsertId/RowsAffected 失败臂、COUNT 成功后再查询失败臂、迁移函数永不
// 返回错误的 return err 臂、聚合 ErrNoRows 等），见 coverage 台账。

import "testing"

// GetReservation 未命中 → ErrReservationNotFound。
func TestServerModel_GetReservationNoRows(t *testing.T) {
	m := newTestAdminModel(t)
	if _, err := m.GetReservation(9999, 9999); err != ErrReservationNotFound {
		t.Fatalf("GetReservation missing = %v, want ErrReservationNotFound", err)
	}
}

// getReviewReport / GetReviewReport 未命中 → ErrReviewReportNotFound。
func TestUserReviewModel_ReportNoRows(t *testing.T) {
	m := newTestAdminModel(t)
	if _, err := m.getReviewReport(9999, 9999); err != ErrReviewReportNotFound {
		t.Fatalf("getReviewReport missing = %v, want ErrReviewReportNotFound", err)
	}
	if _, err := m.GetReviewReport(9999); err != ErrReviewReportNotFound {
		t.Fatalf("GetReviewReport missing = %v, want ErrReviewReportNotFound", err)
	}
}
