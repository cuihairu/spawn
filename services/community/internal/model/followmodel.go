package model

import (
	"database/sql"
	"fmt"
	"time"
)

// FollowModel 关注关系数据访问层（MySQL/SQLite 双驱动，表结构见
// CreateFollowsTable）。单表 (user_id, target_type, target_id) 唯一约束
// 表达两组关系（user→topic / user→user）。
// 不加进程内缓存：关系为写多读少的关联查询，单查走唯一索引已足够；
// Follow/Unfollow 的 bool 返回由「影响行数」判定，与原内存仓集合语义一致。
// 连接池钳制为 1（与 TopicModel/PostModel 同款单实例假设）。
type FollowModel struct {
	db *sql.DB
}

// FollowStore 关注关系仓储接口：ServiceContext 面向接口依赖，测试可注入
// 故障实现。
type FollowStore interface {
	FollowTopic(userId, topicId int64) bool
	UnfollowTopic(userId, topicId int64) bool
	ListFollowingTopicIds(userId int64) []int64
	FollowUser(userId, targetUserId int64) bool
	UnfollowUser(userId, targetUserId int64) bool
}

var _ FollowStore = (*FollowModel)(nil)

// NewFollowModel 创建关注关系模型（连接池钳制为 1）
func NewFollowModel(db *sql.DB) *FollowModel {
	db.SetMaxOpenConns(1)
	return &FollowModel{db: db}
}

// CreateFollowsTable 创建关注关系表（开发使用）。先尝试 SQLite 方言，
// 失败回落 MySQL（双格式策略同前）；唯一索引保证重复关注写入冲突。
func (m *FollowModel) CreateFollowsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS follows (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			target_type VARCHAR(16) NOT NULL,
			target_id INTEGER NOT NULL,
			created_at VARCHAR(32) NOT NULL,
			UNIQUE (user_id, target_type, target_id)
		)
	`

	if _, err := m.db.Exec(query); err != nil {
		query = `
			CREATE TABLE IF NOT EXISTS follows (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				user_id BIGINT NOT NULL,
				target_type VARCHAR(16) NOT NULL,
				target_id BIGINT NOT NULL,
				created_at VARCHAR(32) NOT NULL,
				UNIQUE KEY uk_follow (user_id, target_type, target_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		if _, err := m.db.Exec(query); err != nil {
			return fmt.Errorf("创建关注关系表失败（尝试了 SQLite 和 MySQL 格式）: %w", err)
		}
	}

	return nil
}

// insertFollow 写入一条关系；双方言幂等写（SQLite INSERT OR IGNORE /
// MySQL INSERT IGNORE），冲突（已关注）返回 false。
func (m *FollowModel) insertFollow(userId int64, targetType string, targetId int64) bool {
	now := time.Now().UTC().Format(time.RFC3339)

	result, err := m.db.Exec(
		`INSERT OR IGNORE INTO follows (user_id, target_type, target_id, created_at) VALUES (?, ?, ?, ?)`,
		userId, targetType, targetId, now,
	)
	if err != nil {
		// SQLite 方言不被接受（MySQL 在线）：回落 MySQL 幂等写法
		result, err = m.db.Exec(
			`INSERT IGNORE INTO follows (user_id, target_type, target_id, created_at) VALUES (?, ?, ?, ?)`,
			userId, targetType, targetId, now,
		)
		if err != nil {
			return false
		}
	}

	n, err := result.RowsAffected()
	if err != nil {
		return false
	}
	return n == 1
}

// deleteFollow 删除一条关系；无匹配行（未关注）返回 false。
func (m *FollowModel) deleteFollow(userId int64, targetType string, targetId int64) bool {
	result, err := m.db.Exec(
		`DELETE FROM follows WHERE user_id = ? AND target_type = ? AND target_id = ?`,
		userId, targetType, targetId,
	)
	if err != nil {
		return false
	}

	n, err := result.RowsAffected()
	if err != nil {
		return false
	}
	return n == 1
}

// FollowTopic 关注话题：重复关注返回 false（幂等）。
func (m *FollowModel) FollowTopic(userId, topicId int64) bool {
	return m.insertFollow(userId, "topic", topicId)
}

// UnfollowTopic 取消关注话题：本无关系返回 false。
func (m *FollowModel) UnfollowTopic(userId, topicId int64) bool {
	return m.deleteFollow(userId, "topic", topicId)
}

// ListFollowingTopicIds 列出用户关注的话题 id，升序（与原内存仓排序一致）。
func (m *FollowModel) ListFollowingTopicIds(userId int64) []int64 {
	rows, err := m.db.Query(
		`SELECT target_id FROM follows WHERE user_id = ? AND target_type = 'topic' ORDER BY target_id`,
		userId,
	)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	return out
}

// FollowUser 关注用户：重复关注返回 false（幂等）。
func (m *FollowModel) FollowUser(userId, targetUserId int64) bool {
	return m.insertFollow(userId, "user", targetUserId)
}

// UnfollowUser 取消关注用户：本无关系返回 false。
func (m *FollowModel) UnfollowUser(userId, targetUserId int64) bool {
	return m.deleteFollow(userId, "user", targetUserId)
}
