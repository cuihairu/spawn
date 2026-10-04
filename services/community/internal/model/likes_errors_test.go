package model

import (
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

// TestPostModel_LikeRelationFailurePropagates 点赞关系表缺失 →
// insertLikeRelation 双方言连败（SQLite OR IGNORE 与 MySQL IGNORE 均报错）→
// Like 传播错误，计数不更新。
func TestPostModel_LikeRelationFailurePropagates(t *testing.T) {
	m := newPostModel(t, true)
	if _, err := m.db.Exec(`DROP TABLE post_likes`); err != nil {
		t.Fatalf("drop post_likes: %v", err)
	}
	if _, err := m.Like(1, 7); err == nil {
		t.Fatal("Like with missing relation table must error")
	}
}

// TestPostModel_ListLikedPosts_MissingTableDegrades 关系表缺失（旧库未建）
// → 查询失败按空集降级，不阻塞读取。
func TestPostModel_ListLikedPosts_MissingTableDegrades(t *testing.T) {
	m := newPostModel(t, true)
	if _, err := m.db.Exec(`DROP TABLE post_likes`); err != nil {
		t.Fatalf("drop post_likes: %v", err)
	}
	posts, total := m.ListLikedPosts(7, 10, 0)
	if posts != nil || total != 0 {
		t.Fatalf("posts = %v, total = %d, want nil/0", posts, total)
	}
}

// TestPostModel_ListLikedPosts_ScanErrorDegrades 关联的脏行（数值列塞文本）
// → 行扫描失败 → 按空集降级。
func TestPostModel_ListLikedPosts_ScanErrorDegrades(t *testing.T) {
	m := newPostModel(t, true)
	handInsertDirtyPost(t, m, 99, "[]", "[]", "not-a-number")
	if _, err := m.db.Exec(
		`INSERT INTO post_likes (user_id, post_id, created_at) VALUES (7, 99, '2026-10-05T00:00:00Z')`); err != nil {
		t.Fatalf("insert relation: %v", err)
	}
	posts, total := m.ListLikedPosts(7, 10, 0)
	if posts != nil || total != 0 {
		t.Fatalf("posts = %v, total = %d, want nil/0", posts, total)
	}
}

// TestPostModel_ListByFollow_ClosedDBDegrades 底层不可达 → allVisible 返 nil
// → ListByFollow 按空集降级。
func TestPostModel_ListByFollow_ClosedDBDegrades(t *testing.T) {
	m := newPostModel(t, true)
	if err := m.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	posts, total := m.ListByFollow(nil, []int64{1}, 10, 0)
	if posts != nil || total != 0 {
		t.Fatalf("posts = %v, total = %d, want nil/0", posts, total)
	}
}

// TestPostModel_ListByFollow_DraftFilteredAndTieBreak 关注流过滤与排序：
// 草稿不入流；时间戳不同按时间倒序，同秒（RFC3339 秒级精度下相邻 Create
// 的常态）按 id 倒序兜底。
func TestPostModel_ListByFollow_DraftFilteredAndTieBreak(t *testing.T) {
	m := newPostModel(t, true)

	a, err := m.Create(1, 42, "alice", &types.CreatePostReq{Title: "a", Content: "ca"})
	if err != nil {
		t.Fatalf("create a: %v", err)
	}
	b, err := m.Create(1, 42, "alice", &types.CreatePostReq{Title: "b", Content: "cb"})
	if err != nil {
		t.Fatalf("create b: %v", err)
	}
	older, err := m.Create(1, 42, "alice", &types.CreatePostReq{Title: "old", Content: "co"})
	if err != nil {
		t.Fatalf("create older: %v", err)
	}
	d, err := m.Create(1, 42, "alice", &types.CreatePostReq{Title: "draft", Content: "cd"})
	if err != nil {
		t.Fatalf("create draft: %v", err)
	}

	// a/b 保持同秒 → 排序走 id 倒序兜底分支；older 拨旧 → 走时间戳比较分支。
	rawPostUpdate(t, m, older.Id, "created_at", "2025-01-01T00:00:00Z")
	// 一条转草稿 → 过滤 continue 分支。
	rawPostUpdate(t, m, d.Id, "status", "draft")

	posts, total := m.ListByFollow(nil, []int64{42}, 10, 0)
	if total != 3 || len(posts) != 3 {
		t.Fatalf("posts = %d, total = %d, want 3/3 (draft excluded)", len(posts), total)
	}
	// 同秒对按 id 倒序在前，更旧的时间戳殿后
	if posts[0].Id != b.Id || posts[1].Id != a.Id || posts[2].Id != older.Id {
		t.Fatalf("order = [%d %d %d], want [%d %d %d]",
			posts[0].Id, posts[1].Id, posts[2].Id, b.Id, a.Id, older.Id)
	}
}
