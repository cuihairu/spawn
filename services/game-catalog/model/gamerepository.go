package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"math"
	"os"
	"sort"
	"strings"
	"sync"

	"github.com/google/uuid"
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

type GameRepository struct {
	mu    sync.RWMutex
	games []*Game
	index map[string]*Game
}

// GameStore 游戏仓储接口：ServiceContext 面向接口依赖，测试可注入故障实现。
type GameStore interface {
	List(filter GameFilter) ([]*Game, int)
	Get(id string) (*Game, error)
	Create(game *Game) (*Game, error)
	Featured(limit int) []*Game
	Recommend(userId string, genres []string, limit int) []*Game
}

var _ GameStore = (*GameRepository)(nil)

func NewGameRepository(source string) (*GameRepository, error) {
	repo := &GameRepository{}
	if err := repo.load(source); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *GameRepository) load(source string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			r.games = defaultSeedGames()
			r.buildIndex()
			return nil
		}
		return fmt.Errorf("read game data: %w", err)
	}

	var games []*Game
	if err := json.Unmarshal(data, &games); err != nil {
		return fmt.Errorf("unmarshal game data: %w", err)
	}
	if len(games) == 0 {
		games = defaultSeedGames()
	}

	r.games = games
	r.buildIndex()
	return nil
}

func (r *GameRepository) buildIndex() {
	r.index = make(map[string]*Game, len(r.games))
	for _, g := range r.games {
		if g == nil || g.Id == "" {
			continue
		}
		r.index[g.Id] = g
	}
}

func (r *GameRepository) List(filter GameFilter) ([]*Game, int) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []*Game
	for _, game := range r.games {
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

func (r *GameRepository) Get(id string) (*Game, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	game, ok := r.index[id]
	if !ok {
		return nil, ErrGameNotFound
	}
	return game, nil
}

func (r *GameRepository) Create(game *Game) (*Game, error) {
	if game == nil {
		return nil, errors.New("game payload required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	game.Id = fmt.Sprintf("game-%s", uuid.NewString())
	r.games = append(r.games, game)
	r.index[game.Id] = game
	return game, nil
}

func (r *GameRepository) Featured(limit int) []*Game {
	r.mu.RLock()
	defer r.mu.RUnlock()

	cloned := append([]*Game(nil), r.games...)
	sort.SliceStable(cloned, func(i, j int) bool {
		return cloned[i].TrendingScore > cloned[j].TrendingScore
	})

	if limit <= 0 || limit > len(cloned) {
		limit = len(cloned)
	}

	return cloned[:limit]
}

func (r *GameRepository) Recommend(userId string, genres []string, limit int) []*Game {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if limit <= 0 {
		limit = 5
	}

	var pool []*Game
	if len(genres) == 0 {
		pool = append(pool, r.games...)
	} else {
		for _, game := range r.games {
			if containsAny(game.Genres, genres) {
				pool = append(pool, game)
			}
		}
	}

	if len(pool) == 0 {
		pool = append(pool, r.games...)
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
