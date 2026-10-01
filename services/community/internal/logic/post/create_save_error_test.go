package post

import (
	"errors"
	"testing"

	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/internal/types"
)

// failingPostStore 仅 Create 投毒的故障注入仓储（其余方法在用例链路中不可达，
// 嵌入接口即可满足 PostStore）。
type failingPostStore struct {
	model.PostStore
	createErr error
}

func (s failingPostStore) Create(int64, int64, string, *types.CreatePostReq) (*types.Post, error) {
	return nil, s.createErr
}

// TestCreatePost_SaveError 帖子写入失败（仓储层故障注入）→ CreatePost 原样
// 返回仓储错误。话题存在性检查在此之前已通过（TopicRepo 保持真实实现）。
func TestCreatePost_SaveError(t *testing.T) {
	svcCtx := newTestServiceContext(t)
	svcCtx.PostRepo = failingPostStore{createErr: errors.New("disk on fire")}

	_, err := NewCreatePostLogic(authContext(1001, "tester"), svcCtx).
		CreatePost(&types.CreatePostReq{TopicId: 1, Title: "t", Content: "c"})
	if err == nil {
		t.Fatal("expected CreatePost to fail when post persistence fails")
	}
}
