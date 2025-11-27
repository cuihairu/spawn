package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
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
	GameId   string
	AuthorId int64
	Tag      string
	Page     int
	PageSize int
}

type GuideRepository struct {
	mu     sync.RWMutex
	guides []*Guide
	index  map[int64]*Guide
	nextId int64
}

func NewGuideRepository(source string) (*GuideRepository, error) {
	repo := &GuideRepository{
		index:  make(map[int64]*Guide),
		nextId: 1,
	}
	if err := repo.load(source); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *GuideRepository) load(source string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			r.guides = defaultSeedGuides()
			r.buildIndex()
			return nil
		}
		return fmt.Errorf("read guide data: %w", err)
	}

	var guides []*Guide
	if err := json.Unmarshal(data, &guides); err != nil {
		return fmt.Errorf("unmarshal guide data: %w", err)
	}
	if len(guides) == 0 {
		guides = defaultSeedGuides()
	}

	r.guides = guides
	r.buildIndex()
	return nil
}

func (r *GuideRepository) buildIndex() {
	r.index = make(map[int64]*Guide, len(r.guides))
	maxId := int64(0)
	for _, g := range r.guides {
		if g == nil {
			continue
		}
		r.index[g.Id] = g
		if g.Id > maxId {
			maxId = g.Id
		}
	}
	r.nextId = maxId + 1
}

func (r *GuideRepository) List(filter GuideFilter) ([]*Guide, int) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []*Guide
	for _, guide := range r.guides {
		if !matchesGuide(guide, filter) {
			continue
		}
		filtered = append(filtered, guide)
	}

	total := len(filtered)
	start := (filter.Page - 1) * filter.PageSize
	end := start + filter.PageSize

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

func (r *GuideRepository) Get(id int64) (*Guide, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	guide, ok := r.index[id]
	if !ok {
		return nil, ErrGuideNotFound
	}
	return guide, nil
}

func (r *GuideRepository) Create(guide *Guide) (*Guide, error) {
	if guide == nil {
		return nil, errors.New("guide payload required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	guide.Id = r.nextId
	r.nextId++
	guide.CreatedAt = time.Now().Format(time.RFC3339)
	guide.UpdatedAt = guide.CreatedAt
	guide.Views = 0
	guide.Likes = 0
	guide.IsPublished = false

	r.guides = append(r.guides, guide)
	r.index[guide.Id] = guide
	return guide, nil
}

func (r *GuideRepository) Update(id int64, updates map[string]interface{}) (*Guide, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	guide, ok := r.index[id]
	if !ok {
		return nil, ErrGuideNotFound
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
	return guide, nil
}

func (r *GuideRepository) Publish(id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	guide, ok := r.index[id]
	if !ok {
		return ErrGuideNotFound
	}

	guide.IsPublished = true
	guide.UpdatedAt = time.Now().Format(time.RFC3339)
	return nil
}

func (r *GuideRepository) Like(id int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	guide, ok := r.index[id]
	if !ok {
		return 0, ErrGuideNotFound
	}

	guide.Likes++
	return guide.Likes, nil
}

func (r *GuideRepository) IncrementViews(id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	guide, ok := r.index[id]
	if !ok {
		return ErrGuideNotFound
	}

	guide.Views++
	return nil
}

func matchesGuide(guide *Guide, filter GuideFilter) bool {
	if filter.GameId != "" && guide.GameId != filter.GameId {
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
