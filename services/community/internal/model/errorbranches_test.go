package model

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tappi/tappi/services/community/internal/types"
)

// --- persist.go 故障注入 ---

// TestEnsureParentDir_NoParentNeeded 路径无目录分母（"." 或 "/"）时直接返回 nil。
func TestEnsureParentDir_NoParentNeeded(t *testing.T) {
	if err := ensureParentDir("relative.json"); err != nil {
		t.Fatalf(`ensureParentDir("relative.json") = %v, want nil (dir ".")`, err)
	}
	if err := ensureParentDir("/root.json"); err != nil {
		t.Fatalf(`ensureParentDir("/root.json") = %v, want nil (dir "/")`, err)
	}
}

// TestReadJSONFile_EmptyFile 空文件 → "empty json file" 错误。
func TestReadJSONFile_EmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "empty.json")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatalf("write empty file: %v", err)
	}
	var out []int
	err := readJSONFile(path, &out)
	if err == nil || err.Error() != "empty json file" {
		t.Fatalf("readJSONFile = %v, want empty json file", err)
	}
}

// TestWriteJSONAtomic_ParentCreationFails 父路径是一个普通文件 → MkdirAll 失败。
func TestWriteJSONAtomic_ParentCreationFails(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	if err := writeJSONAtomic(filepath.Join(blocker, "sub", "x.json"), map[string]int{"a": 1}); err == nil {
		t.Fatal("expected MkdirAll under a regular file to fail")
	}
}

// TestWriteJSONAtomic_MarshalFails 不可 JSON 序列化的值（chan）→ marshal 错误。
func TestWriteJSONAtomic_MarshalFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	if err := writeJSONAtomic(path, make(chan int)); err == nil {
		t.Fatal("expected json.MarshalIndent of chan to fail")
	}
}

// TestWriteJSONAtomic_TmpWriteFails 临时文件路径被同名目录占用 → WriteFile 失败。
func TestWriteJSONAtomic_TmpWriteFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "x.json")
	if err := os.MkdirAll(path+".tmp", 0o755); err != nil {
		t.Fatalf("mkdir tmp blocker: %v", err)
	}
	if err := writeJSONAtomic(path, map[string]int{"a": 1}); err == nil {
		t.Fatal("expected WriteFile onto a directory path to fail")
	}
}

// --- follow_repository.go ---

// TestFollowRepository_SeedLoadFalseAndSorts 从种子 JSON 装载关系（覆盖 load
// 填充循环），并逐一触达重复/缺失关系的 false 返回与保存、列举时的排序比较器。
func TestFollowRepository_SeedLoadFalseAndSorts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "follows.json")
	seed, err := json.Marshal(followState{
		UserTopics: map[int64][]int64{7: {1, 2, 3}},
		UserUsers:  map[int64][]int64{7: {3, 4}},
	})
	if err != nil {
		t.Fatalf("marshal seed: %v", err)
	}
	if err := os.WriteFile(path, seed, 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	repo, err := NewFollowRepository(path)
	if err != nil {
		t.Fatalf("NewFollowRepository: %v", err)
	}

	// 装载后的列举（含排序比较器）
	got := repo.ListFollowingTopicIds(7)
	if len(got) != 3 || got[0] != 1 || got[2] != 3 {
		t.Fatalf("ListFollowingTopicIds(7) = %v, want [1 2 3]", got)
	}
	if repo.ListFollowingTopicIds(99) != nil {
		t.Fatal("expected nil for unknown user")
	}

	// false 分支：重复关注、无关系集、关系不存在
	if repo.FollowTopic(7, 1) {
		t.Fatal("duplicate FollowTopic must return false")
	}
	if repo.UnfollowTopic(99, 1) {
		t.Fatal("UnfollowTopic without relation set must return false")
	}
	if repo.UnfollowTopic(7, 99) {
		t.Fatal("UnfollowTopic of missing topic must return false")
	}
	if repo.FollowUser(7, 3) {
		t.Fatal("duplicate FollowUser must return false")
	}
	if repo.UnfollowUser(99, 3) {
		t.Fatal("UnfollowUser without relation set must return false")
	}
	if repo.UnfollowUser(7, 99) {
		t.Fatal("UnfollowUser of missing target must return false")
	}

	// 触发持久化排序（≥2 元素的 user_topics / user_users 集合）
	if !repo.FollowTopic(7, 4) {
		t.Fatal("new FollowTopic must return true")
	}
	if !repo.FollowUser(7, 5) {
		t.Fatal("new FollowUser must return true")
	}
	var st followState
	if err := readJSONFile(path, &st); err != nil {
		t.Fatalf("read saved follows: %v", err)
	}
	if got := st.UserTopics[7]; len(got) != 4 {
		t.Fatalf("persisted user_topics[7] = %v, want 4 entries", got)
	}
	if got := st.UserUsers[7]; len(got) != 3 {
		t.Fatalf("persisted user_users[7] = %v, want 3 entries", got)
	}
}

// --- topic_repository.go ---

// TestTopicRepository_LoadEmptyArraySeedsDefaults 存在但为空数组的种子文件 →
// 回落到内置默认话题。
func TestTopicRepository_LoadEmptyArraySeedsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "topics.json")
	if err := os.WriteFile(path, []byte("[]"), 0o644); err != nil {
		t.Fatalf("write empty array: %v", err)
	}
	repo, err := NewTopicRepository(path)
	if err != nil {
		t.Fatalf("NewTopicRepository: %v", err)
	}
	if _, err := repo.Get(1); err != nil {
		t.Fatalf("expected default seed topic 1 after empty array, got %v", err)
	}
}

// TestTopicRepository_NilElementsSkipped 种子里的 null 元素在建索引与列表时
// 被跳过。
func TestTopicRepository_NilElementsSkipped(t *testing.T) {
	path := filepath.Join(t.TempDir(), "topics.json")
	seed, err := json.Marshal([]*types.Topic{nil, {Id: 9, Name: "t9", Description: "d"}})
	if err != nil {
		t.Fatalf("marshal seed: %v", err)
	}
	if err := os.WriteFile(path, seed, 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	repo, err := NewTopicRepository(path)
	if err != nil {
		t.Fatalf("NewTopicRepository: %v", err)
	}
	topics, total := repo.List("", false, 10, 0)
	if total != 1 || len(topics) != 1 || topics[0].Id != 9 {
		t.Fatalf("List = %+v total=%d, want only topic 9", topics, total)
	}
}

// TestTopicRepository_CreateSaveError 落盘临时路径被目录占用 → Create 的
// saveLocked 失败并原样返回错误。
func TestTopicRepository_CreateSaveError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "topics.json")
	repo, err := NewTopicRepository(path) // 缺文件 → 自动播种并首次落盘
	if err != nil {
		t.Fatalf("NewTopicRepository: %v", err)
	}
	if err := os.MkdirAll(path+".tmp", 0o755); err != nil {
		t.Fatalf("mkdir tmp blocker: %v", err)
	}
	if _, err := repo.Create(&types.CreateTopicReq{Name: "x"}); err == nil {
		t.Fatal("expected Create to fail when tmp path is a directory")
	}
}

// TestTopicRepository_ListPaginationClamps List 自身的 offset/limit/越界钳制。
func TestTopicRepository_ListPaginationClamps(t *testing.T) {
	path := filepath.Join(t.TempDir(), "topics.json")
	repo, err := NewTopicRepository(path)
	if err != nil {
		t.Fatalf("NewTopicRepository: %v", err)
	}

	if _, total := repo.List("", false, -1, -5); total < 1 {
		t.Fatalf("List with negative paging total = %d, want seeds", total)
	}
	if got, _ := repo.List("", false, 1, 99999); len(got) != 0 {
		t.Fatalf("List past end = %+v, want empty", got)
	}
}

// TestTopicRepository_IncrementMissingTopic 不存在话题的计数自增 →
// ErrTopicNotFound。
func TestTopicRepository_IncrementMissingTopic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "topics.json")
	repo, err := NewTopicRepository(path)
	if err != nil {
		t.Fatalf("NewTopicRepository: %v", err)
	}
	if err := repo.IncrementPostCount(99999, 1); !errors.Is(err, ErrTopicNotFound) {
		t.Fatalf("IncrementPostCount = %v, want ErrTopicNotFound", err)
	}
	if err := repo.IncrementFollowerCount(99999, 1); !errors.Is(err, ErrTopicNotFound) {
		t.Fatalf("IncrementFollowerCount = %v, want ErrTopicNotFound", err)
	}
}

// --- post_repository.go ---

// TestPostRepository_LoadEmptyArraySeedsDefaults 存在但为空数组的帖子种子
// 文件 → 回落到内置默认帖子。
func TestPostRepository_LoadEmptyArraySeedsDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "posts.json")
	if err := os.WriteFile(path, []byte("[]"), 0o644); err != nil {
		t.Fatalf("write empty array: %v", err)
	}
	repo, err := NewPostRepository(path)
	if err != nil {
		t.Fatalf("NewPostRepository: %v", err)
	}
	if _, err := repo.Get(1); err != nil {
		t.Fatalf("expected default seed post 1 after empty array, got %v", err)
	}
}

// seedPostRepo 写入 [null, 正常帖子(id5, author42)] 种子并装载。
func seedPostRepo(t *testing.T) *PostRepository {
	t.Helper()
	path := filepath.Join(t.TempDir(), "posts.json")
	seed, err := json.Marshal([]*types.Post{nil, {
		Id: 5, TopicId: 1, AuthorId: 42, AuthorName: "a42",
		Title: "t", Content: "c", Type: "discussion",
		Status: "published", CreatedAt: time.Now().UTC().Format(time.RFC3339),
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}})
	if err != nil {
		t.Fatalf("marshal seed: %v", err)
	}
	if err := os.WriteFile(path, seed, 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	repo, err := NewPostRepository(path)
	if err != nil {
		t.Fatalf("NewPostRepository: %v", err)
	}
	return repo
}

// TestPostRepository_NilElementAndListFilterBranches null 元素在建索引、列表
// 时被跳过；列表的状态/类型过滤与 offset/limit 钳制分支全覆盖。
func TestPostRepository_NilElementAndListFilterBranches(t *testing.T) {
	repo := seedPostRepo(t)

	// 状态不匹配（含 null 元素在 275 行被跳过）
	if _, total := repo.List(PostListFilter{Status: "draft", Limit: 50}); total != 0 {
		t.Fatalf("draft filter total = %d, want 0", total)
	}
	// 类型不匹配
	if _, total := repo.List(PostListFilter{Type: "meme", Limit: 50}); total != 0 {
		t.Fatalf("type filter total = %d, want 0", total)
	}
	// offset 负数 / limit 非正 / 越界起点
	if _, total := repo.List(PostListFilter{Offset: -1, Limit: 50}); total != 1 {
		t.Fatalf("negative offset total = %d, want 1", total)
	}
	if posts, total := repo.List(PostListFilter{Limit: -3}); total != 1 || len(posts) != 1 {
		t.Fatalf("non-positive limit = %+v total=%d, want 1 item", posts, total)
	}
	if got, _ := repo.List(PostListFilter{Offset: 99999, Limit: 10}); len(got) != 0 {
		t.Fatalf("offset past end = %+v, want empty", got)
	}
	// Hot：null 元素跳过 + limit<=0 回落 20
	hot := repo.Hot(0)
	if len(hot) != 1 || hot[0].Id != 5 {
		t.Fatalf("Hot(0) = %+v, want only post 5", hot)
	}
}

// TestPostRepository_UpdateImagesAndSaveFailures 成功的 Images 更新；随后
// 占住临时路径，依次触达 Update/Create/Like/Share 的 saveLocked 失败分支。
func TestPostRepository_UpdateImagesAndSaveFailures(t *testing.T) {
	repo := seedPostRepo(t)

	updated, err := repo.Update(5, 42, &types.UpdatePostReq{Images: []string{"a.png"}})
	if err != nil {
		t.Fatalf("Update images: %v", err)
	}
	if len(updated.Images) != 1 || updated.Images[0] != "a.png" {
		t.Fatalf("Images not applied: %+v", updated.Images)
	}

	if err := os.MkdirAll(repo.source+".tmp", 0o755); err != nil {
		t.Fatalf("mkdir tmp blocker: %v", err)
	}
	if _, err := repo.Update(5, 42, &types.UpdatePostReq{Title: "t2"}); err == nil {
		t.Fatal("expected Update to fail when tmp path is a directory")
	}
	if _, err := repo.Create(1, 42, "a42", &types.CreatePostReq{TopicId: 1, Title: "n", Content: "c"}); err == nil {
		t.Fatal("expected Create to fail when tmp path is a directory")
	}
	if _, err := repo.Like(5); err == nil {
		t.Fatal("expected Like to fail when tmp path is a directory")
	}
	if _, err := repo.Share(5); err == nil {
		t.Fatal("expected Share to fail when tmp path is a directory")
	}
}

// --- hotScore 分桶 ---

// TestHotScore_ParseFailureReturnsBase CreatedAt 非 RFC3339 → 解析失败直接
// 返回原始分数。
func TestHotScore_ParseFailureReturnsBase(t *testing.T) {
	p := types.Post{LikeCount: 10, CreatedAt: "not-a-time"}
	if got := hotScore(p); got != 30 { // 10*3 + 0 + 0 + 0
		t.Fatalf("hotScore = %v, want base 30", got)
	}
}

// TestHotScore_DecayBuckets 老帖子分别命中 decay=4（3~7 天）与默认
// decay=8（>7 天）分桶。
func TestHotScore_DecayBuckets(t *testing.T) {
	base := float64(96 * 3) // LikeCount=96

	mid := types.Post{LikeCount: 96, CreatedAt: time.Now().Add(-96 * time.Hour).Format(time.RFC3339)}
	gotMid := hotScore(mid)
	// decay = 4 * max(1, hours/24) ≈ 16
	if !(gotMid > base/20 && gotMid <= base/16) {
		t.Fatalf("4-day score = %v, want ≈ base/16 (decay 4 bucket)", gotMid)
	}

	old := types.Post{LikeCount: 96, CreatedAt: time.Now().Add(-720 * time.Hour).Format(time.RFC3339)}
	gotOld := hotScore(old)
	// decay = 8 * max(1, hours/24) ≈ 240
	if !(gotOld > 0 && gotOld < base/100) {
		t.Fatalf("30-day score = %v, want ≈ base/240 (default decay 8 bucket)", gotOld)
	}
}
