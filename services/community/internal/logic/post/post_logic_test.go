package post

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

// newTestServiceContext 为逻辑层单测构造真实服务上下文：
// 仓储全部落在 t.TempDir()，缺文件时自动播种（话题 1/2、帖子 1/2），不 mock 真实契约。
func newTestServiceContext(t *testing.T) *svc.ServiceContext {
	t.Helper()

	dir := t.TempDir()
	var c config.Config
	c.Auth.JWTSecret = "test-jwt-secret"
	c.DataSource.TopicsFile = filepath.Join(dir, "topics.json")
	c.DataSource.PostsFile = filepath.Join(dir, "posts.json")
	c.DataSource.FollowsFile = filepath.Join(dir, "follows.json")

	return svc.NewServiceContext(c)
}

// authContext 模拟鉴权中间件注入的用户信息（键名与 middleware 保持一致）。
func authContext(userId int64, username string) context.Context {
	ctx := context.WithValue(context.Background(), "user_id", userId)
	return context.WithValue(ctx, "username", username)
}

// requireHTTPStatus 断言逻辑层返回的 httperr 状态码。
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

// --- CreatePost ---

func TestCreatePost(t *testing.T) {
	t.Run("unauthorized without user in context", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewCreatePostLogic(context.Background(), svcCtx)
		_, err := l.CreatePost(&types.CreatePostReq{TopicId: 1, Title: "t", Content: "c"})
		requireHTTPStatus(t, err, 401)
	})

	t.Run("validation branches", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewCreatePostLogic(authContext(1001, "demo"), svcCtx)

		cases := []struct {
			name string
			req  *types.CreatePostReq
		}{
			{"nil request", nil},
			{"topic id zero", &types.CreatePostReq{Title: "t", Content: "c"}},
			{"topic id negative", &types.CreatePostReq{TopicId: -1, Title: "t", Content: "c"}},
			{"blank title", &types.CreatePostReq{TopicId: 1, Title: "  ", Content: "c"}},
			{"blank content", &types.CreatePostReq{TopicId: 1, Title: "t", Content: "  "}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				_, err := l.CreatePost(tc.req)
				requireHTTPStatus(t, err, 400)
			})
		}
	})

	t.Run("unknown topic", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewCreatePostLogic(authContext(1001, "demo"), svcCtx)
		_, err := l.CreatePost(&types.CreatePostReq{TopicId: 9999, Title: "t", Content: "c"})
		if !errors.Is(err, model.ErrTopicNotFound) {
			t.Fatalf("err = %v, want ErrTopicNotFound", err)
		}
	})

	t.Run("success attributes author and bumps topic count", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewCreatePostLogic(authContext(2001, "alice"), svcCtx)

		beforeTopic, err := svcCtx.TopicRepo.Get(1)
		if err != nil {
			t.Fatalf("TopicRepo.Get: %v", err)
		}
		beforeCount := beforeTopic.PostCount // 值拷贝：TopicRepo 返回仓储内指针，后续自增会原地修改

		resp, err := l.CreatePost(&types.CreatePostReq{
			TopicId: 1, Title: "新人报道", Content: "大家好", Type: "discussion", Tags: []string{"新人"},
		})
		if err != nil {
			t.Fatalf("CreatePost: %v", err)
		}
		if resp.Post.Id <= 0 {
			t.Fatalf("post id = %d, want positive", resp.Post.Id)
		}
		if resp.Post.AuthorId != 2001 || resp.Post.AuthorName != "alice" {
			t.Fatalf("author = %d/%q, want 2001/alice", resp.Post.AuthorId, resp.Post.AuthorName)
		}
		if resp.Post.TopicId != 1 {
			t.Fatalf("topic = %d, want 1", resp.Post.TopicId)
		}

		after, err := svcCtx.TopicRepo.Get(1)
		if err != nil {
			t.Fatalf("TopicRepo.Get: %v", err)
		}
		if after.PostCount != beforeCount+1 {
			t.Fatalf("topic post count = %d, want %d", after.PostCount, beforeCount+1)
		}
	})
}

// --- DeletePost ---

func TestDeletePost(t *testing.T) {
	t.Run("unauthorized without user in context", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewDeletePostLogic(context.Background(), svcCtx)
		_, err := l.DeletePost(&types.DeletePostReq{Id: 1})
		requireHTTPStatus(t, err, 401)
	})

	t.Run("validation branches", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewDeletePostLogic(authContext(1001, "demo"), svcCtx)

		if _, err := l.DeletePost(nil); err == nil {
			t.Fatal("nil request must be rejected")
		} else {
			requireHTTPStatus(t, err, 400)
		}
		if _, err := l.DeletePost(&types.DeletePostReq{Id: 0}); err == nil {
			t.Fatal("zero id must be rejected")
		} else {
			requireHTTPStatus(t, err, 400)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewDeletePostLogic(authContext(1001, "demo"), svcCtx)
		_, err := l.DeletePost(&types.DeletePostReq{Id: 9999})
		if !errors.Is(err, model.ErrPostNotFound) {
			t.Fatalf("err = %v, want ErrPostNotFound", err)
		}
	})

	t.Run("forbidden for non-author", func(t *testing.T) {
		svcCtx := newTestServiceContext(t) // 种子帖子 1 作者为 1001
		l := NewDeletePostLogic(authContext(9999, "stranger"), svcCtx)
		_, err := l.DeletePost(&types.DeletePostReq{Id: 1})
		if !errors.Is(err, model.ErrPermissionDenied) {
			t.Fatalf("err = %v, want ErrPermissionDenied", err)
		}
	})

	t.Run("author deletes own post", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewDeletePostLogic(authContext(1001, "demo"), svcCtx)
		resp, err := l.DeletePost(&types.DeletePostReq{Id: 1})
		if err != nil {
			t.Fatalf("DeletePost: %v", err)
		}
		if resp.Code != 0 {
			t.Fatalf("resp code = %d, want 0", resp.Code)
		}
		if _, err := svcCtx.PostRepo.Get(1); !errors.Is(err, model.ErrPostNotFound) {
			t.Fatalf("Get after delete = %v, want ErrPostNotFound", err)
		}
	})
}

// --- GetPost ---

func TestGetPost(t *testing.T) {
	t.Run("validation branches", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewGetPostLogic(context.Background(), svcCtx)

		if _, err := l.GetPost(nil); err == nil {
			t.Fatal("nil request must be rejected")
		} else {
			requireHTTPStatus(t, err, 400)
		}
		if _, err := l.GetPost(&types.GetPostReq{Id: -1}); err == nil {
			t.Fatal("negative id must be rejected")
		} else {
			requireHTTPStatus(t, err, 400)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewGetPostLogic(context.Background(), svcCtx)
		if _, err := l.GetPost(&types.GetPostReq{Id: 9999}); !errors.Is(err, model.ErrPostNotFound) {
			t.Fatalf("err = %v, want ErrPostNotFound", err)
		}
	})

	t.Run("success increments views", func(t *testing.T) {
		svcCtx := newTestServiceContext(t) // 种子帖子 1 ViewCount=120
		l := NewGetPostLogic(context.Background(), svcCtx)

		resp, err := l.GetPost(&types.GetPostReq{Id: 1})
		if err != nil {
			t.Fatalf("GetPost: %v", err)
		}
		if resp.Post.Id != 1 {
			t.Fatalf("post id = %d, want 1", resp.Post.Id)
		}
		if resp.Post.ViewCount != 121 {
			t.Fatalf("views = %d, want 121", resp.Post.ViewCount)
		}
	})
}

// --- GetPosts ---

func TestGetPosts(t *testing.T) {
	newLogic := func(t *testing.T) (*GetPostsLogic, *svc.ServiceContext) {
		t.Helper()
		svcCtx := newTestServiceContext(t) // 2 条 published 种子帖子
		return NewGetPostsLogic(context.Background(), svcCtx), svcCtx
	}

	t.Run("default limit", func(t *testing.T) {
		l, _ := newLogic(t)
		resp, err := l.GetPosts(&types.GetPostsReq{})
		if err != nil {
			t.Fatalf("GetPosts: %v", err)
		}
		if resp.Total != 2 || len(resp.Posts) != 2 {
			t.Fatalf("total = %d len = %d, want 2/2", resp.Total, len(resp.Posts))
		}
	})

	t.Run("limit clamped to 100", func(t *testing.T) {
		l, _ := newLogic(t)
		resp, err := l.GetPosts(&types.GetPostsReq{Limit: 500})
		if err != nil {
			t.Fatalf("GetPosts: %v", err)
		}
		if resp.Total != 2 {
			t.Fatalf("total = %d, want 2", resp.Total)
		}
	})

	t.Run("negative offset treated as zero", func(t *testing.T) {
		l, _ := newLogic(t)
		resp, err := l.GetPosts(&types.GetPostsReq{Limit: 10, Offset: -5})
		if err != nil {
			t.Fatalf("GetPosts: %v", err)
		}
		if len(resp.Posts) != 2 {
			t.Fatalf("len = %d, want 2", len(resp.Posts))
		}
	})

	t.Run("filters pass through to repository", func(t *testing.T) {
		l, _ := newLogic(t)
		resp, err := l.GetPosts(&types.GetPostsReq{TopicId: 1, Limit: 10})
		if err != nil {
			t.Fatalf("GetPosts: %v", err)
		}
		if resp.Total != 1 || resp.Posts[0].Id != 1 {
			t.Fatalf("total = %d posts = %+v, want single post 1", resp.Total, resp.Posts)
		}

		resp, err = l.GetPosts(&types.GetPostsReq{AuthorId: 1002, Limit: 10})
		if err != nil {
			t.Fatalf("GetPosts: %v", err)
		}
		if resp.Total != 1 || resp.Posts[0].Id != 2 {
			t.Fatalf("total = %d posts = %+v, want single post 2", resp.Total, resp.Posts)
		}
	})
}

// --- GetHotPosts ---

func TestGetHotPosts(t *testing.T) {
	t.Run("default limit", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewGetHotPostsLogic(context.Background(), svcCtx)
		resp, err := l.GetHotPosts(&types.GetPostsReq{})
		if err != nil {
			t.Fatalf("GetHotPosts: %v", err)
		}
		if resp.Total != int64(len(resp.Posts)) {
			t.Fatalf("total = %d len = %d, must agree", resp.Total, len(resp.Posts))
		}
		if resp.Total != 2 {
			t.Fatalf("total = %d, want 2 seeded hot candidates", resp.Total)
		}
	})

	t.Run("limit clamped to 100", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewGetHotPostsLogic(context.Background(), svcCtx)
		resp, err := l.GetHotPosts(&types.GetPostsReq{Limit: 1000})
		if err != nil {
			t.Fatalf("GetHotPosts: %v", err)
		}
		if resp.Total != 2 {
			t.Fatalf("total = %d, want 2", resp.Total)
		}
	})
}

// --- LikePost / SharePost ---

func TestLikePost(t *testing.T) {
	t.Run("validation branches", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewLikePostLogic(context.Background(), svcCtx)

		if _, err := l.LikePost(nil); err == nil {
			t.Fatal("nil request must be rejected")
		} else {
			requireHTTPStatus(t, err, 400)
		}
		if _, err := l.LikePost(&types.LikePostReq{Id: 0}); err == nil {
			t.Fatal("zero id must be rejected")
		} else {
			requireHTTPStatus(t, err, 400)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewLikePostLogic(context.Background(), svcCtx)
		if _, err := l.LikePost(&types.LikePostReq{Id: 9999}); !errors.Is(err, model.ErrPostNotFound) {
			t.Fatalf("err = %v, want ErrPostNotFound", err)
		}
	})

	t.Run("success increments likes", func(t *testing.T) {
		svcCtx := newTestServiceContext(t) // 种子帖子 1 LikeCount=15
		l := NewLikePostLogic(context.Background(), svcCtx)
		resp, err := l.LikePost(&types.LikePostReq{Id: 1})
		if err != nil {
			t.Fatalf("LikePost: %v", err)
		}
		if resp.Code != 0 {
			t.Fatalf("resp code = %d, want 0", resp.Code)
		}
		p, _ := svcCtx.PostRepo.Get(1)
		if p.LikeCount != 16 {
			t.Fatalf("likes = %d, want 16", p.LikeCount)
		}
	})
}

func TestSharePost(t *testing.T) {
	t.Run("validation branches", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewSharePostLogic(context.Background(), svcCtx)

		if _, err := l.SharePost(nil); err == nil {
			t.Fatal("nil request must be rejected")
		} else {
			requireHTTPStatus(t, err, 400)
		}
		if _, err := l.SharePost(&types.SharePostReq{Id: 0}); err == nil {
			t.Fatal("zero id must be rejected")
		} else {
			requireHTTPStatus(t, err, 400)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewSharePostLogic(context.Background(), svcCtx)
		if _, err := l.SharePost(&types.SharePostReq{Id: 9999}); !errors.Is(err, model.ErrPostNotFound) {
			t.Fatalf("err = %v, want ErrPostNotFound", err)
		}
	})

	t.Run("success increments shares", func(t *testing.T) {
		svcCtx := newTestServiceContext(t) // 种子帖子 1 ShareCount=3
		l := NewSharePostLogic(context.Background(), svcCtx)
		resp, err := l.SharePost(&types.SharePostReq{Id: 1})
		if err != nil {
			t.Fatalf("SharePost: %v", err)
		}
		if resp.Code != 0 {
			t.Fatalf("resp code = %d, want 0", resp.Code)
		}
		p, _ := svcCtx.PostRepo.Get(1)
		if p.ShareCount != 4 {
			t.Fatalf("shares = %d, want 4", p.ShareCount)
		}
	})
}

// --- UpdatePost ---

func TestUpdatePost(t *testing.T) {
	t.Run("unauthorized without user in context", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewUpdatePostLogic(context.Background(), svcCtx)
		_, err := l.UpdatePost(&types.UpdatePostReq{Id: 1, Title: "x"})
		requireHTTPStatus(t, err, 401)
	})

	t.Run("validation branches", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewUpdatePostLogic(authContext(1001, "demo"), svcCtx)

		if _, err := l.UpdatePost(nil); err == nil {
			t.Fatal("nil request must be rejected")
		} else {
			requireHTTPStatus(t, err, 400)
		}
		if _, err := l.UpdatePost(&types.UpdatePostReq{Id: 0}); err == nil {
			t.Fatal("zero id must be rejected")
		} else {
			requireHTTPStatus(t, err, 400)
		}
	})

	t.Run("not found", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewUpdatePostLogic(authContext(1001, "demo"), svcCtx)
		_, err := l.UpdatePost(&types.UpdatePostReq{Id: 9999, Title: "x"})
		if !errors.Is(err, model.ErrPostNotFound) {
			t.Fatalf("err = %v, want ErrPostNotFound", err)
		}
	})

	t.Run("forbidden for non-author", func(t *testing.T) {
		svcCtx := newTestServiceContext(t) // 种子帖子 1 作者为 1001
		l := NewUpdatePostLogic(authContext(9999, "stranger"), svcCtx)
		_, err := l.UpdatePost(&types.UpdatePostReq{Id: 1, Title: "x"})
		if !errors.Is(err, model.ErrPermissionDenied) {
			t.Fatalf("err = %v, want ErrPermissionDenied", err)
		}
	})

	t.Run("author updates title and content", func(t *testing.T) {
		svcCtx := newTestServiceContext(t)
		l := NewUpdatePostLogic(authContext(1001, "demo"), svcCtx)
		resp, err := l.UpdatePost(&types.UpdatePostReq{Id: 1, Title: "新标题", Content: "新内容"})
		if err != nil {
			t.Fatalf("UpdatePost: %v", err)
		}
		if resp.Post.Title != "新标题" || resp.Post.Content != "新内容" {
			t.Fatalf("post = %+v, want updated title/content", resp.Post)
		}
	})
}
