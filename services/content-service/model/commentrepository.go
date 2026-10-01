package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

var ErrCommentNotFound = errors.New("comment not found")

type Comment struct {
	Id         int64  `json:"id"`
	TargetType string `json:"target_type"` // "guide", "game", etc.
	TargetId   int64  `json:"target_id"`
	UserId     int64  `json:"user_id"`
	UserName   string `json:"user_name"`
	Content    string `json:"content"`
	ParentId   int64  `json:"parent_id,omitempty"`
	ReplyToId  int64  `json:"reply_to_id,omitempty"`
	Likes      int    `json:"likes"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type CommentFilter struct {
	TargetType string
	TargetId   int64
	Page       int
	PageSize   int
}

type CommentRepository struct {
	mu       sync.RWMutex
	comments []*Comment
	index    map[int64]*Comment
	nextId   int64
}

// CommentStore 评论仓储接口：ServiceContext 面向接口依赖，测试可注入故障实现。
type CommentStore interface {
	List(filter CommentFilter) ([]*Comment, int)
	Get(id int64) (*Comment, error)
	Create(comment *Comment) (*Comment, error)
	Delete(id int64) error
	Like(id int64) (int, error)
}

var _ CommentStore = (*CommentRepository)(nil)

func NewCommentRepository(source string) (*CommentRepository, error) {
	repo := &CommentRepository{
		index:  make(map[int64]*Comment),
		nextId: 1,
	}
	if err := repo.load(source); err != nil {
		return nil, err
	}
	return repo, nil
}

func (r *CommentRepository) load(source string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			r.comments = defaultSeedComments()
			r.buildIndex()
			return nil
		}
		return fmt.Errorf("read comment data: %w", err)
	}

	var comments []*Comment
	if err := json.Unmarshal(data, &comments); err != nil {
		return fmt.Errorf("unmarshal comment data: %w", err)
	}
	if len(comments) == 0 {
		comments = defaultSeedComments()
	}

	r.comments = comments
	r.buildIndex()
	return nil
}

func (r *CommentRepository) buildIndex() {
	r.index = make(map[int64]*Comment, len(r.comments))
	maxId := int64(0)
	for _, c := range r.comments {
		if c == nil {
			continue
		}
		r.index[c.Id] = c
		if c.Id > maxId {
			maxId = c.Id
		}
	}
	r.nextId = maxId + 1
}

func (r *CommentRepository) List(filter CommentFilter) ([]*Comment, int) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var filtered []*Comment
	for _, comment := range r.comments {
		if comment == nil {
			continue
		}
		if !matchesComment(comment, filter) {
			continue
		}
		filtered = append(filtered, comment)
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

	result := make([]*Comment, end-start)
	copy(result, filtered[start:end])

	return result, total
}

func (r *CommentRepository) Get(id int64) (*Comment, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	comment, ok := r.index[id]
	if !ok {
		return nil, ErrCommentNotFound
	}
	return comment, nil
}

func (r *CommentRepository) Create(comment *Comment) (*Comment, error) {
	if comment == nil {
		return nil, errors.New("comment payload required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	comment.Id = r.nextId
	r.nextId++
	comment.CreatedAt = time.Now().Format(time.RFC3339)
	comment.UpdatedAt = comment.CreatedAt
	comment.Likes = 0

	r.comments = append(r.comments, comment)
	r.index[comment.Id] = comment
	return comment, nil
}

func (r *CommentRepository) Delete(id int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, ok := r.index[id]; !ok {
		return ErrCommentNotFound
	}

	delete(r.index, id)

	// Remove from slice
	for i, comment := range r.comments {
		if comment.Id == id {
			r.comments = append(r.comments[:i], r.comments[i+1:]...)
			break
		}
	}

	return nil
}

func (r *CommentRepository) Like(id int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	comment, ok := r.index[id]
	if !ok {
		return 0, ErrCommentNotFound
	}

	comment.Likes++
	return comment.Likes, nil
}

func matchesComment(comment *Comment, filter CommentFilter) bool {
	if filter.TargetType != "" && comment.TargetType != filter.TargetType {
		return false
	}

	if filter.TargetId > 0 && comment.TargetId != filter.TargetId {
		return false
	}

	return true
}

func defaultSeedComments() []*Comment {
	return []*Comment{
		{
			Id:         1,
			TargetType: "guide",
			TargetId:   1,
			UserId:     2001,
			UserName:   "玩家A",
			Content:    "这个攻略写得太好了！帮了我大忙！",
			Likes:      15,
			CreatedAt:  "2024-01-16T08:30:00Z",
			UpdatedAt:  "2024-01-16T08:30:00Z",
		},
		{
			Id:         2,
			TargetType: "guide",
			TargetId:   1,
			UserId:     2002,
			UserName:   "游戏爱好者",
			Content:    "前期确实按照这个思路打会轻松很多",
			Likes:      8,
			CreatedAt:  "2024-01-17T10:15:00Z",
			UpdatedAt:  "2024-01-17T10:15:00Z",
		},
		{
			Id:         3,
			TargetType: "guide",
			TargetId:   2,
			UserId:     2003,
			UserName:   "射击之王",
			Content:    "这些训练方法确实有效，枪法进步明显",
			ParentId:   0,
			Likes:      23,
			CreatedAt:  "2024-01-19T14:20:00Z",
			UpdatedAt:  "2024-01-19T14:20:00Z",
		},
	}
}
