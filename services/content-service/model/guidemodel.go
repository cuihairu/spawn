package model

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/tappi/tappi/services/content-service/internal/cache"
)

var ErrGuideNotFound = errors.New("guide not found")

type Guide struct {
	Id          int64    `json:"id"`
	GameId      string   `json:"game_id"`
	GameTitle   string   `json:"game_title"`
	Title       string   `json:"title"`
	Content     string   `json:"content"`
	Summary     string   `json:"summary"`
	CoverImage  string   `json:"cover_image"`
	AuthorId    int64    `json:"author_id"`
	AuthorName  string   `json:"author_name"`
	Tags        []string `json:"tags"`
	Views       int      `json:"views"`
	Likes       int      `json:"likes"`
	IsPublished bool     `json:"is_published"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

type GuideFilter struct {
	GameId        string
	AuthorId      int64
	Tag           string
	Page          int
	PageSize      int
	PublishedOnly bool
}

// 缓存参数：进程内 TTL+LRU（设计决策见 docs/development-guide.md
// 「数据库集成 → 设计决策」）。60s TTL 同时兜底任何失效遗漏的最终一致性。
const (
	defaultGuideCacheTTL = 60 * time.Second
	defaultGuideCacheMax = 4096
)

// GuideModel 攻略数据访问层（MySQL/SQLite 双驱动，表结构见 CreateGuidesTable）。
// Get 读进程内缓存；Create/Update/Publish/Like/IncrementViews 走统一失效点；
// 不做负缓存（未找到/错误不写入）。List 每次全量读取后应用侧过滤——攻略为
// 数百行量级，且分页钳制语义需与原内存仓保持一致，不下推 SQL。
// 单实例部署 + SQLite 文件库/内存库的多连接隔离：连接池钳制为 1，
// 串行化数据库访问（写速率低，非瓶颈）。
type GuideModel struct {
	db    *sql.DB
	cache *cache.Cache[string, Guide]
}

// GuideStore 攻略仓储接口：ServiceContext 面向接口依赖，测试可注入故障实现。
type GuideStore interface {
	List(filter GuideFilter) ([]*Guide, int)
	Get(id int64) (*Guide, error)
	Create(guide *Guide) (*Guide, error)
	Update(id int64, updates map[string]interface{}) (*Guide, error)
	Publish(id int64) error
	Like(id int64) (int, error)
	IncrementViews(id int64) error
}

var _ GuideStore = (*GuideModel)(nil)

// NewGuideModel 创建攻略模型（缓存默认开启；连接池钳制为 1）
func NewGuideModel(db *sql.DB) *GuideModel {
	db.SetMaxOpenConns(1)
	return &GuideModel{
		db:    db,
		cache: cache.New[string, Guide](defaultGuideCacheTTL, defaultGuideCacheMax),
	}
}

const guideColumns = "id, game_id, game_title, title, content, summary, cover_image, author_id, author_name, tags, views, likes, is_published, created_at, updated_at"

// CreateGuidesTable 创建攻略表（开发使用）。先尝试 SQLite 方言，失败回落
// MySQL（与 user-service.CreateUsersTable 同款双格式策略）。
func (m *GuideModel) CreateGuidesTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS guides (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			game_id VARCHAR(64) NOT NULL,
			game_title VARCHAR(256) NOT NULL,
			title VARCHAR(256) NOT NULL,
			content TEXT NOT NULL,
			summary VARCHAR(1024) NOT NULL,
			cover_image VARCHAR(512) NOT NULL,
			author_id INTEGER NOT NULL DEFAULT 0,
			author_name VARCHAR(64) NOT NULL,
			tags VARCHAR(512) NOT NULL,
			views INTEGER NOT NULL DEFAULT 0,
			likes INTEGER NOT NULL DEFAULT 0,
			is_published INTEGER NOT NULL DEFAULT 0,
			created_at VARCHAR(32) NOT NULL,
			updated_at VARCHAR(32) NOT NULL
		)
	`

	if _, err := m.db.Exec(query); err != nil {
		query = `
			CREATE TABLE IF NOT EXISTS guides (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				game_id VARCHAR(64) NOT NULL,
				game_title VARCHAR(256) NOT NULL,
				title VARCHAR(256) NOT NULL,
				content TEXT NOT NULL,
				summary VARCHAR(1024) NOT NULL,
				cover_image VARCHAR(512) NOT NULL,
				author_id BIGINT NOT NULL DEFAULT 0,
				author_name VARCHAR(64) NOT NULL,
				tags VARCHAR(512) NOT NULL,
				views INT NOT NULL DEFAULT 0,
				likes INT NOT NULL DEFAULT 0,
				is_published TINYINT NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		if _, err := m.db.Exec(query); err != nil {
			return fmt.Errorf("创建攻略表失败（尝试了 SQLite 和 MySQL 格式）: %w", err)
		}
	}

	return nil
}

// Seed 按给定条目写入攻略（显式保留 id，跳过 nil 与非正 id），
// 供内嵌种子初始化与测试装载。
func (m *GuideModel) Seed(guides []*Guide) error {
	for _, guide := range guides {
		if guide == nil || guide.Id <= 0 {
			continue
		}
		if err := m.insertRow(guide); err != nil {
			return err
		}
	}
	return nil
}

// SeedIfEmpty 表为空时写入内嵌种子数据（首次启动初始化演示内容）。
func (m *GuideModel) SeedIfEmpty() error {
	var count int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM guides`).Scan(&count); err != nil {
		return fmt.Errorf("统计攻略数失败: %w", err)
	}
	if count > 0 {
		return nil
	}
	return m.Seed(defaultSeedGuides())
}

func (m *GuideModel) insertRow(guide *Guide) error {
	// Seed 显式保留 id（自增序从最大 id 之后继续），与原内存仓装载语义一致
	_, err := m.db.Exec(
		`INSERT INTO guides (id, game_id, game_title, title, content, summary, cover_image,
			author_id, author_name, tags, views, likes, is_published, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		guide.Id, guide.GameId, guide.GameTitle, guide.Title, guide.Content, guide.Summary,
		guide.CoverImage, guide.AuthorId, guide.AuthorName,
		encodeStringList(guide.Tags), guide.Views, guide.Likes, boolToInt(guide.IsPublished),
		guide.CreatedAt, guide.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("写入攻略失败: %w", err)
	}
	return nil
}

// encodeStringList 序列化字符串数组为 JSON 文本（nil 归一为空数组）。
func encodeStringList(list []string) string {
	if list == nil {
		list = []string{}
	}
	b, _ := json.Marshal(list)
	return string(b)
}

// decodeStringList 反序列化 JSON 文本为字符串数组（空串视为空数组）。
func decodeStringList(raw string) ([]string, error) {
	if raw == "" {
		return []string{}, nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, fmt.Errorf("解析攻略数组字段失败: %w", err)
	}
	return list, nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// cloneGuide 深拷贝 Guide 值（切片字段单独复制，杜绝别名共享）。
func cloneGuide(g Guide) Guide {
	g.Tags = append([]string(nil), g.Tags...)
	return g
}

func guideKey(id int64) string { return "id:" + strconv.FormatInt(id, 10) }

// getFromDB 绕过缓存直查数据库（写路径须基于库内真值）。
func (m *GuideModel) getFromDB(id int64) (*Guide, error) {
	guide := &Guide{}
	var tags string
	var published int64
	err := m.db.QueryRow(`SELECT `+guideColumns+` FROM guides WHERE id = ?`, id).
		Scan(&guide.Id, &guide.GameId, &guide.GameTitle, &guide.Title, &guide.Content,
			&guide.Summary, &guide.CoverImage, &guide.AuthorId, &guide.AuthorName,
			&tags, &guide.Views, &guide.Likes, &published, &guide.CreatedAt, &guide.UpdatedAt)

	if err == sql.ErrNoRows {
		return nil, ErrGuideNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询攻略失败: %w", err)
	}
	if guide.Tags, err = decodeStringList(tags); err != nil {
		return nil, err
	}
	guide.IsPublished = published != 0
	return guide, nil
}

// Get 按 id 查找攻略（读缓存）。未命中返回 ErrGuideNotFound 哨兵。
func (m *GuideModel) Get(id int64) (*Guide, error) {
	key := guideKey(id)
	if cached, ok := m.cache.Get(key); ok {
		guide := cloneGuide(cached) // 值副本：切片字段深拷，调用方改动不回灌缓存
		return &guide, nil
	}

	guide, err := m.getFromDB(id)
	if err != nil {
		return nil, err
	}

	m.cache.Set(key, cloneGuide(*guide))
	return guide, nil
}

// Create 新建攻略：重置计数与发布态、写 RFC3339 时间戳，自增主键落库，
// 失效新行缓存键（负缓存不存在，通常无键可删；防御同 id 重建路径）。
func (m *GuideModel) Create(guide *Guide) (*Guide, error) {
	if guide == nil {
		return nil, errors.New("guide payload required")
	}

	now := time.Now().Format(time.RFC3339)
	guide.Id = 0
	guide.CreatedAt = now
	guide.UpdatedAt = now
	guide.Views = 0
	guide.Likes = 0
	guide.IsPublished = false

	result, err := m.db.Exec(
		`INSERT INTO guides (game_id, game_title, title, content, summary, cover_image,
			author_id, author_name, tags, views, likes, is_published, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		guide.GameId, guide.GameTitle, guide.Title, guide.Content, guide.Summary,
		guide.CoverImage, guide.AuthorId, guide.AuthorName,
		encodeStringList(guide.Tags), guide.Views, guide.Likes, boolToInt(guide.IsPublished),
		guide.CreatedAt, guide.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("写入攻略失败: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("获取攻略ID失败: %w", err)
	}
	guide.Id = id

	m.cache.Delete(guideKey(id))
	return guide, nil
}

// Update 按字段增量更新：title/content 非空才生效，summary/cover_image
// 传 string 即生效（允许清空），tags 仅接受 []string；UpdatedAt 刷新。
// 先绕缓存读库内真值再改，语义与原内存仓一致。
func (m *GuideModel) Update(id int64, updates map[string]interface{}) (*Guide, error) {
	guide, err := m.getFromDB(id)
	if err != nil {
		return nil, err
	}

	if title, ok := updates["title"].(string); ok && title != "" {
		guide.Title = title
	}
	if content, ok := updates["content"].(string); ok && content != "" {
		guide.Content = content
	}
	if summary, ok := updates["summary"].(string); ok {
		guide.Summary = summary
	}
	if coverImage, ok := updates["cover_image"].(string); ok {
		guide.CoverImage = coverImage
	}
	if tags, ok := updates["tags"].([]string); ok {
		guide.Tags = tags
	}

	guide.UpdatedAt = time.Now().Format(time.RFC3339)

	if _, err := m.db.Exec(
		`UPDATE guides SET title = ?, content = ?, summary = ?, cover_image = ?, tags = ?, updated_at = ?
		 WHERE id = ?`,
		guide.Title, guide.Content, guide.Summary, guide.CoverImage,
		encodeStringList(guide.Tags), guide.UpdatedAt, id,
	); err != nil {
		return nil, fmt.Errorf("更新攻略失败: %w", err)
	}

	m.cache.Delete(guideKey(id))
	return guide, nil
}

// Publish 发布攻略（is_published=1，UpdatedAt 刷新）。
func (m *GuideModel) Publish(id int64) error {
	result, err := m.db.Exec(
		`UPDATE guides SET is_published = 1, updated_at = ? WHERE id = ?`,
		time.Now().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("发布攻略失败: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("确认攻略存在失败: %w", err)
	} else if n == 0 {
		return ErrGuideNotFound
	}

	m.cache.Delete(guideKey(id))
	return nil
}

// Like 点赞并返回最新计数。
func (m *GuideModel) Like(id int64) (int, error) {
	result, err := m.db.Exec(`UPDATE guides SET likes = likes + 1 WHERE id = ?`, id)
	if err != nil {
		return 0, fmt.Errorf("点赞攻略失败: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return 0, fmt.Errorf("确认攻略存在失败: %w", err)
	} else if n == 0 {
		return 0, ErrGuideNotFound
	}

	var likes int
	if err := m.db.QueryRow(`SELECT likes FROM guides WHERE id = ?`, id).Scan(&likes); err != nil {
		return 0, fmt.Errorf("查询攻略点赞数失败: %w", err)
	}

	m.cache.Delete(guideKey(id))
	return likes, nil
}

// IncrementViews 浏览数自增。
func (m *GuideModel) IncrementViews(id int64) error {
	result, err := m.db.Exec(`UPDATE guides SET views = views + 1 WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("攻略浏览数自增失败: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("确认攻略存在失败: %w", err)
	} else if n == 0 {
		return ErrGuideNotFound
	}

	m.cache.Delete(guideKey(id))
	return nil
}

// all 全量读取攻略目录，按 id 升序——原内存仓为插入序，自增 id 恒单调，
// id 序与插入序严格一致，语义零漂移。
func (m *GuideModel) all() ([]*Guide, error) {
	rows, err := m.db.Query(`SELECT ` + guideColumns + ` FROM guides ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("查询攻略列表失败: %w", err)
	}
	defer rows.Close()

	var guides []*Guide
	for rows.Next() {
		guide := &Guide{}
		var tags string
		var published int64
		if err := rows.Scan(&guide.Id, &guide.GameId, &guide.GameTitle, &guide.Title,
			&guide.Content, &guide.Summary, &guide.CoverImage, &guide.AuthorId,
			&guide.AuthorName, &tags, &guide.Views, &guide.Likes, &published,
			&guide.CreatedAt, &guide.UpdatedAt); err != nil {
			return nil, fmt.Errorf("读取攻略行失败: %w", err)
		}
		if guide.Tags, err = decodeStringList(tags); err != nil {
			return nil, err
		}
		guide.IsPublished = published != 0
		guides = append(guides, guide)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历攻略行失败: %w", err)
	}
	return guides, nil
}

// List 按过滤条件列出攻略（应用侧过滤 + 分页钳制，非法 page/size 安全降级）。
func (m *GuideModel) List(filter GuideFilter) ([]*Guide, int) {
	guides, err := m.all()
	if err != nil {
		return nil, 0
	}

	var filtered []*Guide
	for _, guide := range guides {
		if guide == nil {
			continue
		}
		if !matchesGuide(guide, filter) {
			continue
		}
		filtered = append(filtered, guide)
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

	result := make([]*Guide, end-start)
	copy(result, filtered[start:end])

	return result, total
}

func matchesGuide(guide *Guide, filter GuideFilter) bool {
	if filter.GameId != "" && guide.GameId != filter.GameId {
		return false
	}

	if filter.PublishedOnly && !guide.IsPublished {
		return false
	}

	if filter.AuthorId > 0 && guide.AuthorId != filter.AuthorId {
		return false
	}

	if filter.Tag != "" {
		found := false
		for _, tag := range guide.Tags {
			if tag == filter.Tag {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return true
}

func defaultSeedGuides() []*Guide {
	return []*Guide{
		{
			Id:          1,
			GameId:      "game-elden-ring",
			GameTitle:   "Elden Ring",
			Title:       "新手入门指南：如何在交界地存活下来",
			Content:     "艾尔登法环对新手来说可能很有挑战性。本指南将帮助你了解基础机制、职业选择和前期区域探索技巧...",
			Summary:     "完整的新手生存指南，包含职业推荐和探索路线",
			CoverImage:  "https://images.tappi.dev/guide-elden-ring-1.jpg",
			AuthorId:    1001,
			AuthorName:  "魂系老猎人",
			Tags:        []string{"新手", "入门", "攻略"},
			Views:       15234,
			Likes:       892,
			IsPublished: true,
			CreatedAt:   "2024-01-15T10:30:00Z",
			UpdatedAt:   "2024-01-20T14:20:00Z",
		},
		{
			Id:          2,
			GameId:      "game-valorant",
			GameTitle:   "Valorant",
			Title:       "瓦罗兰特枪法训练完全指南",
			Content:     "想要提升你的枪法吗？本指南包含了详细的准星设置、练习方法和实战技巧...",
			Summary:     "从基础到进阶的枪法训练方法",
			CoverImage:  "https://images.tappi.dev/guide-valorant-1.jpg",
			AuthorId:    1002,
			AuthorName:  "FPS大师",
			Tags:        []string{"技巧", "训练", "枪法"},
			Views:       23451,
			Likes:       1456,
			IsPublished: true,
			CreatedAt:   "2024-01-18T09:15:00Z",
			UpdatedAt:   "2024-01-18T09:15:00Z",
		},
	}
}
