package model

// GP-306 业务通知投递 outbox（本地持久化补偿）：活动消息向预约人群扇出时
// community 短时不可达，先落本表（at-least-once），由定时 Relay 重投。
// 重投安全性由 community 侧 notifications 表 UNIQUE(user_id, actor_id,
// type, target_id) 保证——同一批次重放对已落库用户被 INSERT OR IGNORE 静默
// 忽略，只补投缺失用户，不产生重复站内信。outbox 只做交付队列，不参与
// 活动发布主流程。（与 game-catalog/notifyoutboxmodel.go 同款形态。）

import (
	"database/sql"
	"fmt"
)

// 交付状态：pending 待投 / delivered 已投 / dropped 重试耗尽丢弃（留痕
// 可审计，不永久占队）。
const (
	NotifyOutboxPending   = "pending"
	NotifyOutboxDelivered = "delivered"
	NotifyOutboxDropped   = "dropped"
)

// NotifyOutboxRow 单条待投/已投业务通知（payload 为 Notification 原样 JSON，
// Relay 逐条重投）。
type NotifyOutboxRow struct {
	Id          int64  `json:"id"`
	NotifyKey   string `json:"notify_key"`
	NotifyType  string `json:"notify_type"`
	Payload     string `json:"payload"`
	Status      string `json:"status"`
	Attempts    int    `json:"attempts"`
	LastError   string `json:"last_error"`
	NextRetryAt string `json:"next_retry_at"`
	DeliveredAt string `json:"delivered_at"`
	CreatedAt   string `json:"created_at"`
}

const notifyOutboxColumns = "id, notify_key, notify_type, payload, status, attempts, last_error, next_retry_at, delivered_at, created_at"

func scanNotifyOutbox(r *NotifyOutboxRow) []any {
	return []any{&r.Id, &r.NotifyKey, &r.NotifyType, &r.Payload, &r.Status, &r.Attempts,
		&r.LastError, &r.NextRetryAt, &r.DeliveredAt, &r.CreatedAt}
}

// NotifyOutboxModel 业务通知 outbox 数据访问。
type NotifyOutboxModel struct {
	db *sql.DB
}

func NewNotifyOutboxModel(db *sql.DB) *NotifyOutboxModel {
	return &NotifyOutboxModel{db: db}
}

func (m *NotifyOutboxModel) CreateTables() error {
	_, err := m.db.Exec(`CREATE TABLE IF NOT EXISTS notify_outbox (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		notify_key VARCHAR(64) NOT NULL UNIQUE,
		notify_type VARCHAR(32) NOT NULL,
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
	_, err2 := m.db.Exec(`CREATE TABLE IF NOT EXISTS notify_outbox (
		id BIGINT PRIMARY KEY AUTO_INCREMENT,
		notify_key VARCHAR(64) NOT NULL UNIQUE,
		notify_type VARCHAR(32) NOT NULL,
		payload TEXT NOT NULL,
		status VARCHAR(16) NOT NULL DEFAULT 'pending',
		attempts INT NOT NULL DEFAULT 0,
		last_error VARCHAR(512) NOT NULL DEFAULT '',
		next_retry_at VARCHAR(40) NOT NULL DEFAULT '',
		delivered_at VARCHAR(32) NOT NULL DEFAULT '',
		created_at VARCHAR(32) NOT NULL,
		KEY idx_notify_outbox_retry (status, next_retry_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err2 != nil {
		return fmt.Errorf("创建业务通知 outbox 表失败（尝试了 SQLite 和 MySQL 格式）: %v", err2)
	}
	return nil
}

// Enqueue 通知入队（幂等：notify_key 唯一，重复入队忽略，不重置既有的
// 失败退避/尝试计数）。nextRetryAt 为同步首投失败后最早可重投时间。
func (m *NotifyOutboxModel) Enqueue(notifyKey, notifyType, payload, nextRetryAt string) error {
	now := nowUTC()
	_, err := m.db.Exec(
		`INSERT OR IGNORE INTO notify_outbox (notify_key, notify_type, payload, status, attempts, last_error, next_retry_at, delivered_at, created_at)
		 VALUES (?, ?, ?, 'pending', 0, '', ?, '', ?)`,
		notifyKey, notifyType, payload, nextRetryAt, now,
	)
	if err != nil {
		if _, err2 := m.db.Exec(
			`INSERT IGNORE INTO notify_outbox (notify_key, notify_type, payload, status, attempts, last_error, next_retry_at, delivered_at, created_at)
			 VALUES (?, ?, ?, 'pending', 0, '', ?, '', ?)`,
			notifyKey, notifyType, payload, nextRetryAt, now,
		); err2 != nil {
			return err2
		}
	}
	return nil
}

// ListRetry 到期待投行（pending 且 next_retry_at <= now，按 id 升序，有界）。
func (m *NotifyOutboxModel) ListRetry(now string, limit int) ([]*NotifyOutboxRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := fmt.Sprintf(`SELECT %s FROM notify_outbox WHERE status = '%s' AND next_retry_at <= ? ORDER BY id ASC LIMIT %d`,
		notifyOutboxColumns, NotifyOutboxPending, limit)
	rows, err := m.db.Query(q, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*NotifyOutboxRow, 0, limit)
	for rows.Next() {
		var r NotifyOutboxRow
		if err := rows.Scan(scanNotifyOutbox(&r)...); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// MarkDelivered 投递成功（幂等：仅 pending → delivered 转移）。
func (m *NotifyOutboxModel) MarkDelivered(notifyKey, at string) error {
	_, err := m.db.Exec(
		`UPDATE notify_outbox SET status = ?, delivered_at = ? WHERE notify_key = ? AND status = ?`,
		NotifyOutboxDelivered, at, notifyKey, NotifyOutboxPending,
	)
	return err
}

// MarkFailed 记一次重试失败：attempts 覆盖，按调用方给的退避时间定下次
// 重投（Relay 按 attempts 指数退避）。
func (m *NotifyOutboxModel) MarkFailed(notifyKey string, attempts int, lastErr, nextRetryAt string) error {
	_, err := m.db.Exec(
		`UPDATE notify_outbox SET attempts = ?, last_error = ?, next_retry_at = ? WHERE notify_key = ? AND status = ?`,
		attempts, lastErr, nextRetryAt, notifyKey, NotifyOutboxPending,
	)
	return err
}

// MarkDropped 重试耗尽：置 dropped 留痕（不再重投，审计可查）。
func (m *NotifyOutboxModel) MarkDropped(notifyKey string, attempts int, lastErr string) error {
	_, err := m.db.Exec(
		`UPDATE notify_outbox SET status = ?, attempts = ?, last_error = ? WHERE notify_key = ?`,
		NotifyOutboxDropped, attempts, lastErr, notifyKey,
	)
	return err
}

// Prune 清理已终态行：delivered 按 delivered_at，dropped 按 created_at
// （两者都早于 before 删除），返回删除行数。
func (m *NotifyOutboxModel) Prune(before string) (int64, error) {
	res, err := m.db.Exec(
		`DELETE FROM notify_outbox WHERE (status = ? AND delivered_at <> '' AND delivered_at < ?) OR (status = ? AND created_at < ?)`,
		NotifyOutboxDelivered, before, NotifyOutboxDropped, before,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
