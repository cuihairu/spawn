package model

// GP-303 回流人群判定源：账号×游戏活跃记录（登录行为事实）。
//
// 事实由 courier-adapter 在游戏会话签发成功后经 S2S POST /internal/player-activity
// 上报（outbox at-least-once 重投）；本表保存最近一次活跃与上一次活跃。
// 评估回流活动时按「上一次活跃时刻」（previous_active_at）与活动配置的
// 缺席阈值判定：玩家曾活跃、且距本次回流前已缺席 >= 阈值天数，即为回流。
// 无任何历史记录（previous_active_at 为空）= 首见，不是回流（fail-closed）。
//
// 口径（缺席天数阈值）为运营按活动配置 audience.returning_threshold_days，
// 机制层不写死业务数值（默认 30，见 logic.rewardrules 的
// defaultReturningThresholdDays）。
//
// 时间一律规范为 UTC RFC3339Nano 存储（定长同格式，字符串比较即时间比较，
// 与 "+08:00" 偏移混存不同——跨时区偏移必须解析成时刻再比）。

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// ErrPlayerActivityNotFound 活跃记录不存在（无任何历史）。
var ErrPlayerActivityNotFound = errors.New("player activity not found")

// PlayerActivity 账号×游戏活跃行。
type PlayerActivity struct {
	AccountId        string `json:"account_id"`
	GameId           string `json:"game_id"`
	LastActiveAt     string `json:"last_active_at"`
	PreviousActiveAt string `json:"previous_active_at"`
	UpdatedAt        string `json:"updated_at"`
}

// PlayerActivityModel 活跃记录数据访问。
type PlayerActivityModel struct {
	db *sql.DB
}

func NewPlayerActivityModel(db *sql.DB) *PlayerActivityModel {
	return &PlayerActivityModel{db: db}
}

// CreateTables 建活跃表（SQLite 先行、失败回 MySQL，与 GP-301 同款）。
func (m *PlayerActivityModel) CreateTables() error {
	_, err := m.db.Exec(`CREATE TABLE IF NOT EXISTS activity_player_activity (
		account_id        VARCHAR(64) NOT NULL,
		game_id           VARCHAR(64) NOT NULL,
		last_active_at    VARCHAR(40) NOT NULL DEFAULT '',
		previous_active_at VARCHAR(40) NOT NULL DEFAULT '',
		updated_at        VARCHAR(40) NOT NULL DEFAULT '',
		PRIMARY KEY (account_id, game_id)
	)`)
	if err == nil {
		return nil
	}
	_, err2 := m.db.Exec(`CREATE TABLE IF NOT EXISTS activity_player_activity (
		account_id        VARCHAR(64) NOT NULL,
		game_id           VARCHAR(64) NOT NULL,
		last_active_at    VARCHAR(40) NOT NULL DEFAULT '',
		previous_active_at VARCHAR(40) NOT NULL DEFAULT '',
		updated_at        VARCHAR(40) NOT NULL DEFAULT '',
		PRIMARY KEY (account_id, game_id)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err2 != nil {
		return fmt.Errorf("创建玩家活跃表失败（尝试了 SQLite 和 MySQL 格式）: %v", err2)
	}
	return nil
}

// RecordActivity 记录一次活跃事实（S2S 上报）。activeAt 必须为规范 UTC
// RFC3339Nano（logic 层已规范化）。仅在 activeAt > 当前 last_active_at 时
// 推进（previous = 旧 last）；乱序/重复重放为 no-op——防 outbox at-least-once
// 重投把 last 回退或把 previous 覆盖坏。首见插入 previous 为空（无历史）。
func (m *PlayerActivityModel) RecordActivity(accountID, gameID, activeAt string) error {
	now := nowUTCnano()
	// SQLite：单语句条件推进，原子（单连接串行化 + 条件表达式防乱序重放）。
	_, err := m.db.Exec(
		`INSERT INTO activity_player_activity (account_id, game_id, last_active_at, previous_active_at, updated_at)
		 VALUES (?, ?, ?, '', ?)
		 ON CONFLICT(account_id, game_id) DO UPDATE SET
		   previous_active_at = CASE WHEN excluded.last_active_at > last_active_at THEN last_active_at ELSE previous_active_at END,
		   last_active_at     = CASE WHEN excluded.last_active_at > last_active_at THEN excluded.last_active_at ELSE last_active_at END,
		   updated_at         = CASE WHEN excluded.last_active_at > last_active_at THEN excluded.updated_at ELSE updated_at END`,
		accountID, gameID, activeAt, now,
	)
	if err == nil {
		return nil
	}
	// MySQL：ON DUPLICATE KEY UPDATE 同款条件推进。
	_, err2 := m.db.Exec(
		`INSERT INTO activity_player_activity (account_id, game_id, last_active_at, previous_active_at, updated_at)
		 VALUES (?, ?, ?, '', ?)
		 ON DUPLICATE KEY UPDATE
		   previous_active_at = IF(VALUES(last_active_at) > last_active_at, last_active_at, previous_active_at),
		   last_active_at     = IF(VALUES(last_active_at) > last_active_at, VALUES(last_active_at), last_active_at),
		   updated_at         = IF(VALUES(last_active_at) > last_active_at, VALUES(updated_at), updated_at)`,
		accountID, gameID, activeAt, now,
	)
	if err2 != nil {
		return fmt.Errorf("记录活跃事实失败: %v", err2)
	}
	return nil
}

// GetPlayerActivity 读一行；不存在返回 ErrPlayerActivityNotFound。
func (m *PlayerActivityModel) GetPlayerActivity(accountID, gameID string) (*PlayerActivity, error) {
	var a PlayerActivity
	err := m.db.QueryRow(
		`SELECT account_id, game_id, last_active_at, previous_active_at, updated_at
		 FROM activity_player_activity WHERE account_id = ? AND game_id = ?`,
		accountID, gameID,
	).Scan(&a.AccountId, &a.GameId, &a.LastActiveAt, &a.PreviousActiveAt, &a.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrPlayerActivityNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// nowUTCnano 规范当前时刻：UTC RFC3339Nano（定长同格式可字符串比较）。
func nowUTCnano() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
