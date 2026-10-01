package model

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/tappi/tappi/services/content-service/internal/cache"
)

var ErrCommentNotFound = errors.New("comment not found")

type Comment struct {
	Id         int64  `json:"id"`
	TargetType string `json:"target_type"` // "guide", "game", etc.
	TargetId   int64  `json:"target_id"`
	UserId     int64  `json:"user_id"`
	UserName   string `json:"user_name"`
	Content    string `json:"content"`
	ParentId   int64  `json:"parent_id,omitempty"`
	ReplyToId  int64  `json:"reply_to_id,omitempty"`
	Likes      int    `json:"likes"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type CommentFilter struct {
	TargetType string
	TargetId   int64
	Page       int
	PageSize   int
}

// 缓存参数：进程内 TTL+LRU（设计决策见 docs/development-guide.md
// 「数据库集成 → 设计决策」）。60s TTL 同时兜底任何失效遗漏的最终一致性。
const (
	defaultCommentCacheTTL = 60 * time.Second
	defaultCommentCacheMax = 4096
)

// CommentModel 评论数据访问层（MySQL/SQLite 双驱动，表结构见 CreateCommentsTable）。
// Get 读进程内缓存；Create/Delete/Like 走统一失效点；不做负缓存。
// List 每次全量读取后应用侧过滤；连接池钳制为 1（单实例 + SQLite 多连接隔离，
// 与 GuideModel 同款）。
type CommentModel struct {
	db    *sql.DB
	cache *cache.Cache[string, Comment]
}

// CommentStore 评论仓储接口：ServiceContext 面向接口依赖，测试可注入故障实现。
type CommentStore interface {
	List(filter CommentFilter) ([]*Comment, int)
	Get(id int64) (*Comment, error)
	Create(comment *Comment) (*Comment, error)
	Delete(id int64) error
	Like(id int64) (int, error)
}

var _ CommentStore = (*CommentModel)(nil)

// NewCommentModel 创建评论模型（缓存默认开启；连接池钳制为 1）
func NewCommentModel(db *sql.DB) *CommentModel {
	db.SetMaxOpenConns(1)
	return &CommentModel{
		db:    db,
		cache: cache.New[string, Comment](defaultCommentCacheTTL, defaultCommentCacheMax),
	}
}

const commentColumns = "id, target_type, target_id, user_id, user_name, content, parent_id, reply_to_id, likes, created_at, updated_at"

// CreateCommentsTable 创建评论表（开发使用）。先尝试 SQLite 方言，失败回落
// MySQL（与 user-service.CreateUsersTable 同款双格式策略）。
func (m *CommentModel) CreateCommentsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			target_type VARCHAR(32) NOT NULL,
			target_id INTEGER NOT NULL DEFAULT 0,
			user_id INTEGER NOT NULL DEFAULT 0,
			user_name VARCHAR(64) NOT NULL,
			content TEXT NOT NULL,
			parent_id INTEGER NOT NULL DEFAULT 0,
			reply_to_id INTEGER NOT NULL DEFAULT 0,
			likes INTEGER NOT NULL DEFAULT 0,
			created_at VARCHAR(32) NOT NULL,
			updated_at VARCHAR(32) NOT NULL
		)
	`

	if _, err := m.db.Exec(query); err != nil {
		query = `
			CREATE TABLE IF NOT EXISTS comments (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				target_type VARCHAR(32) NOT NULL,
				target_id BIGINT NOT NULL DEFAULT 0,
				user_id BIGINT NOT NULL DEFAULT 0,
				user_name VARCHAR(64) NOT NULL,
				content TEXT NOT NULL,
				parent_id BIGINT NOT NULL DEFAULT 0,
				reply_to_id BIGINT NOT NULL DEFAULT 0,
				likes INT NOT NULL DEFAULT 0,
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

// Seed 按给定条目写入评论（显式保留 id，跳过 nil 与非正 id），
// 供内嵌种子初始化与测试装载。
func (m *CommentModel) Seed(comments []*Comment) error {
	for _, comment := range comments {
		if comment == nil || comment.Id <= 0 {
			continue
		}
		if err := m.insertRow(comment); err != nil {
			return err
		}
	}
	return nil
}

// SeedIfEmpty 表为空时写入内嵌种子数据（首次启动初始化演示内容）。
func (m *CommentModel) SeedIfEmpty() error {
	var count int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM comments`).Scan(&count); err != nil {
		return fmt.Errorf("统计评论数失败: %w", err)
	}
	if count > 0 {
		return nil
	}
	return m.Seed(defaultSeedComments())
}

func (m *CommentModel) insertRow(comment *Comment) error {
	// Seed 显式保留 id（自增序从最大 id 之后继续），与原内存仓装载语义一致
	_, err := m.db.Exec(
		`INSERT INTO comments (`+commentColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		comment.Id, comment.TargetType, comment.TargetId, comment.UserId, comment.UserName,
		comment.Content, comment.ParentId, comment.ReplyToId,
		comment.Likes, comment.CreatedAt, comment.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("写入评论失败: %w", err)
	}
	return nil
}

func commentKey(id int64) string { return "id:" + strconv.FormatInt(id, 10) }

// getFromDB 绕过缓存直查数据库（写路径须基于库内真值）。
func (m *CommentModel) getFromDB(id int64) (*Comment, error) {
	comment := &Comment{}
	err := m.db.QueryRow(`SELECT `+commentColumns+` FROM comments WHERE id = ?`, id).
		Scan(&comment.Id, &comment.TargetType, &comment.TargetId, &comment.UserId,
			&comment.UserName, &comment.Content, &comment.ParentId, &comment.ReplyToId,
			&comment.Likes, &comment.CreatedAt, &comment.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrCommentNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询评论失败: %w", err)
	}
	return comment, nil
}

// Get 按 id 查找评论（读缓存）。未命中返回 ErrCommentNotFound 哨兵。
func (m *CommentModel) Get(id int64) (*Comment, error) {
	key := commentKey(id)
	if cached, ok := m.cache.Get(key); ok {
		return &cached, nil
	}

	comment, err := m.getFromDB(id)
	if err != nil {
		return nil, err
	}

	m.cache.Set(key, *comment)
	return comment, nil
}

// Create 新建评论：重置点赞数、写 RFC3339 时间戳，自增主键落库，
// 失效新行缓存键（负缓存不存在，通常无键可删；防御同 id 重建路径）。
func (m *CommentModel) Create(comment *Comment) (*Comment, error) {
	if comment == nil {
		return nil, errors.New("comment payload required")
	}

	now := time.Now().Format(time.RFC3339)
	comment.Id = 0
	comment.CreatedAt = now
	comment.UpdatedAt = now
	comment.Likes = 0

	result, err := m.db.Exec(
		// Create 不写 id：自增主键落库，LastInsertId 取回（id 列仅 Seed 显式使用）
		`INSERT INTO comments (target_type, target_id, user_id, user_name, content,
			parent_id, reply_to_id, likes, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		comment.TargetType, comment.TargetId, comment.UserId, comment.UserName,
		comment.Content, comment.ParentId, comment.ReplyToId,
		comment.Likes, comment.CreatedAt, comment.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("写入评论失败: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("获取评论ID失败: %w", err)
	}
	comment.Id = id

	m.cache.Delete(commentKey(id))
	return comment, nil
}

// Delete 删除评论；自增主键不复用已删 id（与原内存仓语义一致）。
func (m *CommentModel) Delete(id int64) error {
	result, err := m.db.Exec(`DELETE FROM comments WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("删除评论失败: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("确认评论存在失败: %w", err)
	} else if n == 0 {
		return ErrCommentNotFound
	}

	m.cache.Delete(commentKey(id))
	return nil
}

// Like 点赞并返回最新计数。
func (m *CommentModel) Like(id int64) (int, error) {
	result, err := m.db.Exec(`UPDATE comments SET likes = likes + 1 WHERE id = ?`, id)
	if err != nil {
		return 0, fmt.Errorf("点赞评论失败: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return 0, fmt.Errorf("确认评论存在失败: %w", err)
	} else if n == 0 {
		return 0, ErrCommentNotFound
	}

	var likes int
	if err := m.db.QueryRow(`SELECT likes FROM comments WHERE id = ?`, id).Scan(&likes); err != nil {
		return 0, fmt.Errorf("查询评论点赞数失败: %w", err)
	}

	m.cache.Delete(commentKey(id))
	return likes, nil
}

// all 全量读取评论，按 id 升序——原内存仓为插入序，自增 id 恒单调，
// id 序与插入序严格一致，语义零漂移。
func (m *CommentModel) all() ([]*Comment, error) {
	rows, err := m.db.Query(`SELECT ` + commentColumns + ` FROM comments ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("查询评论列表失败: %w", err)
	}
	defer rows.Close()

	var comments []*Comment
	for rows.Next() {
		comment := &Comment{}
		if err := rows.Scan(&comment.Id, &comment.TargetType, &comment.TargetId,
			&comment.UserId, &comment.UserName, &comment.Content, &comment.ParentId,
			&comment.ReplyToId, &comment.Likes, &comment.CreatedAt, &comment.UpdatedAt); err != nil {
			return nil, fmt.Errorf("读取评论行失败: %w", err)
		}
		comments = append(comments, comment)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历评论行失败: %w", err)
	}
	return comments, nil
}

// List 按过滤条件列出评论（应用侧过滤 + 分页钳制，非法 page/size 安全降级）。
func (m *CommentModel) List(filter CommentFilter) ([]*Comment, int) {
	comments, err := m.all()
	if err != nil {
		return nil, 0
	}

	var filtered []*Comment
	for _, comment := range comments {
		if comment == nil {
			continue
		}
		if !matchesComment(comment, filter) {
			continue
		}
		filtered = append(filtered, comment)
	}

	total := len(filtered)
	start := (filter.Page - 1) * filter.PageSize
	// 非法分页参数（page<1 或 size<0）钳制为空页/首页，避免负索引切片 panic
	if start < 0 {
		start = 0
	}
	end := start + filter.PageSize
	if end < start {
		end = start
	}

	if start > total {
		start = total
	}
	if end > total {
		end = total
	}

	result := make([]*Comment, end-start)
	copy(result, filtered[start:end])

	return result, total
}

func matchesComment(comment *Comment, filter CommentFilter) bool {
	if filter.TargetType != "" && comment.TargetType != filter.TargetType {
		return false
	}

	if filter.TargetId > 0 && comment.TargetId != filter.TargetId {
		return false
	}

	return true
}

func defaultSeedComments() []*Comment {
	return []*Comment{
		{
			Id:         1,
			TargetType: "guide",
			TargetId:   1,
			UserId:     2001,
			UserName:   "玩家A",
			Content:    "这个攻略写得太好了！帮了我大忙！",
			Likes:      15,
			CreatedAt:  "2024-01-16T08:30:00Z",
			UpdatedAt:  "2024-01-16T08:30:00Z",
		},
		{
			Id:         2,
			TargetType: "guide",
			TargetId:   1,
			UserId:     2002,
			UserName:   "游戏爱好者",
			Content:    "前期确实按照这个思路打会轻松很多",
			Likes:      8,
			CreatedAt:  "2024-01-17T10:15:00Z",
			UpdatedAt:  "2024-01-17T10:15:00Z",
		},
		{
			Id:         3,
			TargetType: "guide",
			TargetId:   2,
			UserId:     2003,
			UserName:   "射击之王",
			Content:    "这些训练方法确实有效，枪法进步明显",
			ParentId:   0,
			Likes:      23,
			CreatedAt:  "2024-01-19T14:20:00Z",
			UpdatedAt:  "2024-01-19T14:20:00Z",
		},
	}
}
