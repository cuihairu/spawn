package model

import (
	"database/sql"
	"fmt"
	"time"
)

// GuideFavorite 用户-攻略收藏关系（主键 (user_id, guide_id) 天然幂等去重）。
type GuideFavorite struct {
	UserId    int64  `json:"user_id"`
	GuideId   int64  `json:"guide_id"`
	CreatedAt string `json:"created_at"`
}

// FavoriteStore 攻略收藏仓储接口：ServiceContext 面向接口依赖，测试可注入实现。
type FavoriteStore interface {
	Add(userId, guideId int64) error
	Remove(userId, guideId int64) error
	Exists(userId, guideId int64) (bool, error)
	CountByGuide(guideId int64) (int64, error)
	ListByUser(userId int64, limit, offset int) ([]*Guide, int, error)
}

var _ FavoriteStore = (*FavoriteModel)(nil)

type FavoriteModel struct {
	db *sql.DB
}

// NewFavoriteModel 创建收藏模型（连接池钳制为 1，与 GuideModel 同款）。
func NewFavoriteModel(db *sql.DB) *FavoriteModel {
	db.SetMaxOpenConns(1)
	return &FavoriteModel{db: db}
}

// CreateFavoritesTable 创建收藏表。先尝试 SQLite 方言，失败回落 MySQL
// （与 CreateGuidesTable 同款双格式策略）。
func (m *FavoriteModel) CreateFavoritesTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS guide_favorites (
			user_id INTEGER NOT NULL,
			guide_id INTEGER NOT NULL,
			created_at VARCHAR(32) NOT NULL DEFAULT '',
			PRIMARY KEY (user_id, guide_id)
		)
	`

	if _, err := m.db.Exec(query); err != nil {
		query = `
			CREATE TABLE IF NOT EXISTS guide_favorites (
				user_id BIGINT NOT NULL,
				guide_id BIGINT NOT NULL,
				created_at VARCHAR(32) NOT NULL DEFAULT '',
				PRIMARY KEY (user_id, guide_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		if _, err := m.db.Exec(query); err != nil {
			return fmt.Errorf("创建收藏表失败（尝试了 SQLite 和 MySQL 格式）: %w", err)
		}
	}

	return nil
}

// Add 收藏攻略：主键去重，重复收藏静默幂等。
func (m *FavoriteModel) Add(userId, guideId int64) error {
	now := time.Now().Format(time.RFC3339)
	if _, err := m.db.Exec(
		`INSERT OR IGNORE INTO guide_favorites (user_id, guide_id, created_at) VALUES (?, ?, ?)`,
		userId, guideId, now,
	); err != nil {
		// SQLite 方言失败回落 MySQL（INSERT IGNORE 同语义）
		if _, err := m.db.Exec(
			`INSERT IGNORE INTO guide_favorites (user_id, guide_id, created_at) VALUES (?, ?, ?)`,
			userId, guideId, now,
		); err != nil {
			return fmt.Errorf("收藏攻略失败: %w", err)
		}
	}
	return nil
}

// Remove 取消收藏：删除不存在的收藏同样视为成功（幂等）。
func (m *FavoriteModel) Remove(userId, guideId int64) error {
	if _, err := m.db.Exec(
		`DELETE FROM guide_favorites WHERE user_id = ? AND guide_id = ?`,
		userId, guideId,
	); err != nil {
		return fmt.Errorf("取消收藏失败: %w", err)
	}
	return nil
}

func (m *FavoriteModel) Exists(userId, guideId int64) (bool, error) {
	var count int64
	if err := m.db.QueryRow(
		`SELECT COUNT(1) FROM guide_favorites WHERE user_id = ? AND guide_id = ?`,
		userId, guideId,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("查询收藏状态失败: %w", err)
	}
	return count > 0, nil
}

func (m *FavoriteModel) CountByGuide(guideId int64) (int64, error) {
	var count int64
	if err := m.db.QueryRow(
		`SELECT COUNT(1) FROM guide_favorites WHERE guide_id = ?`,
		guideId,
	).Scan(&count); err != nil {
		return 0, fmt.Errorf("统计收藏数失败: %w", err)
	}
	return count, nil
}

// favoriteGuideColumns 与 guideColumns 同列序，但限定表别名 g——
// 两表都有 created_at，裸列名在 JOIN 下会歧义。
const favoriteGuideColumns = "g.id, g.game_id, g.game_title, g.title, g.content, g.format, g.summary, " +
	"g.cover_image, g.author_id, g.author_name, g.tags, g.views, g.likes, g.is_published, g.created_at, g.updated_at"

// ListByUser 按收藏时间新→旧列出用户收藏的攻略，附带总数。
func (m *FavoriteModel) ListByUser(userId int64, limit, offset int) ([]*Guide, int, error) {
	var total int
	if err := m.db.QueryRow(
		`SELECT COUNT(1) FROM guide_favorites WHERE user_id = ?`,
		userId,
	).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计收藏总数失败: %w", err)
	}

	rows, err := m.db.Query(
		`SELECT `+favoriteGuideColumns+` FROM guide_favorites f
		 JOIN guides g ON f.guide_id = g.id
		 WHERE f.user_id = ?
		 ORDER BY f.created_at DESC, f.guide_id DESC
		 LIMIT ? OFFSET ?`,
		userId, limit, offset,
	)
	if err != nil {
		return nil, 0, fmt.Errorf("查询收藏列表失败: %w", err)
	}
	defer rows.Close()

	var guides []*Guide
	for rows.Next() {
		guide := &Guide{}
		var tags string
		var published int64
		if err := rows.Scan(&guide.Id, &guide.GameId, &guide.GameTitle, &guide.Title,
			&guide.Content, &guide.Format, &guide.Summary, &guide.CoverImage, &guide.AuthorId,
			&guide.AuthorName, &tags, &guide.Views, &guide.Likes, &published,
			&guide.CreatedAt, &guide.UpdatedAt); err != nil {
			return nil, 0, fmt.Errorf("读取收藏行失败: %w", err)
		}
		if guide.Tags, err = decodeStringList(tags); err != nil {
			return nil, 0, err
		}
		guide.IsPublished = published != 0
		guides = append(guides, guide)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历收藏行失败: %w", err)
	}
	return guides, total, nil
}
