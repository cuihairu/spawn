package model

import (
	"errors"
	"fmt"
	"math"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/tappi/tappi/services/community/internal/types"
)

type PostRepository struct {
	mu     sync.RWMutex
	source string
	posts  []*types.Post
	index  map[int64]*types.Post
	nextId int64
}

func NewPostRepository(source string) (*PostRepository, error) {
	repo := &PostRepository{
		source: source,
		index:  make(map[int64]*types.Post),
		nextId: 1,
	}
	if err := repo.load(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *PostRepository) load() error {
	var posts []*types.Post
	err := readJSONFile(r.source, &posts)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			posts = defaultSeedPosts()
			r.posts = posts
			r.buildIndexLocked()
			_ = r.saveLocked()
			return nil
		}
		return fmt.Errorf("read posts: %w", err)
	}
	if len(posts) == 0 {
		posts = defaultSeedPosts()
	}
	r.posts = posts
	r.buildIndexLocked()
	return nil
}

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

func (r *PostRepository) buildIndexLocked() {
	r.index = make(map[int64]*types.Post, len(r.posts))
	maxId := int64(0)
	for _, p := range r.posts {
		if p == nil {
			continue
		}
		r.index[p.Id] = p
		if p.Id > maxId {
			maxId = p.Id
		}
	}
	r.nextId = maxId + 1
}

func (r *PostRepository) saveLocked() error {
	cp := make([]*types.Post, 0, len(r.posts))
	for _, p := range r.posts {
		if p != nil {
			cp = append(cp, p)
		}
	}
	sort.SliceStable(cp, func(i, j int) bool { return cp[i].Id < cp[j].Id })
	return writeJSONAtomic(r.source, cp)
}

func (r *PostRepository) Create(topicId, authorId int64, authorName string, req *types.CreatePostReq) (*types.Post, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	postType := strings.TrimSpace(req.Type)
	if postType == "" {
		postType = "discussion"
	}

	p := &types.Post{
		Id:           r.nextId,
		TopicId:      topicId,
		AuthorId:     authorId,
		AuthorName:   strings.TrimSpace(authorName),
		Title:        strings.TrimSpace(req.Title),
		Content:      strings.TrimSpace(req.Content),
		Images:       req.Images,
		Type:         postType,
		Tags:         req.Tags,
		ViewCount:    0,
		LikeCount:    0,
		CommentCount: 0,
		ShareCount:   0,
		IsPinned:     false,
		IsHot:        false,
		Status:       "published",
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	r.nextId++
	r.posts = append(r.posts, p)
	r.index[p.Id] = p
	if err := r.saveLocked(); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *PostRepository) Get(id int64) (*types.Post, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.index[id]
	if !ok || p.Status == "deleted" {
		return nil, ErrPostNotFound
	}
	return p, nil
}

func (r *PostRepository) IncrementViews(id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.index[id]
	if !ok || p.Status == "deleted" {
		return ErrPostNotFound
	}
	p.ViewCount++
	return r.saveLocked()
}

func (r *PostRepository) Update(id, requesterId int64, req *types.UpdatePostReq) (*types.Post, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.index[id]
	if !ok || p.Status == "deleted" {
		return nil, ErrPostNotFound
	}
	if p.AuthorId != requesterId {
		return nil, errors.New("permission denied")
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
	if err := r.saveLocked(); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *PostRepository) Delete(id, requesterId int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.index[id]
	if !ok || p.Status == "deleted" {
		return ErrPostNotFound
	}
	if p.AuthorId != requesterId {
		return errors.New("permission denied")
	}
	p.Status = "deleted"
	p.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return r.saveLocked()
}

func (r *PostRepository) Like(id int64) (*types.Post, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.index[id]
	if !ok || p.Status == "deleted" {
		return nil, ErrPostNotFound
	}
	p.LikeCount++
	if err := r.saveLocked(); err != nil {
		return nil, err
	}
	return p, nil
}

func (r *PostRepository) Share(id int64) (*types.Post, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.index[id]
	if !ok || p.Status == "deleted" {
		return nil, ErrPostNotFound
	}
	p.ShareCount++
	if err := r.saveLocked(); err != nil {
		return nil, err
	}
	return p, nil
}

type PostListFilter struct {
	TopicId  int64
	AuthorId int64
	Type     string
	Status   string
	IsHot    bool
	Limit    int64
	Offset   int64
}

func (r *PostRepository) List(filter PostListFilter) ([]types.Post, int64) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	status := strings.TrimSpace(filter.Status)
	if status == "" {
		status = "published"
	}

	var filtered []*types.Post
	for _, p := range r.posts {
		if p == nil || p.Status == "deleted" {
			continue
		}
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
			if hotScore(*p) <= 0 {
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

	out := make([]types.Post, 0, end-start)
	for _, p := range filtered[start:end] {
		out = append(out, *p)
	}
	return out, total
}

func (r *PostRepository) Hot(limit int64) []types.Post {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var candidates []*types.Post
	for _, p := range r.posts {
		if p == nil || p.Status != "published" {
			continue
		}
		candidates = append(candidates, p)
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		pi, pj := candidates[i], candidates[j]
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
		cp := *p
		cp.IsHot = true
		out = append(out, cp)
	}
	return out
}

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
