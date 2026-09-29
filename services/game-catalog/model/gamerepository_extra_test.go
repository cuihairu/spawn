package model

import (
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// seedGamesForSort 提供一组趋势分/评分/发售日各不相同的游戏，
// 用来区分 sortGames 的不同排序分支。
const seedGamesForSort = `[
	{"id":"g-low","title":"Low","description":"low trending","genres":["RPG"],"platforms":["PC"],"release_date":"2021-01-01","developer":"A","publisher":"A","tags":["x"],"score":10.0,"trending_score":10,"cover_image":""},
	{"id":"g-mid","title":"Mid","description":"mid trending","genres":["FPS"],"platforms":["Switch"],"release_date":"2022-02-02","developer":"B","publisher":"B","tags":["y"],"score":9.0,"trending_score":50,"cover_image":""},
	{"id":"g-high","title":"High","description":"high trending","genres":["RPG","Action"],"platforms":["PC","PS5"],"release_date":"2023-03-03","developer":"C","publisher":"C","tags":["x","y"],"score":8.0,"trending_score":90,"cover_image":""}
]`

// 同一趋势分时按 score 兜底排序（sortGames default 分支的次级比较）。
const seedGamesTiedTrending = `[
	{"id":"t-better","title":"Better","description":"","genres":[],"platforms":[],"release_date":"","developer":"","publisher":"","tags":[],"score":9.5,"trending_score":50,"cover_image":""},
	{"id":"t-worse","title":"Worse","description":"","genres":[],"platforms":[],"release_date":"","developer":"","publisher":"","tags":[],"score":7.0,"trending_score":50,"cover_image":""}
]`

// TestGameRepositoryGet 覆盖索引命中与未命中（ErrGameNotFound）。
func TestGameRepositoryGet(t *testing.T) {
	repo, err := NewGameRepository(writeTempGameFile(t, seedGamesForSort))
	if err != nil {
		t.Fatalf("NewGameRepository: %v", err)
	}

	game, err := repo.Get("g-high")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if game.Title != "High" {
		t.Fatalf("title = %q, want High", game.Title)
	}

	// 返回的是索引内的同一指针（调用方可见后续更新）
	game.Score = 8.5
	again, err := repo.Get("g-high")
	if err != nil || again.Score != 8.5 {
		t.Fatalf("Get again = %+v, %v; want same pointer", again, err)
	}

	if _, err := repo.Get("missing"); !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("Get(missing) err = %v, want ErrGameNotFound", err)
	}
}

// TestGameRepositoryCreate 覆盖正常创建：id 生成、索引同步、列表可见。
func TestGameRepositoryCreate(t *testing.T) {
	repo, err := NewGameRepository(writeTempGameFile(t, seedGamesForSort))
	if err != nil {
		t.Fatalf("NewGameRepository: %v", err)
	}

	created, err := repo.Create(&Game{Title: "Brand New", Score: 7.7, TrendingScore: 5})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !strings.HasPrefix(created.Id, "game-") {
		t.Fatalf("generated id = %q, want game- prefix", created.Id)
	}

	fetched, err := repo.Get(created.Id)
	if err != nil {
		t.Fatalf("Get(created): %v", err)
	}
	if fetched != created {
		t.Fatalf("index and returned pointer differ: %p vs %p", fetched, created)
	}

	_, total := repo.List(GameFilter{Limit: 10})
	if total != 4 {
		t.Fatalf("total after Create = %d, want 4", total)
	}

	// 两次创建的 id 必须不同
	other, err := repo.Create(&Game{Title: "Another"})
	if err != nil {
		t.Fatalf("Create second: %v", err)
	}
	if other.Id == created.Id {
		t.Fatalf("duplicate generated id %q", other.Id)
	}
}

// TestGameRepositoryCreateNilPayload 空载荷必须报错且不改动仓储。
func TestGameRepositoryCreateNilPayload(t *testing.T) {
	repo, err := NewGameRepository(writeTempGameFile(t, seedGamesForSort))
	if err != nil {
		t.Fatalf("NewGameRepository: %v", err)
	}

	if _, err := repo.Create(nil); err == nil {
		t.Fatal("Create(nil) must fail")
	}
	_, total := repo.List(GameFilter{Limit: 10})
	if total != 3 {
		t.Fatalf("total = %d, want 3 (unchanged)", total)
	}
}

// TestGameRepositoryFeatured 覆盖趋势分排序与 limit 归一化。
func TestGameRepositoryFeatured(t *testing.T) {
	repo, err := NewGameRepository(writeTempGameFile(t, seedGamesForSort))
	if err != nil {
		t.Fatalf("NewGameRepository: %v", err)
	}

	all := repo.Featured(0) // limit<=0 → 全量
	if len(all) != 3 {
		t.Fatalf("Featured(0) = %d games, want 3", len(all))
	}
	if all[0].Id != "g-high" || all[1].Id != "g-mid" || all[2].Id != "g-low" {
		t.Fatalf("trending order = %s,%s,%s", all[0].Id, all[1].Id, all[2].Id)
	}

	if got := repo.Featured(2); len(got) != 2 || got[0].Id != "g-high" {
		t.Fatalf("Featured(2) = %d games head %s", len(got), got[0].Id)
	}
	if got := repo.Featured(99); len(got) != 3 {
		t.Fatalf("Featured(99) = %d games, want clamp to 3", len(got))
	}
	if got := repo.Featured(-1); len(got) != 3 {
		t.Fatalf("Featured(-1) = %d games, want clamp to 3", len(got))
	}

	// 返回的是切片副本：就地重排不污染仓储内部顺序
	reordered := repo.Featured(3)
	reordered[0], reordered[2] = reordered[2], reordered[0]
	if again := repo.Featured(1); again[0].Id != "g-high" {
		t.Fatalf("repository order mutated: head = %s", again[0].Id)
	}
}

// TestGameRepositoryFeaturedTie 同趋势分时按 score 降序。
func TestGameRepositoryFeaturedTie(t *testing.T) {
	repo, err := NewGameRepository(writeTempGameFile(t, seedGamesTiedTrending))
	if err != nil {
		t.Fatalf("NewGameRepository: %v", err)
	}
	featured := repo.Featured(2)
	if featured[0].Id != "t-better" {
		t.Fatalf("tie head = %s, want higher score first", featured[0].Id)
	}
}

// TestGameRepositoryListReleaseDateSort 覆盖 release_date 排序分支（含大小写归一）。
func TestGameRepositoryListReleaseDateSort(t *testing.T) {
	repo, err := NewGameRepository(writeTempGameFile(t, seedGamesForSort))
	if err != nil {
		t.Fatalf("NewGameRepository: %v", err)
	}

	for _, sortBy := range []string{"release_date", "RELEASE_DATE"} {
		games, _ := repo.List(GameFilter{Sort: sortBy, Limit: 10})
		if len(games) != 3 {
			t.Fatalf("sort=%q returned %d games, want 3", sortBy, len(games))
		}
		if games[0].Id != "g-high" || games[2].Id != "g-low" {
			t.Fatalf("sort=%q order = %s..%s", sortBy, games[0].Id, games[2].Id)
		}
	}
}

// TestGameRepositoryListPagination 覆盖 offset/limit 的 clamp 与越界分支。
func TestGameRepositoryListPagination(t *testing.T) {
	repo, err := NewGameRepository(writeTempGameFile(t, seedGamesForSort))
	if err != nil {
		t.Fatalf("NewGameRepository: %v", err)
	}

	cases := []struct {
		name      string
		filter    GameFilter
		wantCount int
		wantTotal int
	}{
		{"offset beyond total", GameFilter{Offset: 10, Limit: 5}, 0, 3},
		{"zero limit", GameFilter{Limit: 0}, 0, 3},
		{"negative offset and limit", GameFilter{Offset: -5, Limit: -5}, 0, 3},
		// 负 limit 会让 end 落到 start 之前，由 start > end 兜底（不越界切片）
		{"negative limit with valid offset", GameFilter{Offset: 1, Limit: -5}, 0, 3},
		{"partial page", GameFilter{Offset: 1, Limit: 1}, 1, 3},
		{"limit beyond total", GameFilter{Limit: 100}, 3, 3},
	}
	for _, c := range cases {
		games, total := repo.List(c.filter)
		if total != c.wantTotal || len(games) != c.wantCount {
			t.Fatalf("%s: got %d games/total %d, want %d/%d",
				c.name, len(games), total, c.wantCount, c.wantTotal)
		}
	}
}

// TestGameRepositoryListFilterDimensions 覆盖 matches 的每个维度与大小写/空白归一。
func TestGameRepositoryListFilterDimensions(t *testing.T) {
	repo, err := NewGameRepository(writeTempGameFile(t, seedGamesForSort))
	if err != nil {
		t.Fatalf("NewGameRepository: %v", err)
	}

	// wantIDs 按 List 的默认排序（趋势分降序）给出期望顺序
	cases := []struct {
		name    string
		filter  GameFilter
		wantIDs []string
	}{
		{name: "keyword in title", filter: GameFilter{Keyword: "high"}, wantIDs: []string{"g-high"}},
		{name: "keyword case-insensitive", filter: GameFilter{Keyword: "MID"}, wantIDs: []string{"g-mid"}},
		{name: "keyword in description", filter: GameFilter{Keyword: "LOW TRENDING"}, wantIDs: []string{"g-low"}},
		{name: "keyword miss", filter: GameFilter{Keyword: "zelda"}},
		{name: "genre hit", filter: GameFilter{Genre: "RPG"}, wantIDs: []string{"g-high", "g-low"}},
		{name: "genre padded and lowercased", filter: GameFilter{Genre: "  fps  "}, wantIDs: []string{"g-mid"}},
		{name: "genre miss", filter: GameFilter{Genre: "MMO"}},
		{name: "platform hit", filter: GameFilter{Platform: "Switch"}, wantIDs: []string{"g-mid"}},
		{name: "platform miss", filter: GameFilter{Platform: "Mobile"}},
		{name: "tag hit", filter: GameFilter{Tag: "x"}, wantIDs: []string{"g-high", "g-low"}},
		{name: "tag miss", filter: GameFilter{Tag: "nope"}},
		{name: "combined all match", filter: GameFilter{Keyword: "High", Genre: "Action", Platform: "PS5", Tag: "y"}, wantIDs: []string{"g-high"}},
		{name: "combined one miss", filter: GameFilter{Genre: "Action", Platform: "Switch"}},
		{name: "empty filter", filter: GameFilter{}, wantIDs: []string{"g-high", "g-mid", "g-low"}},
	}
	for _, c := range cases {
		c.filter.Limit = 10
		games, total := repo.List(c.filter)
		if total != len(c.wantIDs) || len(games) != len(c.wantIDs) {
			t.Fatalf("%s: got %d games (total %d), want %d", c.name, len(games), total, len(c.wantIDs))
		}
		for i, want := range c.wantIDs {
			if games[i].Id != want {
				t.Fatalf("%s: result[%d] = %s, want %s", c.name, i, games[i].Id, want)
			}
		}
	}
}

// TestGameRepositoryRecommendAllGenres 推荐未指定类型时使用全量池，
// limit<=0 走默认 5。
func TestGameRepositoryRecommendAllGenres(t *testing.T) {
	repo, err := NewGameRepository(writeTempGameFile(t, seedGamesForSort))
	if err != nil {
		t.Fatalf("NewGameRepository: %v", err)
	}

	recs := repo.Recommend("user-a", nil, 0)
	if len(recs) != 3 {
		t.Fatalf("Recommend(nil genres, 0) = %d games, want 3 (default limit 5)", len(recs))
	}
	if recs[0].TrendingScore < recs[2].TrendingScore {
		t.Fatalf("recommendations not sorted by trending score: %+v", recs)
	}

	// 类型池非空时按类型过滤
	if recs := repo.Recommend("", []string{"FPS"}, 5); len(recs) != 1 || recs[0].Id != "g-mid" {
		t.Fatalf("Recommend(FPS) = %+v, want only g-mid", recs)
	}
	// limit 大于池子大小时不会越界
	if recs := repo.Recommend("", []string{"RPG", "Action"}, 50); len(recs) != 2 {
		t.Fatalf("Recommend(limit 50) = %d games, want 2", len(recs))
	}
	// limit 小于池子大小时按 limit 截断
	if recs := repo.Recommend("", nil, 2); len(recs) != 2 {
		t.Fatalf("Recommend(limit 2) = %d games, want 2", len(recs))
	}
}

// TestGameRepositoryRecommendTie 同趋势分时推荐按 score 降序。
func TestGameRepositoryRecommendTie(t *testing.T) {
	repo, err := NewGameRepository(writeTempGameFile(t, seedGamesTiedTrending))
	if err != nil {
		t.Fatalf("NewGameRepository: %v", err)
	}
	recs := repo.Recommend("", nil, 2)
	if len(recs) != 2 || recs[0].Id != "t-better" {
		t.Fatalf("Recommend tie = %+v, want higher score first", recs)
	}
}

// TestGameRepositoryLoadFallbacks 覆盖文件缺失/空数组回落内置种子，
// 以及索引跳过 nil 与空 id 条目。
//
// 未覆盖的 1 个语句：defaultSeedGames 中的 panic 分支——defaultGameSeed 是
// 编译期常量字面量，解析必然成功，panic 不可达（无需构造非法常量来凑覆盖）。
func TestGameRepositoryLoadFallbacks(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "not-there.json")
	repo, err := NewGameRepository(missing)
	if err != nil {
		t.Fatalf("NewGameRepository(missing): %v", err)
	}
	if _, total := repo.List(GameFilter{Limit: 100}); total != 8 {
		t.Fatalf("missing file seed total = %d, want 8", total)
	}
	if _, err := repo.Get("game-elden-ring"); err != nil {
		t.Fatalf("seeded game not indexed: %v", err)
	}

	empty, err := NewGameRepository(writeTempGameFile(t, `[]`))
	if err != nil {
		t.Fatalf("NewGameRepository([]): %v", err)
	}
	if _, total := empty.List(GameFilter{Limit: 100}); total != 8 {
		t.Fatalf("empty array seed total = %d, want 8", total)
	}

	// 坏条目不进入索引（因此按 id 查不到），但也不影响正常条目
	dirty := writeTempGameFile(t, `[null,{"id":"","title":"no id"},{"id":"g-ok","title":"Ok"}]`)
	dirtyRepo, err := NewGameRepository(dirty)
	if err != nil {
		t.Fatalf("NewGameRepository(dirty): %v", err)
	}
	if _, err := dirtyRepo.Get(""); !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("Get(\"\") err = %v, want ErrGameNotFound", err)
	}
	if _, err := dirtyRepo.Get("g-ok"); err != nil {
		t.Fatalf("Get(g-ok): %v", err)
	}
}

// TestGameRepositoryLoadErrors 非法 JSON 与不可读路径必须报错（不回落）。
func TestGameRepositoryLoadErrors(t *testing.T) {
	bad := writeTempGameFile(t, `{not json`)
	if _, err := NewGameRepository(bad); err == nil {
		t.Fatal("malformed JSON must fail")
	} else if !strings.Contains(err.Error(), "unmarshal game data") {
		t.Fatalf("err = %v, want unmarshal context", err)
	}

	// 目录路径：读取失败且不是 ErrNotExist
	if _, err := NewGameRepository(t.TempDir()); err == nil {
		t.Fatal("unreadable source must fail")
	} else if !strings.Contains(err.Error(), "read game data") {
		t.Fatalf("err = %v, want read context", err)
	}
}

// TestGameRepositoryConcurrentAccess 在 -race 下同时读写的冒烟测试。
func TestGameRepositoryConcurrentAccess(t *testing.T) {
	repo, err := NewGameRepository(writeTempGameFile(t, seedGamesForSort))
	if err != nil {
		t.Fatalf("NewGameRepository: %v", err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := repo.Create(&Game{Title: "Concurrent", TrendingScore: 1}); err != nil {
				t.Errorf("Create: %v", err)
			}
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			repo.Featured(2)
			repo.Recommend("user-c", []string{"RPG"}, 2)
			repo.List(GameFilter{Limit: 5})
			if _, err := repo.Get("g-mid"); err != nil {
				t.Errorf("Get: %v", err)
			}
		}()
	}
	wg.Wait()

	if _, total := repo.List(GameFilter{Limit: 100}); total != 11 {
		t.Fatalf("total = %d, want 11", total)
	}
}
