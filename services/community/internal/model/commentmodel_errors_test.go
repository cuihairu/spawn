package model

import (
	"testing"

	"github.com/tappi/tappi/services/community/internal/types"
)

// newClosedCommentModel 建表后立即关闭底层库，专打各方法的错误返回路径。
func newClosedCommentModel(t *testing.T) *CommentModel {
	t.Helper()
	_, cm := newCommentModel(t)
	if err := cm.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	return cm
}

func TestCreateCommentsTable_ClosedDBErrors(t *testing.T) {
	cm := newClosedCommentModel(t)
	if err := cm.CreateCommentsTable(); err == nil {
		t.Fatal("CreateCommentsTable on closed db must error")
	}
}

func TestCommentCreate_ClosedDBErrors(t *testing.T) {
	cm := newClosedCommentModel(t)
	_, err := cm.Create(&types.Comment{PostId: 1, AuthorId: 7, Content: "c"})
	if err == nil {
		t.Fatal("Create on closed db must error")
	}
}

func TestCommentListByPost_ClosedDBErrors(t *testing.T) {
	cm := newClosedCommentModel(t)
	if _, err := cm.ListByPost(1, 10, 0); err == nil {
		t.Fatal("ListByPost on closed db must error")
	}
}

func TestCommentCountByPost_ClosedDBErrors(t *testing.T) {
	cm := newClosedCommentModel(t)
	if _, err := cm.CountByPost(1); err == nil {
		t.Fatal("CountByPost on closed db must error")
	}
}

func TestCommentGetByID_ClosedDBErrors(t *testing.T) {
	cm := newClosedCommentModel(t)
	if _, err := cm.GetByID(1); err == nil {
		t.Fatal("GetByID on closed db must error")
	}
}

func TestCommentDelete_ClosedDBErrors(t *testing.T) {
	cm := newClosedCommentModel(t)
	if err := cm.Delete(1); err == nil {
		t.Fatal("Delete on closed db must error")
	}
}

// TestCommentDelete_NotFound 删除不存在的评论 → ErrCommentNotFound。
func TestCommentDelete_NotFound(t *testing.T) {
	_, cm := newCommentModel(t)
	if err := cm.Delete(999); err == nil {
		t.Fatal("Delete of missing comment must error")
	}
}
