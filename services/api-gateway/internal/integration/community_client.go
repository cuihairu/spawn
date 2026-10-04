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
