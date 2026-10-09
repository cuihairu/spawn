package model

import (
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/data-panel/internal/types"

	_ "github.com/mattn/go-sqlite3"
)

// newTestStatsModel 临时 SQLite 库 + 建表（不走种子，测试自行装载）。
func newTestStatsModel(t *testing.T) *PlayerStatsModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "stats.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	m := NewPlayerStatsModel(db)
	if err := m.CreateTable(); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return m
}

func recordReq(gameId string, deltas map[string]int64) *types.RecordStatReq {
	req := &types.RecordStatReq{GameId: gameId}
	if v, ok := deltas["matches"]; ok {
		req.Matches = v
	}
	if v, ok := deltas["wins"]; ok {
		req.Wins = v
	}
	if v, ok := deltas["kills"]; ok {
		req.Kills = v
	}
	if v, ok := deltas["deaths"]; ok {
		req.Deaths = v
	}
	if v, ok := deltas["assists"]; ok {
		req.Assists = v
	}
	if v, ok := deltas["score"]; ok {
		req.Score = v
	}
	if v, ok := deltas["rank_points"]; ok {
		req.RankPoints = v
	}
	return req
}

// TestRecordInsertThenIncrement 首次摄入建行、再次摄入单调累加。
func TestRecordInsertThenIncrement(t *testing.T) {
	m := newTestStatsModel(t)

	first, err := m.Record(1001, recordReq("game-elden-ring", map[string]int64{
		"matches": 1, "wins": 1, "kills": 8, "deaths": 3,
	}))
	if err != nil {
		t.Fatalf("first record: %v", err)
	}
	if first.Matches != 1 || first.Wins != 1 || first.WinRate != 1 {
		t.Fatalf("first stat mismatch: %+v", first)
	}
	if first.LastPlayedAt == "" || first.CreatedAt == "" || first.UpdatedAt == "" {
		t.Fatalf("timestamps must be set: %+v", first)
	}

	second, err := m.Record(1001, recordReq("game-elden-ring", map[string]int64{
		"matches": 2, "wins": 0, "kills": 10, "deaths": 5,
	}))
	if err != nil {
		t.Fatalf("second record: %v", err)
	}
	// 单调累加：2 + 1 = 3 场，2 + 0 胜不变；kd = (8+10)/（3+5）
	if second.Matches != 3 || second.Wins != 1 {
		t.Fatalf("increment mismatch: matches=%d wins=%d, want 3/1", second.Matches, second.Wins)
	}
	if second.Kills != 18 || second.Deaths != 8 {
		t.Fatalf("kills/deaths mismatch: %+v", second)
	}
	if second.Kd != 2.25 || second.WinRate != 0.3333 {
		t.Fatalf("ratio mismatch: kd=%v win_rate=%v", second.Kd, second.WinRate)
	}
}

// TestRecordPerUserIsolated 同 game_id 不同 user 各自成行。
func TestRecordPerUserIsolated(t *testing.T) {
	m := newTestStatsModel(t)

	if _, err := m.Record(1001, recordReq("game-valorant", map[string]int64{"matches": 5})); err != nil {
		t.Fatalf("record user 1001: %v", err)
	}
	if _, err := m.Record(1002, recordReq("game-valorant", map[string]int64{"matches": 7})); err != nil {
		t.Fatalf("record user 1002: %v", err)
	}

	a, err := m.GetGame(1001, "game-valorant")
	if err != nil || a.Matches != 5 {
		t.Fatalf("user 1001 stat: %+v err=%v", a, err)
	}
	b, err := m.GetGame(1002, "game-valorant")
	if err != nil || b.Matches != 7 {
		t.Fatalf("user 1002 stat: %+v err=%v", b, err)
	}
}

// TestRecordTitleAndPlayedAtMerge game_title 非空覆盖/空保留；last_played_at 取最大。
func TestRecordTitleAndPlayedAtMerge(t *testing.T) {
	m := newTestStatsModel(t)

	if _, err := m.Record(1001, &types.RecordStatReq{
		GameId: "game-apex", GameTitle: "Apex Legends", Matches: 1, PlayedAt: "2026-10-01T08:00:00Z",
	}); err != nil {
		t.Fatalf("record 1: %v", err)
	}
	// 空标题不覆盖；更早的 played_at 不回退
	stat, err := m.Record(1001, &types.RecordStatReq{
		GameId: "game-apex", Matches: 1, PlayedAt: "2026-09-01T08:00:00Z",
	})
	if err != nil {
		t.Fatalf("record 2: %v", err)
	}
	if stat.GameTitle != "Apex Legends" {
		t.Fatalf("title was overwritten by empty value: %q", stat.GameTitle)
	}
	if stat.LastPlayedAt != "2026-10-01T08:00:00Z" {
		t.Fatalf("last_played_at regressed: %q", stat.LastPlayedAt)
	}
}

// TestSummary 聚合跨游戏：总数/胜率/kd/游戏数/最近游玩。
func TestSummary(t *testing.T) {
	m := newTestStatsModel(t)

	summary, err := m.Summary(1001)
	if err != nil {
		t.Fatalf("empty summary: %v", err)
	}
	if summary.TotalMatches != 0 || summary.GameCount != 0 || summary.WinRate != 0 {
		t.Fatalf("empty summary must be all-zero: %+v", summary)
	}

	if _, err := m.Record(1001, recordReq("game-elden-ring", map[string]int64{
		"matches": 3, "wins": 2, "kills": 20, "deaths": 10, "score": 900, "rank_points": 12,
	})); err != nil {
		t.Fatalf("record a: %v", err)
	}
	if _, err := m.Record(1001, recordReq("game-valorant", map[string]int64{
		"matches": 4, "wins": 1, "kills": 10, "deaths": 15, "score": 400, "rank_points": 20,
	})); err != nil {
		t.Fatalf("record b: %v", err)
	}

	summary, err = m.Summary(1001)
	if err != nil {
		t.Fatalf("summary: %v", err)
	}
	if summary.TotalMatches != 7 || summary.TotalWins != 3 {
		t.Fatalf("totals mismatch: matches=%d wins=%d", summary.TotalMatches, summary.TotalWins)
	}
	if summary.GameCount != 2 || summary.TotalScore != 1300 || summary.TotalRankPoints != 32 {
		t.Fatalf("aggregate mismatch: %+v", summary)
	}
	if summary.WinRate != 0.4286 || summary.Kd != 1.2 {
		t.Fatalf("ratios mismatch: win_rate=%v kd=%v", summary.WinRate, summary.Kd)
	}
}

// TestListGames 列表按场次降序 + 分页 + total；空用户返回空切片非 nil。
func TestListGames(t *testing.T) {
	m := newTestStatsModel(t)

	games, total, err := m.ListGames(1001, 20, 0)
	if err != nil || total != 0 || games == nil || len(games) != 0 {
		t.Fatalf("empty list: games=%v total=%d err=%v", games, total, err)
	}

	if _, err := m.Record(1001, recordReq("game-elden-ring", map[string]int64{"matches": 3})); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Record(1001, recordReq("game-valorant", map[string]int64{"matches": 9})); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Record(1001, recordReq("game-apex", map[string]int64{"matches": 9})); err != nil {
		t.Fatal(err)
	}

	games, total, err = m.ListGames(1001, 2, 0)
	if err != nil || total != 3 || len(games) != 2 {
		t.Fatalf("page 1: n=%d total=%d err=%v", len(games), total, err)
	}
	// 场次降序；9/9 并列按 game_id 升序（apex < valorant）
	if games[0].GameId != "game-apex" || games[1].GameId != "game-valorant" {
		t.Fatalf("order mismatch: %s, %s", games[0].GameId, games[1].GameId)
	}

	games, _, err = m.ListGames(1001, 2, 2)
	if err != nil || len(games) != 1 || games[0].GameId != "game-elden-ring" {
		t.Fatalf("page 2: %+v err=%v", games, err)
	}
}

// TestGetGameNotFound 未命中返回 ErrStatNotFound 哨兵。
func TestGetGameNotFound(t *testing.T) {
	m := newTestStatsModel(t)

	if _, err := m.GetGame(1001, "game-elden-ring"); !errors.Is(err, ErrStatNotFound) {
		t.Fatalf("missing stat err=%v, want ErrStatNotFound", err)
	}
}

// TestSeedIfEmpty 空表写入种子、非空表跳过。
func TestSeedIfEmpty(t *testing.T) {
	m := newTestStatsModel(t)

	if err := m.SeedIfEmpty(); err != nil {
		t.Fatalf("seed: %v", err)
	}
	_, total, err := m.ListGames(1001, 100, 0)
	if err != nil || total == 0 {
		t.Fatalf("seeded rows missing: total=%d err=%v", total, err)
	}

	// 再次 SeedIfEmpty 幂等（非空跳过）：总数不变
	_, again, err := m.ListGames(1001, 100, 0)
	if err != nil || again != total {
		t.Fatalf("seed not idempotent: %d -> %d err=%v", total, again, err)
	}
}
