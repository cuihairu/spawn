package model

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// newGuideModel 打开临时文件 SQLite（连接池由模型钳制为 1）并建表；
// seed=true 时写入内嵌种子。
func newGuideModel(t *testing.T, seed bool) *GuideModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "content.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	m := NewGuideModel(db)
	if err := m.CreateGuidesTable(); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if seed {
		if err := m.SeedIfEmpty(); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return m
}

func seedGuides(t *testing.T, m *GuideModel, guides ...*Guide) {
	t.Helper()
	if err := m.Seed(guides); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

// sampleGuideFixture 与原内存仓 sampleGuides 等价的三条样本。
func sampleGuideFixture() []*Guide {
	return []*Guide{
		{Id: 1, GameId: "game-a", GameTitle: "Game A", Title: "Guide One", Content: "c1",
			Summary: "s1", AuthorId: 100, AuthorName: "alice", Tags: []string{"t1", "t2"},
			IsPublished: true, Views: 10, Likes: 5,
			CreatedAt: "2020-01-01T00:00:00Z", UpdatedAt: "2020-01-01T00:00:00Z"},
		{Id: 2, GameId: "game-b", GameTitle: "Game B", Title: "Guide Two", Content: "c2",
			Summary: "s2", AuthorId: 200, AuthorName: "bob", Tags: []string{"t2"},
			IsPublished: false, Views: 20, Likes: 6,
			CreatedAt: "2020-01-02T00:00:00Z", UpdatedAt: "2020-01-02T00:00:00Z"},
		{Id: 3, GameId: "game-a", GameTitle: "Game A", Title: "Guide Three", Content: "c3",
			Summary: "", AuthorId: 100, AuthorName: "alice", Tags: []string{},
			IsPublished: true, Views: 30, Likes: 7,
			CreatedAt: "2020-01-03T00:00:00Z", UpdatedAt: "2020-01-03T00:00:00Z"},
	}
}

func loadSampleGuides(t *testing.T) *GuideModel {
	t.Helper()
	m := newGuideModel(t, false)
	seedGuides(t, m, sampleGuideFixture()...)
	return m
}

func guideIds(guides []*Guide) []int64 {
	ids := make([]int64, 0, len(guides))
	for _, g := range guides {
		ids = append(ids, g.Id)
	}
	return ids
}

func equalInt64(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// --- 种子与初始化 ---

func TestSeedIfEmpty_PopulatesAndSkipsNonEmpty(t *testing.T) {
	m := newGuideModel(t, true)

	g, err := m.Get(1)
	if err != nil || g.Title != "新手入门指南：如何在交界地存活下来" {
		t.Fatalf("Get(1) = %+v, %v; want seeded guide", g, err)
	}

	// 自增 id 接种子最大 id 之后
	created, err := m.Create(&Guide{Title: "new", Content: "x", GameId: "g"})
	if err != nil || created.Id != 3 {
		t.Fatalf("created id = %d, %v; want 3", created.Id, err)
	}

	// 非空表再跑 SeedIfEmpty 必须是 no-op
	if err := m.SeedIfEmpty(); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if _, total := m.List(GuideFilter{Page: 1, PageSize: 10}); total != 3 {
		t.Fatalf("total after re-seed = %d, want 3", total)
	}
}

func TestSeed_SkipsNilAndNonPositiveID(t *testing.T) {
	m := newGuideModel(t, false)
	seedGuides(t, m,
		nil,
		&Guide{Title: "no id"},
		&Guide{Id: 7, GameId: "game-a", Title: "Seven"},
	)

	g, err := m.Get(7)
	if err != nil || g.Title != "Seven" {
		t.Fatalf("Get(7) = %+v, %v", g, err)
	}
	if _, total := m.List(GuideFilter{Page: 1, PageSize: 10}); total != 1 {
		t.Fatalf("total = %d, want 1 (nil/zero-id skipped)", total)
	}
	created, err := m.Create(&Guide{Title: "new", Content: "x"})
	if err != nil || created.Id != 8 {
		t.Fatalf("created id = %d, %v; want 8", created.Id, err)
	}
}

// TestCreateGuidesTable_Idempotent 表已存在时建表为 no-op（重开连接后数据保留）。
func TestCreateGuidesTable_Idempotent(t *testing.T) {
	dir := t.TempDir()
	dsn := "file:" + filepath.Join(dir, "content.db")

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	m := NewGuideModel(db)
	if err := m.CreateGuidesTable(); err != nil {
		t.Fatalf("create table: %v", err)
	}
	seedGuides(t, m, sampleGuideFixture()...)
	db.Close()

	db2, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	m2 := NewGuideModel(db2)
	if err := m2.CreateGuidesTable(); err != nil {
		t.Fatalf("create table 2: %v", err)
	}
	if g, err := m2.Get(3); err != nil || g.GameId != "game-a" {
		t.Fatalf("Get(3) after reopen = %+v, %v; want persisted guide", g, err)
	}
}

// --- List：过滤与分页 ---

func TestGuideModelList_Filters(t *testing.T) {
	repo := loadSampleGuides(t)

	cases := []struct {
		name   string
		filter GuideFilter
		want   []int64
	}{
		{"no filter", GuideFilter{Page: 1, PageSize: 10}, []int64{1, 2, 3}},
		{"by gameId", GuideFilter{Page: 1, PageSize: 10, GameId: "game-a"}, []int64{1, 3}},
		{"by gameId miss", GuideFilter{Page: 1, PageSize: 10, GameId: "game-z"}, nil},
		{"published only", GuideFilter{Page: 1, PageSize: 10, PublishedOnly: true}, []int64{1, 3}},
		{"by author", GuideFilter{Page: 1, PageSize: 10, AuthorId: 100}, []int64{1, 3}},
		{"author zero = no filter", GuideFilter{Page: 1, PageSize: 10, AuthorId: 0}, []int64{1, 2, 3}},
		{"by tag", GuideFilter{Page: 1, PageSize: 10, Tag: "t2"}, []int64{1, 2}},
		{"by tag miss", GuideFilter{Page: 1, PageSize: 10, Tag: "nope"}, nil},
		{"keyword title", GuideFilter{Page: 1, PageSize: 10, Keyword: "Guide One"}, []int64{1}},
		{"keyword title case-insensitive", GuideFilter{Page: 1, PageSize: 10, Keyword: "guide"}, []int64{1, 2, 3}},
		{"keyword summary", GuideFilter{Page: 1, PageSize: 10, Keyword: "s2"}, []int64{2}},
		{"keyword content", GuideFilter{Page: 1, PageSize: 10, Keyword: "c3"}, []int64{3}},
		{"keyword miss", GuideFilter{Page: 1, PageSize: 10, Keyword: "nope"}, nil},
		{"keyword empty = no filter", GuideFilter{Page: 1, PageSize: 10, Keyword: ""}, []int64{1, 2, 3}},
		{"keyword + game", GuideFilter{Page: 1, PageSize: 10, GameId: "game-a", Keyword: "guide"}, []int64{1, 3}},
		{"keyword + published", GuideFilter{Page: 1, PageSize: 10, PublishedOnly: true, Keyword: "guide"}, []int64{1, 3}},
		{"game + published", GuideFilter{Page: 1, PageSize: 10, GameId: "game-a", PublishedOnly: true}, []int64{1, 3}},
		{"game + tag", GuideFilter{Page: 1, PageSize: 10, GameId: "game-a", Tag: "t1"}, []int64{1}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			guides, total := repo.List(tc.filter)
			if total != len(tc.want) {
				t.Fatalf("total = %d, want %d", total, len(tc.want))
			}
			if !equalInt64(guideIds(guides), tc.want) {
				t.Fatalf("ids = %v, want %v", guideIds(guides), tc.want)
			}
		})
	}
}

func TestGuideModelList_Pagination(t *testing.T) {
	repo := loadSampleGuides(t) // 3 条

	cases := []struct {
		name      string
		filter    GuideFilter
		wantIds   []int64
		wantTotal int
	}{
		{"first page", GuideFilter{Page: 1, PageSize: 2}, []int64{1, 2}, 3},
		{"second page", GuideFilter{Page: 2, PageSize: 2}, []int64{3}, 3},
		{"page beyond range", GuideFilter{Page: 9, PageSize: 2}, nil, 3},
		{"oversized page", GuideFilter{Page: 1, PageSize: 10}, []int64{1, 2, 3}, 3},
		// 边界：page/size 非法时必须安全降级为空页/首页，而非 panic
		{"page zero falls back to first", GuideFilter{Page: 0, PageSize: 2}, []int64{1, 2}, 3},
		{"negative page", GuideFilter{Page: -1, PageSize: 2}, []int64{1, 2}, 3},
		{"page size zero", GuideFilter{Page: 1, PageSize: 0}, nil, 3},
		{"negative page size", GuideFilter{Page: 1, PageSize: -5}, nil, 3},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			guides, total := repo.List(tc.filter)
			if total != tc.wantTotal {
				t.Fatalf("total = %d, want %d", total, tc.wantTotal)
			}
			if !equalInt64(guideIds(guides), tc.wantIds) {
				t.Fatalf("ids = %v, want %v", guideIds(guides), tc.wantIds)
			}
		})
	}
}

// --- Get / Create / Update / Publish / Like / IncrementViews ---

func TestGuideModelGet_NotFound(t *testing.T) {
	repo := loadSampleGuides(t)

	if _, err := repo.Get(999); !errors.Is(err, ErrGuideNotFound) {
		t.Fatalf("err = %v, want ErrGuideNotFound", err)
	}
	// 无负缓存：随后写入同 id 应立即可见
	seedGuides(t, repo, &Guide{Id: 999, Title: "Late Arrival"})
	if _, err := repo.Get(999); err != nil {
		t.Fatalf("Get(999) after late insert: %v", err)
	}
}

func TestGuideModel_CacheHitServesAfterDBClose(t *testing.T) {
	m := newGuideModel(t, true)

	if _, err := m.Get(1); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	if err := m.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	g, err := m.Get(1)
	if err != nil || g.Title != "新手入门指南：如何在交界地存活下来" {
		t.Fatalf("cached guide = %+v, err=%v", g, err)
	}
	// 未命中路径在关库后应报错而非哨兵（区别于 not-found）
	if _, err := m.Get(2); err == nil || errors.Is(err, ErrGuideNotFound) {
		t.Fatalf("err = %v, want non-sentinel db error", err)
	}
}

func TestGuideModel_CacheValueCopyIsolation(t *testing.T) {
	m := newGuideModel(t, true)

	first, err := m.Get(1)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	first.Tags[0] = "hacked"
	first.Title = "hacked"

	second, err := m.Get(1)
	if err != nil {
		t.Fatalf("get again: %v", err)
	}
	if second.Title == "hacked" || second.Tags[0] == "hacked" {
		t.Fatalf("cache value aliased: %+v", second)
	}
}

func TestGuideModelCreate(t *testing.T) {
	repo := loadSampleGuides(t)

	if _, err := repo.Create(nil); err == nil {
		t.Fatal("nil payload must be rejected")
	}

	created, err := repo.Create(&Guide{
		Title: "fresh", Content: "body", GameId: "game-a",
		Views: 99, Likes: 99, IsPublished: true, // 客户端传入的计数字段必须被重置
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Id != 4 {
		t.Fatalf("id = %d, want 4", created.Id)
	}
	if created.Views != 0 || created.Likes != 0 || created.IsPublished {
		t.Fatalf("counters must reset on create: %+v", created)
	}
	if _, err := time.Parse(time.RFC3339, created.CreatedAt); err != nil {
		t.Fatalf("CreatedAt %q not RFC3339: %v", created.CreatedAt, err)
	}
	if created.UpdatedAt != created.CreatedAt {
		t.Fatalf("UpdatedAt %q must equal CreatedAt on create", created.UpdatedAt)
	}

	again, _ := repo.Create(&Guide{Title: "second", Content: "b"})
	if again.Id != 5 {
		t.Fatalf("second id = %d, want 5", again.Id)
	}
}

func TestGuideModelUpdate(t *testing.T) {
	repo := loadSampleGuides(t) // UpdatedAt 均为 2020 年，断言刷新可行

	if _, err := repo.Update(999, map[string]interface{}{"title": "x"}); !errors.Is(err, ErrGuideNotFound) {
		t.Fatalf("err = %v, want ErrGuideNotFound", err)
	}

	updated, err := repo.Update(1, map[string]interface{}{
		"title":       "renamed",  // 非空 → 生效
		"content":     "new body", // 非空 → 生效
		"summary":     "",         // string 类型即生效（允许清空）
		"cover_image": "",         // 同上
		"tags":        []string{"x"},
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.Title != "renamed" || updated.Content != "new body" {
		t.Fatalf("title/content not applied: %+v", updated)
	}
	if updated.Summary != "" || updated.CoverImage != "" {
		t.Fatalf("summary/cover_image must be clearable: %+v", updated)
	}
	if len(updated.Tags) != 1 || updated.Tags[0] != "x" {
		t.Fatalf("tags not applied: %+v", updated.Tags)
	}
	if updated.UpdatedAt == "2020-01-01T00:00:00Z" {
		t.Fatal("UpdatedAt must be refreshed")
	}

	// 更新结果落库且缓存已失效（Get 取到新值）
	after, err := repo.Get(1)
	if err != nil || after.Title != "renamed" || after.Summary != "" {
		t.Fatalf("Get after update = %+v, %v", after, err)
	}

	// 空字符串 title/content 不生效（仓储守卫）
	kept, _ := repo.Update(1, map[string]interface{}{"title": "", "content": ""})
	if kept.Title != "renamed" || kept.Content != "new body" {
		t.Fatalf("empty title/content must be ignored: %+v", kept)
	}

	// 类型不符的值忽略
	typed, _ := repo.Update(1, map[string]interface{}{"title": 123, "tags": "not-a-slice"})
	if typed.Title != "renamed" || typed.Tags[0] != "x" {
		t.Fatalf("wrong-typed updates must be ignored: %+v", typed)
	}
}

func TestGuideModelPublish(t *testing.T) {
	repo := loadSampleGuides(t)

	if err := repo.Publish(999); !errors.Is(err, ErrGuideNotFound) {
		t.Fatalf("err = %v, want ErrGuideNotFound", err)
	}

	if err := repo.Publish(2); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	g, _ := repo.Get(2)
	if !g.IsPublished {
		t.Fatal("guide must be published")
	}
	if g.UpdatedAt == "2020-01-02T00:00:00Z" {
		t.Fatal("UpdatedAt must be refreshed on publish")
	}
}

func TestGuideModelLike(t *testing.T) {
	repo := loadSampleGuides(t)

	if _, err := repo.Like(999); !errors.Is(err, ErrGuideNotFound) {
		t.Fatalf("err = %v, want ErrGuideNotFound", err)
	}

	for want := 6; want <= 8; want++ {
		n, err := repo.Like(1)
		if err != nil || n != want {
			t.Fatalf("Like = %d, %v; want %d", n, err, want)
		}
	}
}

func TestGuideModelIncrementViews(t *testing.T) {
	repo := loadSampleGuides(t)

	if err := repo.IncrementViews(999); !errors.Is(err, ErrGuideNotFound) {
		t.Fatalf("err = %v, want ErrGuideNotFound", err)
	}

	g, _ := repo.Get(1)
	before := g.Views
	if err := repo.IncrementViews(1); err != nil {
		t.Fatalf("IncrementViews: %v", err)
	}
	g, _ = repo.Get(1)
	if g.Views != before+1 {
		t.Fatalf("views = %d, want %d", g.Views, before+1)
	}
}

// --- 关库错误传播 ---

func TestGuideModel_ClosedDBErrorPaths(t *testing.T) {
	m := newGuideModel(t, true)
	if err := m.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if guides, total := m.List(GuideFilter{Page: 1, PageSize: 5}); guides != nil || total != 0 {
		t.Fatalf("List on closed db = %d/%d, want nil/0", len(guides), total)
	}
	if _, err := m.Get(1); err == nil || errors.Is(err, ErrGuideNotFound) {
		t.Fatalf("Get on closed db = %v, want non-sentinel error", err)
	}
	if _, err := m.Create(&Guide{Title: "x", Content: "x"}); err == nil {
		t.Fatal("Create on closed db must error")
	}
	if _, err := m.Update(1, map[string]interface{}{"summary": "s"}); err == nil {
		t.Fatal("Update on closed db must error")
	}
	if err := m.Publish(1); err == nil {
		t.Fatal("Publish on closed db must error")
	}
	if _, err := m.Like(1); err == nil {
		t.Fatal("Like on closed db must error")
	}
	if err := m.IncrementViews(1); err == nil {
		t.Fatal("IncrementViews on closed db must error")
	}
	if err := m.SeedIfEmpty(); err == nil {
		t.Fatal("SeedIfEmpty on closed db must error")
	}
	if err := m.CreateGuidesTable(); err == nil {
		t.Fatal("CreateGuidesTable on closed db must error")
	}
}

// --- 并发 ---

// TestGuideModel_ConcurrentAccess 在 -race 下验证并发读写安全
// （连接池钳制为 1，串行化数据库访问）。
func TestGuideModel_ConcurrentAccess(t *testing.T) {
	repo, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "content.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer repo.Close()
	m := NewGuideModel(repo)
	if err := m.CreateGuidesTable(); err != nil {
		t.Fatalf("create table: %v", err)
	}
	seedGuides(t, m, sampleGuideFixture()...)

	const writers = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	created := make([]int64, 0, writers)

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			g, err := m.Create(&Guide{Title: fmt.Sprintf("g%d", i), Content: "x"})
			if err != nil {
				t.Errorf("create: %v", err)
				return
			}
			mu.Lock()
			created = append(created, g.Id)
			mu.Unlock()
		}(i)

		wg.Add(1)
		go func() {
			defer wg.Done()
			m.List(GuideFilter{Page: 1, PageSize: 10, PublishedOnly: true})
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.IncrementViews(1)
			_, _ = m.Like(1)
			_ = m.Publish(1)
			_, _ = m.Update(1, map[string]interface{}{"summary": "s"})
			_, _ = m.Get(1)
		}()
	}
	wg.Wait()

	if _, total := m.List(GuideFilter{Page: 1, PageSize: 10}); total != len(sampleGuideFixture())+writers {
		t.Fatalf("total = %d, want %d", total, len(sampleGuideFixture())+writers)
	}
	if len(created) != writers {
		t.Fatalf("created = %d, want %d", len(created), writers)
	}
}

// --- 脏数据列与写失败传播（直插 SQL 构造，与 game-catalog 同款手法） ---

// handInsertDirtyGuide 绕过模型直插一行脏数据（tags/views 列可控），
// 模拟历史库中的损坏行——正常写入路径经 encodeStringList 归一化不可达。
func handInsertDirtyGuide(t *testing.T, m *GuideModel, tags, views string) {
	t.Helper()
	_, err := m.db.Exec(
		`INSERT INTO guides (id, game_id, game_title, title, content, summary, cover_image,
			author_id, author_name, tags, views, likes, is_published, created_at, updated_at)
		 VALUES (9, 'game-dirty', 'Dirty', 'dirty row', 'c', 's', '', 1, 'a', ?, ?, 0, 1,
			'2020-01-01T00:00:00Z', '2020-01-01T00:00:00Z')`,
		tags, views)
	if err != nil {
		t.Fatalf("hand insert dirty guide: %v", err)
	}
}

func TestGuideModel_EmptyTagsColumnDecodesToEmptySlice(t *testing.T) {
	m := loadSampleGuides(t)
	handInsertDirtyGuide(t, m, "", "0") // tags 空串

	g, err := m.Get(9)
	if err != nil || g.Tags == nil || len(g.Tags) != 0 {
		t.Fatalf("Get(9) tags = %#v err = %v, want empty non-nil slice", g.Tags, err)
	}
}

func TestGuideModel_CorruptTagsJSONFailsGetAndList(t *testing.T) {
	m := loadSampleGuides(t)
	handInsertDirtyGuide(t, m, "notjson", "0")

	// Get：decode 错误自 getFromDB 透传
	if _, err := m.Get(9); err == nil || errors.Is(err, ErrGuideNotFound) {
		t.Fatalf("Get(9) err = %v, want tags decode error", err)
	}
	// all()：decode 错误经 List 透传（List 吞错降级空页）
	if guides, total := m.List(GuideFilter{Page: 1, PageSize: 10}); guides != nil || total != 0 {
		t.Fatalf("List on corrupt tags = %d/%d, want nil/0", len(guides), total)
	}
}

func TestGuideModel_TextInNumericColumnScanError(t *testing.T) {
	m := loadSampleGuides(t)
	handInsertDirtyGuide(t, m, "[]", "abc") // views 列混入文本

	if guides, total := m.List(GuideFilter{Page: 1, PageSize: 10}); guides != nil || total != 0 {
		t.Fatalf("List with text-in-numeric = %d/%d, want nil/0", len(guides), total)
	}
}

func TestGuideSeed_DuplicateIDError(t *testing.T) {
	m := newGuideModel(t, false)
	seedGuides(t, m, &Guide{Id: 1, GameId: "g", Title: "first"})

	// 同 id 再种：唯一约束冲突沿 insertRow → Seed 传播
	if err := m.Seed([]*Guide{{Id: 1, GameId: "g", Title: "dup"}}); err == nil {
		t.Fatal("duplicate id seed must error")
	}
}

func TestGuideModel_UpdateWriteFailurePropagates(t *testing.T) {
	m := loadSampleGuides(t)

	// 触发器令 UPDATE 中止：读路径不受影响，覆盖 Update 的写失败传播分支
	if _, err := m.db.Exec(`CREATE TRIGGER block_update BEFORE UPDATE ON guides
		BEGIN SELECT RAISE(ABORT, 'blocked'); END;`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if _, err := m.Update(1, map[string]interface{}{"title": "x"}); err == nil {
		t.Fatal("update under abort trigger must error")
	}
}
