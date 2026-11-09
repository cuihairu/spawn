package model

// GP-302 活动奖励评估队列：订单 PAID 后入队（order_id 唯一），分发器
// 推送 promotions /internal/reward-evaluations（HMAC 签名 S2S）。at-least-
// once + 消费端幂等（promotions 按 (order_id, activity_id) 唯一去重），
// 失败指数退避重投（与发货账本恢复同款，进程重启续扫）。

import (
	"database/sql"
	"errors"
	"strings"
)

// 评估队列状态。
const (
	EvalStatusPending = "pending"
	EvalStatusDone    = "done"
	EvalStatusFailed  = "failed"
)

// ErrEvalNotFound 队列行不存在。
var ErrEvalNotFound = errors.New("promotion evaluation not found")

// PromotionEval 评估队列行（payload 为订单事实 JSON 快照）。
type PromotionEval struct {
	Id          int64  `json:"id"`
	OrderId     string `json:"order_id"`
	Channel     string `json:"channel"`
	Payload     string `json:"payload"`
	Status      string `json:"status"`
	Attempts    int    `json:"attempts"`
	LastError   string `json:"last_error"`
	NextRetryAt string `json:"next_retry_at"`
	DoneAt      string `json:"done_at"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// PromotionEvalModel 评估队列数据访问。
type PromotionEvalModel struct {
	db *sql.DB
}

// NewPromotionEvalModel 构造。
func NewPromotionEvalModel(db *sql.DB) *PromotionEvalModel {
	return &PromotionEvalModel{db: db}
}

// CreateTables 建队列表（UNIQUE(order_id) = 单订单单评估事件）。
func (m *PromotionEvalModel) CreateTables() error {
	tables := []struct{ sqlite, mysql string }{
		{
			`CREATE TABLE IF NOT EXISTS promotion_evaluations (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				order_id VARCHAR(64) NOT NULL,
				channel VARCHAR(32) NOT NULL DEFAULT '',
				payload TEXT NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'pending',
				attempts INTEGER NOT NULL DEFAULT 0,
				last_error TEXT NOT NULL DEFAULT '',
				next_retry_at VARCHAR(48) NOT NULL DEFAULT '',
				done_at VARCHAR(32) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL,
				UNIQUE (order_id)
			)`,
			`CREATE TABLE IF NOT EXISTS promotion_evaluations (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				order_id VARCHAR(64) NOT NULL,
				channel VARCHAR(32) NOT NULL DEFAULT '',
				payload TEXT NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'pending',
				attempts INT NOT NULL DEFAULT 0,
				last_error TEXT NOT NULL,
				next_retry_at VARCHAR(48) NOT NULL DEFAULT '',
				done_at VARCHAR(32) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL,
				UNIQUE KEY uk_eval_order (order_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
	}
	for _, t := range tables {
		if _, err := m.db.Exec(t.sqlite); err != nil {
			if _, err2 := m.db.Exec(t.mysql); err2 != nil {
				return errors.New("创建奖励评估队列表失败（尝试了 SQLite 和 MySQL 格式）")
			}
		}
	}
	return nil
}

// Enqueue 入队（幂等：同订单已入队直接跳过）。
func (m *PromotionEvalModel) Enqueue(orderID, channel, payload string) error {
	now := nowUTC()
	_, err := m.db.Exec(
		`INSERT INTO promotion_evaluations (order_id, channel, payload, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(order_id) DO NOTHING`,
		orderID, channel, payload, EvalStatusPending, now, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "syntax") || strings.Contains(err.Error(), "near") {
			_, err = m.db.Exec(
				`INSERT IGNORE INTO promotion_evaluations (order_id, channel, payload, status, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				orderID, channel, payload, EvalStatusPending, now, now,
			)
		}
	}
	return err
}

// ListRetry 取到期待投行（pending/failed 且 next_retry_at ≤ now，有界）。
func (m *PromotionEvalModel) ListRetry(now string, limit int) ([]*PromotionEval, error) {
	rows, err := m.db.Query(
		`SELECT id, order_id, channel, payload, status, attempts, last_error, next_retry_at, done_at, created_at, updated_at
		   FROM promotion_evaluations
		  WHERE status IN ('pending', 'failed') AND (next_retry_at = '' OR next_retry_at <= ?)
		  ORDER BY id ASC LIMIT ?`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*PromotionEval
	for rows.Next() {
		var e PromotionEval
		if err := rows.Scan(&e.Id, &e.OrderId, &e.Channel, &e.Payload, &e.Status,
			&e.Attempts, &e.LastError, &e.NextRetryAt, &e.DoneAt, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &e)
	}
	return out, rows.Err()
}

// MarkDone 记投递成功。
func (m *PromotionEvalModel) MarkDone(orderID string) error {
	now := nowUTC()
	_, err := m.db.Exec(
		`UPDATE promotion_evaluations SET status = 'done', done_at = ?, updated_at = ? WHERE order_id = ?`,
		now, now, orderID,
	)
	return err
}

// MarkFailed 记失败并定下次重试时间（指数退避，封顶 6 倍）。
func (m *PromotionEvalModel) MarkFailed(orderID string, attempts int, cause string, next string) error {
	now := nowUTC()
	_, err := m.db.Exec(
		`UPDATE promotion_evaluations SET status = 'failed', attempts = ?, last_error = ?, next_retry_at = ?, updated_at = ? WHERE order_id = ?`,
		attempts, cause, next, now, orderID,
	)
	return err
}
