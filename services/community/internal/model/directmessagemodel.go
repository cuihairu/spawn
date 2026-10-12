package model

import (
	"database/sql"
	"fmt"

	"github.com/tappi/tappi/services/community/internal/types"
)

// DirectMessageStore 站内私信仓储接口（与 NotificationStore 同款双驱动）。
// 会话以 peer（对端用户）聚合：我发的取 recipient，我收的取 sender；
// is_read 只翻转收件方行，发件方副本不受影响。
// 不建二级索引：本地部署量级全表扫足够，跨库索引语法差异不值得（SQLite
// 支持 IF NOT EXISTS，MySQL 不支持），量大后再议。
type DirectMessageStore interface {
	CreateDirectMessagesTable() error
	Send(msg *types.DirectMessage) error
	Conversations(userId int64) ([]*types.DMConversation, error)
	Thread(userId, peer, limit, offset int64) ([]*types.DirectMessage, int64, error)
	MarkPeerRead(userId, peer int64) (int64, error)
	CountUnread(userId int64) (int64, error)
}

type DirectMessageModel struct {
	db *sql.DB
}

// NewDirectMessageModel 创建私信模型（连接池钳制为 1，与其他模型共享同池）
func NewDirectMessageModel(db *sql.DB) *DirectMessageModel {
	db.SetMaxOpenConns(1)
	return &DirectMessageModel{db: db}
}

const dmColumns = `id, sender_id, recipient_id, content, is_read, created_at`

func (m *DirectMessageModel) CreateDirectMessagesTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS direct_messages (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			sender_id INTEGER NOT NULL,
			recipient_id INTEGER NOT NULL,
			content VARCHAR(2000) NOT NULL,
			is_read INTEGER NOT NULL DEFAULT 0,
			created_at VARCHAR(32) NOT NULL
		)
	`
	if _, err := m.db.Exec(query); err != nil {
		query = `
			CREATE TABLE IF NOT EXISTS direct_messages (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				sender_id BIGINT NOT NULL,
				recipient_id BIGINT NOT NULL,
				content VARCHAR(2000) NOT NULL,
				is_read TINYINT NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		if _, err := m.db.Exec(query); err != nil {
			return fmt.Errorf("创建私信表失败（尝试了 SQLite 和 MySQL 格式）: %w", err)
		}
	}
	return nil
}

// Send 写入一条私信。
func (m *DirectMessageModel) Send(msg *types.DirectMessage) error {
	res, err := m.db.Exec(
		`INSERT INTO direct_messages (sender_id, recipient_id, content, is_read, created_at)
		 VALUES (?, ?, ?, 0, ?)`,
		msg.SenderId, msg.RecipientId, msg.Content, msg.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("写入私信失败: %w", err)
	}
	if id, err := res.LastInsertId(); err == nil {
		msg.Id = id
	}
	return nil
}

// Conversations 会话列表：peer 聚合（最后一条消息 + 各自未读数），
// 按最后活跃倒序。三步组装保持 SQLite/MySQL 方言兼容。
func (m *DirectMessageModel) Conversations(userId int64) ([]*types.DMConversation, error) {
	// 1) 各 peer 的最后消息 id
	rows, err := m.db.Query(`
		SELECT peer, MAX(id) FROM (
			SELECT CASE WHEN sender_id = ? THEN recipient_id ELSE sender_id END AS peer, id
			FROM direct_messages WHERE sender_id = ? OR recipient_id = ?
		) GROUP BY peer ORDER BY MAX(id) DESC`, userId, userId, userId)
	if err != nil {
		return nil, fmt.Errorf("查询会话失败: %w", err)
	}
	type peerLast struct {
		peer   int64
		lastId int64
	}
	peers := make([]peerLast, 0)
	for rows.Next() {
		var pl peerLast
		if err := rows.Scan(&pl.peer, &pl.lastId); err != nil {
			rows.Close()
			return nil, fmt.Errorf("读取会话失败: %w", err)
		}
		peers = append(peers, pl)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历会话失败: %w", err)
	}
	if len(peers) == 0 {
		return []*types.DMConversation{}, nil
	}

	// 2) 最后一条消息内容
	lastById := make(map[int64]*types.DirectMessage, len(peers))
	for _, pl := range peers {
		msg := &types.DirectMessage{}
		var isRead int64
		err := m.db.QueryRow(
			`SELECT `+dmColumns+` FROM direct_messages WHERE id = ?`, pl.lastId,
		).Scan(&msg.Id, &msg.SenderId, &msg.RecipientId, &msg.Content, &isRead, &msg.CreatedAt)
		if err != nil {
			return nil, fmt.Errorf("查询会话末条失败: %w", err)
		}
		msg.IsRead = isRead != 0
		lastById[pl.peer] = msg
	}

	// 3) 各 peer 未读数（我收到的、未读）
	unreadBy := make(map[int64]int64)
	if userId > 0 {
		rows, err := m.db.Query(
			`SELECT sender_id, COUNT(1) FROM direct_messages WHERE recipient_id = ? AND is_read = 0 GROUP BY sender_id`,
			userId,
		)
		if err != nil {
			return nil, fmt.Errorf("统计会话未读失败: %w", err)
		}
		for rows.Next() {
			var peer, n int64
			if err := rows.Scan(&peer, &n); err != nil {
				rows.Close()
				return nil, fmt.Errorf("读取会话未读失败: %w", err)
			}
			unreadBy[peer] = n
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("遍历会话未读失败: %w", err)
		}
	}

	list := make([]*types.DMConversation, 0, len(peers))
	for _, pl := range peers {
		msg := lastById[pl.peer]
		list = append(list, &types.DMConversation{
			PeerId:      pl.peer,
			LastMessage: msg.Content,
			LastAt:      msg.CreatedAt,
			Unread:      unreadBy[pl.peer],
		})
	}
	return list, nil
}

// Thread 与某 peer 的往来消息，id 倒序（新在前）+ 全量 total。
func (m *DirectMessageModel) Thread(userId, peer, limit, offset int64) ([]*types.DirectMessage, int64, error) {
	pair := `(sender_id = ? AND recipient_id = ?) OR (sender_id = ? AND recipient_id = ?)`
	var total int64
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM direct_messages WHERE `+pair, userId, peer, peer, userId).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计私信数失败: %w", err)
	}

	rows, err := m.db.Query(
		`SELECT `+dmColumns+` FROM direct_messages WHERE `+pair+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		userId, peer, peer, userId, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("查询私信失败: %w", err)
	}
	defer rows.Close()

	list := make([]*types.DirectMessage, 0)
	for rows.Next() {
		msg := &types.DirectMessage{}
		var isRead int64
		if err := rows.Scan(&msg.Id, &msg.SenderId, &msg.RecipientId, &msg.Content, &isRead, &msg.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("读取私信失败: %w", err)
		}
		msg.IsRead = isRead != 0
		list = append(list, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历私信失败: %w", err)
	}
	return list, total, nil
}

// MarkPeerRead 我收到的、来自 peer 的消息置已读；返回实际翻转行数
// （0 = 无新消息，幂等成功）。
func (m *DirectMessageModel) MarkPeerRead(userId, peer int64) (int64, error) {
	res, err := m.db.Exec(
		`UPDATE direct_messages SET is_read = 1 WHERE recipient_id = ? AND sender_id = ? AND is_read = 0`,
		userId, peer,
	)
	if err != nil {
		return 0, fmt.Errorf("标记私信已读失败: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("标记私信已读失败: %w", err)
	}
	return n, nil
}

func (m *DirectMessageModel) CountUnread(userId int64) (int64, error) {
	var count int64
	if err := m.db.QueryRow(
		`SELECT COUNT(*) FROM direct_messages WHERE recipient_id = ? AND is_read = 0`, userId,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("统计未读私信失败: %w", err)
	}
	return count, nil
}
