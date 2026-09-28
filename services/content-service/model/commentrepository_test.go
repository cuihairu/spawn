package model

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const sampleComments = `[
	{"id":1,"target_type":"guide","target_id":1,"user_id":11,"user_name":"u1","content":"c1","likes":3,"created_at":"2020-01-01T00:00:00Z","updated_at":"2020-01-01T00:00:00Z"},
	{"id":2,"target_type":"guide","target_id":1,"user_id":12,"user_name":"u2","content":"c2","likes":4,"created_at":"2020-01-02T00:00:00Z","updated_at":"2020-01-02T00:00:00Z"},
	{"id":3,"target_type":"guide","target_id":2,"user_id":13,"user_name":"u3","content":"c3","likes":5,"created_at":"2020-01-03T00:00:00Z","updated_at":"2020-01-03T00:00:00Z"}
]`

func writeCommentsFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "comments.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write comment file: %v", err)
	}
	return path
}

func loadSampleComments(t *testing.T) *CommentRepository {
	t.Helper()
	repo, err := NewCommentRepository(writeCommentsFile(t, sampleComments))
	if err != nil {
		t.Fatalf("NewCommentRepository: %v", err)
	}
	return repo
}

func commentIds(comments []*Comment) []int64 {
	ids := make([]int64, 0, len(comments))
	for _, c := range comments {
		ids = append(ids, c.Id)
	}
	return ids
}

// --- 加载与初始化 ---

func TestNewCommentRepository_SeedsWhenFileMissing(t *testing.T) {
	repo, err := NewCommentRepository(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("NewCommentRepository: %v", err)
	}

	_, total := repo.List(CommentFilter{Page: 1, PageSize: 10})
	if total != 3 {
		t.Fatalf("seeded total = %d, want 3", total)
	}
	c, err := repo.Get(1)
	if err != nil || c.Content != "这个攻略写得太好了！帮了我大忙！" {
		t.Fatalf("Get(1) = %+v, %v; want seeded comment", c, err)
	}
	created, err := repo.Create(&Comment{Content: "new"})
	if err != nil || created.Id != 4 {
		t.Fatalf("created id = %d, %v; want 4", created.Id, err)
	}
}

func TestNewCommentRepository_LoadsFromFile(t *testing.T) {
	repo := loadSampleComments(t)

	c, err := repo.Get(3)
	if err != nil || c.TargetId != 2 {
		t.Fatalf("Get(3) = %+v, %v", c, err)
	}
	created, err := repo.Create(&Comment{Content: "new"})
	if err != nil || created.Id != 4 {
		t.Fatalf("created id = %d, %v; want 4", created.Id, err)
	}
}

func TestNewCommentRepository_EmptyArraySeeds(t *testing.T) {
	repo, err := NewCommentRepository(writeCommentsFile(t, `[]`))
	if err != nil {
		t.Fatalf("NewCommentRepository: %v", err)
	}
	if _, total := repo.List(CommentFilter{Page: 1, PageSize: 10}); total != 3 {
		t.Fatalf("empty file must fall back to seeds, total = %d, want 3", total)
	}
}

func TestNewCommentRepository_InvalidJSON(t *testing.T) {
	_, err := NewCommentRepository(writeCommentsFile(t, `not-json`))
	if err == nil || !strings.Contains(err.Error(), "unmarshal comment data") {
		t.Fatalf("err = %v, want unmarshal failure", err)
	}
}

func TestNewCommentRepository_ReadError(t *testing.T) {
	_, err := NewCommentRepository(t.TempDir()) // 目录 → 非 NotExist 错误
	if err == nil || !strings.Contains(err.Error(), "read comment data") {
		t.Fatalf("err = %v, want read failure", err)
	}
}

func TestNewCommentRepository_NilEntriesSkipped(t *testing.T) {
	repo, err := NewCommentRepository(writeCommentsFile(t,
		`[null,{"id":9,"target_type":"guide","target_id":1,"content":"nine"}]`))
	if err != nil {
		t.Fatalf("NewCommentRepository: %v", err)
	}

	if _, err := repo.Get(9); err != nil {
		t.Fatalf("Get(9): %v", err)
	}
	if _, total := repo.List(CommentFilter{Page: 1, PageSize: 10}); total != 1 {
		t.Fatalf("total = %d, want 1 (nil entry skipped)", total)
	}
}

// --- List：过滤与分页 ---

func TestCommentRepositoryList_Filters(t *testing.T) {
	repo := loadSampleComments(t)

	cases := []struct {
		name   string
		filter CommentFilter
		want   []int64
	}{
		{"no filter", CommentFilter{Page: 1, PageSize: 10}, []int64{1, 2, 3}},
		{"by target type", CommentFilter{Page: 1, PageSize: 10, TargetType: "guide"}, []int64{1, 2, 3}},
		{"by target type miss", CommentFilter{Page: 1, PageSize: 10, TargetType: "reply"}, nil},
		{"by target id", CommentFilter{Page: 1, PageSize: 10, TargetId: 2}, []int64{3}},
		{"target id zero = no filter", CommentFilter{Page: 1, PageSize: 10, TargetId: 0}, []int64{1, 2, 3}},
		{"type + id", CommentFilter{Page: 1, PageSize: 10, TargetType: "guide", TargetId: 1}, []int64{1, 2}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comments, total := repo.List(tc.filter)
			if total != len(tc.want) {
				t.Fatalf("total = %d, want %d", total, len(tc.want))
			}
			if !equalInt64(commentIds(comments), tc.want) {
				t.Fatalf("ids = %v, want %v", commentIds(comments), tc.want)
			}
		})
	}
}

func TestCommentRepositoryList_Pagination(t *testing.T) {
	repo := loadSampleComments(t) // 3 条

	cases := []struct {
		name      string
		filter    CommentFilter
		wantIds   []int64
		wantTotal int
	}{
		{"first page", CommentFilter{Page: 1, PageSize: 2}, []int64{1, 2}, 3},
		{"second page", CommentFilter{Page: 2, PageSize: 2}, []int64{3}, 3},
		{"page beyond range", CommentFilter{Page: 4, PageSize: 2}, nil, 3},
		{"oversized page", CommentFilter{Page: 1, PageSize: 10}, []int64{1, 2, 3}, 3},
		// 回归：HTTP 层 ?page=0（logic 未钳制）曾使仓储以负索引切片 panic
		{"page zero falls back to first", CommentFilter{Page: 0, PageSize: 3}, []int64{1, 2, 3}, 3},
		{"negative page", CommentFilter{Page: -2, PageSize: 2}, []int64{1, 2}, 3},
		{"page size zero", CommentFilter{Page: 2, PageSize: 0}, nil, 3},
		{"negative page size", CommentFilter{Page: 1, PageSize: -1}, nil, 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			comments, total := repo.List(tc.filter)
			if total != tc.wantTotal {
				t.Fatalf("total = %d, want %d", total, tc.wantTotal)
			}
			if !equalInt64(commentIds(comments), tc.wantIds) {
				t.Fatalf("ids = %v, want %v", commentIds(comments), tc.wantIds)
			}
		})
	}
}

// --- Get / Create / Delete / Like ---

func TestCommentRepositoryGet_NotFound(t *testing.T) {
	repo := loadSampleComments(t)

	if _, err := repo.Get(404); !errors.Is(err, ErrCommentNotFound) {
		t.Fatalf("err = %v, want ErrCommentNotFound", err)
	}
}

func TestCommentRepositoryCreate(t *testing.T) {
	repo := loadSampleComments(t)

	if _, err := repo.Create(nil); err == nil {
		t.Fatal("nil payload must be rejected")
	}

	created, err := repo.Create(&Comment{Content: "hello", Likes: 99})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Id != 4 {
		t.Fatalf("id = %d, want 4", created.Id)
	}
	if created.Likes != 0 {
		t.Fatalf("likes must reset on create, got %d", created.Likes)
	}
	if _, err := time.Parse(time.RFC3339, created.CreatedAt); err != nil {
		t.Fatalf("CreatedAt %q not RFC3339: %v", created.CreatedAt, err)
	}
	if created.UpdatedAt != created.CreatedAt {
		t.Fatalf("UpdatedAt must equal CreatedAt on create")
	}
}

func TestCommentRepositoryDelete(t *testing.T) {
	t.Run("first of slice", func(t *testing.T) {
		repo := loadSampleComments(t)
		if err := repo.Delete(1); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := repo.Get(1); !errors.Is(err, ErrCommentNotFound) {
			t.Fatalf("Get(1) err = %v, want ErrCommentNotFound", err)
		}
		comments, total := repo.List(CommentFilter{Page: 1, PageSize: 10})
		if total != 2 || !equalInt64(commentIds(comments), []int64{2, 3}) {
			t.Fatalf("remaining = %v (total %d), want [2 3]", commentIds(comments), total)
		}
	})

	t.Run("middle of slice", func(t *testing.T) {
		repo := loadSampleComments(t)
		if err := repo.Delete(2); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		comments, total := repo.List(CommentFilter{Page: 1, PageSize: 10})
		if total != 2 || !equalInt64(commentIds(comments), []int64{1, 3}) {
			t.Fatalf("remaining = %v (total %d), want [1 3]", commentIds(comments), total)
		}
	})

	t.Run("not found", func(t *testing.T) {
		repo := loadSampleComments(t)
		if err := repo.Delete(999); !errors.Is(err, ErrCommentNotFound) {
			t.Fatalf("err = %v, want ErrCommentNotFound", err)
		}
	})

	t.Run("deleted id is not reused", func(t *testing.T) {
		repo := loadSampleComments(t)
		if err := repo.Delete(3); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		created, err := repo.Create(&Comment{Content: "after delete"})
		if err != nil || created.Id != 4 {
			t.Fatalf("created id = %d, %v; want 4", created.Id, err)
		}
	})
}

func TestCommentRepositoryLike(t *testing.T) {
	repo := loadSampleComments(t)

	if _, err := repo.Like(999); !errors.Is(err, ErrCommentNotFound) {
		t.Fatalf("err = %v, want ErrCommentNotFound", err)
	}

	n, err := repo.Like(1)
	if err != nil || n != 4 {
		t.Fatalf("Like = %d, %v; want 4", n, err)
	}
	n, _ = repo.Like(1)
	if n != 5 {
		t.Fatalf("second Like = %d, want 5", n)
	}
}

// --- 并发 ---

func TestCommentRepository_ConcurrentAccess(t *testing.T) {
	repo := loadSampleComments(t)

	const workers = 8
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c, err := repo.Create(&Comment{Content: "concurrent"})
			if err != nil {
				t.Errorf("create: %v", err)
				return
			}
			if err := repo.Delete(c.Id); err != nil {
				t.Errorf("delete own comment %d: %v", c.Id, err)
			}
		}(i)

		wg.Add(1)
		go func() {
			defer wg.Done()
			repo.List(CommentFilter{Page: 1, PageSize: 10, TargetType: "guide"})
			_, _ = repo.Like(1)
			_, _ = repo.Get(1)
		}()
	}
	wg.Wait()

	if _, total := repo.List(CommentFilter{Page: 1, PageSize: 10}); total != 3 {
		t.Fatalf("total = %d, want 3 (created comments deleted again)", total)
	}
}
