package model

// GP-303 回流判定源 outbox（本地持久化补偿）：游戏会话签发成功后向
// promotions 上报账号×游戏活跃事实。上报先落本表再由 Relay 定时重投——
// 活跃事实是回流人群的唯一判定源，promotions 短时不可达若即丢，玩家的
// previous_active 会停在旧时刻，评估时缺口被高估从而**误判为回流**（错发
// 方向），故按 outbox 口径持久化（与渠道归因 outbox 同款）。
//
// 重投安全性：promotions 侧活跃写入为条件推进（仅 incoming > last 才推进，
// 乱序/重复重放 no-op），同一条事实重复上报不改变判定结果。

import (
	"database/sql"
	"fmt"
	"time"
)

// 交付状态：pending 待上报 / delivered 已上报 / dropped 重试耗尽丢弃（留痕
// 可审计，不永久占队）。
const (
	ActivityOutboxPending   = "pending"
	ActivityOutboxDelivered = "delivered"
	ActivityOutboxDropped   = "dropped"
)

// ActivityOutboxRow 单条活跃上报意图（payload 为上报请求体 JSON）。
type ActivityOutboxRow struct {
	Id          int64  `json:"id"`
	ActKey      string `json:"act_key"`
	AccountId   string `json:"account_id"`
	GameId      string `json:"game_id"`
	ActiveAt    string `json:"active_at"`
	Payload     string `json:"payload"`
	Status      string `json:"status"`
	Attempts    int    `json:"attempts"`
	LastError   string `json:"last_error"`
	NextRetryAt string `json:"next_retry_at"`
	DeliveredAt string `json:"delivered_at"`
	CreatedAt   string `json:"created_at"`
}

const activityOutboxColumns = "id, act_key, account_id, game_id, active_at, payload, status, attempts, last_error, next_retry_at, delivered_at, created_at"

func scanActivityOutbox(r *ActivityOutboxRow) []any {
	return []any{&r.Id, &r.ActKey, &r.AccountId, &r.GameId, &r.ActiveAt, &r.Payload,
		&r.Status, &r.Attempts, &r.LastError, &r.NextRetryAt, &r.DeliveredAt, &r.CreatedAt}
}

// ActivityOutboxModel 活跃上报 outbox 数据访问。
type ActivityOutboxModel struct {
	db *sql.DB
}

func NewActivityOutboxModel(db *sql.DB) *ActivityOutboxModel {
	return &ActivityOutboxModel{db: db}
}

func (m *ActivityOutboxModel) CreateTables() error {
	_, err := m.db.Exec(`CREATE TABLE IF NOT EXISTS player_activity_outbox (
		id            INTEGER PRIMARY KEY AUTOINCREMENT,
		act_key       VARCHAR(160) NOT NULL UNIQUE,
		account_id    VARCHAR(64) NOT NULL DEFAULT '',
		game_id       VARCHAR(64) NOT NULL DEFAULT '',
		active_at     VARCHAR(40) NOT NULL DEFAULT '',
		payload       TEXT NOT NULL,
		status        VARCHAR(16) NOT NULL DEFAULT 'pending',
		attempts      INTEGER NOT NULL DEFAULT 0,
		last_error    VARCHAR(512) NOT NULL DEFAULT '',
		next_retry_at VARCHAR(40) NOT NULL DEFAULT '',
		delivered_at  VARCHAR(40) NOT NULL DEFAULT '',
		created_at    VARCHAR(40) NOT NULL
	)`)
	if err == nil {
		return nil
	}
	_, err2 := m.db.Exec(`CREATE TABLE IF NOT EXISTS player_activity_outbox (
		id            BIGINT PRIMARY KEY AUTO_INCREMENT,
		act_key       VARCHAR(160) NOT NULL UNIQUE,
		account_id    VARCHAR(64) NOT NULL DEFAULT '',
		game_id       VARCHAR(64) NOT NULL DEFAULT '',
		active_at     VARCHAR(40) NOT NULL DEFAULT '',
		payload       TEXT NOT NULL,
		status        VARCHAR(16) NOT NULL DEFAULT 'pending',
		attempts      INT NOT NULL DEFAULT 0,
		last_error    VARCHAR(512) NOT NULL DEFAULT '',
		next_retry_at VARCHAR(40) NOT NULL DEFAULT '',
		delivered_at  VARCHAR(40) NOT NULL DEFAULT '',
		created_at    VARCHAR(40) NOT NULL,
		KEY idx_activity_outbox_retry (status, next_retry_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err2 != nil {
		return fmt.Errorf("创建活跃上报 outbox 表失败（尝试了 SQLite 和 MySQL 格式）: %v", err2)
	}
	return nil
}

// Enqueue 活跃上报意图入队（幂等：act_key 唯一，重复入队忽略，不重置既有
// 的失败退避/尝试计数）。nextRetryAt 为同步首投失败后最早可重投时间。
func (m *ActivityOutboxModel) Enqueue(actKey, accountID, gameID, activeAt, payload, nextRetryAt string) error {
	now := nowRFC3339()
	_, err := m.db.Exec(
		`INSERT OR IGNORE INTO player_activity_outbox (act_key, account_id, game_id, active_at, payload, status, attempts, last_error, next_retry_at, delivered_at, created_at)
		 VALUES (?, ?, ?, ?, ?, 'pending', 0, '', ?, '', ?)`,
		actKey, accountID, gameID, activeAt, payload, nextRetryAt, now,
	)
	if err != nil {
		if _, err2 := m.db.Exec(
			`INSERT IGNORE INTO player_activity_outbox (act_key, account_id, game_id, active_at, payload, status, attempts, last_error, next_retry_at, delivered_at, created_at)
			 VALUES (?, ?, ?, ?, ?, 'pending', 0, '', ?, '', ?)`,
			actKey, accountID, gameID, activeAt, payload, nextRetryAt, now,
		); err2 != nil {
			return err2
		}
	}
	return nil
}

// ListRetry 到期待上报行（pending 且 next_retry_at <= now，按 id 升序，有界）。
func (m *ActivityOutboxModel) ListRetry(now string, limit int) ([]*ActivityOutboxRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := fmt.Sprintf(`SELECT %s FROM player_activity_outbox WHERE status = '%s' AND next_retry_at <= ? ORDER BY id ASC LIMIT %d`,
		activityOutboxColumns, ActivityOutboxPending, limit)
	rows, err := m.db.Query(q, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*ActivityOutboxRow, 0, limit)
	for rows.Next() {
		var r ActivityOutboxRow
		if err := rows.Scan(scanActivityOutbox(&r)...); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// MarkDelivered 上报受理（幂等：仅 pending → delivered 转移）。
func (m *ActivityOutboxModel) MarkDelivered(actKey, at string) error {
	_, err := m.db.Exec(
		`UPDATE player_activity_outbox SET status = ?, delivered_at = ? WHERE act_key = ? AND status = ?`,
		ActivityOutboxDelivered, at, actKey, ActivityOutboxPending,
	)
	return err
}

// MarkFailed 记一次重投失败：attempts 覆盖，按调用方给的退避时间定下次重投。
func (m *ActivityOutboxModel) MarkFailed(actKey string, attempts int, lastErr, nextRetryAt string) error {
	_, err := m.db.Exec(
		`UPDATE player_activity_outbox SET attempts = ?, last_error = ?, next_retry_at = ? WHERE act_key = ? AND status = ?`,
		attempts, lastErr, nextRetryAt, actKey, ActivityOutboxPending,
	)
	return err
}

// MarkDropped 重试耗尽：置 dropped 留痕（不再重投，审计可查）。
func (m *ActivityOutboxModel) MarkDropped(actKey string, attempts int, lastErr string) error {
	_, err := m.db.Exec(
		`UPDATE player_activity_outbox SET status = ?, attempts = ?, last_error = ? WHERE act_key = ?`,
		ActivityOutboxDropped, attempts, lastErr, actKey,
	)
	return err
}

// Prune 清理已终态行：delivered 按 delivered_at，dropped 按 created_at
// （两者都早于 before 删除），返回删除行数。
func (m *ActivityOutboxModel) Prune(before string) (int64, error) {
	res, err := m.db.Exec(
		`DELETE FROM player_activity_outbox WHERE (status = ? AND delivered_at <> '' AND delivered_at < ?) OR (status = ? AND created_at < ?)`,
		ActivityOutboxDelivered, before, ActivityOutboxDropped, before,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func nowRFC3339() string {
	return time.Now().UTC().Format(time.RFC3339Nano)
}
