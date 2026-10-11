package model

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// playerStatsSchema 建表语句的测试副本：缺列与脏行用例需要精确控制列集合。
const playerStatsSchema = `
CREATE TABLE player_stats (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    game_id VARCHAR(64) NOT NULL,
    game_title VARCHAR(128) NOT NULL DEFAULT '',
    matches INTEGER NOT NULL DEFAULT 0,
    wins INTEGER NOT NULL DEFAULT 0,
    kills INTEGER NOT NULL DEFAULT 0,
    deaths INTEGER NOT NULL DEFAULT 0,
    assists INTEGER NOT NULL DEFAULT 0,
    score INTEGER NOT NULL DEFAULT 0,
    rank_points INTEGER NOT NULL DEFAULT 0,
    last_played_at VARCHAR(32) NOT NULL DEFAULT '',
    created_at VARCHAR(32) NOT NULL,
    updated_at VARCHAR(32) NOT NULL
)`

// newRawStatsModel 建临时库并按给定 schema 建表（不走 NewPlayerStatsModel 的
// CreateTable），供缺列/脏行故障注入用例使用。
func newRawStatsModel(t *testing.T, schema string, inserts ...string) *PlayerStatsModel {
	t.Helper()

	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	if _, err := db.Exec(schema); err != nil {
		t.Fatalf("exec schema: %v", err)
	}
	for _, q := range inserts {
		if _, err := db.Exec(q); err != nil {
			t.Fatalf("exec %q: %v", q, err)
		}
	}
	return NewPlayerStatsModel(db)
}

// newClosedStatsModel 建表后立即关闭底层库，专打各方法的错误返回路径。
func newClosedStatsModel(t *testing.T) *PlayerStatsModel {
	t.Helper()
	m := newTestStatsModel(t)
	if err := m.db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}
	return m
}

// TestGetGame_CacheHitSecondRead 二次读取命中缓存（值副本语义：改动不回灌）。
func TestGetGame_CacheHitSecondRead(t *testing.T) {
	m := newTestStatsModel(t)

	if _, err := m.Record(1001, recordReq("game-elden-ring", map[string]int64{"matches": 2})); err != nil {
		t.Fatalf("record: %v", err)
	}
	first, err := m.GetGame(1001, "game-elden-ring")
	if err != nil {
		t.Fatalf("first get: %v", err)
	}
	first.Matches = 999 // 调用方改动不得回灌缓存

	second, err := m.GetGame(1001, "game-elden-ring")
	if err != nil {
		t.Fatalf("second get: %v", err)
	}
	if second.Matches != 2 {
		t.Fatalf("cache was mutated by caller copy: matches = %d, want 2", second.Matches)
	}
}

// TestSeed_NilRowSkipped 种子行允许 nil 占位（跳过不报错）。
func TestSeed_NilRowSkipped(t *testing.T) {
	m := newTestStatsModel(t)

	if err := m.Seed([]*seedStatRow{nil, {
		UserId: 1001, GameId: "game-apex", GameTitle: "Apex Legends", Matches: 1,
	}}); err != nil {
		t.Fatalf("seed with nil row: %v", err)
	}
	games, total, err := m.ListGames(1001, 10, 0)
	if err != nil || total != 1 || len(games) != 1 {
		t.Fatalf("after seed: total=%d len=%d err=%v", total, len(games), err)
	}
}

// --- 底层不可达 → 各方法错误传播（格式化包裹后返回） ---

func TestCreateTable_ClosedDBErrors(t *testing.T) {
	m := newClosedStatsModel(t)
	if err := m.CreateTable(); err == nil {
		t.Fatal("CreateTable on closed db must error")
	}
}

func TestSeed_ClosedDBErrors(t *testing.T) {
	m := newClosedStatsModel(t)
	err := m.Seed([]*seedStatRow{{UserId: 1, GameId: "g", Matches: 1}})
	if err == nil {
		t.Fatal("Seed on closed db must error")
	}
}

func TestSeedIfEmpty_ClosedDBErrors(t *testing.T) {
	m := newClosedStatsModel(t)
	if err := m.SeedIfEmpty(); err == nil {
		t.Fatal("SeedIfEmpty on closed db must error")
	}
}

// TestRecord_ClosedDBErrors 双方言连败（SQLite ON CONFLICT 与 MySQL
// ON DUPLICATE KEY 均报错）→「写入战绩失败」错误返回。
func TestRecord_ClosedDBErrors(t *testing.T) {
	m := newClosedStatsModel(t)
	if _, err := m.Record(1001, recordReq("game-apex", map[string]int64{"matches": 1})); err == nil {
		t.Fatal("Record on closed db must error")
	}
}

func TestListGames_ClosedDBErrors(t *testing.T) {
	m := newClosedStatsModel(t)
	if _, _, err := m.ListGames(1001, 10, 0); err == nil {
		t.Fatal("ListGames on closed db must error")
	}
}

func TestSummary_ClosedDBErrors(t *testing.T) {
	m := newClosedStatsModel(t)
	if _, err := m.Summary(1001); err == nil {
		t.Fatal("Summary on closed db must error")
	}
}

// TestGetGame_ClosedDBErrors 非 NotFound 的查询错误原样透传（非哨兵）。
func TestGetGame_ClosedDBErrors(t *testing.T) {
	m := newClosedStatsModel(t)
	_, err := m.GetGame(1001, "game-apex")
	if err == nil || errors.Is(err, ErrStatNotFound) {
		t.Fatalf("GetGame on closed db err=%v, want non-sentinel error", err)
	}
}

// TestListGames_MissingColumnQueryFails 关库会让先执行的 COUNT 先失败
// （够不到列表查询），故用缺列表：COUNT(*) 正常、SELECT 具名列报
// no such column → 列表查询错误分支。
func TestListGames_MissingColumnQueryFails(t *testing.T) {
	m := newRawStatsModel(t, `CREATE TABLE player_stats (id INTEGER PRIMARY KEY, user_id INTEGER NOT NULL)`,
		`INSERT INTO player_stats (id, user_id) VALUES (1, 1001)`)

	if _, _, err := m.ListGames(1001, 10, 0); err == nil {
		t.Fatal("ListGames on column-missing table must error")
	}
}

// TestListGames_DirtyRowScanFails 列齐全但 matches 存文本：COUNT 与
// SELECT 均成功，Scan 到 int64 失败 → 行扫描错误分支。
func TestListGames_DirtyRowScanFails(t *testing.T) {
	m := newRawStatsModel(t, playerStatsSchema, `INSERT INTO player_stats
		(id, user_id, game_id, matches, created_at, updated_at)
		VALUES (1, 1001, 'game-apex', 'not-a-number', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)

	if _, _, err := m.ListGames(1001, 10, 0); err == nil {
		t.Fatal("ListGames on dirty row must error")
	}
}

// TestSeedIfEmpty_SkipsWhenRowsExist 已有数据（count>0）→ 跳过种子，
// 一行都不插。对应生产库已迁移数据、容器重启不重灌的语义。
func TestSeedIfEmpty_SkipsWhenRowsExist(t *testing.T) {
	m := newRawStatsModel(t, playerStatsSchema, `INSERT INTO player_stats
		(id, user_id, game_id, matches, wins, kills, deaths, assists, score, rank_points,
		 last_played_at, created_at, updated_at)
		VALUES (1, 1001, 'game-apex', 3, 2, 20, 5, 4, 900, 30,
		 '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')`)

	if err := m.SeedIfEmpty(); err != nil {
		t.Fatalf("SeedIfEmpty with existing row: %v", err)
	}
	var count int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM player_stats`).Scan(&count); err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1 (seed skipped)", count)
	}
}
