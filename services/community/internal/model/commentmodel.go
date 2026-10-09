package model

import (
	"database/sql"
	"fmt"

	"github.com/tappi/tappi/services/community/internal/types"
)

// CommentStore 帖子评论仓储接口（与 PostStore/TopicStore 同款双驱动）。
// 社区评论是单层结构：reply_to_author_name 冗余记录被回复者昵称用于展示，
// parent_id 保留给后续嵌套回复扩展（当前写入恒为 0）。
type CommentStore interface {
	CreateCommentsTable() error
	Create(comment *types.Comment) (int64, error)
	ListByPost(postId int64, limit, offset int64) ([]*types.Comment, error)
	CountByPost(postId int64) (int64, error)
	GetByID(id int64) (*types.Comment, error)
	Delete(id int64) error
}

type CommentModel struct {
	db *sql.DB
}

func NewCommentModel(db *sql.DB) *CommentModel {
	return &CommentModel{db: db}
}

const commentColumns = `id, post_id, author_id, author_name, content, parent_id, reply_to_author_name, created_at, updated_at`

func (m *CommentModel) CreateCommentsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			post_id INTEGER NOT NULL,
			author_id INTEGER NOT NULL DEFAULT 0,
			author_name VARCHAR(64) NOT NULL DEFAULT '',
			content TEXT NOT NULL,
			parent_id INTEGER NOT NULL DEFAULT 0,
			reply_to_author_name VARCHAR(64) NOT NULL DEFAULT '',
			created_at VARCHAR(32) NOT NULL,
			updated_at VARCHAR(32) NOT NULL
		)
	`
	if _, err := m.db.Exec(query); err != nil {
		query = `
			CREATE TABLE IF NOT EXISTS comments (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				post_id BIGINT NOT NULL,
				author_id BIGINT NOT NULL DEFAULT 0,
				author_name VARCHAR(64) NOT NULL DEFAULT '',
				content TEXT NOT NULL,
				parent_id BIGINT NOT NULL DEFAULT 0,
				reply_to_author_name VARCHAR(64) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		if _, err := m.db.Exec(query); err != nil {
			return fmt.Errorf("创建评论表失败（尝试了 SQLite 和 MySQL 格式）: %w", err)
		}
	}
	return nil
}

func (m *CommentModel) Create(comment *types.Comment) (int64, error) {
	res, err := m.db.Exec(
		`INSERT INTO comments (post_id, author_id, author_name, content, parent_id, reply_to_author_name, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		comment.PostId, comment.AuthorId, comment.AuthorName, comment.Content,
		comment.ParentId, comment.ReplyToAuthorName, comment.CreatedAt, comment.UpdatedAt,
	)
	if err != nil {
		return 0, fmt.Errorf("写入评论失败: %w", err)
	}
	return res.LastInsertId()
}

func (m *CommentModel) ListByPost(postId int64, limit, offset int64) ([]*types.Comment, error) {
	rows, err := m.db.Query(
		`SELECT `+commentColumns+` FROM comments WHERE post_id = ? ORDER BY id ASC LIMIT ? OFFSET ?`,
		postId, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("查询评论失败: %w", err)
	}
	defer rows.Close()
	return scanComments(rows)
}

func (m *CommentModel) CountByPost(postId int64) (int64, error) {
	var count int64
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM comments WHERE post_id = ?`, postId).Scan(&count); err != nil {
		return 0, fmt.Errorf("统计评论数失败: %w", err)
	}
	return count, nil
}

func (m *CommentModel) GetByID(id int64) (*types.Comment, error) {
	row := m.db.QueryRow(`SELECT `+commentColumns+` FROM comments WHERE id = ?`, id)
	comment := &types.Comment{}
	err := row.Scan(
		&comment.Id, &comment.PostId, &comment.AuthorId, &comment.AuthorName, &comment.Content,
		&comment.ParentId, &comment.ReplyToAuthorName, &comment.CreatedAt, &comment.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrCommentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询评论失败: %w", err)
	}
	return comment, nil
}

func (m *CommentModel) Delete(id int64) error {
	res, err := m.db.Exec(`DELETE FROM comments WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除评论失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrCommentNotFound
	}
	return nil
}

func scanComments(rows *sql.Rows) ([]*types.Comment, error) {
	list := make([]*types.Comment, 0)
	for rows.Next() {
		comment := &types.Comment{}
		if err := rows.Scan(
			&comment.Id, &comment.PostId, &comment.AuthorId, &comment.AuthorName, &comment.Content,
			&comment.ParentId, &comment.ReplyToAuthorName, &comment.CreatedAt, &comment.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("读取评论失败: %w", err)
		}
		list = append(list, comment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历评论失败: %w", err)
	}
	return list, nil
}
