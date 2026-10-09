package model

import (
	"database/sql"
	"fmt"

	"github.com/tappi/tappi/services/community/internal/types"
)

// NotificationStore 站内通知仓储接口（与 CommentStore 同款双驱动）。
// 去噪契约：UNIQUE (user_id, actor_id, type, target_id) + INSERT OR IGNORE，
// 同一行为者对同一目标的同类动作只产生一条通知（重复点赞不刷屏）。
// 无 404 语义路径，故不定义哨兵错误。
type NotificationStore interface {
	CreateNotificationsTable() error
	Create(n *types.Notification) error
	ListByUser(userId, limit, offset int64) ([]*types.Notification, int64, error)
	CountUnread(userId int64) (int64, error)
	MarkAllRead(userId int64) error
}

type NotificationModel struct {
	db *sql.DB
}

// NewNotificationModel 创建通知模型（连接池钳制为 1，与其他模型共享同池）
func NewNotificationModel(db *sql.DB) *NotificationModel {
	db.SetMaxOpenConns(1)
	return &NotificationModel{db: db}
}

const notificationColumns = `id, user_id, actor_id, actor_name, type, target_id, content, is_read, created_at`

func (m *NotificationModel) CreateNotificationsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS notifications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			actor_id INTEGER NOT NULL DEFAULT 0,
			actor_name VARCHAR(64) NOT NULL DEFAULT '',
			type VARCHAR(32) NOT NULL,
			target_id INTEGER NOT NULL DEFAULT 0,
			content VARCHAR(256) NOT NULL DEFAULT '',
			is_read INTEGER NOT NULL DEFAULT 0,
			created_at VARCHAR(32) NOT NULL,
			UNIQUE (user_id, actor_id, type, target_id)
		)
	`
	if _, err := m.db.Exec(query); err != nil {
		query = `
			CREATE TABLE IF NOT EXISTS notifications (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				user_id BIGINT NOT NULL,
				actor_id BIGINT NOT NULL DEFAULT 0,
				actor_name VARCHAR(64) NOT NULL DEFAULT '',
				type VARCHAR(32) NOT NULL,
				target_id BIGINT NOT NULL DEFAULT 0,
				content VARCHAR(256) NOT NULL DEFAULT '',
				is_read TINYINT NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				UNIQUE KEY uk_notif_dedup (user_id, actor_id, type, target_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		if _, err := m.db.Exec(query); err != nil {
			return fmt.Errorf("创建通知表失败（尝试了 SQLite 和 MySQL 格式）: %w", err)
		}
	}
	return nil
}

// Create 幂等写入通知：唯一键冲突静默忽略（SQLite INSERT OR IGNORE，
// MySQL 语法失败回落 INSERT IGNORE，与点赞关系写入同款策略）。
func (m *NotificationModel) Create(n *types.Notification) error {
	_, err := m.db.Exec(
		`INSERT OR IGNORE INTO notifications (user_id, actor_id, actor_name, type, target_id, content, is_read, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		n.UserId, n.ActorId, n.ActorName, n.Type, n.TargetId, n.Content, boolToInt(n.IsRead), n.CreatedAt,
	)
	if err == nil {
		return nil
	}
	if _, retryErr := m.db.Exec(
		`INSERT IGNORE INTO notifications (user_id, actor_id, actor_name, type, target_id, content, is_read, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		n.UserId, n.ActorId, n.ActorName, n.Type, n.TargetId, n.Content, boolToInt(n.IsRead), n.CreatedAt,
	); retryErr != nil {
		return fmt.Errorf("写入通知失败（SQLite: %v；MySQL: %w）", err, retryErr)
	}
	return nil
}

// ListByUser 收件人通知，id 倒序（新在前）+ 全量 total。
func (m *NotificationModel) ListByUser(userId, limit, offset int64) ([]*types.Notification, int64, error) {
	var total int64
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM notifications WHERE user_id = ?`, userId).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计通知数失败: %w", err)
	}

	rows, err := m.db.Query(
		`SELECT `+notificationColumns+` FROM notifications WHERE user_id = ? ORDER BY id DESC LIMIT ? OFFSET ?`,
		userId, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("查询通知失败: %w", err)
	}
	defer rows.Close()

	list := make([]*types.Notification, 0)
	for rows.Next() {
		n := &types.Notification{}
		var isRead int64
		if err := rows.Scan(
			&n.Id, &n.UserId, &n.ActorId, &n.ActorName, &n.Type, &n.TargetId, &n.Content, &isRead, &n.CreatedAt,
		); err != nil {
			return nil, 0, fmt.Errorf("读取通知失败: %w", err)
		}
		n.IsRead = isRead != 0
		list = append(list, n)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历通知失败: %w", err)
	}
	return list, total, nil
}

func (m *NotificationModel) CountUnread(userId int64) (int64, error) {
	var count int64
	if err := m.db.QueryRow(
		`SELECT COUNT(*) FROM notifications WHERE user_id = ? AND is_read = 0`, userId,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("统计未读通知失败: %w", err)
	}
	return count, nil
}

func (m *NotificationModel) MarkAllRead(userId int64) error {
	if _, err := m.db.Exec(`UPDATE notifications SET is_read = 1 WHERE user_id = ? AND is_read = 0`, userId); err != nil {
		return fmt.Errorf("标记通知已读失败: %w", err)
	}
	return nil
}
