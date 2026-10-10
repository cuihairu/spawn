package model

import (
	"errors"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

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
