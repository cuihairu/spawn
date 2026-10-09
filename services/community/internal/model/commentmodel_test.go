package model

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"

	_ "github.com/mattn/go-sqlite3"
)

// newCommentModel 打开临时文件 SQLite，建帖子表（评论挂在帖子上）与评论表。
func newCommentModel(t *testing.T) (*PostModel, *CommentModel) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "community.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	pm := NewPostModel(db)
	if err := pm.CreatePostsTable(); err != nil {
		t.Fatalf("create posts table: %v", err)
	}
	cm := NewCommentModel(db)
	if err := cm.CreateCommentsTable(); err != nil {
		t.Fatalf("create comments table: %v", err)
	}
	return pm, cm
}

// seedPostForComments 建一条供评论挂靠的帖子，返回帖子 id。
func seedPostForComments(t *testing.T, pm *PostModel, authorId int64) int64 {
	t.Helper()
	p, err := pm.Create(1, authorId, "作者甲", &types.CreatePostReq{Title: "评论宿主帖", Content: "正文"})
	if err != nil {
		t.Fatalf("create host post: %v", err)
	}
	return p.Id
}

func TestCommentCreateAndList(t *testing.T) {
	pm, cm := newCommentModel(t)
	postId := seedPostForComments(t, pm, 7)

	c1, err := cm.Create(&types.Comment{
		PostId: postId, AuthorId: 7, AuthorName: "作者甲", Content: "第一条",
		CreatedAt: "2026-10-09T00:00:00Z", UpdatedAt: "2026-10-09T00:00:00Z",
	})
	if err != nil || c1 <= 0 {
		t.Fatalf("create c1: id=%d err=%v", c1, err)
	}
	c2, err := cm.Create(&types.Comment{
		PostId: postId, AuthorId: 8, AuthorName: "读者乙", Content: "第二条",
		ParentId: c1, ReplyToAuthorName: "作者甲",
		CreatedAt: "2026-10-09T00:01:00Z", UpdatedAt: "2026-10-09T00:01:00Z",
	})
	if err != nil {
		t.Fatalf("create c2: %v", err)
	}

	list, err := cm.ListByPost(postId, 10, 0)
	if err != nil || len(list) != 2 {
		t.Fatalf("list: n=%d err=%v", len(list), err)
	}
	// 升序 + 冗余字段与父指针落库一致
	if list[0].Id != c1 || list[0].Content != "第一条" || list[0].ReplyToAuthorName != "" {
		t.Fatalf("c1 mismatch: %+v", list[0])
	}
	if list[1].Id != c2 || list[1].ParentId != c1 || list[1].ReplyToAuthorName != "作者甲" {
		t.Fatalf("c2 mismatch: %+v", list[1])
	}

	total, err := cm.CountByPost(postId)
	if err != nil || total != 2 {
		t.Fatalf("count: %d err=%v", total, err)
	}

	// 分页：offset 越过第一条
	page2, err := cm.ListByPost(postId, 1, 1)
	if err != nil || len(page2) != 1 || page2[0].Id != c2 {
		t.Fatalf("page2: %+v err=%v", page2, err)
	}

	// 空帖子返回空列表而非 nil 语义差异
	empty, err := cm.ListByPost(9999, 10, 0)
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty list: n=%d err=%v", len(empty), err)
	}
}

func TestCommentGetAndDelete(t *testing.T) {
	pm, cm := newCommentModel(t)
	postId := seedPostForComments(t, pm, 7)

	id, err := cm.Create(&types.Comment{
		PostId: postId, AuthorId: 7, AuthorName: "作者甲", Content: "待删",
		CreatedAt: "2026-10-09T00:00:00Z", UpdatedAt: "2026-10-09T00:00:00Z",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	got, err := cm.GetByID(id)
	if err != nil || got.Id != id || got.AuthorId != 7 {
		t.Fatalf("get: %+v err=%v", got, err)
	}

	if err := cm.Delete(id); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := cm.GetByID(id); !errors.Is(err, ErrCommentNotFound) {
		t.Fatalf("get after delete err=%v, want ErrCommentNotFound", err)
	}
	// 重复删除同样 404 语义
	if err := cm.Delete(id); !errors.Is(err, ErrCommentNotFound) {
		t.Fatalf("re-delete err=%v, want ErrCommentNotFound", err)
	}
}

func TestCommentGetByIDNotFound(t *testing.T) {
	_, cm := newCommentModel(t)
	if _, err := cm.GetByID(424242); !errors.Is(err, ErrCommentNotFound) {
		t.Fatalf("err=%v, want ErrCommentNotFound", err)
	}
}

func TestIncrementComments(t *testing.T) {
	pm, _ := newCommentModel(t)
	postId := seedPostForComments(t, pm, 7)

	p, err := pm.IncrementComments(postId)
	if err != nil || p.CommentCount != 1 {
		t.Fatalf("first increment: count=%d err=%v", p.CommentCount, err)
	}
	// 单调累加：第二次自增到 2；缓存失效后读到新值
	p2, err := pm.IncrementComments(postId)
	if err != nil || p2.CommentCount != 2 {
		t.Fatalf("second increment: count=%d err=%v", p2.CommentCount, err)
	}
	// 不存在的帖子透传 not found
	if _, err := pm.IncrementComments(88888); !errors.Is(err, ErrPostNotFound) {
		t.Fatalf("missing post err=%v, want ErrPostNotFound", err)
	}
}
