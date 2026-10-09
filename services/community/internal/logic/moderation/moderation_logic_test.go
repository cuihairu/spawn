package moderation

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/community/internal/config"
	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"
)

// newTestServiceContext 与 comment/post 包同款：真实服务上下文 + SQLite 临时库。
// 词表走内置默认（空配置回落），管理员按用例需要显式授予。
func newTestServiceContext(t *testing.T) *svc.ServiceContext {
	t.Helper()

	var c config.Config
	c.Auth.JWTSecret = "test-jwt-secret"
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "community.db")

	return svc.NewServiceContext(c)
}

func authContext(userId int64, username string) context.Context {
	ctx := context.WithValue(context.Background(), "user_id", userId)
	return context.WithValue(ctx, "username", username)
}

func requireHTTPStatus(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want HTTP %d", want)
	}
	var appErr *httperr.Error
	if !errors.As(err, &appErr) {
		t.Fatalf("err = %v (%T), want *httperr.Error with status %d", err, err, want)
	}
	if appErr.Status != want {
		t.Fatalf("status = %d, want %d (message %q)", appErr.Status, want, appErr.Message)
	}
}

// hostPost 建一条宿主帖子并返回 id（作者 id=7「作者甲」）。
func hostPost(t *testing.T, s *svc.ServiceContext) int64 {
	t.Helper()
	p, err := s.PostRepo.Create(1, 7, "作者甲", &types.CreatePostReq{Title: "宿主", Content: "正文"})
	if err != nil {
		t.Fatalf("create host post: %v", err)
	}
	return p.Id
}

func TestReportPost(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)

	l := NewReportPostLogic(authContext(8, "读者乙"), s)
	if resp, err := l.ReportPost(&types.ReportPostReq{Id: postId, Reason: "垃圾广告"}); err != nil || resp == nil {
		t.Fatalf("report post: resp=%v err=%v", resp, err)
	}

	// 同一用户重复举报幂等：仍成功且只有一条记录
	if _, err := l.ReportPost(&types.ReportPostReq{Id: postId}); err != nil {
		t.Fatalf("duplicate report should be idempotent: %v", err)
	}
	list, total, err := s.ReportRepo.ListByStatus("", 20, 0)
	if err != nil || total != 1 || len(list) != 1 {
		t.Fatalf("total=%d len=%d err=%v, want 1", total, len(list), err)
	}
	if list[0].TargetType != "post" || list[0].TargetId != postId || list[0].Status != "pending" || list[0].ReporterId != 8 {
		t.Fatalf("report mismatch: %+v", list[0])
	}

	// 举报不存在的帖子 → ErrPostNotFound（HTTP 层映射 404）
	if _, err := l.ReportPost(&types.ReportPostReq{Id: 999}); !errors.Is(err, model.ErrPostNotFound) {
		t.Fatalf("report missing post: err=%v, want ErrPostNotFound", err)
	}
}

func TestListReports_AdminOnly(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)
	if err := s.ReportRepo.Create(&types.Report{
		ReporterId: 8, TargetType: "post", TargetId: postId,
		Reason: "违规", Status: "pending", CreatedAt: "2026-10-10T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed report: %v", err)
	}

	// 非 admin → 403
	l := NewListReportsLogic(authContext(8, "读者乙"), s)
	if _, err := l.ListReports(&types.ListReportsReq{}); err == nil {
		t.Fatalf("non-admin list: err = nil, want 403")
	} else {
		requireHTTPStatus(t, err, 403)
	}

	// admin（直接授予 AdminSet，等同配置 AdminUserIds）
	s.AdminSet = map[int64]bool{1: true}
	admin := NewListReportsLogic(authContext(1, "管理员"), s)
	resp, err := admin.ListReports(&types.ListReportsReq{})
	if err != nil {
		t.Fatalf("admin list: %v", err)
	}
	if resp == nil || resp.Total != 1 || len(resp.Reports) != 1 {
		t.Fatalf("admin list resp mismatch: %+v", resp)
	}

	// status 过滤 + 分页钳制
	if resp, err := admin.ListReports(&types.ListReportsReq{Status: "pending"}); err != nil || resp.Total != 1 {
		t.Fatalf("filter pending: resp=%v err=%v", resp, err)
	}
	if resp, err := admin.ListReports(&types.ListReportsReq{Status: "resolved"}); err != nil || resp.Total != 0 {
		t.Fatalf("filter resolved: resp=%v err=%v", resp, err)
	}
	if resp, err := admin.ListReports(&types.ListReportsReq{Status: "resolved", Limit: 5, Offset: 0}); err != nil {
		t.Fatalf("clamped list: %v", err)
	} else if resp.Reports == nil {
		t.Fatalf("empty result should be empty slice, got nil")
	}
}

func TestHandleReport_Dismiss(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)
	if err := s.ReportRepo.Create(&types.Report{
		ReporterId: 8, TargetType: "post", TargetId: postId,
		Reason: "误报", Status: "pending", CreatedAt: "2026-10-10T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed report: %v", err)
	}
	s.AdminSet = map[int64]bool{1: true}

	l := NewHandleReportLogic(authContext(1, "管理员"), s)
	if resp, err := l.HandleReport(&types.HandleReportReq{Id: 1, Action: "dismiss"}); err != nil || resp == nil {
		t.Fatalf("dismiss: resp=%v err=%v", resp, err)
	}

	// 驳回：内容保留，状态流转，二次处置拒绝
	if _, err := s.PostRepo.Get(postId); err != nil {
		t.Fatalf("dismissed post should remain visible: %v", err)
	}
	r, err := s.ReportRepo.GetByID(1)
	if err != nil || r.Status != "dismissed" || r.HandledBy != 1 || r.HandledAt == "" {
		t.Fatalf("after dismiss: report=%+v err=%v", r, err)
	}
	if _, err := l.HandleReport(&types.HandleReportReq{Id: 1, Action: "resolve"}); err == nil {
		t.Fatalf("re-handle should fail, want 400")
	} else {
		requireHTTPStatus(t, err, 400)
	}
}

func TestHandleReport_ResolveRemovesContent(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)
	commentId, err := s.CommentRepo.Create(&types.Comment{
		PostId: postId, AuthorId: 8, AuthorName: "读者乙", Content: "违规评论",
		CreatedAt: "2026-10-10T00:00:00Z", UpdatedAt: "2026-10-10T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("seed comment: %v", err)
	}

	if err := s.ReportRepo.Create(&types.Report{
		ReporterId: 9, TargetType: "post", TargetId: postId,
		Reason: "违规帖", Status: "pending", CreatedAt: "2026-10-10T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed post report: %v", err)
	}
	if err := s.ReportRepo.Create(&types.Report{
		ReporterId: 9, TargetType: "comment", TargetId: commentId,
		Reason: "违规评", Status: "pending", CreatedAt: "2026-10-10T00:00:01Z",
	}); err != nil {
		t.Fatalf("seed comment report: %v", err)
	}
	s.AdminSet = map[int64]bool{1: true}

	l := NewHandleReportLogic(authContext(1, "管理员"), s)

	// resolve 帖子举报：帖子软删（Get 同不存在），状态 resolved
	if _, err := l.HandleReport(&types.HandleReportReq{Id: 1, Action: "resolve"}); err != nil {
		t.Fatalf("resolve post report: %v", err)
	}
	if _, err := s.PostRepo.Get(postId); err == nil {
		t.Fatalf("resolved post should be gone")
	}
	r, err := s.ReportRepo.GetByID(1)
	if err != nil || r.Status != "resolved" || r.HandledBy != 1 || r.HandledAt == "" {
		t.Fatalf("after resolve: report=%+v err=%v", r, err)
	}

	// resolve 评论举报：评论物理删除
	if _, err := l.HandleReport(&types.HandleReportReq{Id: 2, Action: "resolve"}); err != nil {
		t.Fatalf("resolve comment report: %v", err)
	}
	if _, err := s.CommentRepo.GetByID(commentId); err == nil {
		t.Fatalf("resolved comment should be gone")
	}
}

func TestHandleReport_Guards(t *testing.T) {
	s := newTestServiceContext(t)
	postId := hostPost(t, s)
	if err := s.ReportRepo.Create(&types.Report{
		ReporterId: 8, TargetType: "post", TargetId: postId,
		Reason: "违规", Status: "pending", CreatedAt: "2026-10-10T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed report: %v", err)
	}

	// 非 admin → 403
	l := NewHandleReportLogic(authContext(8, "读者乙"), s)
	if _, err := l.HandleReport(&types.HandleReportReq{Id: 1, Action: "dismiss"}); err == nil {
		t.Fatalf("non-admin handle: err = nil, want 403")
	} else {
		requireHTTPStatus(t, err, 403)
	}

	s.AdminSet = map[int64]bool{1: true}
	admin := NewHandleReportLogic(authContext(1, "管理员"), s)

	// 非法 action → 400
	if _, err := admin.HandleReport(&types.HandleReportReq{Id: 1, Action: "delete"}); err == nil {
		t.Fatalf("invalid action: err = nil, want 400")
	} else {
		requireHTTPStatus(t, err, 400)
	}

	// 不存在的举报 → ErrReportNotFound（HTTP 层映射 404）
	if _, err := admin.HandleReport(&types.HandleReportReq{Id: 999, Action: "dismiss"}); !errors.Is(err, model.ErrReportNotFound) {
		t.Fatalf("missing report: err=%v, want ErrReportNotFound", err)
	}
}
