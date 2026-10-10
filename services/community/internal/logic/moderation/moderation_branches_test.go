package moderation

import (
	"context"
	"errors"
	"testing"

	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/internal/types"
)

// failingReportStore 仅 UpdateStatus 投毒的故障注入仓储（其余方法不可达，
// 嵌入接口即可满足 ReportStore）。
type failingReportStore struct {
	model.ReportStore
	updateErr error
}

func (s failingReportStore) UpdateStatus(int64, string, int64) error {
	return s.updateErr
}

// --- ReportPost：守卫分支 ---

func TestReportPost_Unauthorized(t *testing.T) {
	s := newTestServiceContext(t)
	_, err := NewReportPostLogic(context.Background(), s).
		ReportPost(&types.ReportPostReq{Id: 1})
	requireHTTPStatus(t, err, 401)
}

func TestReportPost_InvalidRequest(t *testing.T) {
	s := newTestServiceContext(t)
	l := NewReportPostLogic(authContext(8, "读者乙"), s)
	for _, req := range []*types.ReportPostReq{nil, {Id: 0}, {Id: -1}} {
		if _, err := l.ReportPost(req); err == nil {
			t.Fatalf("req=%+v: err = nil, want 400", req)
		} else {
			requireHTTPStatus(t, err, 400)
		}
	}
}

// --- ListReports：nil 请求与分页钳制（防御分支） ---

func TestListReports_NilRequestAndClamps(t *testing.T) {
	s := newTestServiceContext(t)
	s.AdminSet = map[int64]bool{1: true}
	admin := NewListReportsLogic(authContext(1, "管理员"), s)

	resp, err := admin.ListReports(nil)
	if err != nil || resp == nil || resp.Total != 0 {
		t.Fatalf("nil req: resp=%v err=%v, want empty page", resp, err)
	}
	if resp.Reports == nil {
		t.Fatalf("empty result should be empty slice, got nil")
	}

	// limit<=0 → 20、limit>100 → 100、offset<0 → 0
	resp2, err := admin.ListReports(&types.ListReportsReq{Limit: 500, Offset: -5})
	if err != nil || resp2 == nil || resp2.Total != 0 {
		t.Fatalf("clamped req: resp=%v err=%v", resp2, err)
	}
}

// --- HandleReport：nil 请求与非法 id ---

func TestHandleReport_NilAndInvalidRequest(t *testing.T) {
	s := newTestServiceContext(t)
	s.AdminSet = map[int64]bool{1: true}
	admin := NewHandleReportLogic(authContext(1, "管理员"), s)
	if _, err := admin.HandleReport(nil); err == nil {
		t.Fatalf("nil req: err = nil, want 400")
	} else {
		requireHTTPStatus(t, err, 400)
	}
	if _, err := admin.HandleReport(&types.HandleReportReq{Id: 0, Action: "dismiss"}); err == nil {
		t.Fatalf("zero id: err = nil, want 400")
	} else {
		requireHTTPStatus(t, err, 400)
	}
}

// resolve 遇到未支持的 target_type → 400，帖子举报同路径不受影响

func TestHandleReport_UnsupportedTargetType(t *testing.T) {
	s := newTestServiceContext(t)
	if err := s.ReportRepo.Create(&types.Report{
		ReporterId: 8, TargetType: "video", TargetId: 1,
		Reason: "不支持的目标", Status: "pending", CreatedAt: "2026-10-10T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed report: %v", err)
	}
	s.AdminSet = map[int64]bool{1: true}

	l := NewHandleReportLogic(authContext(1, "管理员"), s)
	if _, err := l.HandleReport(&types.HandleReportReq{Id: 1, Action: "resolve"}); err == nil {
		t.Fatalf("unsupported target: err = nil, want 400")
	} else {
		requireHTTPStatus(t, err, 400)
	}
	// 状态未流转（处置失败不落库）
	r, err := s.ReportRepo.GetByID(1)
	if err != nil || r.Status != "pending" {
		t.Fatalf("report after failed resolve: %+v err=%v, want still pending", r, err)
	}
}

// 处置写状态失败 → 错误透传（内容已移除但状态未落库，属降级路径）

func TestHandleReport_StatusUpdateError(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)
	if err := s.ReportRepo.Create(&types.Report{
		ReporterId: 8, TargetType: "post", TargetId: postId,
		Reason: "违规", Status: "pending", CreatedAt: "2026-10-10T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed report: %v", err)
	}
	s.ReportRepo = failingReportStore{ReportStore: s.ReportRepo, updateErr: errors.New("disk on fire")}
	s.AdminSet = map[int64]bool{1: true}

	l := NewHandleReportLogic(authContext(1, "管理员"), s)
	if _, err := l.HandleReport(&types.HandleReportReq{Id: 1, Action: "dismiss"}); err == nil {
		t.Fatalf("status update failure: err = nil, want error")
	}
}
