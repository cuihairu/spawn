package model

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/tappi/tappi/services/community/internal/cache"
	"github.com/tappi/tappi/services/community/internal/types"
)

// 缓存参数：进程内 TTL+LRU（设计决策见 docs/development-guide.md
// 「数据库集成 → 设计决策」）。60s TTL 同时兜底任何失效遗漏的最终一致性。
const (
	defaultTopicCacheTTL = 60 * time.Second
	defaultTopicCacheMax = 4096
)

// TopicModel 话题数据访问层（MySQL/SQLite 双驱动，表结构见 CreateTopicsTable）。
// Get 读进程内缓存；Create/IncrementXxx 走统一失效点；不做负缓存。
// List 每次全量读取后应用侧过滤——话题为数十行量级，且 keyword 大小写
// 不敏感匹配与分页钳制语义需与原文件仓保持一致，不下推 SQL。
// 单实例部署 + SQLite 文件库/内存库的多连接隔离：连接池钳制为 1，
// 串行化数据库访问（写速率低，非瓶颈）。
type TopicModel struct {
	db    *sql.DB
	cache *cache.Cache[string, types.Topic]
}

// TopicStore 话题仓储接口：ServiceContext 面向接口依赖，测试可注入故障实现。
type TopicStore interface {
	Get(id int64) (*types.Topic, error)
	Create(req *types.CreateTopicReq) (*types.Topic, error)
	List(keyword string, isOfficial bool, limit, offset int64) ([]types.Topic, int64)
	IncrementPostCount(topicId, delta int64) error
	IncrementFollowerCount(topicId, delta int64) error
}

var _ TopicStore = (*TopicModel)(nil)

// NewTopicModel 创建话题模型（缓存默认开启；连接池钳制为 1）
func NewTopicModel(db *sql.DB) *TopicModel {
	db.SetMaxOpenConns(1)
	return &TopicModel{
		db:    db,
		cache: cache.New[string, types.Topic](defaultTopicCacheTTL, defaultTopicCacheMax),
	}
}

const topicColumns = "id, name, description, icon, cover_image, is_official, post_count, follower_count, created_at, updated_at"

// CreateTopicsTable 创建话题表（开发使用）。先尝试 SQLite 方言，失败回落
// MySQL（与 user-service.CreateUsersTable 同款双格式策略）。
func (m *TopicModel) CreateTopicsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS topics (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			name VARCHAR(128) NOT NULL,
			description VARCHAR(1024) NOT NULL DEFAULT '',
			icon VARCHAR(512) NOT NULL DEFAULT '',
			cover_image VARCHAR(512) NOT NULL DEFAULT '',
			is_official INTEGER NOT NULL DEFAULT 0,
			post_count INTEGER NOT NULL DEFAULT 0,
			follower_count INTEGER NOT NULL DEFAULT 0,
			created_at VARCHAR(32) NOT NULL,
			updated_at VARCHAR(32) NOT NULL
		)
	`

	if _, err := m.db.Exec(query); err != nil {
		query = `
			CREATE TABLE IF NOT EXISTS topics (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				name VARCHAR(128) NOT NULL,
				description VARCHAR(1024) NOT NULL DEFAULT '',
				icon VARCHAR(512) NOT NULL DEFAULT '',
				cover_image VARCHAR(512) NOT NULL DEFAULT '',
				is_official TINYINT NOT NULL DEFAULT 0,
				post_count INT NOT NULL DEFAULT 0,
				follower_count INT NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		if _, err := m.db.Exec(query); err != nil {
			return fmt.Errorf("创建话题表失败（尝试了 SQLite 和 MySQL 格式）: %w", err)
		}
	}

	return nil
}

// Seed 按给定条目写入话题（显式保留 id，跳过 nil 与非正 id），
// 供内嵌种子初始化与测试装载。
func (m *TopicModel) Seed(topics []*types.Topic) error {
	for _, topic := range topics {
		if topic == nil || topic.Id <= 0 {
			continue
		}
		if _, err := m.db.Exec(
			`INSERT INTO topics (`+topicColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			topic.Id, topic.Name, topic.Description, topic.Icon, topic.CoverImage,
			boolToInt(topic.IsOfficial), topic.PostCount, topic.FollowerCount,
			topic.CreatedAt, topic.UpdatedAt,
		); err != nil {
			return fmt.Errorf("写入话题失败: %w", err)
		}
	}
	return nil
}

// SeedIfEmpty 表为空时写入内嵌种子数据（首次启动初始化演示内容）。
func (m *TopicModel) SeedIfEmpty() error {
	var count int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM topics`).Scan(&count); err != nil {
		return fmt.Errorf("统计话题数失败: %w", err)
	}
	if count > 0 {
		return nil
	}
	return m.Seed(defaultSeedTopics())
}

func scanTopic(scan func(dest ...interface{}) error) (*types.Topic, error) {
	t := &types.Topic{}
	var official int64
	if err := scan(&t.Id, &t.Name, &t.Description, &t.Icon, &t.CoverImage,
		&official, &t.PostCount, &t.FollowerCount, &t.CreatedAt, &t.UpdatedAt); err != nil {
		return nil, err
	}
	t.IsOfficial = official != 0
	return t, nil
}

func topicKey(id int64) string { return "id:" + strconv.FormatInt(id, 10) }

// getFromDB 绕过缓存直查数据库（写路径须基于库内真值）。
func (m *TopicModel) getFromDB(id int64) (*types.Topic, error) {
	t, err := scanTopic(m.db.QueryRow(`SELECT `+topicColumns+` FROM topics WHERE id = ?`, id).Scan)
	if err == sql.ErrNoRows {
		return nil, ErrTopicNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询话题失败: %w", err)
	}
	return t, nil
}

// Get 按 id 查找话题（读缓存）。未命中返回 ErrTopicNotFound 哨兵。
func (m *TopicModel) Get(id int64) (*types.Topic, error) {
	key := topicKey(id)
	if cached, ok := m.cache.Get(key); ok {
		t := cached // Topic 无切片字段，值拷贝即隔离
		return &t, nil
	}

	t, err := m.getFromDB(id)
	if err != nil {
		return nil, err
	}

	m.cache.Set(key, *t)
	return t, nil
}

// Create 新建话题：TrimSpace 各字段、计数归零、非官方，自增主键落库，
// 失效新行缓存键（防御同 id 重建路径）。
func (m *TopicModel) Create(req *types.CreateTopicReq) (*types.Topic, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	t := &types.Topic{
		Name:          strings.TrimSpace(req.Name),
		Description:   strings.TrimSpace(req.Description),
		Icon:          strings.TrimSpace(req.Icon),
		CoverImage:    strings.TrimSpace(req.CoverImage),
		PostCount:     0,
		FollowerCount: 0,
		IsOfficial:    false,
		CreatedAt:     now,
		UpdatedAt:     now,
	}

	result, err := m.db.Exec(
		`INSERT INTO topics (name, description, icon, cover_image, is_official,
			post_count, follower_count, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.Name, t.Description, t.Icon, t.CoverImage, boolToInt(t.IsOfficial),
		t.PostCount, t.FollowerCount, t.CreatedAt, t.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("写入话题失败: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("获取话题ID失败: %w", err)
	}
	t.Id = id

	m.cache.Delete(topicKey(id))
	return t, nil
}

// List 按关键词（名称/描述，大小写不敏感）与官方标记过滤话题，分页钳制
// （offset<0 → 0；limit<=0 → 20）与原文件仓一致。
func (m *TopicModel) List(keyword string, isOfficial bool, limit, offset int64) ([]types.Topic, int64) {
	rows, err := m.db.Query(`SELECT ` + topicColumns + ` FROM topics ORDER BY id`)
	if err != nil {
		return nil, 0
	}
	defer rows.Close()

	kw := strings.ToLower(strings.TrimSpace(keyword))
	var filtered []types.Topic
	for rows.Next() {
		t, err := scanTopic(rows.Scan)
		if err != nil {
			return nil, 0
		}
		if isOfficial && !t.IsOfficial {
			continue
		}
		if kw != "" {
			if !strings.Contains(strings.ToLower(t.Name), kw) && !strings.Contains(strings.ToLower(t.Description), kw) {
				continue
			}
		}
		filtered = append(filtered, *t)
	}
	if err := rows.Err(); err != nil {
		return nil, 0
	}

	total := int64(len(filtered))
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 20
	}

	start := int(offset)
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + int(limit)
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[start:end], total
}

// IncrementPostCount 帖子计数增减（下限钳制 0，UpdatedAt 刷新）。
func (m *TopicModel) IncrementPostCount(topicId, delta int64) error {
	return m.incrementCount(`post_count`, topicId, delta)
}

// IncrementFollowerCount 关注者计数增减（下限钳制 0，UpdatedAt 刷新）。
func (m *TopicModel) IncrementFollowerCount(topicId, delta int64) error {
	return m.incrementCount(`follower_count`, topicId, delta)
}

func (m *TopicModel) incrementCount(column string, topicId, delta int64) error {
	result, err := m.db.Exec(
		`UPDATE topics SET `+column+` = CASE WHEN `+column+` + ? < 0 THEN 0 ELSE `+column+` + ? END,
		 updated_at = ? WHERE id = ?`,
		delta, delta, time.Now().UTC().Format(time.RFC3339), topicId,
	)
	if err != nil {
		return fmt.Errorf("更新话题计数失败: %w", err)
	}
	if n, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("确认话题存在失败: %w", err)
	} else if n == 0 {
		return ErrTopicNotFound
	}

	m.cache.Delete(topicKey(topicId))
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// defaultSeedTopics 与原文件仓种子一致的两条演示话题（时间戳取启动时刻）。
func defaultSeedTopics() []*types.Topic {
	now := time.Now().UTC().Format(time.RFC3339)
	return []*types.Topic{
		{
			Id:          1,
			Name:        "《黑神话：悟空》",
			Description: "讨论黑神话相关的一切：剧情、战斗、装备与周边",
			IsOfficial:  true,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			Id:          2,
			Name:        "开放世界",
			Description: "探索、支线、风景、任务路线分享",
			IsOfficial:  false,
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}
}
