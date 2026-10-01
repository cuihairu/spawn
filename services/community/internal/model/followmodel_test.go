package model

import (
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newFollowModel 打开临时文件 SQLite 并建表（关注关系表不设种子，从空表起步）。
func newFollowModel(t *testing.T) *FollowModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "community.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	m := NewFollowModel(db)
	if err := m.CreateFollowsTable(); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return m
}

// --- Follow / Unfollow 布尔语义 ---

func TestFollowModel_TopicFollowBoolSemantics(t *testing.T) {
	m := newFollowModel(t)

	if !m.FollowTopic(7, 1) {
		t.Fatal("first FollowTopic must return true")
	}
	if m.FollowTopic(7, 1) {
		t.Fatal("duplicate FollowTopic must return false (idempotent)")
	}
	if !m.UnfollowTopic(7, 1) {
		t.Fatal("UnfollowTopic of existing relation must return true")
	}
	if m.UnfollowTopic(7, 1) {
		t.Fatal("second UnfollowTopic must return false (nothing removed)")
	}
	if m.UnfollowTopic(7, 999) {
		t.Fatal("UnfollowTopic of never-followed topic must return false")
	}
}

func TestFollowModel_UserFollowBoolSemantics(t *testing.T) {
	m := newFollowModel(t)

	if !m.FollowUser(7, 42) {
		t.Fatal("first FollowUser must return true")
	}
	if m.FollowUser(7, 42) {
		t.Fatal("duplicate FollowUser must return false")
	}
	if !m.UnfollowUser(7, 42) {
		t.Fatal("UnfollowUser of existing relation must return true")
	}
	if m.UnfollowUser(7, 42) || m.UnfollowUser(42, 7) {
		t.Fatal("UnfollowUser of missing relation must return false")
	}
}

func TestFollowModel_TargetTypesAreIndependent(t *testing.T) {
	m := newFollowModel(t)

	// 同一数字 id 在 topic 与 user 两类目标下互不干扰
	if !m.FollowTopic(7, 5) || !m.FollowUser(7, 5) {
		t.Fatal("both relations on id 5 must be allowed")
	}
	if got := m.ListFollowingTopicIds(7); len(got) != 1 || got[0] != 5 {
		t.Fatalf("ListFollowingTopicIds = %v, want [5] (user-follow must not leak in)", got)
	}
	if !m.UnfollowUser(7, 5) {
		t.Fatal("UnfollowUser must not touch topic relation")
	}
	if got := m.ListFollowingTopicIds(7); len(got) != 1 || got[0] != 5 {
		t.Fatalf("topic relation must survive user unfollow, got %v", got)
	}
}

// --- ListFollowingTopicIds ---

func TestFollowModel_ListFollowingTopicIdsSortedAndScoped(t *testing.T) {
	m := newFollowModel(t)

	// 乱序关注，验证输出升序
	for _, topicId := range []int64{30, 10, 20} {
		if !m.FollowTopic(7, topicId) {
			t.Fatalf("FollowTopic(7,%d) failed", topicId)
		}
	}
	if !m.FollowTopic(8, 10) {
		t.Fatal("FollowTopic(8,10) failed")
	}

	if got := m.ListFollowingTopicIds(7); len(got) != 3 || got[0] != 10 || got[1] != 20 || got[2] != 30 {
		t.Fatalf("ListFollowingTopicIds(7) = %v, want [10 20 30]", got)
	}
	// 其他用户隔离
	if got := m.ListFollowingTopicIds(8); len(got) != 1 || got[0] != 10 {
		t.Fatalf("ListFollowingTopicIds(8) = %v, want [10]", got)
	}
	// 未知用户返回空切片（非 nil 由调用方 range 安全性决定；两者皆可，这里允许 nil/空）
	if got := m.ListFollowingTopicIds(999); len(got) != 0 {
		t.Fatalf("ListFollowingTopicIds(999) = %v, want empty", got)
	}
}

func TestFollowModel_ListScanErrorReturnsNil(t *testing.T) {
	m := newFollowModel(t)

	// 数字列塞文本 → 行扫描失败 → 列表降级 nil
	if _, err := m.db.Exec(
		`INSERT INTO follows (user_id, target_type, target_id, created_at) VALUES (7, 'topic', 'abc', 'x')`); err != nil {
		t.Fatalf("hand insert: %v", err)
	}
	if got := m.ListFollowingTopicIds(7); got != nil {
		t.Fatalf("list with text target_id = %v, want nil", got)
	}
}

func TestFollowModel_ConcurrentFollowExactlyOneTrue(t *testing.T) {
	m := newFollowModel(t)

	const workers = 8
	results := make([]bool, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i] = m.FollowTopic(7, 1)
		}(i)
	}
	wg.Wait()

	trues := 0
	for _, ok := range results {
		if ok {
			trues++
		}
	}
	if trues != 1 {
		t.Fatalf("concurrent duplicate follow: %d trues, want exactly 1", trues)
	}
}

// --- 错误路径与持久化 ---

func TestFollowModel_ClosedDBReturnsFalseAndNil(t *testing.T) {
	m := newFollowModel(t)
	if err := m.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if m.FollowTopic(1, 1) {
		t.Fatal("FollowTopic on closed db must return false")
	}
	if m.UnfollowTopic(1, 1) {
		t.Fatal("UnfollowTopic on closed db must return false")
	}
	if m.FollowUser(1, 1) {
		t.Fatal("FollowUser on closed db must return false")
	}
	if m.UnfollowUser(1, 1) {
		t.Fatal("UnfollowUser on closed db must return false")
	}
	if got := m.ListFollowingTopicIds(1); got != nil {
		t.Fatalf("ListFollowingTopicIds on closed db = %v, want nil", got)
	}
	if err := m.CreateFollowsTable(); err == nil {
		t.Fatal("CreateFollowsTable on closed db must error")
	}
}

func TestFollowModel_InsertFallbackBothFailReturnsFalse(t *testing.T) {
	m := newFollowModel(t)
	if err := m.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	// 双方言依次失败（SQLite OR IGNORE 报错 → MySQL IGNORE 也报错）→ false
	if m.FollowTopic(1, 1) {
		t.Fatal("insert with both dialects failing must return false")
	}
}

func TestFollowModel_PersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "community.db")

	db1, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	m1 := NewFollowModel(db1)
	if err := m1.CreateFollowsTable(); err != nil {
		t.Fatalf("create: %v", err)
	}
	if !m1.FollowTopic(7, 1) || !m1.FollowTopic(7, 2) {
		t.Fatal("seed follows failed")
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close 1: %v", err)
	}

	db2, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open 2: %v", err)
	}
	defer db2.Close()
	m2 := NewFollowModel(db2)

	// 重开后关系仍在：重复关注依旧 false、列表完整
	if m2.FollowTopic(7, 1) {
		t.Fatal("duplicate follow after reopen must return false")
	}
	if got := m2.ListFollowingTopicIds(7); len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("reopened list = %v, want [1 2]", got)
	}
	if !m2.UnfollowTopic(7, 2) {
		t.Fatal("unfollow after reopen must return true")
	}
}
