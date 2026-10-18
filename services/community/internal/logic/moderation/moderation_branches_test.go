package moderation

import (
	"context"
	"errors"
	"testing"

	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/internal/types"
)

// failingReportStore 仅 UpdateStatusIfPending 投毒的故障注入仓储（其余方法
// 不可达，嵌入接口即可满足 ReportStore）。
type failingReportStore struct {
	model.ReportStore
	updateErr error
}

func (s failingReportStore) UpdateStatusIfPending(int64, string, int64) (bool, error) {
	return false, s.updateErr
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

// failingReportListStore 仅 ListByStatus 投毒。
type failingReportListStore struct {
	model.ReportStore
	err error
}

func (s failingReportListStore) ListByStatus(string, int64, int64) ([]*types.Report, int64, error) {
	return nil, 0, s.err
}

// failingReportCreateStore 仅 Create 投毒。
type failingReportCreateStore struct {
	model.ReportStore
	err error
}

func (s failingReportCreateStore) Create(*types.Report) error { return s.err }

// --- 鉴权上下文缺失：handler 链路被 Auth 中间件拦截，logic 直调才可达 ---

func TestListReports_WithoutAuthContextFails(t *testing.T) {
	s := newTestServiceContext(t)
	if _, err := NewListReportsLogic(context.Background(), s).
		ListReports(&types.ListReportsReq{}); err == nil {
		t.Fatal("list reports without auth context must fail")
	}
}

func TestHandleReport_WithoutAuthContextFails(t *testing.T) {
	s := newTestServiceContext(t)
	if _, err := NewHandleReportLogic(context.Background(), s).
		HandleReport(&types.HandleReportReq{Id: 1, Action: "dismiss"}); err == nil {
		t.Fatal("handle report without auth context must fail")
	}
}

// TestListReports_ListByStatusErrorPropagates 队列查询失败 → 错误透传。
func TestListReports_ListByStatusErrorPropagates(t *testing.T) {
	s := newTestServiceContext(t)
	s.AdminSet = map[int64]bool{1: true}
	s.ReportRepo = failingReportListStore{ReportStore: s.ReportRepo, err: errors.New("queue on fire")}

	if _, err := NewListReportsLogic(authContext(1, "管理员"), s).
		ListReports(&types.ListReportsReq{}); err == nil {
		t.Fatal("list failure must propagate")
	}
}

// stalePendingStore GetByID 永远返回 pending 快照，其余透传真实仓储：
// 模拟「先查后改」窗口内举报已被并发处置、后到者读到旧状态的竞态。
type stalePendingStore struct {
	model.ReportStore
}

func (s stalePendingStore) GetByID(id int64) (*types.Report, error) {
	r, err := s.ReportStore.GetByID(id)
	if err != nil {
		return nil, err
	}
	stale := *r
	stale.Status = "pending"
	return &stale, nil
}

// TestHandleReport_ConcurrentHandleLosesRace 原子占位：读到 pending 旧快照但
// 条件更新竞争失败（已被并发处置）→ 409 Conflict，先到者的结果不被覆盖。
func TestHandleReport_ConcurrentHandleLosesRace(t *testing.T) {
	s := newTestServiceContext(t)
	s.AdminSet = map[int64]bool{1: true}
	postId := hostPost(t, s)
	if err := s.ReportRepo.Create(&types.Report{
		ReporterId: 8, TargetType: "post", TargetId: postId,
		Reason: "违规", Status: "pending", CreatedAt: "2026-10-10T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed report: %v", err)
	}

	// 竞争对手抢先处置成功（真实仓储条件更新），再蒙上过期读快照
	realRepo := s.ReportRepo
	if ok, err := realRepo.UpdateStatusIfPending(1, "dismissed", 2); err != nil || !ok {
		t.Fatalf("pre-claim: ok=%v err=%v, want claim success", ok, err)
	}
	s.ReportRepo = stalePendingStore{ReportStore: realRepo}

	_, err := NewHandleReportLogic(authContext(1, "管理员"), s).
		HandleReport(&types.HandleReportReq{Id: 1, Action: "resolve"})
	requireHTTPStatus(t, err, 409)

	// 先到者的处置结果不被覆盖（经真实仓储读取，绕开过期快照）
	r, err := realRepo.GetByID(1)
	if err != nil || r.Status != "dismissed" || r.HandledBy != 2 {
		t.Fatalf("report after race = %+v err=%v, want dismissed by 2", r, err)
	}
}

// TestReportStore_UpdateStatusIfPending_DoubleHandle 模型层双处置：
// 第一次占位成功，第二次返回 false 且状态保留首次结果。
func TestReportStore_UpdateStatusIfPending_DoubleHandle(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)
	if err := s.ReportRepo.Create(&types.Report{
		ReporterId: 8, TargetType: "post", TargetId: postId,
		Reason: "违规", Status: "pending", CreatedAt: "2026-10-10T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed report: %v", err)
	}

	ok, err := s.ReportRepo.UpdateStatusIfPending(1, "resolved", 1)
	if err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v, want true", ok, err)
	}
	ok, err = s.ReportRepo.UpdateStatusIfPending(1, "dismissed", 2)
	if err != nil {
		t.Fatalf("second claim err: %v", err)
	}
	if ok {
		t.Fatal("second claim must lose (pending-only guard)")
	}
	r, err := s.ReportRepo.GetByID(1)
	if err != nil || r.Status != "resolved" || r.HandledBy != 1 {
		t.Fatalf("report = %+v err=%v, want resolved by 1", r, err)
	}
}

// TestHandleReport_ResolveUpdateStatusErrorPropagates resolve 路径：内容移除
// 成功后状态落库失败 → 错误透传（dismiss 方向已有同款用例）。
func TestHandleReport_ResolveUpdateStatusErrorPropagates(t *testing.T) {
	s := newTestServiceContext(t)
	s.AdminSet = map[int64]bool{1: true}
	p, err := s.PostRepo.Create(1, 7, "作者甲", &types.CreatePostReq{Title: "违规帖", Content: "正文"})
	if err != nil {
		t.Fatalf("seed post: %v", err)
	}
	if err := s.ReportRepo.Create(&types.Report{
		ReporterId: 8, TargetType: "post", TargetId: p.Id,
		Reason: "违规", Status: "pending", CreatedAt: "2026-10-10T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed report: %v", err)
	}
	s.ReportRepo = failingReportStore{ReportStore: s.ReportRepo, updateErr: errors.New("status write on fire")}

	if _, err := NewHandleReportLogic(authContext(1, "管理员"), s).
		HandleReport(&types.HandleReportReq{Id: 1, Action: "resolve"}); err == nil {
		t.Fatal("resolve status failure must propagate")
	}
}

// TestReportPost_CreateErrorPropagates 举报写库失败 → 错误透传。
func TestReportPost_CreateErrorPropagates(t *testing.T) {
	s := newTestServiceContext(t)
	s.ReportRepo = failingReportCreateStore{ReportStore: s.ReportRepo, err: errors.New("report write on fire")}

	if _, err := NewReportPostLogic(authContext(8, "读者乙"), s).
		ReportPost(&types.ReportPostReq{Id: 1, Reason: "违规"}); err == nil {
		t.Fatal("report create failure must propagate")
	}
}
