package model

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"github.com/tappi/tappi/services/data-panel/internal/cache"
	"github.com/tappi/tappi/services/data-panel/internal/types"
)

// 缓存参数：进程内 TTL+LRU（与 community 模型层同款，见
// docs/development-guide.md「数据库集成 → 设计决策」）。
const (
	defaultStatCacheTTL = 60 * time.Second
	defaultStatCacheMax = 4096
)

// PlayerStatsModel 战绩数据访问层（MySQL/SQLite 双驱动，表结构见 CreateTable）。
// 摄入为单调增量累加（单语句 upsert col=col+delta，并发安全、永不回退）；
// GetGame 读进程内缓存（值副本隔离），写路径统一失效；汇总/列表走聚合查询不缓存。
// 单实例部署 + SQLite 多连接隔离：连接池钳制为 1。
type PlayerStatsModel struct {
	db    *sql.DB
	cache *cache.Cache[string, types.PlayerStat]
}

// PlayerStatsStore 战绩仓储接口：ServiceContext 面向接口依赖，测试可注入故障实现。
type PlayerStatsStore interface {
	Record(userId int64, req *types.RecordStatReq) (*types.PlayerStat, error)
	Summary(userId int64) (*types.StatSummary, error)
	ListGames(userId, limit, offset int64) ([]types.PlayerStat, int64, error)
	GetGame(userId int64, gameId string) (*types.PlayerStat, error)
	CreateTable() error
	SeedIfEmpty() error
}

var _ PlayerStatsStore = (*PlayerStatsModel)(nil)

// NewPlayerStatsModel 创建战绩模型（缓存默认开启；连接池钳制为 1）
func NewPlayerStatsModel(db *sql.DB) *PlayerStatsModel {
	db.SetMaxOpenConns(1)
	return &PlayerStatsModel{
		db:    db,
		cache: cache.New[string, types.PlayerStat](defaultStatCacheTTL, defaultStatCacheMax),
	}
}

const statColumns = "user_id, game_id, game_title, matches, wins, kills, deaths, assists, score, rank_points, last_played_at, created_at, updated_at"

// CreateTable 创建战绩表（开发使用）。先尝试 SQLite 方言，失败回落
// MySQL（与 user-service/community.CreateXxxTable 同款双格式策略）。
func (m *PlayerStatsModel) CreateTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS player_stats (
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
			updated_at VARCHAR(32) NOT NULL,
			UNIQUE (user_id, game_id)
		)
	`

	if _, err := m.db.Exec(query); err != nil {
		query = `
			CREATE TABLE IF NOT EXISTS player_stats (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				user_id BIGINT NOT NULL,
				game_id VARCHAR(64) NOT NULL,
				game_title VARCHAR(128) NOT NULL DEFAULT '',
				matches INT NOT NULL DEFAULT 0,
				wins INT NOT NULL DEFAULT 0,
				kills INT NOT NULL DEFAULT 0,
				deaths INT NOT NULL DEFAULT 0,
				assists INT NOT NULL DEFAULT 0,
				score INT NOT NULL DEFAULT 0,
				rank_points INT NOT NULL DEFAULT 0,
				last_played_at VARCHAR(32) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL,
				UNIQUE KEY uk_user_game (user_id, game_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		if _, err := m.db.Exec(query); err != nil {
			return fmt.Errorf("创建战绩表失败（尝试了 SQLite 和 MySQL 格式）: %w", err)
		}
	}

	return nil
}

// Seed 按给定行写入战绩（演示数据与测试装载），供 SeedIfEmpty 复用。
func (m *PlayerStatsModel) Seed(rows []*seedStatRow) error {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, row := range rows {
		if row == nil {
			continue
		}
		if _, err := m.db.Exec(
			`INSERT INTO player_stats (`+statColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			row.UserId, row.GameId, row.GameTitle,
			row.Matches, row.Wins, row.Kills, row.Deaths, row.Assists,
			row.Score, row.RankPoints, row.LastPlayedAt, now, now,
		); err != nil {
			return fmt.Errorf("写入战绩种子失败: %w", err)
		}
	}
	return nil
}

// seedStatRow 种子行（id 不外露，归属固定演示用户）。
type seedStatRow struct {
	UserId       int64
	GameId       string
	GameTitle    string
	Matches      int64
	Wins         int64
	Kills        int64
	Deaths       int64
	Assists      int64
	Score        int64
	RankPoints   int64
	LastPlayedAt string
}

// SeedIfEmpty 表为空时写入内嵌演示战绩（首次启动初始化面板演示数据）。
func (m *PlayerStatsModel) SeedIfEmpty() error {
	var count int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM player_stats`).Scan(&count); err != nil {
		return fmt.Errorf("统计战绩行数失败: %w", err)
	}
	if count > 0 {
		return nil
	}
	return m.Seed(defaultSeedStats())
}

func defaultSeedStats() []*seedStatRow {
	raw := []byte(defaultStatSeed)
	var rows []*seedStatRow
	if err := json.Unmarshal(raw, &rows); err != nil {
		panic(fmt.Sprintf("invalid embedded stat seed: %v", err))
	}
	return rows
}

const defaultStatSeed = `[
	{
		"UserId": 1001, "GameId": "game-elden-ring", "GameTitle": "Elden Ring",
		"Matches": 42, "Wins": 25, "Kills": 310, "Deaths": 180, "Assists": 95,
		"Score": 12040, "RankPoints": 860, "LastPlayedAt": "2026-10-01T08:00:00Z"
	},
	{
		"UserId": 1001, "GameId": "game-valorant", "GameTitle": "Valorant",
		"Matches": 87, "Wins": 44, "Kills": 620, "Deaths": 510, "Assists": 240,
		"Score": 18900, "RankPoints": 1120, "LastPlayedAt": "2026-10-05T21:30:00Z"
	},
	{
		"UserId": 1002, "GameId": "game-apex", "GameTitle": "Apex Legends",
		"Matches": 66, "Wins": 9, "Kills": 340, "Deaths": 572, "Assists": 130,
		"Score": 9800, "RankPoints": 480, "LastPlayedAt": "2026-10-06T13:05:00Z"
	}
]`

// statKey 缓存键：按 (user_id, game_id) 定位。
func statKey(userId int64, gameId string) string {
	return fmt.Sprintf("user:%d:game:%s", userId, gameId)
}

// Record 单调增量摄入：单语句 upsert，col = col + delta；
// game_title 非空时覆盖、空值保留存量；last_played_at 取最大值推进
// （RFC3339 同格式字符串字典序与时间序一致）。返回更新后的完整累积行。
func (m *PlayerStatsModel) Record(userId int64, req *types.RecordStatReq) (*types.PlayerStat, error) {
	playedAt := req.PlayedAt
	if playedAt == "" {
		playedAt = time.Now().UTC().Format(time.RFC3339)
	}
	now := time.Now().UTC().Format(time.RFC3339)

	// SQLite 方言（ON CONFLICT）；失败回落 MySQL（ON DUPLICATE KEY），
	// 与建表的双格式策略同款 try-then-fallback。
	query := `
		INSERT INTO player_stats (` + statColumns + `)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(user_id, game_id) DO UPDATE SET
			game_title = CASE WHEN excluded.game_title != '' THEN excluded.game_title ELSE player_stats.game_title END,
			matches = matches + excluded.matches,
			wins = wins + excluded.wins,
			kills = kills + excluded.kills,
			deaths = deaths + excluded.deaths,
			assists = assists + excluded.assists,
			score = score + excluded.score,
			rank_points = rank_points + excluded.rank_points,
			last_played_at = CASE WHEN excluded.last_played_at > player_stats.last_played_at THEN excluded.last_played_at ELSE player_stats.last_played_at END,
			updated_at = excluded.updated_at
	`
	args := []any{
		userId, req.GameId, req.GameTitle,
		req.Matches, req.Wins, req.Kills, req.Deaths, req.Assists,
		req.Score, req.RankPoints, playedAt, now, now,
	}

	if _, err := m.db.Exec(query, args...); err != nil {
		query = `
			INSERT INTO player_stats (` + statColumns + `)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON DUPLICATE KEY UPDATE
				game_title = IF(VALUES(game_title) != '', VALUES(game_title), game_title),
				matches = matches + VALUES(matches),
				wins = wins + VALUES(wins),
				kills = kills + VALUES(kills),
				deaths = deaths + VALUES(deaths),
				assists = assists + VALUES(assists),
				score = score + VALUES(score),
				rank_points = rank_points + VALUES(rank_points),
				last_played_at = IF(VALUES(last_played_at) > last_played_at, VALUES(last_played_at), last_played_at),
				updated_at = VALUES(updated_at)
		`
		if _, err := m.db.Exec(query, args...); err != nil {
			return nil, fmt.Errorf("写入战绩失败: %w", err)
		}
	}

	// 写后失效缓存键，回读给调用方完整累积视图
	m.cache.Delete(statKey(userId, req.GameId))
	return m.GetGame(userId, req.GameId)
}

// Summary 跨游戏聚合（单条聚合查询下推 SQL；无战绩返回全零汇总而非 404）。
func (m *PlayerStatsModel) Summary(userId int64) (*types.StatSummary, error) {
	var (
		totalMatches, totalWins, totalKills, totalDeaths, totalAssists, totalScore, totalRank, gameCount sql.NullInt64
		lastPlayed                                                                                       sql.NullString
	)
	err := m.db.QueryRow(
		`SELECT COUNT(*), SUM(matches), SUM(wins), SUM(kills), SUM(deaths), SUM(assists),
			SUM(score), SUM(rank_points), MAX(last_played_at)
		FROM player_stats WHERE user_id = ?`, userId,
	).Scan(&gameCount, &totalMatches, &totalWins, &totalKills, &totalDeaths, &totalAssists,
		&totalScore, &totalRank, &lastPlayed)
	if err != nil {
		return nil, fmt.Errorf("聚合战绩失败: %w", err)
	}

	return &types.StatSummary{
		UserId:          userId,
		TotalMatches:    totalMatches.Int64,
		TotalWins:       totalWins.Int64,
		WinRate:         ratio(totalWins.Int64, totalMatches.Int64),
		TotalKills:      totalKills.Int64,
		TotalDeaths:     totalDeaths.Int64,
		TotalAssists:    totalAssists.Int64,
		Kd:              ratio(totalKills.Int64, totalDeaths.Int64),
		TotalScore:      totalScore.Int64,
		TotalRankPoints: totalRank.Int64,
		GameCount:       gameCount.Int64,
		LastPlayedAt:    lastPlayed.String,
	}, nil
}

// ListGames 按用户列战绩明细：场次降序、game_id 升序破并列，分页在 SQL 下推。
func (m *PlayerStatsModel) ListGames(userId, limit, offset int64) ([]types.PlayerStat, int64, error) {
	var total int64
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM player_stats WHERE user_id = ?`, userId).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计战绩游戏数失败: %w", err)
	}

	rows, err := m.db.Query(
		`SELECT game_id, game_title, matches, wins, kills, deaths, assists, score, rank_points, last_played_at, created_at, updated_at
		FROM player_stats WHERE user_id = ? ORDER BY matches DESC, game_id ASC LIMIT ? OFFSET ?`,
		userId, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("查询战绩列表失败: %w", err)
	}
	defer rows.Close()

	games := make([]types.PlayerStat, 0)
	for rows.Next() {
		stat, scanErr := scanStat(rows.Scan)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		games = append(games, *stat)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历战绩行失败: %w", err)
	}
	return games, total, nil
}

// GetGame 按 (user_id, game_id) 查单条；读缓存（值副本），未命中 ErrStatNotFound。
func (m *PlayerStatsModel) GetGame(userId int64, gameId string) (*types.PlayerStat, error) {
	key := statKey(userId, gameId)
	if cached, ok := m.cache.Get(key); ok {
		stat := cached // 值类型副本：调用方改动不回灌缓存
		return &stat, nil
	}

	row := m.db.QueryRow(
		`SELECT game_id, game_title, matches, wins, kills, deaths, assists, score, rank_points, last_played_at, created_at, updated_at
		FROM player_stats WHERE user_id = ? AND game_id = ?`,
		userId, gameId,
	)
	stat, err := scanStat(row.Scan)
	if err == sql.ErrNoRows {
		return nil, ErrStatNotFound
	}
	if err != nil {
		return nil, err
	}

	m.cache.Set(key, *stat)
	return stat, nil
}

// scanStat 行映射统一入口（win_rate/kd 读侧计算，4 位小数）。
func scanStat(scan func(dest ...any) error) (*types.PlayerStat, error) {
	var s types.PlayerStat
	if err := scan(&s.GameId, &s.GameTitle, &s.Matches, &s.Wins, &s.Kills, &s.Deaths,
		&s.Assists, &s.Score, &s.RankPoints, &s.LastPlayedAt, &s.CreatedAt, &s.UpdatedAt); err != nil {
		return nil, err
	}
	s.WinRate = ratio(s.Wins, s.Matches)
	s.Kd = ratio(s.Kills, s.Deaths)
	return &s, nil
}

// ratio 计算比率（分母为零返回 0），4 位小数截断。
func ratio(numerator, denominator int64) float64 {
	if denominator <= 0 {
		return 0
	}
	return math.Round(float64(numerator)/float64(denominator)*10000) / 10000
}
