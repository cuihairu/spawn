package post

import (
	"os"
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

// TestCreatePost_SaveError 帖子落盘失败（临时路径被同名目录占用）→
// CreatePost 原样返回仓储错误。话题存在性检查在此之前已通过。
func TestCreatePost_SaveError(t *testing.T) {
	svcCtx := newTestServiceContext(t)
	if err := os.MkdirAll(svcCtx.Config.DataSource.PostsFile+".tmp", 0o755); err != nil {
		t.Fatalf("mkdir tmp blocker: %v", err)
	}

	_, err := NewCreatePostLogic(authContext(1001, "tester"), svcCtx).
		CreatePost(&types.CreatePostReq{TopicId: 1, Title: "t", Content: "c"})
	if err == nil {
		t.Fatal("expected CreatePost to fail when posts persistence fails")
	}
}
