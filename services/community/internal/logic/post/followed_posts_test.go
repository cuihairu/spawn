package post

import (
	"context"
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

// TestGetFollowedPosts_EmptyNoFollowing 无关注话题/作者 → 空列表 + total 0。
func TestGetFollowedPosts_EmptyNoFollowing(t *testing.T) {
	svcCtx := newTestServiceContext(t)
	l := NewGetFollowedPostsLogic(authContext(500, "u500"), svcCtx)

	resp, err := l.GetFollowedPosts(&types.GetFollowedPostsReq{})
	if err != nil || resp == nil {
		t.Fatalf("empty following: resp=%v err=%v", resp, err)
	}
	if resp.Total != 0 || len(resp.Posts) != 0 {
		t.Fatalf("total=%d len=%d, want (0, empty)", resp.Total, len(resp.Posts))
	}
}

// TestGetFollowedPosts_FollowAuthor 只关注作者 7，且该用户发了一篇帖 id=3。
func TestGetFollowedPosts_FollowAuthor(t *testing.T) {
	svcCtx := newTestServiceContext(t)

	p, err := svcCtx.PostRepo.Create(1, 7, "authorA", &types.CreatePostReq{Title: "follow-post", Content: "c"})
	if err != nil {
		t.Fatalf("seed followable post: %v", err)
	}
	if !svcCtx.FollowRepo.FollowUser(500, 7) {
		t.Fatal("follow user 7 should succeed")
	}

	l := NewGetFollowedPostsLogic(authContext(500, "u500"), svcCtx)
	resp, err := l.GetFollowedPosts(&types.GetFollowedPostsReq{})
	if err != nil {
		t.Fatalf("GetFollowedPosts: %v", err)
	}
	if resp.Total == 0 || len(resp.Posts) == 0 {
		t.Fatalf("got empty result, want post %d for followed author", p.Id)
	}

	found := false
	for _, pp := range resp.Posts {
		if pp.Id == p.Id && pp.AuthorId == 7 {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("post %+v not found in results (got %v)", p.Id, postIds(resp.Posts))
	}
}

// TestGetFollowedPosts_PagingClamps limit<=0→20、offset<0→0。
func TestGetFollowedPosts_PagingClamps(t *testing.T) {
	svcCtx := newTestServiceContext(t)
	l := NewGetFollowedPostsLogic(authContext(500, "u500"), svcCtx)

	cases := []*types.GetFollowedPostsReq{
		{Limit: 0},               // 后端 clamp 到 20
		{Offset: -1},             // offset 归零
		{Limit: -5, Offset: -10}, // 双负
	}
	for _, req := range cases {
		resp, err := l.GetFollowedPosts(req)
		if err != nil || resp == nil {
			t.Fatalf("req=%+v: resp=%v err=%v, want ok", req, resp, err)
		}
	}
}

// TestGetFollowedPosts_Unauthorized 无鉴权上下文 → 401。
func TestGetFollowedPosts_Unauthorized(t *testing.T) {
	l := NewGetFollowedPostsLogic(context.Background(), newTestServiceContext(t))
	_, err := l.GetFollowedPosts(&types.GetFollowedPostsReq{})
	if err == nil {
		t.Fatal("expected unauthorized error")
	}
}
