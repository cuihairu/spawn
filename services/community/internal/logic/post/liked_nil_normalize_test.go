package post

import (
	"testing"

	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/internal/types"
)

// nilLikesStore 模拟点赞关系表缺失时的降级读数
// （PostModel.ListLikedPosts 查询失败返回 nil, 0）。
type nilLikesStore struct {
	model.PostStore
}

func (s nilLikesStore) ListLikedPosts(int64, int64, int64) ([]types.Post, int64) {
	return nil, 0
}

// TestGetLikedPosts_NilStoreNormalizesToSlice 仓储返回 nil 切片时 logic 归一
// 为空数组——空态契约：posts 恒为数组，客户端不处理 null。真实 PostModel
// 成功路径自身已归一（返回空切片），nil 只出现在查询失败降级，故需注入。
func TestGetLikedPosts_NilStoreNormalizesToSlice(t *testing.T) {
	svcCtx := newTestServiceContext(t)
	svcCtx.PostRepo = nilLikesStore{}

	resp, err := NewGetLikedPostsLogic(authContext(7, "tester"), svcCtx).
		GetLikedPosts(&types.GetLikedPostsReq{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Posts == nil || len(resp.Posts) != 0 {
		t.Fatalf("posts = %#v, want non-nil empty slice", resp.Posts)
	}
}
