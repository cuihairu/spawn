package model

// GP-402 经营分析事实投递 outbox（本地持久化补偿）：data-panel 短时不可达
// 时，order_paid / refund_requested 事实事件先落本表（at-least-once），
// 由定时 Relay 重投；data-panel 侧 platform_events UNIQUE(event_id) 摄入
// 幂等，重投不重复计事实。事件事实由 payments 侧拼装（客户端不可上报
// 金额），outbox 只做交付队列，不参与支付主流程。

import (
	"database/sql"
	"fmt"
)

// 交付状态：pending 待投 / delivered 已投 / dropped 重试耗尽丢弃（留痕
// 可审计，不永久占队）。
const (
	AnalyticsOutboxPending   = "pending"
	AnalyticsOutboxDelivered = "delivered"
	AnalyticsOutboxDropped   = "dropped"
)

// AnalyticsOutboxRow 单条待投/已投事实事件（payload 为
// {"events":[Event]} 原样 JSON，Relay 逐条重投）。
type AnalyticsOutboxRow struct {
	Id          int64  `json:"id"`
	EventId     string `json:"event_id"`
	EventType   string `json:"event_type"`
	Payload     string `json:"payload"`
	Status      string `json:"status"`
	Attempts    int    `json:"attempts"`
	LastError   string `json:"last_error"`
	NextRetryAt string `json:"next_retry_at"`
	DeliveredAt string `json:"delivered_at"`
	CreatedAt   string `json:"created_at"`
}

const analyticsOutboxColumns = "id, event_id, event_type, payload, status, attempts, last_error, next_retry_at, delivered_at, created_at"

func scanAnalyticsOutbox(r *AnalyticsOutboxRow) []any {
	return []any{&r.Id, &r.EventId, &r.EventType, &r.Payload, &r.Status, &r.Attempts,
		&r.LastError, &r.NextRetryAt, &r.DeliveredAt, &r.CreatedAt}
}

// AnalyticsOutboxModel 事实事件 outbox 数据访问。
type AnalyticsOutboxModel struct {
	db *sql.DB
}

func NewAnalyticsOutboxModel(db *sql.DB) *AnalyticsOutboxModel {
	return &AnalyticsOutboxModel{db: db}
}

func (m *AnalyticsOutboxModel) CreateTables() error {
	_, err := m.db.Exec(`CREATE TABLE IF NOT EXISTS analytics_outbox (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		event_id VARCHAR(64) NOT NULL UNIQUE,
		event_type VARCHAR(32) NOT NULL,
		payload TEXT NOT NULL,
		status VARCHAR(16) NOT NULL DEFAULT 'pending',
		attempts INTEGER NOT NULL DEFAULT 0,
		last_error VARCHAR(512) NOT NULL DEFAULT '',
		next_retry_at VARCHAR(40) NOT NULL DEFAULT '',
		delivered_at VARCHAR(32) NOT NULL DEFAULT '',
		created_at VARCHAR(32) NOT NULL
	)`)
	if err == nil {
		return nil
	}
	_, err2 := m.db.Exec(`CREATE TABLE IF NOT EXISTS analytics_outbox (
		id BIGINT PRIMARY KEY AUTO_INCREMENT,
		event_id VARCHAR(64) NOT NULL UNIQUE,
		event_type VARCHAR(32) NOT NULL,
		payload TEXT NOT NULL,
		status VARCHAR(16) NOT NULL DEFAULT 'pending',
		attempts INT NOT NULL DEFAULT 0,
		last_error VARCHAR(512) NOT NULL DEFAULT '',
		next_retry_at VARCHAR(40) NOT NULL DEFAULT '',
		delivered_at VARCHAR(32) NOT NULL DEFAULT '',
		created_at VARCHAR(32) NOT NULL,
		KEY idx_analytics_outbox_retry (status, next_retry_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err2 != nil {
		return fmt.Errorf("创建经营分析 outbox 表失败（尝试了 SQLite 和 MySQL 格式）: %v", err2)
	}
	return nil
}

// Enqueue 事实事件入队（幂等：event_id 唯一，重复入队忽略，不重置既有的
// 失败退避/尝试计数）。nextRetryAt 为同步首投失败后最早可重投时间。
func (m *AnalyticsOutboxModel) Enqueue(eventId, eventType, payload, nextRetryAt string) error {
	now := nowUTC()
	_, err := m.db.Exec(
		`INSERT OR IGNORE INTO analytics_outbox (event_id, event_type, payload, status, attempts, last_error, next_retry_at, delivered_at, created_at)
		 VALUES (?, ?, ?, 'pending', 0, '', ?, '', ?)`,
		eventId, eventType, payload, nextRetryAt, now,
	)
	if err != nil {
		if _, err2 := m.db.Exec(
			`INSERT IGNORE INTO analytics_outbox (event_id, event_type, payload, status, attempts, last_error, next_retry_at, delivered_at, created_at)
			 VALUES (?, ?, ?, 'pending', 0, '', ?, '', ?)`,
			eventId, eventType, payload, nextRetryAt, now,
		); err2 != nil {
			return err2
		}
	}
	return nil
}

// ListRetry 到期待投行（pending 且 next_retry_at <= now，按 id 升序，有界）。
func (m *AnalyticsOutboxModel) ListRetry(now string, limit int) ([]*AnalyticsOutboxRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := fmt.Sprintf(`SELECT %s FROM analytics_outbox WHERE status = '%s' AND next_retry_at <= ? ORDER BY id ASC LIMIT %d`,
		analyticsOutboxColumns, AnalyticsOutboxPending, limit)
	rows, err := m.db.Query(q, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*AnalyticsOutboxRow, 0, limit)
	for rows.Next() {
		var r AnalyticsOutboxRow
		if err := rows.Scan(scanAnalyticsOutbox(&r)...); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// MarkDelivered 投递成功（幂等：仅 pending → delivered 转移）。
func (m *AnalyticsOutboxModel) MarkDelivered(eventId, at string) error {
	_, err := m.db.Exec(
		`UPDATE analytics_outbox SET status = ?, delivered_at = ? WHERE event_id = ? AND status = ?`,
		AnalyticsOutboxDelivered, at, eventId, AnalyticsOutboxPending,
	)
	return err
}

// MarkFailed 记一次重试失败：attempts 覆盖，按调用方给的退避时间定下次
// 重投（Relay 按 attempts 指数退避）。
func (m *AnalyticsOutboxModel) MarkFailed(eventId string, attempts int, lastErr, nextRetryAt string) error {
	_, err := m.db.Exec(
		`UPDATE analytics_outbox SET attempts = ?, last_error = ?, next_retry_at = ? WHERE event_id = ? AND status = ?`,
		attempts, lastErr, nextRetryAt, eventId, AnalyticsOutboxPending,
	)
	return err
}

// MarkDropped 重试耗尽：置 dropped 留痕（不再重投，审计可查）。
func (m *AnalyticsOutboxModel) MarkDropped(eventId string, attempts int, lastErr string) error {
	_, err := m.db.Exec(
		`UPDATE analytics_outbox SET status = ?, attempts = ?, last_error = ? WHERE event_id = ?`,
		AnalyticsOutboxDropped, attempts, lastErr, eventId,
	)
	return err
}

// Prune 清理已终态行：delivered 按 delivered_at，dropped 按 created_at
// （两者都早于 before 删除），返回删除行数。
func (m *AnalyticsOutboxModel) Prune(before string) (int64, error) {
	res, err := m.db.Exec(
		`DELETE FROM analytics_outbox WHERE (status = ? AND delivered_at <> '' AND delivered_at < ?) OR (status = ? AND created_at < ?)`,
		AnalyticsOutboxDelivered, before, AnalyticsOutboxDropped, before,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
