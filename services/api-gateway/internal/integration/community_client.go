package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// CommunityPost community GET /posts/:id 契约里的帖子字段（分享卡只取展示所需）。
type CommunityPost struct {
	Id         int64  `json:"id"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	AuthorName string `json:"author_name"`
}

// CommunityPostSummary 热帖列表项（BFF 聚合端点字段裁剪后的口径）：
// Content 仅在网关内部用于截断出 Summary，不下发；images/tags 等大字段不接收。
type CommunityPostSummary struct {
	Id           int64  `json:"id"`
	TopicId      int64  `json:"topic_id"`
	AuthorId     int64  `json:"author_id"`
	AuthorName   string `json:"author_name"`
	Title        string `json:"title"`
	Content      string `json:"content"`
	Summary      string `json:"-"`
	LikeCount    int64  `json:"like_count"`
	CommentCount int64  `json:"comment_count"`
	CreatedAt    string `json:"created_at"`
}

// CommunityTopicSummary 话题列表项（聚合端点裁剪口径）。
type CommunityTopicSummary struct {
	Id            int64  `json:"id"`
	Name          string `json:"name"`
	PostCount     int64  `json:"post_count"`
	FollowerCount int64  `json:"follower_count"`
	IsOfficial    bool   `json:"is_official"`
}

type CommunityClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewCommunityClient(baseURL string, timeout time.Duration) *CommunityClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &CommunityClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// GetPost 拉帖子详情（公开接口，无鉴权）；非 200 视为错误。
func (c *CommunityClient) GetPost(ctx context.Context, id int64) (*CommunityPost, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, fmt.Sprintf("%s/api/v1/posts/%d", c.baseURL, id), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("community status %d", resp.StatusCode)
	}

	var envelope struct {
		Post CommunityPost `json:"post"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	return &envelope.Post, nil
}

// ListHotPosts 拉热帖列表（公开接口，无鉴权）：GET /api/v1/posts/hot。
// 列表契约返回裸 {posts:[...], total}；非 200 视为错误。
func (c *CommunityClient) ListHotPosts(ctx context.Context, limit int) ([]CommunityPostSummary, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		fmt.Sprintf("%s/api/v1/posts/hot?limit=%d", c.baseURL, limit), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("community status %d", resp.StatusCode)
	}

	var payload struct {
		Posts []CommunityPostSummary `json:"posts"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Posts, nil
}

// ListTopics 拉话题列表（公开接口，无鉴权）：GET /api/v1/topics。
// 列表契约返回裸 {topics:[...], total}；非 200 视为错误。
func (c *CommunityClient) ListTopics(ctx context.Context, limit int) ([]CommunityTopicSummary, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		fmt.Sprintf("%s/api/v1/topics?limit=%d&offset=0", c.baseURL, limit), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("community status %d", resp.StatusCode)
	}

	var payload struct {
		Topics []CommunityTopicSummary `json:"topics"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Topics, nil
}
