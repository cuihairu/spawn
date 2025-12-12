package model

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"sync"
)

type followState struct {
	UserTopics map[int64][]int64 `json:"user_topics"`
	UserUsers  map[int64][]int64 `json:"user_users"`
}

// FollowRepository stores follow relations, persisted in a json file.
// Internally it uses sets for quick operations.
type FollowRepository struct {
	mu         sync.RWMutex
	source     string
	userTopics map[int64]map[int64]struct{}
	userUsers  map[int64]map[int64]struct{}
}

func NewFollowRepository(source string) (*FollowRepository, error) {
	repo := &FollowRepository{
		source:     source,
		userTopics: make(map[int64]map[int64]struct{}),
		userUsers:  make(map[int64]map[int64]struct{}),
	}
	if err := repo.load(); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *FollowRepository) load() error {
	var st followState
	err := readJSONFile(r.source, &st)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_ = r.saveLocked()
			return nil
		}
		return fmt.Errorf("read follows: %w", err)
	}

	for uid, tids := range st.UserTopics {
		set := make(map[int64]struct{}, len(tids))
		for _, tid := range tids {
			set[tid] = struct{}{}
		}
		r.userTopics[uid] = set
	}
	for uid, uids := range st.UserUsers {
		set := make(map[int64]struct{}, len(uids))
		for _, other := range uids {
			set[other] = struct{}{}
		}
		r.userUsers[uid] = set
	}

	return nil
}

func (r *FollowRepository) saveLocked() error {
	st := followState{
		UserTopics: make(map[int64][]int64, len(r.userTopics)),
		UserUsers:  make(map[int64][]int64, len(r.userUsers)),
	}

	for uid, set := range r.userTopics {
		ids := make([]int64, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		sort.SliceStable(ids, func(i, j int) bool { return ids[i] < ids[j] })
		st.UserTopics[uid] = ids
	}

	for uid, set := range r.userUsers {
		ids := make([]int64, 0, len(set))
		for id := range set {
			ids = append(ids, id)
		}
		sort.SliceStable(ids, func(i, j int) bool { return ids[i] < ids[j] })
		st.UserUsers[uid] = ids
	}

	return writeJSONAtomic(r.source, st)
}

func (r *FollowRepository) FollowTopic(userId, topicId int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	set, ok := r.userTopics[userId]
	if !ok {
		set = make(map[int64]struct{})
		r.userTopics[userId] = set
	}
	if _, exists := set[topicId]; exists {
		return false
	}
	set[topicId] = struct{}{}
	_ = r.saveLocked()
	return true
}

func (r *FollowRepository) UnfollowTopic(userId, topicId int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	set, ok := r.userTopics[userId]
	if !ok {
		return false
	}
	if _, exists := set[topicId]; !exists {
		return false
	}
	delete(set, topicId)
	_ = r.saveLocked()
	return true
}

func (r *FollowRepository) ListFollowingTopicIds(userId int64) []int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()
	set := r.userTopics[userId]
	if len(set) == 0 {
		return nil
	}
	out := make([]int64, 0, len(set))
	for tid := range set {
		out = append(out, tid)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func (r *FollowRepository) FollowUser(userId, targetUserId int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	set, ok := r.userUsers[userId]
	if !ok {
		set = make(map[int64]struct{})
		r.userUsers[userId] = set
	}
	if _, exists := set[targetUserId]; exists {
		return false
	}
	set[targetUserId] = struct{}{}
	_ = r.saveLocked()
	return true
}

func (r *FollowRepository) UnfollowUser(userId, targetUserId int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	set, ok := r.userUsers[userId]
	if !ok {
		return false
	}
	if _, exists := set[targetUserId]; !exists {
		return false
	}
	delete(set, targetUserId)
	_ = r.saveLocked()
	return true
}
