package model

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// writeGuidesFile 将 JSON 内容写入临时文件并返回路径。
func writeGuidesFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "guides.json")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write guide file: %v", err)
	}
	return path
}

const sampleGuides = `[
	{"id":1,"game_id":"game-a","game_title":"Game A","title":"Guide One","content":"c1","summary":"s1","author_id":100,"author_name":"alice","tags":["t1","t2"],"is_published":true,"views":10,"likes":5,"created_at":"2020-01-01T00:00:00Z","updated_at":"2020-01-01T00:00:00Z"},
	{"id":2,"game_id":"game-b","game_title":"Game B","title":"Guide Two","content":"c2","summary":"s2","author_id":200,"author_name":"bob","tags":["t2"],"is_published":false,"views":20,"likes":6,"created_at":"2020-01-02T00:00:00Z","updated_at":"2020-01-02T00:00:00Z"},
	{"id":3,"game_id":"game-a","game_title":"Game A","title":"Guide Three","content":"c3","summary":"","author_id":100,"author_name":"alice","tags":[],"is_published":true,"views":30,"likes":7,"created_at":"2020-01-03T00:00:00Z","updated_at":"2020-01-03T00:00:00Z"}
]`

func loadSampleGuides(t *testing.T) *GuideRepository {
	t.Helper()
	repo, err := NewGuideRepository(writeGuidesFile(t, sampleGuides))
	if err != nil {
		t.Fatalf("NewGuideRepository: %v", err)
	}
	return repo
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

// --- 加载与初始化 ---

func TestNewGuideRepository_SeedsWhenFileMissing(t *testing.T) {
	repo, err := NewGuideRepository(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("NewGuideRepository: %v", err)
	}

	_, total := repo.List(GuideFilter{Page: 1, PageSize: 10})
	if total != 2 {
		t.Fatalf("seeded total = %d, want 2", total)
	}
	g, err := repo.Get(1)
	if err != nil || g.Title != "新手入门指南：如何在交界地存活下来" {
		t.Fatalf("Get(1) = %+v, %v; want seeded guide", g, err)
	}

	// nextId 应为种子最大 id + 1
	created, err := repo.Create(&Guide{Title: "new", Content: "x", GameId: "g"})
	if err != nil || created.Id != 3 {
		t.Fatalf("created id = %d, %v; want 3", created.Id, err)
	}
}

func TestNewGuideRepository_LoadsFromFile(t *testing.T) {
	repo := loadSampleGuides(t)

	g, err := repo.Get(3)
	if err != nil || g.GameId != "game-a" {
		t.Fatalf("Get(3) = %+v, %v; want loaded guide", g, err)
	}
	// nextId 取文件内最大 id + 1
	created, err := repo.Create(&Guide{Title: "new", Content: "x"})
	if err != nil || created.Id != 4 {
		t.Fatalf("created id = %d, %v; want 4", created.Id, err)
	}
}

func TestNewGuideRepository_EmptyArraySeeds(t *testing.T) {
	repo, err := NewGuideRepository(writeGuidesFile(t, `[]`))
	if err != nil {
		t.Fatalf("NewGuideRepository: %v", err)
	}
	if _, total := repo.List(GuideFilter{Page: 1, PageSize: 10}); total != 2 {
		t.Fatalf("empty file must fall back to seeds, total = %d, want 2", total)
	}
}

func TestNewGuideRepository_InvalidJSON(t *testing.T) {
	_, err := NewGuideRepository(writeGuidesFile(t, `{not json`))
	if err == nil || !strings.Contains(err.Error(), "unmarshal guide data") {
		t.Fatalf("err = %v, want unmarshal failure", err)
	}
}

func TestNewGuideRepository_ReadError(t *testing.T) {
	dir := t.TempDir() // 目录：os.ReadFile 返回非 NotExist 错误
	_, err := NewGuideRepository(dir)
	if err == nil || !strings.Contains(err.Error(), "read guide data") {
		t.Fatalf("err = %v, want read failure", err)
	}
}

func TestNewGuideRepository_NilEntriesSkipped(t *testing.T) {
	// JSON 数组中的 null 反序列化为 nil 指针：索引必须跳过且列表不得 panic
	repo, err := NewGuideRepository(writeGuidesFile(t,
		`[null,{"id":7,"game_id":"game-a","title":"Seven"}]`))
	if err != nil {
		t.Fatalf("NewGuideRepository: %v", err)
	}

	g, err := repo.Get(7)
	if err != nil || g.Title != "Seven" {
		t.Fatalf("Get(7) = %+v, %v", g, err)
	}
	if _, total := repo.List(GuideFilter{Page: 1, PageSize: 10}); total != 1 {
		t.Fatalf("total = %d, want 1 (nil entry skipped)", total)
	}
	created, err := repo.Create(&Guide{Title: "new", Content: "x"})
	if err != nil || created.Id != 8 {
		t.Fatalf("created id = %d, %v; want 8", created.Id, err)
	}
}

// --- List：过滤与分页 ---

func TestGuideRepositoryList_Filters(t *testing.T) {
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

func TestGuideRepositoryList_Pagination(t *testing.T) {
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

func TestGuideRepositoryGet_NotFound(t *testing.T) {
	repo := loadSampleGuides(t)

	if _, err := repo.Get(999); !errors.Is(err, ErrGuideNotFound) {
		t.Fatalf("err = %v, want ErrGuideNotFound", err)
	}
}

func TestGuideRepositoryCreate(t *testing.T) {
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

func TestGuideRepositoryUpdate(t *testing.T) {
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

func TestGuideRepositoryPublish(t *testing.T) {
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

func TestGuideRepositoryLike(t *testing.T) {
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

func TestGuideRepositoryIncrementViews(t *testing.T) {
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

// --- 并发 ---

// TestGuideRepository_ConcurrentAccess 在 -race 下验证读写锁覆盖所有路径。
func TestGuideRepository_ConcurrentAccess(t *testing.T) {
	repo, err := NewGuideRepository(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatalf("NewGuideRepository: %v", err)
	}

	const writers = 8
	var wg sync.WaitGroup
	var mu sync.Mutex
	created := make([]int64, 0, writers)

	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			g, err := repo.Create(&Guide{Title: fmt.Sprintf("g%d", i), Content: "x"})
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
			repo.List(GuideFilter{Page: 1, PageSize: 10, PublishedOnly: true})
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = repo.IncrementViews(1)
			_, _ = repo.Like(1)
			_ = repo.Publish(1)
			_, _ = repo.Update(1, map[string]interface{}{"summary": "s"})
			_, _ = repo.Get(1)
		}()
	}
	wg.Wait()

	if _, total := repo.List(GuideFilter{Page: 1, PageSize: 10}); total != 2+writers {
		t.Fatalf("total = %d, want %d", total, 2+writers)
	}
	if len(created) != writers {
		t.Fatalf("created = %d, want %d", len(created), writers)
	}
}
