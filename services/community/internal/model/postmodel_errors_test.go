package model

import (
	"testing"
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
