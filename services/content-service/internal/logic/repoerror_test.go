package logic

import (
	"errors"
	"net/http"
	"testing"

	"github.com/tappi/tappi/services/content-service/internal/svc"
	"github.com/tappi/tappi/services/content-service/internal/types"
	"github.com/tappi/tappi/services/content-service/model"
)

// --- 故障注入仓储：内嵌真实仓储，仅覆写指定方法注入错误 ---

type failingGuideRepo struct {
	*model.GuideRepository
	getErr     error
	createErr  error
	updateErr  error
	publishErr error
	likeErr    error
}

func (f *failingGuideRepo) Get(id int64) (*model.Guide, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.GuideRepository.Get(id)
}

func (f *failingGuideRepo) Create(guide *model.Guide) (*model.Guide, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.GuideRepository.Create(guide)
}

func (f *failingGuideRepo) Update(id int64, updates map[string]interface{}) (*model.Guide, error) {
	if f.updateErr != nil {
		return nil, f.updateErr
	}
	return f.GuideRepository.Update(id, updates)
}

func (f *failingGuideRepo) Publish(id int64) error {
	if f.publishErr != nil {
		return f.publishErr
	}
	return f.GuideRepository.Publish(id)
}

func (f *failingGuideRepo) Like(id int64) (int, error) {
	if f.likeErr != nil {
		return 0, f.likeErr
	}
	return f.GuideRepository.Like(id)
}

type failingCommentRepo struct {
	*model.CommentRepository
	getErr    error
	createErr error
	deleteErr error
	likeErr   error
}

func (f *failingCommentRepo) Get(id int64) (*model.Comment, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	return f.CommentRepository.Get(id)
}

func (f *failingCommentRepo) Create(comment *model.Comment) (*model.Comment, error) {
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.CommentRepository.Create(comment)
}

func (f *failingCommentRepo) Delete(id int64) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	return f.CommentRepository.Delete(id)
}

func (f *failingCommentRepo) Like(id int64) (int, error) {
	if f.likeErr != nil {
		return 0, f.likeErr
	}
	return f.CommentRepository.Like(id)
}

var errRepoBoom = errors.New("repository on fire")

// withGuide 将 svcCtx 的攻略仓储替换为故障注入版本。
func withGuide(svcCtx *svc.ServiceContext, f *failingGuideRepo) *svc.ServiceContext {
	svcCtx.GuideRepository = f
	return svcCtx
}

// withComment 将 svcCtx 的评论仓储替换为故障注入版本。
func withComment(svcCtx *svc.ServiceContext, f *failingCommentRepo) *svc.ServiceContext {
	svcCtx.CommentRepository = f
	return svcCtx
}

// requireEnvelope 断言信封响应码且 error 恒为 nil。
func requireEnvelope(t *testing.T, resp *types.CreateCommentResponse, err error, wantCode int) {
	t.Helper()
	if err != nil {
		t.Fatalf("err = %v, want nil (envelope pattern)", err)
	}
	if resp.Code != wantCode {
		t.Fatalf("code = %d, want %d", resp.Code, wantCode)
	}
}

// --- GetGuide ---

func TestGetGuide_GetFailed500(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	withGuide(svcCtx, &failingGuideRepo{GuideRepository: svcCtx.GuideRepository.(*model.GuideRepository), getErr: errRepoBoom})
	l := NewGetGuideLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.GetGuide(&types.GetGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusInternalServerError {
		t.Fatalf("resp=%+v err=%v, want 500", resp, err)
	}
}

// --- CreateGuide ---

func TestCreateGuide_CreateFailed500(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	withGuide(svcCtx, &failingGuideRepo{GuideRepository: svcCtx.GuideRepository.(*model.GuideRepository), createErr: errRepoBoom})
	l := NewCreateGuideLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.CreateGuide(&types.CreateGuideRequest{GameId: "g1", Title: "t", Content: "c"})
	if err != nil || resp.Code != http.StatusInternalServerError {
		t.Fatalf("resp=%+v err=%v, want 500", resp, err)
	}
}

// --- UpdateGuide：Get 500 / Update 404 / Update 500 / Summary 字段 ---

func TestUpdateGuide_GetFailed500(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	withGuide(svcCtx, &failingGuideRepo{GuideRepository: svcCtx.GuideRepository.(*model.GuideRepository), getErr: errRepoBoom})
	l := NewUpdateGuideLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.UpdateGuide(&types.UpdateGuideRequest{Id: 1, Title: "t"})
	if err != nil || resp.Code != http.StatusInternalServerError {
		t.Fatalf("resp=%+v err=%v, want 500", resp, err)
	}
}

func TestUpdateGuide_UpdateNotFound404(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	withGuide(svcCtx, &failingGuideRepo{GuideRepository: svcCtx.GuideRepository.(*model.GuideRepository), updateErr: model.ErrGuideNotFound})
	l := NewUpdateGuideLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.UpdateGuide(&types.UpdateGuideRequest{Id: 1, Title: "t"})
	if err != nil || resp.Code != http.StatusNotFound {
		t.Fatalf("resp=%+v err=%v, want 404", resp, err)
	}
}

func TestUpdateGuide_UpdateFailed500(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	withGuide(svcCtx, &failingGuideRepo{GuideRepository: svcCtx.GuideRepository.(*model.GuideRepository), updateErr: errRepoBoom})
	l := NewUpdateGuideLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.UpdateGuide(&types.UpdateGuideRequest{Id: 1, Title: "t"})
	if err != nil || resp.Code != http.StatusInternalServerError {
		t.Fatalf("resp=%+v err=%v, want 500", resp, err)
	}
}

func TestUpdateGuide_SummaryApplied(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	l := NewUpdateGuideLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.UpdateGuide(&types.UpdateGuideRequest{Id: 1, Summary: "新摘要"})
	if err != nil || resp.Code != http.StatusOK {
		t.Fatalf("resp=%+v err=%v, want 200", resp, err)
	}
	if resp.Data.Summary != "新摘要" {
		t.Fatalf("summary not applied: %+v", resp.Data)
	}
}

// --- PublishGuide：Get 500 / Publish 404 / Publish 500 ---

func TestPublishGuide_GetFailed500(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	withGuide(svcCtx, &failingGuideRepo{GuideRepository: svcCtx.GuideRepository.(*model.GuideRepository), getErr: errRepoBoom})
	l := NewPublishGuideLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.PublishGuide(&types.PublishGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusInternalServerError {
		t.Fatalf("resp=%+v err=%v, want 500", resp, err)
	}
}

func TestPublishGuide_PublishNotFound404(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	withGuide(svcCtx, &failingGuideRepo{GuideRepository: svcCtx.GuideRepository.(*model.GuideRepository), publishErr: model.ErrGuideNotFound})
	l := NewPublishGuideLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.PublishGuide(&types.PublishGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusNotFound {
		t.Fatalf("resp=%+v err=%v, want 404", resp, err)
	}
}

func TestPublishGuide_PublishFailed500(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	withGuide(svcCtx, &failingGuideRepo{GuideRepository: svcCtx.GuideRepository.(*model.GuideRepository), publishErr: errRepoBoom})
	l := NewPublishGuideLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.PublishGuide(&types.PublishGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusInternalServerError {
		t.Fatalf("resp=%+v err=%v, want 500", resp, err)
	}
}

// --- LikeGuide：Get 500 / Like 404 / Like 500 ---

func TestLikeGuide_GetFailed500(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	withGuide(svcCtx, &failingGuideRepo{GuideRepository: svcCtx.GuideRepository.(*model.GuideRepository), getErr: errRepoBoom})
	l := NewLikeGuideLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.LikeGuide(&types.LikeGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusInternalServerError {
		t.Fatalf("resp=%+v err=%v, want 500", resp, err)
	}
}

func TestLikeGuide_LikeNotFound404(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	withGuide(svcCtx, &failingGuideRepo{GuideRepository: svcCtx.GuideRepository.(*model.GuideRepository), likeErr: model.ErrGuideNotFound})
	l := NewLikeGuideLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.LikeGuide(&types.LikeGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusNotFound {
		t.Fatalf("resp=%+v err=%v, want 404", resp, err)
	}
}

func TestLikeGuide_LikeFailed500(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	withGuide(svcCtx, &failingGuideRepo{GuideRepository: svcCtx.GuideRepository.(*model.GuideRepository), likeErr: errRepoBoom})
	l := NewLikeGuideLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.LikeGuide(&types.LikeGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusInternalServerError {
		t.Fatalf("resp=%+v err=%v, want 500", resp, err)
	}
}

// --- CreateComment：目标攻略 Get 500 / 评论落库 500 ---

func TestCreateComment_TargetGuideGetFailed500(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	withGuide(svcCtx, &failingGuideRepo{GuideRepository: svcCtx.GuideRepository.(*model.GuideRepository), getErr: errRepoBoom})
	l := NewCreateCommentLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.CreateComment(&types.CreateCommentRequest{TargetType: "guide", TargetId: 1, Content: "x"})
	requireEnvelope(t, resp, err, http.StatusInternalServerError)
}

func TestCreateComment_CreateFailed500(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	withComment(svcCtx, &failingCommentRepo{CommentRepository: svcCtx.CommentRepository.(*model.CommentRepository), createErr: errRepoBoom})
	l := NewCreateCommentLogic(userCtx(1, "u1"), svcCtx)
	// TargetType=game 跳过攻略存在性检查，直达评论创建
	resp, err := l.CreateComment(&types.CreateCommentRequest{TargetType: "game", TargetId: 1, Content: "x"})
	requireEnvelope(t, resp, err, http.StatusInternalServerError)
}

// --- DeleteComment：Get 500 / Delete 404 / Delete 500 ---

func TestDeleteComment_GetFailed500(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	withComment(svcCtx, &failingCommentRepo{CommentRepository: svcCtx.CommentRepository.(*model.CommentRepository), getErr: errRepoBoom})
	l := NewDeleteCommentLogic(userCtx(2, "u2"), svcCtx)
	resp, err := l.DeleteComment(&types.DeleteCommentRequest{Id: 10})
	if err != nil || resp.Code != http.StatusInternalServerError {
		t.Fatalf("resp=%+v err=%v, want 500", resp, err)
	}
}

// newOwnedComment 通过真实仓储创建一条属于用户 2 的评论，返回其 Id。
func newOwnedComment(t *testing.T, svcCtx *svc.ServiceContext) int64 {
	t.Helper()
	repo := svcCtx.CommentRepository.(*model.CommentRepository)
	created, err := repo.Create(&model.Comment{
		TargetType: "game", TargetId: 1, UserId: 2, UserName: "u2", Content: "mine",
	})
	if err != nil {
		t.Fatalf("seed comment: %v", err)
	}
	return created.Id
}

func TestDeleteComment_DeleteNotFound404(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	id := newOwnedComment(t, svcCtx)
	withComment(svcCtx, &failingCommentRepo{CommentRepository: svcCtx.CommentRepository.(*model.CommentRepository), deleteErr: model.ErrCommentNotFound})
	l := NewDeleteCommentLogic(userCtx(2, "u2"), svcCtx)
	resp, err := l.DeleteComment(&types.DeleteCommentRequest{Id: id})
	if err != nil || resp.Code != http.StatusNotFound {
		t.Fatalf("resp=%+v err=%v, want 404", resp, err)
	}
}

func TestDeleteComment_DeleteFailed500(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	id := newOwnedComment(t, svcCtx)
	withComment(svcCtx, &failingCommentRepo{CommentRepository: svcCtx.CommentRepository.(*model.CommentRepository), deleteErr: errRepoBoom})
	l := NewDeleteCommentLogic(userCtx(2, "u2"), svcCtx)
	resp, err := l.DeleteComment(&types.DeleteCommentRequest{Id: id})
	if err != nil || resp.Code != http.StatusInternalServerError {
		t.Fatalf("resp=%+v err=%v, want 500", resp, err)
	}
}

// --- LikeComment：评论 Get 500 / 二级攻略 Get 500 / Like 404 / Like 500 ---

func TestLikeComment_GetFailed500(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	withComment(svcCtx, &failingCommentRepo{CommentRepository: svcCtx.CommentRepository.(*model.CommentRepository), getErr: errRepoBoom})
	l := NewLikeCommentLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.LikeComment(&types.LikeCommentRequest{Id: 10})
	if err != nil || resp.Code != http.StatusInternalServerError {
		t.Fatalf("resp=%+v err=%v, want 500", resp, err)
	}
}

func TestLikeComment_TargetGuideGetFailed500(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	// 评论存在但指向的攻略查询报非哨兵错误 → 500
	withGuide(svcCtx, &failingGuideRepo{GuideRepository: svcCtx.GuideRepository.(*model.GuideRepository), getErr: errRepoBoom})
	l := NewLikeCommentLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.LikeComment(&types.LikeCommentRequest{Id: 10})
	if err != nil || resp.Code != http.StatusInternalServerError {
		t.Fatalf("resp=%+v err=%v, want 500", resp, err)
	}
}

func TestLikeComment_LikeNotFound404(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	id := newOwnedComment(t, svcCtx) // 挂在 game 下，跳过攻略二级检查
	withComment(svcCtx, &failingCommentRepo{CommentRepository: svcCtx.CommentRepository.(*model.CommentRepository), likeErr: model.ErrCommentNotFound})
	l := NewLikeCommentLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.LikeComment(&types.LikeCommentRequest{Id: id})
	if err != nil || resp.Code != http.StatusNotFound {
		t.Fatalf("resp=%+v err=%v, want 404", resp, err)
	}
}

func TestLikeComment_LikeFailed500(t *testing.T) {
	svcCtx := newErrorBranchSvcCtx(t, "")
	id := newOwnedComment(t, svcCtx)
	withComment(svcCtx, &failingCommentRepo{CommentRepository: svcCtx.CommentRepository.(*model.CommentRepository), likeErr: errRepoBoom})
	l := NewLikeCommentLogic(userCtx(1, "u1"), svcCtx)
	resp, err := l.LikeComment(&types.LikeCommentRequest{Id: id})
	if err != nil || resp.Code != http.StatusInternalServerError {
		t.Fatalf("resp=%+v err=%v, want 500", resp, err)
	}
}
