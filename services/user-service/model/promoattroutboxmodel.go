package model

// GP-403 渠道归因 outbox（本地持久化补偿）：注册携带 click_id 时向 game-catalog
// 解析渠道归因。此前解析为同步 HTTP，game-catalog 短时不可达即丢归因（注册
// 不受影响，但渠道永远拿不回这条注册）。归因意图先落本表，解析失败留
// pending 由定时 Relay 重投；game-catalog 侧归因判定按请求携带的
// registered_at 计算、回写 user_id 幂等（一点击一归因），重投不会重复或错认。

import (
	"database/sql"
	"fmt"
)

// 交付状态：pending 待解析 / delivered 已解析 / dropped 重试耗尽丢弃（留痕
// 可审计，不永久占队）。
const (
	PromoAttrOutboxPending   = "pending"
	PromoAttrOutboxDelivered = "delivered"
	PromoAttrOutboxDropped   = "dropped"
)

// PromoAttrOutboxRow 单条归因意图（payload 为 resolve 请求体 JSON，
// Relay 原样重投）。
type PromoAttrOutboxRow struct {
	Id           int64  `json:"id"`
	AttrKey      string `json:"attr_key"`
	ClickId      string `json:"click_id"`
	UserId       int64  `json:"user_id"`
	RegisteredAt string `json:"registered_at"`
	Payload      string `json:"payload"`
	Status       string `json:"status"`
	Attempts     int    `json:"attempts"`
	LastError    string `json:"last_error"`
	NextRetryAt  string `json:"next_retry_at"`
	DeliveredAt  string `json:"delivered_at"`
	CreatedAt    string `json:"created_at"`
}

const promoAttrOutboxColumns = "id, attr_key, click_id, user_id, registered_at, payload, status, attempts, last_error, next_retry_at, delivered_at, created_at"

func scanPromoAttrOutbox(r *PromoAttrOutboxRow) []any {
	return []any{&r.Id, &r.AttrKey, &r.ClickId, &r.UserId, &r.RegisteredAt, &r.Payload,
		&r.Status, &r.Attempts, &r.LastError, &r.NextRetryAt, &r.DeliveredAt, &r.CreatedAt}
}

// PromoAttrOutboxModel 归因意图 outbox 数据访问。
type PromoAttrOutboxModel struct {
	db *sql.DB
}

func NewPromoAttrOutboxModel(db *sql.DB) *PromoAttrOutboxModel {
	return &PromoAttrOutboxModel{db: db}
}

func (m *PromoAttrOutboxModel) CreateTables() error {
	_, err := m.db.Exec(`CREATE TABLE IF NOT EXISTS promo_attribution_outbox (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		attr_key VARCHAR(128) NOT NULL UNIQUE,
		click_id VARCHAR(64) NOT NULL,
		user_id INTEGER NOT NULL DEFAULT 0,
		registered_at VARCHAR(32) NOT NULL DEFAULT '',
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
	_, err2 := m.db.Exec(`CREATE TABLE IF NOT EXISTS promo_attribution_outbox (
		id BIGINT PRIMARY KEY AUTO_INCREMENT,
		attr_key VARCHAR(128) NOT NULL UNIQUE,
		click_id VARCHAR(64) NOT NULL,
		user_id BIGINT NOT NULL DEFAULT 0,
		registered_at VARCHAR(32) NOT NULL DEFAULT '',
		payload TEXT NOT NULL,
		status VARCHAR(16) NOT NULL DEFAULT 'pending',
		attempts INT NOT NULL DEFAULT 0,
		last_error VARCHAR(512) NOT NULL DEFAULT '',
		next_retry_at VARCHAR(40) NOT NULL DEFAULT '',
		delivered_at VARCHAR(32) NOT NULL DEFAULT '',
		created_at VARCHAR(32) NOT NULL,
		KEY idx_promo_attr_outbox_retry (status, next_retry_at)
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`)
	if err2 != nil {
		return fmt.Errorf("创建渠道归因 outbox 表失败（尝试了 SQLite 和 MySQL 格式）: %v", err2)
	}
	return nil
}

// Enqueue 归因意图入队（幂等：attr_key 唯一，重复入队忽略，不重置既有的
// 失败退避/尝试计数）。nextRetryAt 为同步首投失败后最早可重投时间。
// nowUTC 由 analyticsoutboxmodel.go 提供（同包同形态）。
func (m *PromoAttrOutboxModel) Enqueue(attrKey, clickId string, userId int64, registeredAt, payload, nextRetryAt string) error {
	now := nowUTC()
	_, err := m.db.Exec(
		`INSERT OR IGNORE INTO promo_attribution_outbox (attr_key, click_id, user_id, registered_at, payload, status, attempts, last_error, next_retry_at, delivered_at, created_at)
		 VALUES (?, ?, ?, ?, ?, 'pending', 0, '', ?, '', ?)`,
		attrKey, clickId, userId, registeredAt, payload, nextRetryAt, now,
	)
	if err != nil {
		if _, err2 := m.db.Exec(
			`INSERT IGNORE INTO promo_attribution_outbox (attr_key, click_id, user_id, registered_at, payload, status, attempts, last_error, next_retry_at, delivered_at, created_at)
			 VALUES (?, ?, ?, ?, ?, 'pending', 0, '', ?, '', ?)`,
			attrKey, clickId, userId, registeredAt, payload, nextRetryAt, now,
		); err2 != nil {
			return err2
		}
	}
	return nil
}

// ListRetry 到期待解析行（pending 且 next_retry_at <= now，按 id 升序，有界）。
func (m *PromoAttrOutboxModel) ListRetry(now string, limit int) ([]*PromoAttrOutboxRow, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := fmt.Sprintf(`SELECT %s FROM promo_attribution_outbox WHERE status = '%s' AND next_retry_at <= ? ORDER BY id ASC LIMIT %d`,
		promoAttrOutboxColumns, PromoAttrOutboxPending, limit)
	rows, err := m.db.Query(q, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*PromoAttrOutboxRow, 0, limit)
	for rows.Next() {
		var r PromoAttrOutboxRow
		if err := rows.Scan(scanPromoAttrOutbox(&r)...); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// MarkDelivered 解析受理（幂等：仅 pending → delivered 转移）。
func (m *PromoAttrOutboxModel) MarkDelivered(attrKey, at string) error {
	_, err := m.db.Exec(
		`UPDATE promo_attribution_outbox SET status = ?, delivered_at = ? WHERE attr_key = ? AND status = ?`,
		PromoAttrOutboxDelivered, at, attrKey, PromoAttrOutboxPending,
	)
	return err
}

// MarkFailed 记一次重投失败：attempts 覆盖，按调用方给的退避时间定下次重投
// （Relay 按 attempts 指数退避）。
func (m *PromoAttrOutboxModel) MarkFailed(attrKey string, attempts int, lastErr, nextRetryAt string) error {
	_, err := m.db.Exec(
		`UPDATE promo_attribution_outbox SET attempts = ?, last_error = ?, next_retry_at = ? WHERE attr_key = ? AND status = ?`,
		attempts, lastErr, nextRetryAt, attrKey, PromoAttrOutboxPending,
	)
	return err
}

// MarkDropped 重试耗尽：置 dropped 留痕（不再重投，审计可查）。
func (m *PromoAttrOutboxModel) MarkDropped(attrKey string, attempts int, lastErr string) error {
	_, err := m.db.Exec(
		`UPDATE promo_attribution_outbox SET status = ?, attempts = ?, last_error = ? WHERE attr_key = ?`,
		PromoAttrOutboxDropped, attempts, lastErr, attrKey,
	)
	return err
}

// Prune 清理已终态行：delivered 按 delivered_at，dropped 按 created_at
// （两者都早于 before 删除），返回删除行数。
func (m *PromoAttrOutboxModel) Prune(before string) (int64, error) {
	res, err := m.db.Exec(
		`DELETE FROM promo_attribution_outbox WHERE (status = ? AND delivered_at <> '' AND delivered_at < ?) OR (status = ? AND created_at < ?)`,
		PromoAttrOutboxDelivered, before, PromoAttrOutboxDropped, before,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
