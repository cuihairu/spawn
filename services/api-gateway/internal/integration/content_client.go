package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// GuideSummary 攻略列表项（BFF 聚合端点字段裁剪后的口径）：
// 仅取列表展示所需，去掉 content/tags/cover_image 等大字段。
type GuideSummary struct {
	Id         int64  `json:"id"`
	GameId     string `json:"game_id"`
	GameTitle  string `json:"game_title"`
	Title      string `json:"title"`
	Summary    string `json:"summary"`
	AuthorName string `json:"author_name"`
	Likes      int64  `json:"likes"`
	Views      int64  `json:"views"`
	CreatedAt  string `json:"created_at"`
}

type ContentClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewContentClient(baseURL string, timeout time.Duration) *ContentClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &ContentClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// ListGuides 拉已发布攻略列表（公开接口，无鉴权）：
// GET /api/v1/guides?page=1&page_size=N，契约是 HTTP 200 + {code,message,data,total,page}
// 业务信封，code != 200 视为业务失败。
func (c *ContentClient) ListGuides(ctx context.Context, limit int) ([]GuideSummary, error) {
	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet,
		fmt.Sprintf("%s/api/v1/guides?page=1&page_size=%d", c.baseURL, limit), nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("content status %d", resp.StatusCode)
	}

	var envelope struct {
		Code    int            `json:"code"`
		Message string         `json:"message"`
		Data    []GuideSummary `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&envelope); err != nil {
		return nil, err
	}
	if envelope.Code != 200 {
		return nil, fmt.Errorf("content code %d: %s", envelope.Code, envelope.Message)
	}
	return envelope.Data, nil
}
