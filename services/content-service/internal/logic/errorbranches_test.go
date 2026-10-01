package logic

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/tappi/tappi/services/content-service/client"
	"github.com/tappi/tappi/services/content-service/internal/svc"
	"github.com/tappi/tappi/services/content-service/internal/types"
	"github.com/tappi/tappi/services/content-service/model"
	"github.com/tappi/tappi/services/content-service/utils"
)

// newErrorBranchSvcCtx 建种子数据仓储（guide 1 属于用户 1 且已发布；
// comment 10 指向不存在的 guide 99999）并装配 ServiceContext。
// gameCatalogBaseURL 传不可达地址可触发「获取游戏信息失败」回退分支。
func newErrorBranchSvcCtx(t *testing.T, gameCatalogBaseURL string) *svc.ServiceContext {
	t.Helper()

	guideRepo, commentRepo := seedContentStores(t,
		[]*model.Guide{
			{Id: 1, GameId: "g1", GameTitle: "G1", Title: "pub", Content: "c", AuthorId: 1, AuthorName: "u1", IsPublished: true, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
		},
		[]*model.Comment{
			{Id: 10, TargetType: "guide", TargetId: 99999, UserId: 2, UserName: "u2", Content: "orphan", Likes: 3, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
		},
	)

	if gameCatalogBaseURL == "" {
		gameCatalogBaseURL = "http://127.0.0.1:1" // 不可达：连接立即被拒
	}

	return &svc.ServiceContext{
		GuideRepository:   guideRepo,
		CommentRepository: commentRepo,
		Auth:              utils.NewAuth("err-branch-secret"),
		GameCatalogClient: client.NewGameCatalogClient(gameCatalogBaseURL, 200*time.Millisecond),
	}
}

func userCtx(id int64, username string) context.Context {
	ctx := context.WithValue(context.Background(), "user_id", id)
	if username != "" {
		ctx = context.WithValue(ctx, "username", username)
	}
	return ctx
}

// --- 401：上下文缺 user_id ---

func TestCreateComment_Unauthorized(t *testing.T) {
	l := NewCreateCommentLogic(context.Background(), newErrorBranchSvcCtx(t, ""))
	resp, err := l.CreateComment(&types.CreateCommentRequest{TargetType: "game", TargetId: 1, Content: "x"})
	if err != nil || resp.Code != http.StatusUnauthorized || resp.Message != "用户认证失败" {
		t.Fatalf("resp=%+v err=%v, want 401 用户认证失败", resp, err)
	}
}

func TestCreateGuide_Unauthorized(t *testing.T) {
	l := NewCreateGuideLogic(context.Background(), newErrorBranchSvcCtx(t, ""))
	resp, err := l.CreateGuide(&types.CreateGuideRequest{GameId: "g1", Title: "t", Content: "c"})
	if err != nil || resp.Code != http.StatusUnauthorized {
		t.Fatalf("resp=%+v err=%v, want 401", resp, err)
	}
}

func TestDeleteComment_Unauthorized(t *testing.T) {
	l := NewDeleteCommentLogic(context.Background(), newErrorBranchSvcCtx(t, ""))
	resp, err := l.DeleteComment(&types.DeleteCommentRequest{Id: 10})
	if err != nil || resp.Code != http.StatusUnauthorized {
		t.Fatalf("resp=%+v err=%v, want 401", resp, err)
	}
}

func TestUpdateGuide_Unauthorized(t *testing.T) {
	l := NewUpdateGuideLogic(context.Background(), newErrorBranchSvcCtx(t, ""))
	resp, err := l.UpdateGuide(&types.UpdateGuideRequest{Id: 1, Title: "t"})
	if err != nil || resp.Code != http.StatusUnauthorized {
		t.Fatalf("resp=%+v err=%v, want 401", resp, err)
	}
}

func TestPublishGuide_Unauthorized(t *testing.T) {
	l := NewPublishGuideLogic(context.Background(), newErrorBranchSvcCtx(t, ""))
	resp, err := l.PublishGuide(&types.PublishGuideRequest{Id: 1})
	if err != nil || resp.Code != http.StatusUnauthorized {
		t.Fatalf("resp=%+v err=%v, want 401", resp, err)
	}
}

// --- 评论：草稿/缺失攻略的 404 与昵称回退 ---

func TestCreateComment_TargetGuideMissing404(t *testing.T) {
	l := NewCreateCommentLogic(userCtx(1, "u1"), newErrorBranchSvcCtx(t, ""))
	resp, err := l.CreateComment(&types.CreateCommentRequest{TargetType: "guide", TargetId: 99999, Content: "x"})
	if err != nil || resp.Code != http.StatusNotFound || resp.Message != "攻略不存在" {
		t.Fatalf("resp=%+v err=%v, want 404 攻略不存在", resp, err)
	}
}

func TestCreateComment_UsernameFallback(t *testing.T) {
	l := NewCreateCommentLogic(userCtx(7, ""), newErrorBranchSvcCtx(t, ""))
	resp, err := l.CreateComment(&types.CreateCommentRequest{TargetType: "game", TargetId: 1, Content: "x"})
	if err != nil || resp.Code != http.StatusOK {
		t.Fatalf("resp=%+v err=%v, want 200", resp, err)
	}
	if resp.Data.UserName != "未知用户" {
		t.Fatalf("username = %q, want 未知用户 fallback", resp.Data.UserName)
	}
}

// --- 攻略创建：游戏拉取失败回退 gameId 标题、昵称回退 ---

func TestCreateGuide_GameFetchFailsFallsBackToGameId(t *testing.T) {
	l := NewCreateGuideLogic(userCtx(1, "u1"), newErrorBranchSvcCtx(t, ""))
	resp, err := l.CreateGuide(&types.CreateGuideRequest{GameId: "g-fallback", Title: "t", Content: "c"})
	if err != nil || resp.Code != http.StatusOK {
		t.Fatalf("resp=%+v err=%v, want 200", resp, err)
	}
	if resp.Data.GameTitle != "g-fallback" {
		t.Fatalf("gameTitle = %q, want gameId fallback", resp.Data.GameTitle)
	}
}

func TestCreateGuide_UsernameFallback(t *testing.T) {
	l := NewCreateGuideLogic(userCtx(7, ""), newErrorBranchSvcCtx(t, ""))
	resp, err := l.CreateGuide(&types.CreateGuideRequest{GameId: "g1", Title: "t", Content: "c"})
	if err != nil || resp.Code != http.StatusOK {
		t.Fatalf("resp=%+v err=%v, want 200", resp, err)
	}
	if resp.Data.AuthorName != "未知用户" {
		t.Fatalf("authorName = %q, want 未知用户 fallback", resp.Data.AuthorName)
	}
}

// --- 点赞/删除：目标缺失 404 ---

func TestLikeComment_CommentMissing404(t *testing.T) {
	l := NewLikeCommentLogic(userCtx(1, "u1"), newErrorBranchSvcCtx(t, ""))
	resp, err := l.LikeComment(&types.LikeCommentRequest{Id: 424242})
	if err != nil || resp.Code != http.StatusNotFound || resp.Message != "评论不存在" {
		t.Fatalf("resp=%+v err=%v, want 404 评论不存在", resp, err)
	}
}

func TestLikeComment_TargetGuideMissing404(t *testing.T) {
	// comment 10 指向不存在的 guide 99999 → 二级检查 404
	l := NewLikeCommentLogic(userCtx(1, "u1"), newErrorBranchSvcCtx(t, ""))
	resp, err := l.LikeComment(&types.LikeCommentRequest{Id: 10})
	if err != nil || resp.Code != http.StatusNotFound {
		t.Fatalf("resp=%+v err=%v, want 404", resp, err)
	}
}

// --- 列表：分页参数归一化、草稿攻略评论 404 ---

func TestListComments_PageDefaults(t *testing.T) {
	l := NewListCommentsLogic(context.Background(), newErrorBranchSvcCtx(t, ""))
	resp, err := l.ListComments(&types.ListCommentsRequest{TargetType: "guide", TargetId: 1})
	if err != nil || resp.Code != http.StatusOK {
		t.Fatalf("resp=%+v err=%v, want 200", resp, err)
	}
}

func TestListComments_TargetGuideMissing404(t *testing.T) {
	l := NewListCommentsLogic(context.Background(), newErrorBranchSvcCtx(t, ""))
	resp, err := l.ListComments(&types.ListCommentsRequest{TargetType: "guide", TargetId: 99999})
	if err != nil || resp.Code != http.StatusNotFound {
		t.Fatalf("resp=%+v err=%v, want 404", resp, err)
	}
}

func TestListGuides_PageDefaults(t *testing.T) {
	l := NewListGuidesLogic(context.Background(), newErrorBranchSvcCtx(t, ""))
	resp, err := l.ListGuides(&types.ListGuidesRequest{})
	if err != nil || resp.Code != http.StatusOK {
		t.Fatalf("resp=%+v err=%v, want 200", resp, err)
	}
}

// --- 攻略更新：cover_image/tags 增量字段进 updates 映射 ---

func TestUpdateGuide_CoverImageAndTagsApplied(t *testing.T) {
	l := NewUpdateGuideLogic(userCtx(1, "u1"), newErrorBranchSvcCtx(t, ""))
	resp, err := l.UpdateGuide(&types.UpdateGuideRequest{
		Id: 1, CoverImage: "cover.png", Tags: []string{"a", "b"},
	})
	if err != nil || resp.Code != http.StatusOK {
		t.Fatalf("resp=%+v err=%v, want 200", resp, err)
	}
	if resp.Data.CoverImage != "cover.png" || len(resp.Data.Tags) != 2 {
		t.Fatalf("cover/tags not applied: %+v", resp.Data)
	}
}

// --- mapper nil 防御分支 ---

func TestModelMappers_NilInputs(t *testing.T) {
	if g := modelGuideToType(nil); g.Id != 0 || g.Title != "" {
		t.Fatalf("modelGuideToType(nil) = %+v, want zero Guide", g)
	}
	if c := modelCommentToType(nil); c.Id != 0 || c.Content != "" {
		t.Fatalf("modelCommentToType(nil) = %+v, want zero Comment", c)
	}
}
