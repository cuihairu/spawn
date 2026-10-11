package model

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newClosedPostModel 建表后立即关闭底层库，专打错误返回路径。
func newClosedPostModel(t *testing.T) *PostModel {
	t.Helper()
	m := newPostModel(t, true)
	if err := m.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	return m
}

func TestCreatePostLikesTable_ClosedDBErrors(t *testing.T) {
	m := newClosedPostModel(t)
	if err := m.CreatePostLikesTable(); err == nil {
		t.Fatal("CreatePostLikesTable on closed db must error")
	}
}

func TestIncrementComments_ClosedDBErrors(t *testing.T) {
	m := newClosedPostModel(t)
	if _, err := m.IncrementComments(1); err == nil {
		t.Fatal("IncrementComments on closed db must error")
	}
}

func TestIncrementComments_NotFound(t *testing.T) {
	m := newPostModel(t, true)
	if _, err := m.IncrementComments(424242); err == nil {
		t.Fatal("IncrementComments on missing post must error")
	}
}

// newROPostModel 建文件库并种子后以只读 DSN 重开：读路径全通过、写路径必败，
// 专打 UPDATE 错误臂（关库连首查都过不去，够不到写路径）。
func newROPostModel(t *testing.T) *PostModel {
	t.Helper()
	path := filepath.Join(t.TempDir(), "community.db")

	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	m := NewPostModel(db)
	if err := m.CreatePostsTable(); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := m.SeedIfEmpty(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	ro, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	t.Cleanup(func() { _ = ro.Close() })
	return NewPostModel(ro)
}

// TestRemoveByModerator_ReadOnlyDBUpdateFails 只读库：getFromDB 读通过，
// 软删 UPDATE 触雷 → 「移除帖子失败」。
func TestRemoveByModerator_ReadOnlyDBUpdateFails(t *testing.T) {
	m := newROPostModel(t)
	err := m.RemoveByModerator(1)
	if err == nil || !strings.Contains(err.Error(), "移除帖子失败") {
		t.Fatalf("RemoveByModerator on ro db err = %v, want 移除帖子失败", err)
	}
}

// TestIncrementComments_ReadOnlyDBUpdateFails 只读库：计数自增 UPDATE 触雷。
func TestIncrementComments_ReadOnlyDBUpdateFails(t *testing.T) {
	m := newROPostModel(t)
	if _, err := m.IncrementComments(1); err == nil ||
		!strings.Contains(err.Error(), "帖子评论计数自增失败") {
		t.Fatalf("IncrementComments on ro db err = %v, want 帖子评论计数自增失败", err)
	}
}

// TestIncrementComments_ReReadFailsAfterUpdate 自增 UPDATE 本身成功，但行上
// 触发器把 view_count 污染成文本：第二次 getFromDB Scan 失败 → 错误原样透传。
func TestIncrementComments_ReReadFailsAfterUpdate(t *testing.T) {
	m := newPostModel(t, true)
	if _, err := m.db.Exec(`CREATE TRIGGER corrupt AFTER UPDATE ON posts BEGIN
		UPDATE posts SET view_count = 'corrupt' WHERE id = NEW.id; END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if _, err := m.IncrementComments(1); err == nil {
		t.Fatal("IncrementComments with corrupting trigger must error")
	}
}
