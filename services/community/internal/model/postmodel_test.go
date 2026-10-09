package model

import (
	"database/sql"
	"errors"
	"fmt"
	"math"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/tappi/tappi/services/community/internal/types"

	_ "github.com/mattn/go-sqlite3"
)

// newPostModel 打开临时文件 SQLite 并建表；seed=true 时写入内嵌种子。
func newPostModel(t *testing.T, seed bool) *PostModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "community.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	m := NewPostModel(db)
	if err := m.CreatePostsTable(); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if err := m.CreatePostLikesTable(); err != nil {
		t.Fatalf("create likes table: %v", err)
	}
	if seed {
		if err := m.SeedIfEmpty(); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return m
}

// rawPostUpdate 直接改库（绕过模型 API），用于装配 Hot 排序等场景。
func rawPostUpdate(t *testing.T, m *PostModel, id int64, column string, value interface{}) {
	t.Helper()
	if _, err := m.db.Exec(`UPDATE posts SET `+column+` = ? WHERE id = ?`, value, id); err != nil {
		t.Fatalf("raw update %s of post %d: %v", column, id, err)
	}
}

// handInsertDirtyPost 手工插入脏行（images/tags 传任意文本），
// 构造解码/扫描失败路径。
func handInsertDirtyPost(t *testing.T, m *PostModel, id int64, images, tags, viewCount string) {
	t.Helper()
	_, err := m.db.Exec(
		`INSERT INTO posts (id, topic_id, author_id, author_name, title, content, images, type,
			tags, view_count, like_count, comment_count, share_count, is_pinned, is_hot,
			status, created_at, updated_at)
		 VALUES (?, 1, 1, 'dirty', 't', 'c', ?, 'discussion', ?, ?, 0, 0, 0, 0, 0,
			'published', '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z')`,
		id, images, tags, viewCount,
	)
	if err != nil {
		t.Fatalf("hand insert dirty post: %v", err)
	}
}

// --- 种子与初始化 ---

func TestPostSeedIfEmpty_PopulatesAndSkipsNonEmpty(t *testing.T) {
	m := newPostModel(t, true)

	if _, total := m.List(PostListFilter{Limit: 50}); total != 2 {
		t.Fatalf("seeded total = %d, want 2", total)
	}
	got, err := m.Get(1)
	if err != nil || got.Title != "开荒建议：先别急着刷装" || got.LikeCount != 15 {
		t.Fatalf("Get(1) = %+v, %v; want seeded post", got, err)
	}

	created, err := m.Create(1, 42, "alice", &types.CreatePostReq{Title: "t", Content: "c"})
	if err != nil || created.Id != 3 {
		t.Fatalf("created id = %d, %v; want 3", created.Id, err)
	}

	// 非空表再跑 SeedIfEmpty 必须是 no-op
	if err := m.SeedIfEmpty(); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if _, total := m.List(PostListFilter{Limit: 50}); total != 3 {
		t.Fatalf("total after re-seed = %d, want 3", total)
	}
}

func TestPostSeed_SkipsNilAndNonPositiveID(t *testing.T) {
	m := newPostModel(t, false)
	if err := m.Seed([]*types.Post{
		nil,
		{Title: "no id"},
		{Id: 9, TopicId: 1, AuthorId: 7, AuthorName: "u", Title: "t9", Content: "c9",
			Type: "discussion", Status: "published",
			CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if _, err := m.Get(9); err != nil {
		t.Fatalf("Get(9): %v", err)
	}
	if _, total := m.List(PostListFilter{Limit: 10}); total != 1 {
		t.Fatalf("total = %d, want 1 (nil/zero-id skipped)", total)
	}
}

func TestPostSeed_DuplicateIDError(t *testing.T) {
	m := newPostModel(t, false)
	p := seededPost(5)
	if err := m.Seed([]*types.Post{p, seededPost(5)}); err == nil {
		t.Fatal("duplicate seed id must error")
	}
}

func seededPost(id int64) *types.Post {
	return &types.Post{
		Id: id, TopicId: 1, AuthorId: 7, AuthorName: "u",
		Title: fmt.Sprintf("t%d", id), Content: "c", Type: "discussion",
		Status: "published", Images: []string{}, Tags: []string{"x"},
		CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z",
	}
}

// --- Create / Get ---

func TestPostModelCreate_TrimsAndDefaults(t *testing.T) {
	m := newPostModel(t, true)

	created, err := m.Create(2, 42, "  alice  ", &types.CreatePostReq{
		Title: "  hello  ", Content: "  world  ", Type: "  ", Images: []string{"a.png"}, Tags: []string{"g"},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.AuthorName != "alice" || created.Title != "hello" || created.Content != "world" {
		t.Fatalf("trim not applied: %+v", created)
	}
	if created.Type != "discussion" {
		t.Fatalf("type = %q, want default discussion", created.Type)
	}
	if created.Status != "published" || created.ViewCount != 0 || created.LikeCount != 0 ||
		created.CommentCount != 0 || created.ShareCount != 0 {
		t.Fatalf("defaults not applied: %+v", created)
	}
	if _, err := time.Parse(time.RFC3339, created.CreatedAt); err != nil {
		t.Fatalf("CreatedAt %q not RFC3339: %v", created.CreatedAt, err)
	}

	// 落库后经 Get 读回（缓存已被 Create 失效 → 走 DB），数组字段往返一致
	got, err := m.Get(created.Id)
	if err != nil {
		t.Fatalf("Get created: %v", err)
	}
	if len(got.Images) != 1 || got.Images[0] != "a.png" || len(got.Tags) != 1 || got.Tags[0] != "g" {
		t.Fatalf("images/tags roundtrip broken: %+v", got)
	}
}

func TestPostModelGet_NotFoundAndDeleted(t *testing.T) {
	m := newPostModel(t, true)

	if _, err := m.Get(999); !errors.Is(err, ErrPostNotFound) {
		t.Fatalf("err = %v, want ErrPostNotFound", err)
	}

	// 软删后 Get 与不存在同语义
	if err := m.Delete(1, 1001); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := m.Get(1); !errors.Is(err, ErrPostNotFound) {
		t.Fatalf("deleted Get err = %v, want ErrPostNotFound", err)
	}
}

func TestPostModel_GetServesIsolatedCopies(t *testing.T) {
	m := newPostModel(t, true)

	first, err := m.Get(1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if len(first.Tags) == 0 {
		t.Fatalf("seeded post must carry tags for alias check: %+v", first)
	}
	first.Tags[0] = "mutated" // 改动不得回灌缓存

	second, err := m.Get(1) // 命中缓存
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if len(second.Tags) == 0 || second.Tags[0] == "mutated" {
		t.Fatalf("cache served aliased slice: %+v", second.Tags)
	}
}

func TestPostModel_CacheInvalidatedByWrites(t *testing.T) {
	m := newPostModel(t, true)

	if _, err := m.Get(1); err != nil { // 预热缓存
		t.Fatalf("prime: %v", err)
	}
	if err := m.IncrementViews(1); err != nil {
		t.Fatalf("IncrementViews: %v", err)
	}
	if got, _ := m.Get(1); got.ViewCount != 121 {
		t.Fatalf("view count after increment = %d, want 121 (cache invalidated)", got.ViewCount)
	}

	if _, err := m.Like(1, 1001); err != nil {
		t.Fatalf("Like: %v", err)
	}
	if got, _ := m.Get(1); got.LikeCount != 16 {
		t.Fatalf("like count = %d, want 16", got.LikeCount)
	}

	if _, err := m.Share(1); err != nil {
		t.Fatalf("Share: %v", err)
	}
	if got, _ := m.Get(1); got.ShareCount != 4 {
		t.Fatalf("share count = %d, want 4", got.ShareCount)
	}

	if _, err := m.Update(1, 1001, &types.UpdatePostReq{
		Title: "  新标题  ", Images: []string{"new.png"}, Tags: []string{"t1"},
	}); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ := m.Get(1)
	if got.Title != "新标题" {
		t.Fatalf("title after update = %q, want 新标题", got.Title)
	}
	if len(got.Images) != 1 || got.Images[0] != "new.png" || len(got.Tags) != 1 || got.Tags[0] != "t1" {
		t.Fatalf("images/tags update not applied: %+v", got)
	}
}

func TestPostModel_ConcurrentLikesAllLand(t *testing.T) {
	m := newPostModel(t, true)

	const workers = 8
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := m.Like(1, 1001); err != nil {
				t.Errorf("Like: %v", err)
			}
		}()
	}
	wg.Wait()

	got, err := m.Get(1)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.LikeCount != 15+workers {
		t.Fatalf("like count = %d, want %d", got.LikeCount, 15+workers)
	}
}

// --- Update / Delete 权限与可见性 ---

func TestPostModel_UpdateAndDelete_PermissionAndVisibility(t *testing.T) {
	m := newPostModel(t, true)

	if _, err := m.Update(1, 9999, &types.UpdatePostReq{Title: "hijack"}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("update by non-author = %v, want ErrPermissionDenied", err)
	}
	if err := m.Delete(1, 9999); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("delete by non-author = %v, want ErrPermissionDenied", err)
	}

	// 非空字段才生效：空串/nil 均保留原值
	updated, err := m.Update(1, 1001, &types.UpdatePostReq{Title: "  ", Content: "  ", Images: nil, Tags: nil})
	if err != nil {
		t.Fatalf("no-op Update: %v", err)
	}
	if updated.Title != "开荒建议：先别急着刷装" || updated.Content == "" || updated.Images == nil {
		t.Fatalf("empty update mutated post: %+v", updated)
	}

	// 软删后一切写路径不可达（与不存在同语义）
	if err := m.Delete(1, 1001); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := m.Update(1, 1001, &types.UpdatePostReq{Title: "x"}); !errors.Is(err, ErrPostNotFound) {
		t.Fatalf("update deleted = %v, want ErrPostNotFound", err)
	}
	if _, err := m.Like(1, 1001); !errors.Is(err, ErrPostNotFound) {
		t.Fatalf("like deleted = %v, want ErrPostNotFound", err)
	}
	if _, err := m.Share(1); !errors.Is(err, ErrPostNotFound) {
		t.Fatalf("share deleted = %v, want ErrPostNotFound", err)
	}
	if err := m.IncrementViews(1); !errors.Is(err, ErrPostNotFound) {
		t.Fatalf("view deleted = %v, want ErrPostNotFound", err)
	}
	if err := m.Delete(1, 1001); !errors.Is(err, ErrPostNotFound) {
		t.Fatalf("re-delete = %v, want ErrPostNotFound", err)
	}

	// 软删行从列表消失（List/Hot 均不可见）
	if _, total := m.List(PostListFilter{Limit: 50}); total != 1 {
		t.Fatalf("total after delete = %d, want 1", total)
	}
	if got := m.Hot(50); len(got) != 1 {
		t.Fatalf("Hot after delete = %d items, want 1", len(got))
	}
}

// --- List 过滤与分页 ---

func TestPostModel_ListDefaultsFiltersAndPagination(t *testing.T) {
	m := newPostModel(t, false)

	// 装配：post1 published/discussion、post2 published/share、post3 draft、
	// post4 topic2、post5 零互动（IsHot 过滤应排除）
	posts := []*types.Post{
		{Id: 1, TopicId: 1, AuthorId: 10, AuthorName: "a", Title: "t1", Content: "c",
			Type: "discussion", Status: "published", LikeCount: 5, ViewCount: 50,
			CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
		{Id: 2, TopicId: 1, AuthorId: 20, AuthorName: "b", Title: "t2", Content: "c",
			Type: "share", Status: "published", LikeCount: 5, ViewCount: 50,
			CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
		{Id: 3, TopicId: 1, AuthorId: 10, AuthorName: "a", Title: "t3", Content: "c",
			Type: "discussion", Status: "draft",
			CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
		{Id: 4, TopicId: 2, AuthorId: 10, AuthorName: "a", Title: "t4", Content: "c",
			Type: "discussion", Status: "published", LikeCount: 5, ViewCount: 50,
			CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
		{Id: 5, TopicId: 1, AuthorId: 30, AuthorName: "c", Title: "t5", Content: "c",
			Type: "discussion", Status: "published",
			CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
	}
	if err := m.Seed(posts); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// 默认 status=published：draft 被排除
	if _, total := m.List(PostListFilter{Limit: 50}); total != 4 {
		t.Fatalf("default total = %d, want 4 (draft excluded)", total)
	}
	// 显式 status
	if _, total := m.List(PostListFilter{Status: "draft", Limit: 50}); total != 1 {
		t.Fatalf("draft total = %d, want 1", total)
	}
	// topic / author / type 过滤
	if _, total := m.List(PostListFilter{TopicId: 2, Limit: 50}); total != 1 {
		t.Fatalf("topic filter total = %d, want 1", total)
	}
	if _, total := m.List(PostListFilter{AuthorId: 20, Limit: 50}); total != 1 {
		t.Fatalf("author filter total = %d, want 1", total)
	}
	if _, total := m.List(PostListFilter{Type: "share", Limit: 50}); total != 1 {
		t.Fatalf("type filter total = %d, want 1", total)
	}
	// IsHot：零互动新帖 hotScore=0 被排除，4 条有互动的保留
	hot, hotTotal := m.List(PostListFilter{IsHot: true, Limit: 50})
	if hotTotal != 3 {
		t.Fatalf("IsHot total = %d, want 3 (zero-score fresh post excluded)", hotTotal)
	}
	for _, p := range hot {
		if p.Id == 5 {
			t.Fatal("zero-score post must be excluded from IsHot filter")
		}
	}
	// 分页钳制
	if _, total := m.List(PostListFilter{Limit: -1, Offset: -5}); total != 4 {
		t.Fatalf("negative paging total = %d, want 4", total)
	}
	if got, _ := m.List(PostListFilter{Limit: 2, Offset: 99999}); len(got) != 0 {
		t.Fatalf("offset past end = %+v, want empty", got)
	}
	page1, total := m.List(PostListFilter{Limit: 2, Offset: 0})
	page2, _ := m.List(PostListFilter{Limit: 2, Offset: 2})
	if total != 4 || len(page1) != 2 || len(page2) != 2 {
		t.Fatalf("pagination broken: total=%d p1=%d p2=%d", total, len(page1), len(page2))
	}
	if page1[0].Id == page2[0].Id {
		t.Fatal("pages must not overlap")
	}
}

// --- Hot 排序 ---

func TestPostModel_HotOrderingPrefersPinnedThenScore(t *testing.T) {
	m := newPostModel(t, false)

	// A 置顶但互动最低；B/C 非置顶且 B 分数更高
	posts := []*types.Post{
		{Id: 1, TopicId: 1, AuthorId: 1, AuthorName: "a", Title: "pinned", Content: "c",
			Type: "discussion", Status: "published", IsPinned: true, LikeCount: 1,
			CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
		{Id: 2, TopicId: 1, AuthorId: 1, AuthorName: "a", Title: "hot", Content: "c",
			Type: "discussion", Status: "published", LikeCount: 100, ViewCount: 500,
			CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
		{Id: 3, TopicId: 1, AuthorId: 1, AuthorName: "a", Title: "warm", Content: "c",
			Type: "discussion", Status: "published", LikeCount: 10, ViewCount: 20,
			CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
		{Id: 4, TopicId: 1, AuthorId: 1, AuthorName: "a", Title: "draft", Content: "c",
			Type: "discussion", Status: "draft", LikeCount: 1000,
			CreatedAt: "2024-01-01T00:00:00Z", UpdatedAt: "2024-01-01T00:00:00Z"},
	}
	if err := m.Seed(posts); err != nil {
		t.Fatalf("seed: %v", err)
	}

	hot := m.Hot(0) // limit<=0 → 回落 20 → 全量（draft 帖被排除，高互动也不上榜）
	if len(hot) != 3 {
		t.Fatalf("Hot(0) = %d items, want 3 (draft skipped)", len(hot))
	}
	for _, p := range hot {
		if p.Status != "published" {
			t.Fatalf("non-published post %q leaked into Hot", p.Title)
		}
	}
	if hot[0].Title != "pinned" || hot[1].Title != "hot" || hot[2].Title != "warm" {
		t.Fatalf("Hot order = [%s %s %s], want pinned/hot/warm",
			hot[0].Title, hot[1].Title, hot[2].Title)
	}
	for _, p := range hot {
		if !p.IsHot {
			t.Fatalf("Hot result %q must set IsHot", p.Title)
		}
	}

	// IsHot 标记只作用于返回副本，不落库
	if got, err := m.Get(2); err != nil || got.IsHot {
		t.Fatalf("IsHot leaked into storage: %+v, %v", got, err)
	}

	if top := m.Hot(1); len(top) != 1 || top[0].Title != "pinned" {
		t.Fatalf("Hot(1) = %+v, want only pinned", top)
	}
}

func TestPostModel_HotDecayBuckets(t *testing.T) {
	base := types.Post{
		TopicId: 1, AuthorId: 1, AuthorName: "a", Title: "t", Content: "c",
		Type: "discussion", Status: "published", LikeCount: 10,
	}
	now := time.Now().UTC()

	// v2 公式：base / (ageHours+2)^1.2，平滑重力衰减无阶梯断崖
	fresh := base
	fresh.CreatedAt = now.Format(time.RFC3339)
	if got, want := hotScore(fresh), 30.0/math.Pow(2, 1.2); math.Abs(got-want) > want*0.01 {
		t.Fatalf("fresh score = %v, want ~%v", got, want)
	}

	dayOld := base
	dayOld.CreatedAt = now.Add(-48 * time.Hour).Format(time.RFC3339)
	if got, want := hotScore(dayOld), 30.0/math.Pow(50, 1.2); math.Abs(got-want) > want*0.01 {
		t.Fatalf("48h score = %v, want ~%v", got, want)
	}

	weekOld := base
	weekOld.CreatedAt = now.Add(-100 * time.Hour).Format(time.RFC3339)
	if got, want := hotScore(weekOld), 30.0/math.Pow(102, 1.2); math.Abs(got-want) > want*0.01 {
		t.Fatalf("100h score = %v, want ~%v", got, want)
	}

	ancient := base
	ancient.CreatedAt = now.Add(-400 * time.Hour).Format(time.RFC3339)
	if got, want := hotScore(ancient), 30.0/math.Pow(402, 1.2); math.Abs(got-want) > want*0.01 {
		t.Fatalf("400h score = %v, want ~%v", got, want)
	}

	// 时间衰减单调：越旧分越低
	if !(hotScore(fresh) > hotScore(dayOld) && hotScore(dayOld) > hotScore(weekOld) && hotScore(weekOld) > hotScore(ancient)) {
		t.Fatal("hotScore must strictly decrease with age")
	}

	// 非法时间戳：退化为无衰减 base
	broken := base
	broken.CreatedAt = "not-a-time"
	if got := hotScore(broken); got != 30 {
		t.Fatalf("broken timestamp score = %v, want 30", got)
	}
}

// TestPostModel_HotScoreViewDamping 浏览对数阻尼：100 倍浏览分数增幅远低于
// 100 倍（旧版线性计入时 100 倍浏览就是 100 倍分数，刷量即霸榜）。
func TestPostModel_HotScoreViewDamping(t *testing.T) {
	mk := func(views int64) types.Post {
		return types.Post{Type: "discussion", Status: "published",
			ViewCount: views, CreatedAt: "not-a-time"}
	}
	// 0 → 100 浏览：+log2(101)≈6.66 分；100 → 10000 浏览：+log2(10001)-log2(101)≈6.63 分
	// 每百倍浏览增量递减，第二段增幅略小于第一段，且远低于线性 100 倍。
	first, second := hotScore(mk(100)), hotScore(mk(10000))
	if second-first >= first {
		t.Fatalf("view gain not sublinear: 0→100 adds %v, 100→10000 adds %v", first, second-first)
	}
	if got, want := hotScore(mk(10000)), math.Log2(10001); math.Abs(got-want) > want*0.01 {
		t.Fatalf("views-only score = %v, want ~log2(10001)=%v", got, want)
	}
}

// TestPostModel_HotForUser_BoostFollowed 个性化提权：关注话题/作者的帖子
// 热度 ×1.5，同分之下排到非关注帖前；未命中关注集合的排序不变。
func TestPostModel_HotForUser_BoostFollowed(t *testing.T) {
	m := newPostModel(t, false)

	// A（话题 9）分数略高于 B（话题 1，作者 7）：匿名热榜 A 在前；
	// 关注话题 9 的用户视角下 B 需 ×1.5 —— 构造 B×1.5 后反超 A。
	// A: 10 赞=30 分；B: 8 赞=24 分，24×1.5=36 > 30 → B 反超。
	posts := []*types.Post{
		{Id: 1, TopicId: 9, AuthorId: 2, AuthorName: "x", Title: "stranger", Content: "c",
			Type: "discussion", Status: "published", LikeCount: 10,
			CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Id: 2, TopicId: 1, AuthorId: 7, AuthorName: "y", Title: "followed-author", Content: "c",
			Type: "discussion", Status: "published", LikeCount: 8,
			CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"},
		{Id: 3, TopicId: 2, AuthorId: 2, AuthorName: "x", Title: "followed-topic", Content: "c",
			Type: "discussion", Status: "published", LikeCount: 4,
			CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"},
	}
	if err := m.Seed(posts); err != nil {
		t.Fatalf("seed: %v", err)
	}

	anon := m.Hot(10)
	if anon[0].Title != "stranger" || anon[1].Title != "followed-author" {
		t.Fatalf("anon order = [%s %s], want stranger first", anon[0].Title, anon[1].Title)
	}

	got := m.HotForUser([]int64{2}, []int64{7}, 10)
	if got[0].Title != "followed-author" {
		t.Fatalf("personalized top = %s, want followed-author (author boost)", got[0].Title)
	}
	// followed-topic 帖 4 赞=12 分，×1.5=18 > stranger 无提权后的相对位置保持：
	// stranger 30 分仍高于 18，但它与 followed-topic 的差距应缩小——直接断言
	// 提权帖都排在同分非提权帖之前的语义已由 top1 覆盖，这里验证顺序完整。
	if got[1].Title != "stranger" || got[2].Title != "followed-topic" {
		t.Fatalf("personalized order = [%s %s %s]", got[0].Title, got[1].Title, got[2].Title)
	}

	// 空关注集合 → 与匿名热榜一致
	same := m.HotForUser(nil, nil, 10)
	for i := range same {
		if same[i].Id != anon[i].Id {
			t.Fatalf("empty follows must equal anon: got[%d]=%d want=%d", i, same[i].Id, anon[i].Id)
		}
	}
}

// --- 脏行与错误传播 ---

func TestPostModel_EmptyAndCorruptListColumns(t *testing.T) {
	m := newPostModel(t, false)

	// 空串列 → 解码为非 nil 空切片（"[]" 正常往返之外的另一分支）
	handInsertDirtyPost(t, m, 11, "", `["ok"]`, "10")
	got, err := m.Get(11)
	if err != nil {
		t.Fatalf("Get empty images: %v", err)
	}
	if got.Images == nil || len(got.Images) != 0 {
		t.Fatalf("empty column must decode to non-nil empty slice: %+v", got.Images)
	}
	if len(got.Tags) != 1 || got.Tags[0] != "ok" {
		t.Fatalf("tags = %+v, want [ok]", got.Tags)
	}

	// 坏 tags（好 images）→ 仅 tags 解码失败
	handInsertDirtyPost(t, m, 12, `[]`, "not-json", "10")
	if _, err := m.Get(12); err == nil || errors.Is(err, ErrPostNotFound) {
		t.Fatalf("Get corrupt tags = %v, want decode error", err)
	}
}

func TestPostModel_LikeShareReReadFailurePropagates(t *testing.T) {
	m := newPostModel(t, true)

	// AFTER 触发器在自增 UPDATE 后删行：点赞/分享的二次读取扑空
	if _, err := m.db.Exec(`CREATE TRIGGER delete_row_after_update AFTER UPDATE ON posts
		BEGIN DELETE FROM posts WHERE id = NEW.id; END;`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if _, err := m.Like(1, 1001); !errors.Is(err, ErrPostNotFound) {
		t.Fatalf("Like re-read err = %v, want ErrPostNotFound", err)
	}
	// 触发器已删帖 1；用帖 2 触达 Share 的二次读取失败分支
	if _, err := m.Share(2); !errors.Is(err, ErrPostNotFound) {
		t.Fatalf("Share re-read err = %v, want ErrPostNotFound", err)
	}
}

func TestPostModel_CorruptColumnsFailScan(t *testing.T) {
	m := newPostModel(t, false)
	handInsertDirtyPost(t, m, 7, "not-json", `["ok"]`, "10")

	if _, err := m.Get(7); err == nil || errors.Is(err, ErrPostNotFound) {
		t.Fatalf("Get corrupt images = %v, want decode error", err)
	}
	if got, total := m.List(PostListFilter{Limit: 10}); got != nil || total != 0 {
		t.Fatalf("List with corrupt row = %v/%d, want nil/0 (degraded)", got, total)
	}
	if got := m.Hot(10); got != nil {
		t.Fatalf("Hot with corrupt row = %v, want nil", got)
	}
}

func TestPostModel_TextInNumericColumnScanError(t *testing.T) {
	m := newPostModel(t, false)
	handInsertDirtyPost(t, m, 8, `[]`, `[]`, "abc")

	if _, err := m.Get(8); err == nil || errors.Is(err, ErrPostNotFound) {
		t.Fatalf("Get with text view_count = %v, want scan error", err)
	}
}

func TestPostModel_ClosedDBErrorPaths(t *testing.T) {
	m := newPostModel(t, true)
	if err := m.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if _, err := m.Get(1); err == nil || errors.Is(err, ErrPostNotFound) {
		t.Fatalf("Get on closed db = %v, want non-sentinel error", err)
	}
	if _, err := m.Create(1, 1, "u", &types.CreatePostReq{Title: "t", Content: "c"}); err == nil {
		t.Fatal("Create on closed db must error")
	}
	if err := m.IncrementViews(1); err == nil {
		t.Fatal("IncrementViews on closed db must error")
	}
	if _, err := m.Update(1, 1001, &types.UpdatePostReq{Title: "t"}); err == nil || errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("Update on closed db = %v, want db error", err)
	}
	if err := m.Delete(1, 1001); err == nil {
		t.Fatal("Delete on closed db must error")
	}
	if _, err := m.Like(1, 1001); err == nil {
		t.Fatal("Like on closed db must error")
	}
	if _, err := m.Share(1); err == nil {
		t.Fatal("Share on closed db must error")
	}
	if got, total := m.List(PostListFilter{Limit: 10}); got != nil || total != 0 {
		t.Fatalf("List on closed db = %v/%d, want nil/0", got, total)
	}
	if got := m.Hot(10); got != nil {
		t.Fatalf("Hot on closed db = %v, want nil", got)
	}
	if err := m.SeedIfEmpty(); err == nil {
		t.Fatal("SeedIfEmpty on closed db must error")
	}
	if err := m.CreatePostsTable(); err == nil {
		t.Fatal("CreatePostsTable on closed db must error")
	}
	if err := m.Seed([]*types.Post{seededPost(50)}); err == nil {
		t.Fatal("Seed on closed db must error")
	}
}

func TestPostModel_UpdateWriteFailurePropagates(t *testing.T) {
	m := newPostModel(t, true)

	// 触发器令 UPDATE 中止：读路径不受影响，覆盖各写失败传播分支
	if _, err := m.db.Exec(`CREATE TRIGGER block_post_update BEFORE UPDATE ON posts
		BEGIN SELECT RAISE(ABORT, 'blocked'); END;`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}

	if err := m.IncrementViews(1); err == nil {
		t.Fatal("IncrementViews under abort trigger must error")
	}
	if _, err := m.Update(1, 1001, &types.UpdatePostReq{Title: "x"}); err == nil {
		t.Fatal("Update under abort trigger must error")
	}
	if err := m.Delete(1, 1001); err == nil {
		t.Fatal("Delete under abort trigger must error")
	}
	if _, err := m.Like(1, 1001); err == nil {
		t.Fatal("Like under abort trigger must error")
	}
	if _, err := m.Share(1); err == nil {
		t.Fatal("Share under abort trigger must error")
	}
}

// --- 持久化重开 ---

func TestPostModel_PersistsAcrossReopen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "community.db")

	db1, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open 1: %v", err)
	}
	m1 := NewPostModel(db1)
	if err := m1.CreatePostsTable(); err != nil {
		t.Fatalf("create: %v", err)
	}
	created, err := m1.Create(1, 42, "alice", &types.CreatePostReq{Title: "persist", Content: "c", Tags: []string{"keep"}})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if err := db1.Close(); err != nil {
		t.Fatalf("close 1: %v", err)
	}

	db2, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open 2: %v", err)
	}
	defer db2.Close()
	m2 := NewPostModel(db2)

	got, err := m2.Get(created.Id)
	if err != nil || got.Title != "persist" || len(got.Tags) != 1 || got.Tags[0] != "keep" {
		t.Fatalf("reopened Get = %+v, %v", got, err)
	}
	if _, total := m2.List(PostListFilter{Limit: 50}); total != 1 {
		t.Fatalf("reopened total = %d, want 1", total)
	}
}

// TestPostModel_ListLikedPosts 我的点赞：关系落库/幂等/用户隔离/软删排除/分页窗。
func TestPostModel_ListLikedPosts(t *testing.T) {
	m := newPostModel(t, true)

	// 空集：无点赞 → 空列表 + total 0
	if posts, total := m.ListLikedPosts(5, 20, 0); total != 0 || len(posts) != 0 {
		t.Fatalf("empty likes: want empty/0, got %v total=%d", posts, total)
	}

	// 点赞顺序：先赞 1 再赞 2 → [2,1]（created_at desc，同秒以 post_id desc 兜底）
	if _, err := m.Like(1, 5); err != nil {
		t.Fatalf("Like(1): %v", err)
	}
	if _, err := m.Like(2, 5); err != nil {
		t.Fatalf("Like(2): %v", err)
	}
	posts, total := m.ListLikedPosts(5, 20, 0)
	if total != 2 || ids(posts)[0] != 2 || ids(posts)[1] != 1 {
		t.Fatalf("want [2 1] total=2, got %v total=%d", ids(posts), total)
	}

	// 幂等：重复点赞关系不重复入列，计数单调累加不受影响
	if _, err := m.Like(1, 5); err != nil {
		t.Fatalf("Like(1) again: %v", err)
	}
	if posts, total = m.ListLikedPosts(5, 20, 0); total != 2 {
		t.Fatalf("idempotent like must keep 2 relation rows, got %d", total)
	}
	if got, _ := m.Get(1); got.LikeCount != 17 { // 种子 15 + 2 次
		t.Fatalf("like count = %d, want 17 (counter keeps incrementing)", got.LikeCount)
	}

	// 用户隔离：他人点赞不影响我的列表
	if _, err := m.Like(1, 6); err != nil {
		t.Fatalf("Like(1) by other: %v", err)
	}
	if posts, total = m.ListLikedPosts(6, 20, 0); total != 1 || ids(posts)[0] != 1 {
		t.Fatalf("user 6 want [1] total=1, got %v total=%d", ids(posts), total)
	}
	if posts, total = m.ListLikedPosts(5, 20, 0); total != 2 {
		t.Fatalf("user 5 list must be unchanged, got %d", total)
	}

	// 分页窗：limit=1 offset=1 → 第一页余下的一条；浮动 limit<=0/offset<0 按默认钳制
	if posts, total = m.ListLikedPosts(5, 1, 1); total != 2 || ids(posts)[0] != 1 {
		t.Fatalf("page 2 want [1] total=2, got %v total=%d", ids(posts), total)
	}
	if posts, _ = m.ListLikedPosts(5, -5, 0); len(posts) != 2 {
		t.Fatalf("limit=0 must clamp to 20, got %v", ids(posts))
	}
	if posts, _ = m.ListLikedPosts(5, 0, -3); ids(posts)[0] != 2 {
		t.Fatalf("offset=-3 must clamp to 0, got %v", ids(posts))
	}

	// 软删排除：作者删帖后不出现在任何人我的点赞里
	if err := m.Delete(1, 1001); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if posts, total = m.ListLikedPosts(5, 20, 0); total != 1 || posts[0].Id != 2 {
		t.Fatalf("soft-deleted must be excluded, got %v total=%d", ids(posts), total)
	}
}

func TestPostModel_ListByFollow(t *testing.T) {
	m := newPostModel(t, true)
	// 种子：post 1 → topic 1 / author 1001，post 2 → topic 2 / author 1002

	// 话题命中
	posts, total := m.ListByFollow([]int64{1}, nil, 20, 0)
	if total != 1 || len(posts) != 1 || posts[0].Id != 1 {
		t.Fatalf("topic follow: want [1]/total 1, got %v total=%d", ids(posts), total)
	}
	// 作者命中
	posts, total = m.ListByFollow(nil, []int64{1002}, 20, 0)
	if total != 1 || len(posts) != 1 || posts[0].Id != 2 {
		t.Fatalf("author follow: want [2]/total 1, got %v total=%d", ids(posts), total)
	}
	// 并集去重 + 新帖在前（created_at 倒序）
	created, err := m.Create(1, 1001, "alice", &types.CreatePostReq{Title: "new", Content: "body"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	posts, total = m.ListByFollow([]int64{1}, []int64{1002}, 20, 0)
	if total != 3 || len(posts) != 3 || posts[0].Id != created.Id {
		t.Fatalf("union: want newest-first 3 posts, got %v total=%d", ids(posts), total)
	}
	// 分页：limit/offset 窗口 + total 全量
	posts, total = m.ListByFollow([]int64{1, 2}, []int64{1002}, 1, 1)
	if total != 3 || len(posts) != 1 {
		t.Fatalf("pagination window: want 1 item total=3, got %v total=%d", ids(posts), total)
	}
	// 越界 offset → 空窗但 total 不变
	if posts, total = m.ListByFollow([]int64{1, 2}, nil, 20, 99); total != 3 || len(posts) != 0 {
		t.Fatalf("beyond-end offset: want empty/total 3, got %v total=%d", ids(posts), total)
	}
	// 钳制默认：limit<=0 → 20，offset<0 → 0
	if posts, _ = m.ListByFollow([]int64{1, 2}, nil, 0, -5); len(posts) != 3 {
		t.Fatalf("clamped defaults: want 3 items, got %v", ids(posts))
	}
	// 空关注集 → 空结果
	if posts, total = m.ListByFollow(nil, nil, 20, 0); total != 0 || len(posts) != 0 {
		t.Fatalf("empty follows: want empty/0, got %v total=%d", ids(posts), total)
	}
	// 软删帖不进关注流
	if err := m.Delete(created.Id, 1001); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if posts, total = m.ListByFollow([]int64{1}, nil, 20, 0); total != 1 || posts[0].Id != 1 {
		t.Fatalf("soft-deleted must be excluded, got %v total=%d", ids(posts), total)
	}
}

// ids 提取帖子 id 序列，便于失败信息可读。
func ids(posts []types.Post) []int64 {
	out := make([]int64, 0, len(posts))
	for _, p := range posts {
		out = append(out, p.Id)
	}
	return out
}
