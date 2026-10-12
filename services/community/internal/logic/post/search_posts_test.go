package post

import (
	"context"
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

// TestSearchPosts_ClampsAndEmptyQuery 分页钳制（limit<=0→20、>100→100、
// offset<0→0）与空查询空结果（仓储归一契约）。
func TestSearchPosts_ClampsAndEmptyQuery(t *testing.T) {
	svcCtx := newTestServiceContext(t)
	l := NewSearchPostsLogic(context.Background(), svcCtx)

	if _, err := svcCtx.PostRepo.Create(1, 1001, "demo", &types.CreatePostReq{
		Title: "搜索目标", Content: "全文检索关键词", Type: "discussion",
	}); err != nil {
		t.Fatalf("seed post: %v", err)
	}

	// 命中 + 钳制下限
	resp, err := l.SearchPosts(&types.SearchPostsReq{Q: "搜索目标", Limit: 0, Offset: -3})
	if err != nil {
		t.Fatalf("SearchPosts: %v", err)
	}
	if resp.Total != 1 || len(resp.Posts) != 1 {
		t.Fatalf("want 1 hit, got total=%d posts=%v", resp.Total, postIds(resp.Posts))
	}

	// 钳制上限：limit>100 → 100（结果不足时仅影响窗口不报错）
	resp, err = l.SearchPosts(&types.SearchPostsReq{Q: "搜索目标", Limit: 500, Offset: 0})
	if err != nil {
		t.Fatalf("SearchPosts clamp: %v", err)
	}
	if resp.Total != 1 {
		t.Fatalf("clamp run total = %d, want 1", resp.Total)
	}

	// 空白查询 → 空结果
	resp, err = l.SearchPosts(&types.SearchPostsReq{Q: "   "})
	if err != nil {
		t.Fatalf("SearchPosts blank: %v", err)
	}
	if resp.Total != 0 || resp.Posts == nil || len(resp.Posts) != 0 {
		t.Fatalf("blank query want empty non-nil slice, got total=%d posts=%#v", resp.Total, resp.Posts)
	}
}
