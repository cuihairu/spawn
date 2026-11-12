package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Recommendation struct {
	Id         string   `json:"id"`
	Title      string   `json:"title"`
	CoverImage string   `json:"cover_image"`
	Genres     []string `json:"genres"`
	Platforms  []string `json:"platforms"`
	Score      float64  `json:"score"`
	Tags       []string `json:"tags"`
}

type RecommendationParams struct {
	UserID string
	Genres []string
	Limit  int
}

type GameCatalogClient struct {
	baseURL    string
	httpClient *http.Client
	// internalToken GP-403 渠道归因 S2S（/internal/promo/clicks/:id/resolve）
	// 的 X-Internal-Token 守卫；空 = 归因按自然流量处理。
	internalToken string
}

func NewGameCatalogClient(baseURL string, timeout time.Duration) *GameCatalogClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}

	baseURL = strings.TrimRight(baseURL, "/")
	if baseURL == "" {
		baseURL = "http://localhost:8890"
	}

	return &GameCatalogClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *GameCatalogClient) GetRecommendations(ctx context.Context, params RecommendationParams) ([]Recommendation, error) {
	endpoint := fmt.Sprintf("%s/games/recommendations", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	query := url.Values{}
	if params.UserID != "" {
		query.Set("userId", params.UserID)
	}
	if len(params.Genres) > 0 {
		query.Set("genres", strings.Join(params.Genres, ","))
	}
	if params.Limit > 0 {
		query.Set("limit", strconv.Itoa(params.Limit))
	}
	req.URL.RawQuery = query.Encode()

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("game catalog %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Games []Recommendation `json:"games"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return payload.Games, nil
}

// PromoClickResolution GP-403 单层渠道归因解析结果（game-catalog
// /internal/promo/clicks/:clickId/resolve 响应同构）：渠道标识与归因窗口
// 判定（注册时刻是否在窗口内）。
type PromoClickResolution struct {
	Channel     string `json:"channel"`
	ChannelId   int64  `json:"channel_id"`
	ClickedAt   string `json:"clicked_at"`
	WindowHours int64  `json:"window_hours"`
	InWindow    bool   `json:"in_window"`
}

// ResolvePromoClick GP-403：注册时携带推广点击标识向 game-catalog 解析
// 渠道归因（S2S，X-Internal-Token）。失败不阻断注册（按自然流量处理，
// 归因是增强数据，不是注册前置条件）。
func (c *GameCatalogClient) ResolvePromoClick(ctx context.Context, clickID, registeredAt string, userID int64) (*PromoClickResolution, error) {
	endpoint := fmt.Sprintf("%s/internal/promo/clicks/%s/resolve", c.baseURL, url.PathEscape(clickID))
	body, err := json.Marshal(map[string]any{
		"click_id":      clickID,
		"registered_at": registeredAt,
		"user_id":       userID,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.internalToken != "" {
		req.Header.Set("X-Internal-Token", c.internalToken)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		io.ReadAll(io.LimitReader(resp.Body, 512)) // drain 便于连接复用
		return nil, fmt.Errorf("game catalog resolve %d", resp.StatusCode)
	}
	var out PromoClickResolution
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return &out, nil
}

// SetInternalToken 注入 S2S 令牌（GP-403 渠道归因解析用）。
func (c *GameCatalogClient) SetInternalToken(token string) {
	c.internalToken = token
}
