package model

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/tappi/tappi/services/community/internal/cache"
	"github.com/tappi/tappi/services/community/internal/types"
)

// 缓存参数：进程内 TTL+LRU（设计决策见 docs/development-guide.md
// 「数据库集成 → 设计决策」）。60s TTL 同时兜底任何失效遗漏的最终一致性。
const (
	defaultPostCacheTTL = 60 * time.Second
	defaultPostCacheMax = 4096
)

// PostModel 帖子数据访问层（MySQL/SQLite 双驱动，表结构见 CreatePostsTable）。
// 软删语义：Delete 置 status='deleted'，行保留（id 不复用由软删天然保证）。
// Get 读进程内缓存（值副本隔离）；写路径统一失效点；不做负缓存。
// List/Hot 每次全量读取后应用侧过滤排序（含 hotScore 时间衰减），不下推 SQL。
// 单实例部署 + SQLite 文件库/内存库的多连接隔离：连接池钳制为 1。
type PostModel struct {
	db    *sql.DB
	cache *cache.Cache[string, types.Post]
}

// PostStore 帖子仓储接口：ServiceContext 面向接口依赖，测试可注入故障实现。
type PostStore interface {
	Create(topicId, authorId int64, authorName string, req *types.CreatePostReq) (*types.Post, error)
	Get(id int64) (*types.Post, error)
	IncrementViews(id int64) error
	Update(id, requesterId int64, req *types.UpdatePostReq) (*types.Post, error)
	Delete(id, requesterId int64) error
	Like(id int64) (*types.Post, error)
	Share(id int64) (*types.Post, error)
	List(filter PostListFilter) ([]types.Post, int64)
	Hot(limit int64) []types.Post
}

var _ PostStore = (*PostModel)(nil)

// NewPostModel 创建帖子模型（缓存默认开启；连接池钳制为 1）
func NewPostModel(db *sql.DB) *PostModel {
	db.SetMaxOpenConns(1)
	return &PostModel{
		db:    db,
		cache: cache.New[string, types.Post](defaultPostCacheTTL, defaultPostCacheMax),
	}
}

const postColumns = "id, topic_id, author_id, author_name, title, content, images, type, tags, view_count, like_count, comment_count, share_count, is_pinned, is_hot, status, created_at, updated_at"

// CreatePostsTable 创建帖子表（开发使用）。先尝试 SQLite 方言，失败回落
// MySQL（与 user-service.CreateUsersTable 同款双格式策略）。
func (m *PostModel) CreatePostsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS posts (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			topic_id INTEGER NOT NULL DEFAULT 0,
			author_id INTEGER NOT NULL DEFAULT 0,
			author_name VARCHAR(64) NOT NULL DEFAULT '',
			title VARCHAR(256) NOT NULL,
			content TEXT NOT NULL,
			images TEXT NOT NULL,
			type VARCHAR(32) NOT NULL,
			tags TEXT NOT NULL,
			view_count INTEGER NOT NULL DEFAULT 0,
			like_count INTEGER NOT NULL DEFAULT 0,
			comment_count INTEGER NOT NULL DEFAULT 0,
			share_count INTEGER NOT NULL DEFAULT 0,
			is_pinned INTEGER NOT NULL DEFAULT 0,
			is_hot INTEGER NOT NULL DEFAULT 0,
			status VARCHAR(16) NOT NULL DEFAULT 'published',
			created_at VARCHAR(32) NOT NULL,
			updated_at VARCHAR(32) NOT NULL
		)
	`

	if _, err := m.db.Exec(query); err != nil {
		query = `
			CREATE TABLE IF NOT EXISTS posts (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				topic_id BIGINT NOT NULL DEFAULT 0,
				author_id BIGINT NOT NULL DEFAULT 0,
				author_name VARCHAR(64) NOT NULL DEFAULT '',
				title VARCHAR(256) NOT NULL,
				content TEXT NOT NULL,
				images TEXT NOT NULL,
				type VARCHAR(32) NOT NULL,
				tags TEXT NOT NULL,
				view_count INT NOT NULL DEFAULT 0,
				like_count INT NOT NULL DEFAULT 0,
				comment_count INT NOT NULL DEFAULT 0,
				share_count INT NOT NULL DEFAULT 0,
				is_pinned TINYINT NOT NULL DEFAULT 0,
				is_hot TINYINT NOT NULL DEFAULT 0,
				status VARCHAR(16) NOT NULL DEFAULT 'published',
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		if _, err := m.db.Exec(query); err != nil {
			return fmt.Errorf("创建帖子表失败（尝试了 SQLite 和 MySQL 格式）: %w", err)
		}
	}

	return nil
}

// Seed 按给定条目写入帖子（显式保留 id，跳过 nil 与非正 id），
// 供内嵌种子初始化与测试装载。
func (m *PostModel) Seed(posts []*types.Post) error {
	for _, post := range posts {
		if post == nil || post.Id <= 0 {
			continue
		}
		if _, err := m.db.Exec(
			`INSERT INTO posts (`+postColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			post.Id, post.TopicId, post.AuthorId, post.AuthorName, post.Title, post.Content,
			encodeStringList(post.Images), post.Type, encodeStringList(post.Tags),
			post.ViewCount, post.LikeCount, post.CommentCount, post.ShareCount,
			boolToInt(post.IsPinned), boolToInt(post.IsHot), post.Status,
			post.CreatedAt, post.UpdatedAt,
		); err != nil {
			return fmt.Errorf("写入帖子失败: %w", err)
		}
	}
	return nil
}

// SeedIfEmpty 表为空时写入内嵌种子数据（首次启动初始化演示内容）。
func (m *PostModel) SeedIfEmpty() error {
	var count int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM posts`).Scan(&count); err != nil {
		return fmt.Errorf("统计帖子数失败: %w", err)
	}
	if count > 0 {
		return nil
	}
	return m.Seed(defaultSeedPosts())
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
		return nil, fmt.Errorf("解析帖子数组字段失败: %w", err)
	}
	return list, nil
}

// clonePost 深拷贝 Post 值（切片字段单独复制，杜绝别名共享）。
func clonePost(p types.Post) types.Post {
	p.Images = append([]string(nil), p.Images...)
	p.Tags = append([]string(nil), p.Tags...)
	return p
}

func postKey(id int64) string { return "id:" + strconv.FormatInt(id, 10) }

// scanPost 行扫描：数组列解码 + 布尔列换算。
func scanPost(scan func(dest ...interface{}) error) (*types.Post, error) {
	p := &types.Post{}
	var images, tags string
	var pinned, hot int64
	if err := scan(&p.Id, &p.TopicId, &p.AuthorId, &p.AuthorName, &p.Title, &p.Content,
		&images, &p.Type, &tags, &p.ViewCount, &p.LikeCount, &p.CommentCount,
		&p.ShareCount, &pinned, &hot, &p.Status, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return nil, err
	}
	var err error
	if p.Images, err = decodeStringList(images); err != nil {
		return nil, err
	}
	if p.Tags, err = decodeStringList(tags); err != nil {
		return nil, err
	}
	p.IsPinned = pinned != 0
	p.IsHot = hot != 0
	return p, nil
}

// getFromDB 绕过缓存直查数据库（写路径须基于库内真值）。
func (m *PostModel) getFromDB(id int64) (*types.Post, error) {
	p, err := scanPost(m.db.QueryRow(`SELECT `+postColumns+` FROM posts WHERE id = ?`, id).Scan)
	if err == sql.ErrNoRows {
		return nil, ErrPostNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询帖子失败: %w", err)
	}
	if p.Status == "deleted" {
		return nil, ErrPostNotFound
	}
	return p, nil
}

// Get 按 id 查找帖子（读缓存）。已删除与不存在同样返回 ErrPostNotFound 哨兵。
func (m *PostModel) Get(id int64) (*types.Post, error) {
	key := postKey(id)
	if cached, ok := m.cache.Get(key); ok {
		p := clonePost(cached) // 值副本：切片字段深拷，调用方改动不回灌缓存
		return &p, nil
	}

	p, err := m.getFromDB(id)
	if err != nil {
		return nil, err
	}

	m.cache.Set(key, clonePost(*p))
	return p, nil
}

// Create 新建帖子：TrimSpace 文本、type 默认 discussion、计数归零、published，
// 自增主键落库，失效新行缓存键（防御同 id 重建路径）。
func (m *PostModel) Create(topicId, authorId int64, authorName string, req *types.CreatePostReq) (*types.Post, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	postType := strings.TrimSpace(req.Type)
	if postType == "" {
		postType = "discussion"
	}

	p := &types.Post{
		TopicId:    topicId,
		AuthorId:   authorId,
		AuthorName: strings.TrimSpace(authorName),
		Title:      strings.TrimSpace(req.Title),
		Content:    strings.TrimSpace(req.Content),
		Images:     req.Images,
		Type:       postType,
		Tags:       req.Tags,
		Status:     "published",
		CreatedAt:  now,
		UpdatedAt:  now,
	}

	result, err := m.db.Exec(
		`INSERT INTO posts (topic_id, author_id, author_name, title, content, images,
			type, tags, view_count, like_count, comment_count, share_count,
			is_pinned, is_hot, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.TopicId, p.AuthorId, p.AuthorName, p.Title, p.Content,
		encodeStringList(p.Images), p.Type, encodeStringList(p.Tags),
		0, 0, 0, 0, 0, 0, p.Status, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("写入帖子失败: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("获取帖子ID失败: %w", err)
	}
	p.Id = id

	m.cache.Delete(postKey(id))
	return p, nil
}

// IncrementViews 浏览数自增（已删除 → ErrPostNotFound）。
func (m *PostModel) IncrementViews(id int64) error {
	p, err := m.getFromDB(id)
	if err != nil {
		return err
	}
	if _, err := m.db.Exec(`UPDATE posts SET view_count = view_count + 1 WHERE id = ?`, p.Id); err != nil {
		return fmt.Errorf("帖子浏览数自增失败: %w", err)
	}

	m.cache.Delete(postKey(id))
	return nil
}

// Update 作者本人可改：title/content TrimSpace 非空才生效，images/tags
// 非 nil 即生效；UpdatedAt 刷新。先绕缓存读库内真值再改。
func (m *PostModel) Update(id, requesterId int64, req *types.UpdatePostReq) (*types.Post, error) {
	p, err := m.getFromDB(id)
	if err != nil {
		return nil, err
	}
	if p.AuthorId != requesterId {
		return nil, ErrPermissionDenied
	}

	if title := strings.TrimSpace(req.Title); title != "" {
		p.Title = title
	}
	if content := strings.TrimSpace(req.Content); content != "" {
		p.Content = content
	}
	if req.Images != nil {
		p.Images = req.Images
	}
	if req.Tags != nil {
		p.Tags = req.Tags
	}
	p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)

	if _, err := m.db.Exec(
		`UPDATE posts SET title = ?, content = ?, images = ?, tags = ?, updated_at = ? WHERE id = ?`,
		p.Title, p.Content, encodeStringList(p.Images), encodeStringList(p.Tags),
		p.UpdatedAt, id,
	); err != nil {
		return nil, fmt.Errorf("更新帖子失败: %w", err)
	}

	m.cache.Delete(postKey(id))
	return p, nil
}

// Delete 软删：status 置 deleted（作者本人），行保留。
func (m *PostModel) Delete(id, requesterId int64) error {
	p, err := m.getFromDB(id)
	if err != nil {
		return err
	}
	if p.AuthorId != requesterId {
		return ErrPermissionDenied
	}

	if _, err := m.db.Exec(
		`UPDATE posts SET status = 'deleted', updated_at = ? WHERE id = ?`,
		time.Now().UTC().Format(time.RFC3339), id,
	); err != nil {
		return fmt.Errorf("删除帖子失败: %w", err)
	}

	m.cache.Delete(postKey(id))
	return nil
}

// Like 点赞并返回最新帖子。
func (m *PostModel) Like(id int64) (*types.Post, error) {
	if _, err := m.getFromDB(id); err != nil {
		return nil, err
	}
	if _, err := m.db.Exec(`UPDATE posts SET like_count = like_count + 1 WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("点赞帖子失败: %w", err)
	}

	p, err := m.getFromDB(id)
	if err != nil {
		return nil, err
	}

	m.cache.Delete(postKey(id))
	return p, nil
}

// Share 分享计数自增并返回最新帖子。
func (m *PostModel) Share(id int64) (*types.Post, error) {
	if _, err := m.getFromDB(id); err != nil {
		return nil, err
	}
	if _, err := m.db.Exec(`UPDATE posts SET share_count = share_count + 1 WHERE id = ?`, id); err != nil {
		return nil, fmt.Errorf("帖子分享计数自增失败: %w", err)
	}

	p, err := m.getFromDB(id)
	if err != nil {
		return nil, err
	}

	m.cache.Delete(postKey(id))
	return p, nil
}

// PostListFilter 列表过滤条件（与原文件仓字段一致）。
type PostListFilter struct {
	TopicId  int64
	AuthorId int64
	Type     string
	Status   string
	IsHot    bool
	Limit    int64
	Offset   int64
}

// List 按过滤条件列出帖子（排除软删行；status 默认 published；分页钳制
// offset<0 → 0、limit<=0 → 20，与原文件仓一致）。全量读取后应用侧过滤。
func (m *PostModel) List(filter PostListFilter) ([]types.Post, int64) {
	posts := m.allVisible()
	if posts == nil {
		return nil, 0
	}

	status := strings.TrimSpace(filter.Status)
	if status == "" {
		status = "published"
	}

	var filtered []types.Post
	for _, p := range posts {
		if status != "" && p.Status != status {
			continue
		}
		if filter.TopicId > 0 && p.TopicId != filter.TopicId {
			continue
		}
		if filter.AuthorId > 0 && p.AuthorId != filter.AuthorId {
			continue
		}
		if t := strings.TrimSpace(filter.Type); t != "" && p.Type != t {
			continue
		}
		if filter.IsHot {
			if hotScore(p) <= 0 {
				continue
			}
		}
		filtered = append(filtered, p)
	}

	total := int64(len(filtered))
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	if filter.Limit <= 0 {
		filter.Limit = 20
	}

	start := int(filter.Offset)
	if start > len(filtered) {
		start = len(filtered)
	}
	end := start + int(filter.Limit)
	if end > len(filtered) {
		end = len(filtered)
	}

	return filtered[start:end], total
}

// Hot 热榜：published 候选按置顶优先 + hotScore 降序，limit<=0 回落 20，
// 返回 IsHot=true 的值副本。读失败返回 nil（与 List 的空集降级一致）。
func (m *PostModel) Hot(limit int64) []types.Post {
	posts := m.allVisible()
	if posts == nil {
		return nil
	}

	var candidates []types.Post
	for _, p := range posts {
		if p.Status != "published" {
			continue
		}
		candidates = append(candidates, p)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		pi, pj := &candidates[i], &candidates[j]
		if pi.IsPinned != pj.IsPinned {
			return pi.IsPinned
		}
		return hotScore(*pi) > hotScore(*pj)
	})

	if limit <= 0 {
		limit = 20
	}
	if int(limit) > len(candidates) {
		limit = int64(len(candidates))
	}

	out := make([]types.Post, 0, limit)
	for _, p := range candidates[:limit] {
		p.IsHot = true
		out = append(out, p)
	}
	return out
}

// allVisible 全量读取非软删帖子，按 id 升序——原文件仓为插入序，自增 id
// 恒单调，id 序与插入序严格一致，语义零漂移。读失败返回 nil（List/Hot
// 按空集降级，与原文件仓读错误行为一致）。
func (m *PostModel) allVisible() []types.Post {
	rows, err := m.db.Query(`SELECT ` + postColumns + ` FROM posts ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var posts []types.Post
	for rows.Next() {
		p, err := scanPost(rows.Scan)
		if err != nil {
			return nil
		}
		if p.Status == "deleted" {
			continue
		}
		posts = append(posts, *p)
	}
	if err := rows.Err(); err != nil {
		return nil
	}
	return posts
}

// hotScore 热度评分：互动加权后按帖龄时间衰减（分桶见下），与原文件仓一致。
func hotScore(p types.Post) float64 {
	base := float64(p.LikeCount*3 + p.CommentCount*5 + p.ShareCount*7 + p.ViewCount)

	createdAt, err := time.Parse(time.RFC3339, p.CreatedAt)
	if err != nil {
		return base
	}
	hours := time.Since(createdAt).Hours()

	decay := 1.0
	switch {
	case hours <= 24:
		decay = 1
	case hours <= 72:
		decay = 2
	case hours <= 24*7:
		decay = 4
	default:
		decay = 8
	}

	decay *= math.Max(1, hours/24)
	return base / decay
}

// defaultSeedPosts 与原文件仓种子一致的两条演示帖子（时间戳取启动时刻）。
func defaultSeedPosts() []*types.Post {
	now := time.Now().UTC().Format(time.RFC3339)
	return []*types.Post{
		{
			Id:         1,
			TopicId:    1,
			AuthorId:   1001,
			AuthorName: "demo",
			Title:      "开荒建议：先别急着刷装",
			Content:    "前期把基础动作和怪物招式学会，比盲目堆数值更重要……",
			Type:       "discussion",
			Tags:       []string{"新手", "开荒"},
			ViewCount:  120,
			LikeCount:  15,
			ShareCount: 3,
			Status:     "published",
			CreatedAt:  now,
			UpdatedAt:  now,
		},
		{
			Id:         2,
			TopicId:    2,
			AuthorId:   1002,
			AuthorName: "tester",
			Title:      "开放世界跑图路线分享",
			Content:    "这是一条相对舒服的前期跑图路线，兼顾资源与战斗难度……",
			Type:       "share",
			Tags:       []string{"跑图", "路线"},
			ViewCount:  340,
			LikeCount:  44,
			ShareCount: 9,
			Status:     "published",
			CreatedAt:  now,
			UpdatedAt:  now,
		},
	}
}
