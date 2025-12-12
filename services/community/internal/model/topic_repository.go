package model

import (
	"strings"
	"sync"
	"time"

	"github.com/tappi/tappi/services/community/internal/types"
)

type TopicRepository struct {
	mu     sync.RWMutex
	topics []*types.Topic
	index  map[int64]*types.Topic
	nextId int64
}

func NewTopicRepository() *TopicRepository {
	repo := &TopicRepository{
		index:  make(map[int64]*types.Topic),
		nextId: 1,
	}
	repo.seed()
	return repo
}

func (r *TopicRepository) seed() {
	now := time.Now().UTC().Format(time.RFC3339)
	r.topics = []*types.Topic{
		{
			Id:            1,
			Name:          "《黑神话：悟空》",
			Description:   "讨论黑神话相关的一切：剧情、战斗、装备与周边",
			IsOfficial:    true,
			PostCount:     0,
			FollowerCount: 0,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
		{
			Id:            2,
			Name:          "开放世界",
			Description:   "探索、支线、风景、任务路线分享",
			IsOfficial:    false,
			PostCount:     0,
			FollowerCount: 0,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	}
	r.buildIndexLocked()
}

func (r *TopicRepository) buildIndexLocked() {
	r.index = make(map[int64]*types.Topic, len(r.topics))
	maxId := int64(0)
	for _, t := range r.topics {
		if t == nil {
			continue
		}
		r.index[t.Id] = t
		if t.Id > maxId {
			maxId = t.Id
		}
	}
	r.nextId = maxId + 1
}

func (r *TopicRepository) Get(id int64) (*types.Topic, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	t, ok := r.index[id]
	if !ok {
		return nil, ErrTopicNotFound
	}
	return t, nil
}

func (r *TopicRepository) Create(req *types.CreateTopicReq) (*types.Topic, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC().Format(time.RFC3339)
	t := &types.Topic{
		Id:            r.nextId,
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
	r.nextId++
	r.topics = append(r.topics, t)
	r.index[t.Id] = t
	return t, nil
}

func (r *TopicRepository) List(keyword string, isOfficial bool, limit, offset int64) ([]types.Topic, int64) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	kw := strings.ToLower(strings.TrimSpace(keyword))

	var filtered []*types.Topic
	for _, t := range r.topics {
		if t == nil {
			continue
		}
		if isOfficial && !t.IsOfficial {
			continue
		}
		if kw != "" {
			if !strings.Contains(strings.ToLower(t.Name), kw) && !strings.Contains(strings.ToLower(t.Description), kw) {
				continue
			}
		}
		filtered = append(filtered, t)
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

	out := make([]types.Topic, 0, end-start)
	for _, t := range filtered[start:end] {
		out = append(out, *t)
	}
	return out, total
}

func (r *TopicRepository) IncrementPostCount(topicId int64, delta int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.index[topicId]
	if !ok {
		return ErrTopicNotFound
	}
	t.PostCount += delta
	if t.PostCount < 0 {
		t.PostCount = 0
	}
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}

func (r *TopicRepository) IncrementFollowerCount(topicId int64, delta int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	t, ok := r.index[topicId]
	if !ok {
		return ErrTopicNotFound
	}
	t.FollowerCount += delta
	if t.FollowerCount < 0 {
		t.FollowerCount = 0
	}
	t.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	return nil
}
