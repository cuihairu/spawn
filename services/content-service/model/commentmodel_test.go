package model

import (
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// newCommentModel 打开临时文件 SQLite（连接池由模型钳制为 1）并建表；
// seed=true 时写入内嵌种子。
func newCommentModel(t *testing.T, seed bool) *CommentModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "content.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	m := NewCommentModel(db)
	if err := m.CreateCommentsTable(); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if seed {
		if err := m.SeedIfEmpty(); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return m
}

// sampleCommentFixture 与原内存仓 sampleComments 等价的三条样本。
func sampleCommentFixture() []*Comment {
	return []*Comment{
		{Id: 1, TargetType: "guide", TargetId: 1, UserId: 11, UserName: "u1",
			Content: "c1", Likes: 3, CreatedAt: "2020-01-01T00:00:00Z", UpdatedAt: "2020-01-01T00:00:00Z"},
		{Id: 2, TargetType: "guide", TargetId: 1, UserId: 12, UserName: "u2",
			Content: "c2", Likes: 4, CreatedAt: "2020-01-02T00:00:00Z", UpdatedAt: "2020-01-02T00:00:00Z"},
		{Id: 3, TargetType: "guide", TargetId: 2, UserId: 13, UserName: "u3",
			Content: "c3", Likes: 5, CreatedAt: "2020-01-03T00:00:00Z", UpdatedAt: "2020-01-03T00:00:00Z"},
	}
}

func loadSampleComments(t *testing.T) *CommentModel {
	t.Helper()
	m := newCommentModel(t, false)
	if err := m.Seed(sampleCommentFixture()); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return m
}

func commentIds(comments []*Comment) []int64 {
	ids := make([]int64, 0, len(comments))
	for _, c := range comments {
		ids = append(ids, c.Id)
	}
	return ids
}

// --- 种子与初始化 ---

func TestCommentSeedIfEmpty_PopulatesAndSkipsNonEmpty(t *testing.T) {
	m := newCommentModel(t, true)

	_, total := m.List(CommentFilter{Page: 1, PageSize: 10})
	if total != 3 {
		t.Fatalf("seeded total = %d, want 3", total)
	}
	c, err := m.Get(1)
	if err != nil || c.Content != "这个攻略写得太好了！帮了我大忙！" {
		t.Fatalf("Get(1) = %+v, %v; want seeded comment", c, err)
	}
	created, err := m.Create(&Comment{Content: "new"})
	if err != nil || created.Id != 4 {
		t.Fatalf("created id = %d, %v; want 4", created.Id, err)
	}

	// 非空表再跑 SeedIfEmpty 必须是 no-op
	if err := m.SeedIfEmpty(); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if _, total := m.List(CommentFilter{Page: 1, PageSize: 10}); total != 4 {
		t.Fatalf("total after re-seed = %d, want 4", total)
	}
}

func TestCommentSeed_SkipsNilAndNonPositiveID(t *testing.T) {
	m := newCommentModel(t, false)
	if err := m.Seed([]*Comment{
		nil,
		{Content: "no id"},
		{Id: 9, TargetType: "guide", TargetId: 1, Content: "nine"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := m.Get(9); err != nil {
		t.Fatalf("Get(9): %v", err)
	}
	if _, total := m.List(CommentFilter{Page: 1, PageSize: 10}); total != 1 {
		t.Fatalf("total = %d, want 1 (nil/zero-id skipped)", total)
	}
}

func TestCommentModelGet_NotFound(t *testing.T) {
	repo := loadSampleComments(t)

	if _, err := repo.Get(404); !errors.Is(err, ErrCommentNotFound) {
		t.Fatalf("err = %v, want ErrCommentNotFound", err)
	}
	// 无负缓存：随后写入同 id 应立即可见
	if err := repo.Seed([]*Comment{{Id: 404, TargetType: "guide", Content: "late"}}); err != nil {
		t.Fatalf("late seed: %v", err)
	}
	if _, err := repo.Get(404); err != nil {
		t.Fatalf("Get(404) after late insert: %v", err)
	}
}

func TestCommentModel_CacheHitServesAfterDBClose(t *testing.T) {
	m := newCommentModel(t, true)

	if _, err := m.Get(1); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	if err := m.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	if c, err := m.Get(1); err != nil || c.Content != "这个攻略写得太好了！帮了我大忙！" {
		t.Fatalf("cached comment = %+v, err=%v", c, err)
	}
	if _, err := m.Get(2); err == nil || errors.Is(err, ErrCommentNotFound) {
		t.Fatalf("err = %v, want non-sentinel db error", err)
	}
}

// --- List：过滤与分页 ---

func TestCommentModelList_Filters(t *testing.T) {
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

func TestCommentModelList_Pagination(t *testing.T) {
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

func TestCommentModelCreate(t *testing.T) {
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

func TestCommentModelDelete(t *testing.T) {
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

func TestCommentModelLike(t *testing.T) {
	repo := loadSampleComments(t)

	if _, err := repo.Like(999); !errors.Is(err, ErrCommentNotFound) {
		t.Fatalf("err = %v, want ErrCommentNotFound", err)
	}

	n, err := repo.Like(1)
	if err != nil || n != 4 {
		t.Fatalf("Like = %d, %v; want 4", n, err)
	}
	// 点赞后缓存已失效，Get 取到最新计数
	c, _ := repo.Get(1)
	if c.Likes != 4 {
		t.Fatalf("Get after like = %d, want 4", c.Likes)
	}
	n, _ = repo.Like(1)
	if n != 5 {
		t.Fatalf("second Like = %d, want 5", n)
	}
}

// --- 关库错误传播 ---

func TestCommentModel_ClosedDBErrorPaths(t *testing.T) {
	m := newCommentModel(t, true)
	if err := m.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if comments, total := m.List(CommentFilter{Page: 1, PageSize: 5}); comments != nil || total != 0 {
		t.Fatalf("List on closed db = %d/%d, want nil/0", len(comments), total)
	}
	if _, err := m.Get(1); err == nil || errors.Is(err, ErrCommentNotFound) {
		t.Fatalf("Get on closed db = %v, want non-sentinel error", err)
	}
	if _, err := m.Create(&Comment{Content: "x"}); err == nil {
		t.Fatal("Create on closed db must error")
	}
	if err := m.Delete(1); err == nil {
		t.Fatal("Delete on closed db must error")
	}
	if _, err := m.Like(1); err == nil {
		t.Fatal("Like on closed db must error")
	}
	if err := m.SeedIfEmpty(); err == nil {
		t.Fatal("SeedIfEmpty on closed db must error")
	}
	if err := m.CreateCommentsTable(); err == nil {
		t.Fatal("CreateCommentsTable on closed db must error")
	}
}

// --- 并发 ---

func TestCommentModel_ConcurrentAccess(t *testing.T) {
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

// --- 脏数据列与种子错误（直插 SQL 构造，与 guide 模型同款手法） ---

func TestCommentSeed_DuplicateIDError(t *testing.T) {
	m := newCommentModel(t, false)
	seed := sampleCommentFixture()[:1]
	if err := m.Seed(seed); err != nil {
		t.Fatalf("first seed: %v", err)
	}

	// 同 id 再种：唯一约束冲突沿 insertRow → Seed 传播
	if err := m.Seed(seed); err == nil {
		t.Fatal("duplicate id seed must error")
	}
}

func TestCommentModel_TextInNumericColumnScanError(t *testing.T) {
	repo := loadSampleComments(t)

	// target_id 列混入文本：sqlite 动态类型允许落库，行扫描时爆错
	if _, err := repo.db.Exec(
		`INSERT INTO comments (id, target_type, target_id, user_id, user_name, content,
			parent_id, reply_to_id, likes, created_at, updated_at)
		 VALUES (9, 'guide', 'abc', 1, 'a', 'c', 0, 0, 0,
			'2020-01-01T00:00:00Z', '2020-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("hand insert dirty comment: %v", err)
	}

	if comments, total := repo.List(CommentFilter{Page: 1, PageSize: 10}); comments != nil || total != 0 {
		t.Fatalf("List with text-in-numeric = %d/%d, want nil/0", len(comments), total)
	}
}
