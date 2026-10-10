package model

import (
	"errors"
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

// TestPostModel_RemoveByModerator 管理端下架帖子：帖子被软删（status='deleted'，
// Get 不再返回），且缓存同步失效。
func TestPostModel_RemoveByModerator(t *testing.T) {
	m := newPostModel(t, true)
	created, err := m.Create(1, 1001, "alice", &types.CreatePostReq{
		Title: "待下架", Content: "c", Images: []string{"/uploads/x.png"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	if err := m.RemoveByModerator(created.Id); err != nil {
		t.Fatalf("RemoveByModerator: %v", err)
	}

	// 软删后 Get 走「不存在」语义
	if _, err := m.Get(created.Id); !errors.Is(err, ErrPostNotFound) {
		t.Fatalf("Get after removal err = %v, want ErrPostNotFound", err)
	}

	// 不存在的管理端下架同样返回 ErrPostNotFound
	if err := m.RemoveByModerator(created.Id); !errors.Is(err, ErrPostNotFound) {
		t.Fatalf("second removal err = %v, want ErrPostNotFound", err)
	}
}
