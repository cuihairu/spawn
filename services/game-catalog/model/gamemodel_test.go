package model

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newTestGameModel 打开内存 SQLite 并建表；seed=true 时写入内嵌种子。
func newTestGameModel(t *testing.T, seed bool) *GameModel {
	t.Helper()
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	m := NewGameModel(db)
	if err := m.CreateGamesTable(); err != nil {
		t.Fatalf("create table: %v", err)
	}
	if seed {
		if err := m.SeedIfEmpty(); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	return m
}

func mustSeedGames(t *testing.T, m *GameModel, games ...*Game) {
	t.Helper()
	if err := m.Seed(games); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func sampleFixture() []*Game {
	return []*Game{
		{Id: "g-a", Title: "Alpha RPG", Description: "deep role playing", Genres: []string{"RPG"},
			Platforms: []string{"PC"}, Tags: []string{"story"}, Score: 9.0, TrendingScore: 90,
			ReleaseDate: "2024-01-01", Developer: "dev-a", Publisher: "pub-a", CoverImage: "img-a"},
		{Id: "g-b", Title: "Beta Quest", Description: "RPG adventure", Genres: []string{"RPG", "Action"},
			Platforms: []string{"PC", "PS5"}, Tags: []string{"coop"}, Score: 8.0, TrendingScore: 85,
			ReleaseDate: "2023-05-01"},
		{Id: "g-c", Title: "Gamma Strike", Description: "fast shooter", Genres: []string{"FPS"},
			Platforms: []string{"Switch"}, Tags: []string{"pvp"}, Score: 8.8, TrendingScore: 80,
			ReleaseDate: "2025-01-01"},
	}
}

func TestSeedIfEmpty_PopulatesAndSkipsNonEmpty(t *testing.T) {
	m := newTestGameModel(t, true)

	if _, err := m.Get("game-elden-ring"); err != nil {
		t.Fatalf("seeded game missing: %v", err)
	}
	// 模型层 List 的 Limit=0 返回 0 行（logic 层才兜底 20），取大 limit 验证全量
	games, total := m.List(GameFilter{Limit: 100})
	if total != 8 || len(games) != 8 {
		t.Fatalf("seed count = %d/%d, want 8/8", len(games), total)
	}

	// 非空表再跑 SeedIfEmpty 必须是 no-op
	if err := m.SeedIfEmpty(); err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if _, total := m.List(GameFilter{Limit: 100}); total != 8 {
		t.Fatalf("count after re-seed = %d, want 8", total)
	}
}

func TestGet_MissReturnsSentinel_NoNegativeCache(t *testing.T) {
	m := newTestGameModel(t, true)

	if _, err := m.Get("no-such-game"); !errors.Is(err, ErrGameNotFound) {
		t.Fatalf("err = %v, want ErrGameNotFound", err)
	}

	// 未命中不写缓存：随后写入同 id 应立即可见
	mustSeedGames(t, m, &Game{Id: "no-such-game", Title: "Late Arrival"})
	game, err := m.Get("no-such-game")
	if err != nil || game == nil || game.Title != "Late Arrival" {
		t.Fatalf("game after late insert = %+v, err=%v", game, err)
	}
}

func TestGet_CacheHitServesAfterDBClose(t *testing.T) {
	m := newTestGameModel(t, true)

	if _, err := m.Get("game-elden-ring"); err != nil {
		t.Fatalf("prime cache: %v", err)
	}
	if err := m.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	game, err := m.Get("game-elden-ring")
	if err != nil || game == nil || game.Title != "Elden Ring" {
		t.Fatalf("cached game = %+v, err=%v", game, err)
	}
	// 未命中路径在关库后应报错而非哨兵（区别于 not-found）
	if _, err := m.Get("game-valorant"); err == nil || errors.Is(err, ErrGameNotFound) {
		t.Fatalf("err = %v, want non-sentinel db error", err)
	}
}

func TestGet_ValueCopyIsolation(t *testing.T) {
	m := newTestGameModel(t, true)

	first, err := m.Get("game-elden-ring")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	first.Genres[0] = "hacked"
	first.Title = "hacked"

	second, err := m.Get("game-elden-ring")
	if err != nil {
		t.Fatalf("get again: %v", err)
	}
	if second.Title != "Elden Ring" || second.Genres[0] != "RPG" {
		t.Fatalf("cache value aliased: %+v", second)
	}
}

func TestCreate_AssignsIDPersistsAndRoundTrips(t *testing.T) {
	dir := t.TempDir()
	dsn := "file:" + filepath.Join(dir, "games.db")

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	m := NewGameModel(db)
	if err := m.CreateGamesTable(); err != nil {
		t.Fatalf("create table: %v", err)
	}

	created, err := m.Create(&Game{
		Title: "New Game", Description: "d", Genres: []string{"RPG"},
		Platforms: []string{"PC", "PS5"}, Tags: []string{"new"},
		ReleaseDate: "2026-01-01", Developer: "dev", Publisher: "pub",
		Score: 8.8, TrendingScore: 66, CoverImage: "cover",
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(created.Id) <= len("game-") {
		t.Fatalf("id not assigned: %q", created.Id)
	}
	roundtrip, err := m.Get(created.Id)
	if err != nil {
		t.Fatalf("get created: %v", err)
	}
	if roundtrip.Title != "New Game" || len(roundtrip.Platforms) != 2 ||
		roundtrip.TrendingScore != 66 || roundtrip.CoverImage != "cover" {
		t.Fatalf("roundtrip mismatch: %+v", roundtrip)
	}
	db.Close()

	// 重开连接验证持久化（表已存在则建表语句为 no-op）
	db2, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer db2.Close()
	m2 := NewGameModel(db2)
	if err := m2.CreateGamesTable(); err != nil {
		t.Fatalf("create table 2: %v", err)
	}
	if game, err := m2.Get(created.Id); err != nil || game.Title != "New Game" {
		t.Fatalf("persisted game = %+v, err=%v", game, err)
	}
}

func TestCreate_NilPayloadRejected(t *testing.T) {
	m := newTestGameModel(t, false)
	if _, err := m.Create(nil); err == nil {
		t.Fatal("nil payload must error")
	}
}

func TestList_FiltersSortAndPagination(t *testing.T) {
	m := newTestGameModel(t, false)
	mustSeedGames(t, m, sampleFixture()...)

	if _, total := m.List(GameFilter{Keyword: "rpg"}); total != 2 {
		t.Fatalf("keyword total = %d, want 2", total)
	}
	if _, total := m.List(GameFilter{Genre: "action"}); total != 1 {
		t.Fatalf("genre total = %d, want 1", total)
	}
	if _, total := m.List(GameFilter{Platform: "switch"}); total != 1 {
		t.Fatalf("platform total = %d, want 1", total)
	}
	if _, total := m.List(GameFilter{Tag: "coop"}); total != 1 {
		t.Fatalf("tag total = %d, want 1", total)
	}

	games, total := m.List(GameFilter{Limit: 10})
	if total != 3 || len(games) != 3 {
		t.Fatalf("all = %d/%d, want 3/3", len(games), total)
	}
	if games[0].Id != "g-a" || games[1].Id != "g-b" || games[2].Id != "g-c" {
		t.Fatalf("default order = %s,%s,%s", games[0].Id, games[1].Id, games[2].Id)
	}

	games, _ = m.List(GameFilter{Sort: "release_date", Limit: 10})
	if games[0].Id != "g-c" || games[2].Id != "g-b" {
		t.Fatalf("release order = %s,%s,%s", games[0].Id, games[1].Id, games[2].Id)
	}

	games, total = m.List(GameFilter{Limit: 2, Offset: 1})
	if total != 3 || len(games) != 2 || games[0].Id != "g-b" {
		t.Fatalf("page = %d/%d first=%s", len(games), total, games[0].Id)
	}
	if games, total := m.List(GameFilter{Limit: 5, Offset: 99}); total != 3 || len(games) != 0 {
		t.Fatalf("offset beyond = %d/%d, want 0/3", len(games), total)
	}
}

func TestFeatured_OrdersByTrendingAndClamps(t *testing.T) {
	m := newTestGameModel(t, false)
	mustSeedGames(t, m, sampleFixture()...)

	all := m.Featured(0)
	if len(all) != 3 || all[0].Id != "g-a" || all[1].Id != "g-b" || all[2].Id != "g-c" {
		t.Fatalf("featured all = %s,%s,%s", all[0].Id, all[1].Id, all[2].Id)
	}
	top2 := m.Featured(2)
	if len(top2) != 2 || top2[0].Id != "g-a" || top2[1].Id != "g-b" {
		t.Fatalf("featured top2 = %s,%s", top2[0].Id, top2[1].Id)
	}
	if over := m.Featured(99); len(over) != 3 {
		t.Fatalf("featured over-limit = %d, want 3", len(over))
	}
}

func TestRecommend_GenrePoolRotationAndFallback(t *testing.T) {
	m := newTestGameModel(t, false)
	mustSeedGames(t, m, sampleFixture()...)

	// 题材命中池：RPG 两款按趋势分排序
	rpg := m.Recommend("", []string{"RPG"}, 10)
	if len(rpg) != 2 || rpg[0].Id != "g-a" || rpg[1].Id != "g-b" {
		t.Fatalf("rpg pool = %s,%s", rpg[0].Id, rpg[1].Id)
	}

	// 无命中回落全量池
	fallback := m.Recommend("", []string{"Sports"}, 10)
	if len(fallback) != 3 {
		t.Fatalf("fallback pool = %d, want 3", len(fallback))
	}

	// 同一 userId 结果确定
	first := m.Recommend("user-42", nil, 3)
	second := m.Recommend("user-42", nil, 3)
	if len(first) != 3 {
		t.Fatalf("recommend len = %d", len(first))
	}
	for i := range first {
		if first[i].Id != second[i].Id {
			t.Fatalf("rotation unstable at %d: %s vs %s", i, first[i].Id, second[i].Id)
		}
	}

	// limit<=0 取默认 5（池仅 3 → 全量）
	if got := m.Recommend("", nil, 0); len(got) != 3 {
		t.Fatalf("default-limit recommend = %d, want 3", len(got))
	}
}

func TestSeed_SkipsNilAndEmptyID(t *testing.T) {
	m := newTestGameModel(t, false)
	mustSeedGames(t, m,
		nil,
		&Game{Title: "no id"},
		&Game{Id: "g-ok", Title: "Valid"},
	)
	if _, total := m.List(GameFilter{}); total != 1 {
		t.Fatalf("total = %d, want 1", total)
	}
}

func TestCreateGamesTable_ReportsErrorWhenBothDialectsFail(t *testing.T) {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	db.Close() // 关库后两种方言的 Exec 都会失败

	m := NewGameModel(db)
	if err := m.CreateGamesTable(); err == nil {
		t.Fatal("closed db must fail table creation")
	}
}

// TestClosedDB_ErrorPaths 关库后各入口的错误传播语义：
// 读列表类静默返回零值（接口签名无 error），单条读写显式报错。
func TestClosedDB_ErrorPaths(t *testing.T) {
	m := newTestGameModel(t, true)
	if err := m.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	if games, total := m.List(GameFilter{Limit: 5}); games != nil || total != 0 {
		t.Fatalf("List on closed db = %d/%d, want nil/0", len(games), total)
	}
	if games := m.Featured(3); games != nil {
		t.Fatalf("Featured on closed db = %d games, want nil", len(games))
	}
	if games := m.Recommend("u", nil, 3); games != nil {
		t.Fatalf("Recommend on closed db = %d games, want nil", len(games))
	}
	if _, err := m.Get("game-elden-ring"); err == nil || errors.Is(err, ErrGameNotFound) {
		t.Fatalf("Get on closed db = %v, want non-sentinel error", err)
	}
	if _, err := m.Create(&Game{Title: "x"}); err == nil {
		t.Fatal("Create on closed db must error")
	}
	if err := m.Seed([]*Game{{Id: "g-x", Title: "x"}}); err == nil {
		t.Fatal("Seed on closed db must error")
	}
	if err := m.SeedIfEmpty(); err == nil {
		t.Fatal("SeedIfEmpty on closed db must error")
	}
}

// TestSeed_DuplicateIDPropagatesError 主键冲突错误不被 Seed 吞掉。
func TestSeed_DuplicateIDPropagatesError(t *testing.T) {
	m := newTestGameModel(t, true)
	if err := m.Seed([]*Game{{Id: "game-elden-ring", Title: "dup"}}); err == nil {
		t.Fatal("duplicate primary key must error")
	}
}

// TestCorruptedArrayColumns 数组列存非法 JSON → Get/all 报
// 「解析游戏数组字段失败」而非 panic；列表读静默降级为空结果。
// 三个数组列分别注入，覆盖 Get 与 all 中各自的 decodeList 错误出口。
func TestCorruptedArrayColumns(t *testing.T) {
	for _, col := range []string{"genres", "platforms", "tags"} {
		t.Run(col, func(t *testing.T) {
			m := newTestGameModel(t, false)
			if _, err := m.db.Exec(
				`INSERT INTO games (` + gameColumns + `)
				 VALUES ('g-bad', 'Bad', 'd', '[]', '[]', '[]', '2024-01-01', 'dev', 'pub', 1.0, 1, 'c')`); err != nil {
				t.Fatalf("insert row: %v", err)
			}
			if _, err := m.db.Exec(`UPDATE games SET ` + col + ` = '{oops' WHERE id = 'g-bad'`); err != nil {
				t.Fatalf("corrupt column: %v", err)
			}

			if _, err := m.Get("g-bad"); err == nil || !strings.Contains(err.Error(), "解析游戏数组字段失败") {
				t.Fatalf("Get corrupt %s = %v, want 解析游戏数组字段失败", col, err)
			}
			if games, total := m.List(GameFilter{Limit: 5}); games != nil || total != 0 {
				t.Fatalf("List corrupt %s = %d/%d, want nil/0", col, len(games), total)
			}
		})
	}
}

// TestEmptyArrayColumns 空串数组列（绕过 encodeList 归一化直写的行）
// 解码为空数组而非错误。
func TestEmptyArrayColumns(t *testing.T) {
	m := newTestGameModel(t, false)
	if _, err := m.db.Exec(
		`INSERT INTO games (` + gameColumns + `)
		 VALUES ('g-empty', 'Empty', 'd', '', '', '', '2024-01-01', 'dev', 'pub', 1.0, 1, 'c')`); err != nil {
		t.Fatalf("insert row: %v", err)
	}

	game, err := m.Get("g-empty")
	if err != nil {
		t.Fatalf("Get empty cols: %v", err)
	}
	if len(game.Genres) != 0 || len(game.Platforms) != 0 || len(game.Tags) != 0 {
		t.Fatalf("empty cols decoded = %v/%v/%v, want empty", game.Genres, game.Platforms, game.Tags)
	}
}

// TestScanError_TextInNumericColumn 数值列被直写文本（SQLite 动态类型）
// → all 的 Scan 错误出口，列表读静默降级。
func TestScanError_TextInNumericColumn(t *testing.T) {
	m := newTestGameModel(t, false)
	if _, err := m.db.Exec(
		`INSERT INTO games (` + gameColumns + `)
		 VALUES ('g-badnum', 'BadNum', 'd', '[]', '[]', '[]', '2024-01-01', 'dev', 'pub', 1.0, 'oops', 'c')`); err != nil {
		t.Fatalf("insert row: %v", err)
	}

	if games, total := m.List(GameFilter{Limit: 5}); games != nil || total != 0 {
		t.Fatalf("List badnum = %d/%d, want nil/0", len(games), total)
	}
	if _, err := m.Get("g-badnum"); err == nil || errors.Is(err, ErrGameNotFound) {
		t.Fatalf("Get badnum = %v, want scan error", err)
	}
}

// TestSortGames_TieBreaksByScore 默认排序同趋势分按 score 降序。
func TestSortGames_TieBreaksByScore(t *testing.T) {
	m := newTestGameModel(t, false)
	mustSeedGames(t, m,
		&Game{Id: "g-lo", Title: "Low", TrendingScore: 50, Score: 6.0},
		&Game{Id: "g-hi", Title: "High", TrendingScore: 50, Score: 9.0},
	)
	games, _ := m.List(GameFilter{Limit: 10})
	if games[0].Id != "g-hi" || games[1].Id != "g-lo" {
		t.Fatalf("tie order = %s,%s, want score desc", games[0].Id, games[1].Id)
	}
}

// TestList_NegativeLimitWithOffset offset>0 且 limit<0 时分页窗口倒挂，
// 防御性钳制返回空页（与原内存仓行为一致）。
func TestList_NegativeLimitWithOffset(t *testing.T) {
	m := newTestGameModel(t, true)
	games, total := m.List(GameFilter{Limit: -2, Offset: 5})
	if total != 8 || len(games) != 0 {
		t.Fatalf("inverted window = %d/%d, want 0/8", len(games), total)
	}
}

// TestRecommend_TieBreaksByScore 推荐池同趋势分按 score 降序定序。
func TestRecommend_TieBreaksByScore(t *testing.T) {
	m := newTestGameModel(t, false)
	mustSeedGames(t, m,
		&Game{Id: "g-lo", Title: "Low", Genres: []string{"Puzzle"}, TrendingScore: 50, Score: 6.0},
		&Game{Id: "g-hi", Title: "High", Genres: []string{"Puzzle"}, TrendingScore: 50, Score: 9.0},
	)
	rec := m.Recommend("", []string{"Puzzle"}, 10)
	if len(rec) != 2 || rec[0].Id != "g-hi" || rec[1].Id != "g-lo" {
		t.Fatalf("tie order = %s,%s, want score desc", rec[0].Id, rec[1].Id)
	}
}
