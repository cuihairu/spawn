package model

import "sync"

// FollowRepository stores follow relations in memory.
// userTopics: userId -> set(topicId)
// userUsers: userId -> set(otherUserId)
type FollowRepository struct {
	mu         sync.RWMutex
	userTopics map[int64]map[int64]struct{}
	userUsers  map[int64]map[int64]struct{}
}

func NewFollowRepository() *FollowRepository {
	return &FollowRepository{
		userTopics: make(map[int64]map[int64]struct{}),
		userUsers:  make(map[int64]map[int64]struct{}),
	}
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
	return true
}
