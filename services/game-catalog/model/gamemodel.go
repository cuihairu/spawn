package model

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/tappi/tappi/services/game-catalog/internal/cache"
)

var ErrGameNotFound = errors.New("game not found")

type Game struct {
	Id            string   `json:"id"`
	Title         string   `json:"title"`
	Description   string   `json:"description"`
	Genres        []string `json:"genres"`
	Platforms     []string `json:"platforms"`
	ReleaseDate   string   `json:"release_date"`
	Developer     string   `json:"developer"`
	Publisher     string   `json:"publisher"`
	Tags          []string `json:"tags"`
	Score         float64  `json:"score"`
	TrendingScore int      `json:"trending_score"`
	CoverImage    string   `json:"cover_image"`
}

type GameFilter struct {
	Keyword  string
	Genre    string
	Platform string
	Tag      string
	Limit    int
	Offset   int
	Sort     string
}

// 缓存参数：进程内 TTL+LRU（设计决策见 docs/development-guide.md
// 「数据库集成 → 设计决策」）。60s TTL 同时兜底任何失效遗漏的最终一致性。
const (
	defaultGameCacheTTL = 60 * time.Second
	defaultGameCacheMax = 4096
)

// GameModel 游戏数据访问层（MySQL/SQLite 双驱动，表结构见 CreateGamesTable）。
// Get 读进程内缓存；Create 走统一失效点；不做负缓存（未找到/错误不写入）。
// List/Featured/Recommend 每次全量读取后在应用侧过滤排序——游戏目录为数百行
// 量级，且排序/轮转语义需与原内存仓保持一致，不下推 SQL。
type GameModel struct {
	db    *sql.DB
	cache *cache.Cache[string, Game]
}

// GameStore 游戏仓储接口：ServiceContext 面向接口依赖，测试可注入故障实现。
type GameStore interface {
	List(filter GameFilter) ([]*Game, int)
	Get(id string) (*Game, error)
	Create(game *Game) (*Game, error)
	Featured(limit int) []*Game
	Recommend(userId string, genres []string, limit int) []*Game
}

var _ GameStore = (*GameModel)(nil)

// NewGameModel 创建游戏模型（缓存默认开启）
func NewGameModel(db *sql.DB) *GameModel {
	return &GameModel{
		db:    db,
		cache: cache.New[string, Game](defaultGameCacheTTL, defaultGameCacheMax),
	}
}

const gameColumns = "id, title, description, genres, platforms, tags, release_date, developer, publisher, score, trending_score, cover_image"

// CreateGamesTable 创建游戏表（开发使用）。先尝试 SQLite 方言，失败回落
// MySQL（与 user-service.CreateUsersTable 同款双格式策略）。
func (m *GameModel) CreateGamesTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS games (
			id VARCHAR(64) PRIMARY KEY,
			title VARCHAR(256) NOT NULL,
			description VARCHAR(2048) NOT NULL,
			genres VARCHAR(1024) NOT NULL,
			platforms VARCHAR(1024) NOT NULL,
			tags VARCHAR(1024) NOT NULL,
			release_date VARCHAR(32) NOT NULL,
			developer VARCHAR(256) NOT NULL,
			publisher VARCHAR(256) NOT NULL,
			score DOUBLE NOT NULL DEFAULT 0,
			trending_score INTEGER NOT NULL DEFAULT 0,
			cover_image VARCHAR(512) NOT NULL
		)
	`

	if _, err := m.db.Exec(query); err != nil {
		query = `
			CREATE TABLE IF NOT EXISTS games (
				id VARCHAR(64) PRIMARY KEY,
				title VARCHAR(256) NOT NULL,
				description VARCHAR(2048) NOT NULL,
				genres VARCHAR(1024) NOT NULL,
				platforms VARCHAR(1024) NOT NULL,
				tags VARCHAR(1024) NOT NULL,
				release_date VARCHAR(32) NOT NULL,
				developer VARCHAR(256) NOT NULL,
				publisher VARCHAR(256) NOT NULL,
				score DOUBLE NOT NULL DEFAULT 0,
				trending_score INT NOT NULL DEFAULT 0,
				cover_image VARCHAR(512) NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		if _, err := m.db.Exec(query); err != nil {
			return fmt.Errorf("创建游戏表失败（尝试了 SQLite 和 MySQL 格式）: %w", err)
		}
	}

	return nil
}

// Seed 按给定条目写入游戏（显式保留 id，跳过 nil 与空 id），
// 供内嵌种子初始化与测试装载。
func (m *GameModel) Seed(games []*Game) error {
	for _, game := range games {
		if game == nil || game.Id == "" {
			continue
		}
		if err := m.insertRow(game); err != nil {
			return err
		}
	}
	return nil
}

// SeedIfEmpty 表为空时写入内嵌种子数据（首次启动初始化演示目录）。
func (m *GameModel) SeedIfEmpty() error {
	var count int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM games`).Scan(&count); err != nil {
		return fmt.Errorf("统计游戏数失败: %w", err)
	}
	if count > 0 {
		return nil
	}
	return m.Seed(defaultSeedGames())
}

func (m *GameModel) insertRow(game *Game) error {
	_, err := m.db.Exec(
		`INSERT INTO games (`+gameColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		game.Id, game.Title, game.Description,
		encodeList(game.Genres), encodeList(game.Platforms), encodeList(game.Tags),
		game.ReleaseDate, game.Developer, game.Publisher,
		game.Score, game.TrendingScore, game.CoverImage,
	)
	if err != nil {
		return fmt.Errorf("写入游戏失败: %w", err)
	}
	return nil
}

// encodeList 序列化字符串数组为 JSON 文本（nil 归一为空数组）。
func encodeList(list []string) string {
	if list == nil {
		list = []string{}
	}
	b, _ := json.Marshal(list)
	return string(b)
}

// decodeList 反序列化 JSON 文本为字符串数组（空串视为空数组）。
func decodeList(raw string) ([]string, error) {
	if raw == "" {
		return []string{}, nil
	}
	var list []string
	if err := json.Unmarshal([]byte(raw), &list); err != nil {
		return nil, fmt.Errorf("解析游戏数组字段失败: %w", err)
	}
	return list, nil
}

// cloneGame 深拷贝 Game 值（切片字段单独复制，杜绝别名共享）。
func cloneGame(g Game) Game {
	g.Genres = append([]string(nil), g.Genres...)
	g.Platforms = append([]string(nil), g.Platforms...)
	g.Tags = append([]string(nil), g.Tags...)
	return g
}

func gameKeyID(id string) string { return "id:" + id }

// Get 按 id 查找游戏（读缓存）。未命中返回 ErrGameNotFound 哨兵。
func (m *GameModel) Get(id string) (*Game, error) {
	key := gameKeyID(id)
	if cached, ok := m.cache.Get(key); ok {
		game := cloneGame(cached) // 值副本：切片字段深拷，调用方改动不回灌缓存
		return &game, nil
	}

	game := &Game{}
	var genres, platforms, tags string
	err := m.db.QueryRow(`SELECT `+gameColumns+` FROM games WHERE id = ?`, id).
		Scan(&game.Id, &game.Title, &game.Description, &genres, &platforms, &tags,
			&game.ReleaseDate, &game.Developer, &game.Publisher,
			&game.Score, &game.TrendingScore, &game.CoverImage)

	if err == sql.ErrNoRows {
		return nil, ErrGameNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询游戏失败: %w", err)
	}

	if game.Genres, err = decodeList(genres); err != nil {
		return nil, err
	}
	if game.Platforms, err = decodeList(platforms); err != nil {
		return nil, err
	}
	if game.Tags, err = decodeList(tags); err != nil {
		return nil, err
	}

	m.cache.Set(key, cloneGame(*game))
	return game, nil
}

// Create 新建游戏：分配 uuid 主键并落库，失效新行缓存键
// （负缓存不存在，通常无键可删；防御同 id 重建路径）。
func (m *GameModel) Create(game *Game) (*Game, error) {
	if game == nil {
		return nil, errors.New("game payload required")
	}

	game.Id = fmt.Sprintf("game-%s", uuid.NewString())
	if err := m.insertRow(game); err != nil {
		return nil, err
	}

	m.cache.Delete(gameKeyID(game.Id))
	return game, nil
}

// all 全量读取游戏目录，按 id 排序保证跨方言确定性
// （原内存仓为插入序；并列趋势分的排序由此回退到 id 序）。
func (m *GameModel) all() ([]*Game, error) {
	rows, err := m.db.Query(`SELECT ` + gameColumns + ` FROM games ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("查询游戏列表失败: %w", err)
	}
	defer rows.Close()

	var games []*Game
	for rows.Next() {
		game := &Game{}
		var genres, platforms, tags string
		if err := rows.Scan(&game.Id, &game.Title, &game.Description, &genres, &platforms, &tags,
			&game.ReleaseDate, &game.Developer, &game.Publisher,
			&game.Score, &game.TrendingScore, &game.CoverImage); err != nil {
			return nil, fmt.Errorf("读取游戏行失败: %w", err)
		}
		if game.Genres, err = decodeList(genres); err != nil {
			return nil, err
		}
		if game.Platforms, err = decodeList(platforms); err != nil {
			return nil, err
		}
		if game.Tags, err = decodeList(tags); err != nil {
			return nil, err
		}
		games = append(games, game)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("遍历游戏行失败: %w", err)
	}
	return games, nil
}

// List 按过滤条件列出游戏（应用侧过滤 + 稳定排序 + 分页钳制）。
func (m *GameModel) List(filter GameFilter) ([]*Game, int) {
	games, err := m.all()
	if err != nil {
		return nil, 0
	}

	var filtered []*Game
	for _, game := range games {
		if !matches(game, filter) {
			continue
		}
		filtered = append(filtered, game)
	}

	sortGames(filtered, filter.Sort)

	total := len(filtered)
	start := clamp(filter.Offset, 0, total)
	end := clamp(filter.Offset+filter.Limit, 0, total)
	if start > end {
		start = end
	}
	result := make([]*Game, end-start)
	copy(result, filtered[start:end])

	return result, total
}

// Featured 按趋势分降序列出热门游戏；limit<=0 或超界时返回全量。
func (m *GameModel) Featured(limit int) []*Game {
	games, err := m.all()
	if err != nil {
		return nil
	}

	cloned := append([]*Game(nil), games...)
	sort.SliceStable(cloned, func(i, j int) bool {
		return cloned[i].TrendingScore > cloned[j].TrendingScore
	})

	if limit <= 0 || limit > len(cloned) {
		limit = len(cloned)
	}

	return cloned[:limit]
}

// Recommend 按题材池轮转推荐：userId 经 FNV 哈希决定起点，
// 同一 userId 结果确定；题材无命中时回落全量池；limit<=0 取 5。
func (m *GameModel) Recommend(userId string, genres []string, limit int) []*Game {
	games, err := m.all()
	if err != nil {
		return nil
	}

	if limit <= 0 {
		limit = 5
	}

	var pool []*Game
	if len(genres) == 0 {
		pool = append(pool, games...)
	} else {
		for _, game := range games {
			if containsAny(game.Genres, genres) {
				pool = append(pool, game)
			}
		}
	}

	if len(pool) == 0 {
		pool = append(pool, games...)
	}

	sort.SliceStable(pool, func(i, j int) bool {
		if pool[i].TrendingScore == pool[j].TrendingScore {
			return pool[i].Score > pool[j].Score
		}
		return pool[i].TrendingScore > pool[j].TrendingScore
	})

	start := 0
	if userId != "" && len(pool) > 0 {
		hash := fnv.New32a()
		hash.Write([]byte(userId))
		start = int(hash.Sum32()) % len(pool)
	}

	result := make([]*Game, 0, min(limit, len(pool)))
	for i := 0; i < len(pool) && len(result) < limit; i++ {
		idx := (start + i) % len(pool)
		result = append(result, pool[idx])
	}

	return result
}

func matches(game *Game, filter GameFilter) bool {
	if filter.Keyword != "" {
		keyword := strings.ToLower(filter.Keyword)
		if !strings.Contains(strings.ToLower(game.Title), keyword) &&
			!strings.Contains(strings.ToLower(game.Description), keyword) {
			return false
		}
	}

	if filter.Genre != "" && !contains(game.Genres, filter.Genre) {
		return false
	}

	if filter.Platform != "" && !contains(game.Platforms, filter.Platform) {
		return false
	}

	if filter.Tag != "" && !contains(game.Tags, filter.Tag) {
		return false
	}

	return true
}

func sortGames(games []*Game, sortBy string) {
	switch strings.ToLower(sortBy) {
	case "release_date":
		sort.SliceStable(games, func(i, j int) bool {
			return games[i].ReleaseDate > games[j].ReleaseDate
		})
	default:
		sort.SliceStable(games, func(i, j int) bool {
			if games[i].TrendingScore == games[j].TrendingScore {
				return games[i].Score > games[j].Score
			}
			return games[i].TrendingScore > games[j].TrendingScore
		})
	}
}

func contains(list []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, item := range list {
		if strings.ToLower(item) == target {
			return true
		}
	}
	return false
}

func containsAny(values []string, targets []string) bool {
	for _, target := range targets {
		if contains(values, target) {
			return true
		}
	}
	return false
}

func clamp(value, minValue, maxValue int) int {
	return int(math.Max(float64(minValue), math.Min(float64(value), float64(maxValue))))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func defaultSeedGames() []*Game {
	raw := []byte(defaultGameSeed)
	var games []*Game
	if err := json.Unmarshal(raw, &games); err != nil {
		panic(fmt.Sprintf("invalid embedded game seed: %v", err))
	}
	return games
}

const defaultGameSeed = `[
	{
		"id": "game-elden-ring",
		"title": "Elden Ring",
		"description": "开放世界动作 RPG，玩家探索交界地、挑战半神并修复艾尔登法环。",
		"genres": ["RPG", "Action", "Open World"],
		"platforms": ["PC", "PS5", "Xbox"],
		"release_date": "2022-02-25",
		"developer": "FromSoftware",
		"publisher": "Bandai Namco",
		"tags": ["Soulslike", "Hardcore", "Co-op"],
		"score": 9.7,
		"trending_score": 92,
		"cover_image": "https://images.tappi.dev/elden-ring.jpg"
	},
	{
		"id": "game-valorant",
		"title": "Valorant",
		"description": "战术竞技 FPS，强调精准枪法与特工技能的配合。",
		"genres": ["FPS", "Competitive"],
		"platforms": ["PC"],
		"release_date": "2020-06-02",
		"developer": "Riot Games",
		"publisher": "Riot Games",
		"tags": ["eSports", "Shooter", "5v5"],
		"score": 8.9,
		"trending_score": 84,
		"cover_image": "https://images.tappi.dev/valorant.jpg"
	},
	{
		"id": "game-ffxiv",
		"title": "Final Fantasy XIV",
		"description": "传奇 MMORPG，拥有丰富主线剧情与跨平台社交生态。",
		"genres": ["MMO", "RPG"],
		"platforms": ["PC", "PS5"],
		"release_date": "2013-08-27",
		"developer": "Square Enix",
		"publisher": "Square Enix",
		"tags": ["MMORPG", "Story Rich", "Raids"],
		"score": 9.2,
		"trending_score": 75,
		"cover_image": "https://images.tappi.dev/ffxiv.jpg"
	},
	{
		"id": "game-hades2",
		"title": "Hades II",
		"description": "roguelike 动作游戏续作，围绕时间魔法展开的新冒险。",
		"genres": ["Roguelike", "Action"],
		"platforms": ["PC"],
		"release_date": "2024-05-06",
		"developer": "Supergiant Games",
		"publisher": "Supergiant Games",
		"tags": ["Indie", "Fast-paced", "Singleplayer"],
		"score": 9.1,
		"trending_score": 90,
		"cover_image": "https://images.tappi.dev/hades-2.jpg"
	},
	{
		"id": "game-zelda-totk",
		"title": "The Legend of Zelda: Tears of the Kingdom",
		"description": "塞尔达旷野之息的正统续作，广阔天空与地下探索交织。",
		"genres": ["Adventure", "Open World"],
		"platforms": ["Switch"],
		"release_date": "2023-05-12",
		"developer": "Nintendo",
		"publisher": "Nintendo",
		"tags": ["Story Rich", "Puzzle", "Exploration"],
		"score": 9.8,
		"trending_score": 95,
		"cover_image": "https://images.tappi.dev/zelda-totk.jpg"
	},
	{
		"id": "game-pubg",
		"title": "PUBG: Battlegrounds",
		"description": "大逃杀射击游戏鼻祖，强调真实枪械手感和战术配合。",
		"genres": ["Battle Royale", "Shooter"],
		"platforms": ["PC", "PS5", "Xbox", "Mobile"],
		"release_date": "2017-12-20",
		"developer": "KRAFTON",
		"publisher": "KRAFTON",
		"tags": ["Survival", "Competitive", "Teamplay"],
		"score": 8.1,
		"trending_score": 70,
		"cover_image": "https://images.tappi.dev/pubg.jpg"
	},
	{
		"id": "game-genshin",
		"title": "Genshin Impact",
		"description": "开放世界冒险 RPG，注重角色收集与四元素战斗系统。",
		"genres": ["RPG", "Adventure"],
		"platforms": ["PC", "PS5", "Mobile"],
		"release_date": "2020-09-28",
		"developer": "Cognosphere",
		"publisher": "miHoYo",
		"tags": ["Gacha", "Anime", "Co-op"],
		"score": 8.7,
		"trending_score": 80,
		"cover_image": "https://images.tappi.dev/genshin.jpg"
	},
	{
		"id": "game-apex",
		"title": "Apex Legends",
		"description": "快节奏英雄射击，三人小队在冰冷战场中争夺冠军。",
		"genres": ["Battle Royale", "Shooter"],
		"platforms": ["PC", "PS5", "Xbox", "Switch"],
		"release_date": "2019-02-04",
		"developer": "Respawn Entertainment",
		"publisher": "EA",
		"tags": ["Hero Shooter", "Teamplay", "eSports"],
		"score": 8.8,
		"trending_score": 78,
		"cover_image": "https://images.tappi.dev/apex-legends.jpg"
	}
]`
