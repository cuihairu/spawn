package logic

import (
	"context"
	"testing"

	"github.com/tappi/tappi/services/content-service/internal/svc"
	"github.com/tappi/tappi/services/content-service/internal/types"
	"github.com/tappi/tappi/services/content-service/model"
	"github.com/tappi/tappi/services/content-service/utils"
)

func TestGuideVisibilityAndOwnership(t *testing.T) {
	t.Parallel()

	guideRepo, commentRepo := seedContentStores(t,
		[]*model.Guide{
			{Id: 1, GameId: "g1", GameTitle: "G1", Title: "draft", Content: "c", AuthorId: 1, AuthorName: "u1", IsPublished: false, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
			{Id: 2, GameId: "g1", GameTitle: "G1", Title: "pub1", Content: "c", AuthorId: 1, AuthorName: "u1", IsPublished: true, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
			{Id: 3, GameId: "g2", GameTitle: "G2", Title: "pub2", Content: "c", AuthorId: 2, AuthorName: "u2", IsPublished: true, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
			{Id: 4, GameId: "g1", GameTitle: "G1", Title: "draft2", Content: "c", AuthorId: 1, AuthorName: "u1", IsPublished: false, CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
		},
		[]*model.Comment{
			{Id: 1, TargetType: "guide", TargetId: 2, UserId: 1, UserName: "u1", Content: "c1", CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
			{Id: 2, TargetType: "guide", TargetId: 4, UserId: 2, UserName: "u2", Content: "leak", CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
		},
	)

	svcCtx := &svc.ServiceContext{
		GuideRepository:   guideRepo,
		CommentRepository: commentRepo,
		Auth:              utils.NewAuth("test-secret"),
	}

	t.Run("ListGuides anonymous only sees published", func(t *testing.T) {
		l := NewListGuidesLogic(context.Background(), svcCtx)

		resp, err := l.ListGuides(&types.ListGuidesRequest{AuthorId: 1, Page: 1, PageSize: 20})
		if err != nil {
			t.Fatalf("ListGuides err: %v", err)
		}
		if resp.Code != 200 {
			t.Fatalf("unexpected code: %d", resp.Code)
		}
		if len(resp.Data) != 1 || resp.Data[0].Id != 2 {
			t.Fatalf("unexpected guides: %+v", resp.Data)
		}
	})

	t.Run("ListGuides author sees drafts", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), "user_id", int64(1))
		ctx = context.WithValue(ctx, "username", "u1")
		l := NewListGuidesLogic(ctx, svcCtx)

		resp, err := l.ListGuides(&types.ListGuidesRequest{AuthorId: 1, Page: 1, PageSize: 20})
		if err != nil {
			t.Fatalf("ListGuides err: %v", err)
		}
		if resp.Code != 200 {
			t.Fatalf("unexpected code: %d", resp.Code)
		}
		if len(resp.Data) != 3 {
			t.Fatalf("unexpected count: %d", len(resp.Data))
		}
	})

	t.Run("GetGuide draft hidden from others", func(t *testing.T) {
		anon := NewGetGuideLogic(context.Background(), svcCtx)
		resp, err := anon.GetGuide(&types.GetGuideRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetGuide err: %v", err)
		}
		if resp.Code != 404 {
			t.Fatalf("unexpected code: %d", resp.Code)
		}

		otherCtx := context.WithValue(context.Background(), "user_id", int64(2))
		other := NewGetGuideLogic(otherCtx, svcCtx)
		resp, err = other.GetGuide(&types.GetGuideRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetGuide err: %v", err)
		}
		if resp.Code != 404 {
			t.Fatalf("unexpected code: %d", resp.Code)
		}

		authorCtx := context.WithValue(context.Background(), "user_id", int64(1))
		author := NewGetGuideLogic(authorCtx, svcCtx)
		resp, err = author.GetGuide(&types.GetGuideRequest{Id: 1})
		if err != nil {
			t.Fatalf("GetGuide err: %v", err)
		}
		if resp.Code != 200 || resp.Data.Id != 1 {
			t.Fatalf("unexpected resp: %+v", resp)
		}
	})

	t.Run("UpdateGuide requires ownership", func(t *testing.T) {
		otherCtx := context.WithValue(context.Background(), "user_id", int64(2))
		other := NewUpdateGuideLogic(otherCtx, svcCtx)
		resp, err := other.UpdateGuide(&types.UpdateGuideRequest{Id: 2, Title: "x"})
		if err != nil {
			t.Fatalf("UpdateGuide err: %v", err)
		}
		if resp.Code != 403 {
			t.Fatalf("unexpected code: %d", resp.Code)
		}

		authorCtx := context.WithValue(context.Background(), "user_id", int64(1))
		author := NewUpdateGuideLogic(authorCtx, svcCtx)
		resp, err = author.UpdateGuide(&types.UpdateGuideRequest{Id: 2, Title: "new-title"})
		if err != nil {
			t.Fatalf("UpdateGuide err: %v", err)
		}
		if resp.Code != 200 || resp.Data.Title != "new-title" {
			t.Fatalf("unexpected resp: %+v", resp)
		}
	})

	t.Run("PublishGuide requires ownership", func(t *testing.T) {
		otherCtx := context.WithValue(context.Background(), "user_id", int64(2))
		other := NewPublishGuideLogic(otherCtx, svcCtx)
		resp, err := other.PublishGuide(&types.PublishGuideRequest{Id: 1})
		if err != nil {
			t.Fatalf("PublishGuide err: %v", err)
		}
		if resp.Code != 403 {
			t.Fatalf("unexpected code: %d", resp.Code)
		}

		authorCtx := context.WithValue(context.Background(), "user_id", int64(1))
		author := NewPublishGuideLogic(authorCtx, svcCtx)
		resp, err = author.PublishGuide(&types.PublishGuideRequest{Id: 1})
		if err != nil {
			t.Fatalf("PublishGuide err: %v", err)
		}
		if resp.Code != 200 {
			t.Fatalf("unexpected code: %d", resp.Code)
		}
	})

	t.Run("DeleteComment requires ownership", func(t *testing.T) {
		otherCtx := context.WithValue(context.Background(), "user_id", int64(2))
		other := NewDeleteCommentLogic(otherCtx, svcCtx)
		resp, err := other.DeleteComment(&types.DeleteCommentRequest{Id: 1})
		if err != nil {
			t.Fatalf("DeleteComment err: %v", err)
		}
		if resp.Code != 403 {
			t.Fatalf("unexpected code: %d", resp.Code)
		}

		authorCtx := context.WithValue(context.Background(), "user_id", int64(1))
		author := NewDeleteCommentLogic(authorCtx, svcCtx)
		resp, err = author.DeleteComment(&types.DeleteCommentRequest{Id: 1})
		if err != nil {
			t.Fatalf("DeleteComment err: %v", err)
		}
		if resp.Code != 200 {
			t.Fatalf("unexpected code: %d", resp.Code)
		}
	})

	t.Run("Draft guide disables comments and likes", func(t *testing.T) {
		anonList := NewListCommentsLogic(context.Background(), svcCtx)
		listResp, err := anonList.ListComments(&types.ListCommentsRequest{TargetType: "guide", TargetId: 4, Page: 1, PageSize: 20})
		if err != nil {
			t.Fatalf("ListComments err: %v", err)
		}
		if listResp.Code != 404 {
			t.Fatalf("unexpected code: %d", listResp.Code)
		}

		userCtx := context.WithValue(context.Background(), "user_id", int64(2))
		userCtx = context.WithValue(userCtx, "username", "u2")
		create := NewCreateCommentLogic(userCtx, svcCtx)
		createResp, err := create.CreateComment(&types.CreateCommentRequest{TargetType: "guide", TargetId: 4, Content: "x"})
		if err != nil {
			t.Fatalf("CreateComment err: %v", err)
		}
		if createResp.Code != 404 {
			t.Fatalf("unexpected code: %d", createResp.Code)
		}

		likeGuide := NewLikeGuideLogic(userCtx, svcCtx)
		likeGuideResp, err := likeGuide.LikeGuide(&types.LikeGuideRequest{Id: 4})
		if err != nil {
			t.Fatalf("LikeGuide err: %v", err)
		}
		if likeGuideResp.Code != 404 {
			t.Fatalf("unexpected code: %d", likeGuideResp.Code)
		}

		likeComment := NewLikeCommentLogic(userCtx, svcCtx)
		likeCommentResp, err := likeComment.LikeComment(&types.LikeCommentRequest{Id: 2})
		if err != nil {
			t.Fatalf("LikeComment err: %v", err)
		}
		if likeCommentResp.Code != 404 {
			t.Fatalf("unexpected code: %d", likeCommentResp.Code)
		}
	})
}
