package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GameCatalogClient 游戏目录服务客户端
type GameCatalogClient struct {
	baseURL    string
	httpClient *http.Client
}

// GameInfo 游戏基本信息
type GameInfo struct {
	Id         string   `json:"id"`
	Title      string   `json:"title"`
	CoverImage string   `json:"cover_image"`
	Genres     []string `json:"genres"`
	Platforms  []string `json:"platforms"`
}

// responsePayload 兼容两种返回格式：
//   - game-catalog 实际返回：{"game": {...}}（GET /games/:id）
//   - 早期文档约定的旧格式：{"code": 200, "message": "...", "data": {...}}
//
// Code/Data/Game 均用指针以区分字段缺失与零值。
type responsePayload struct {
	Code    *int      `json:"code"`
	Message string    `json:"message"`
	Data    *GameInfo `json:"data"`
	Game    *GameInfo `json:"game"`
}

// NewGameCatalogClient 创建游戏目录服务客户端
func NewGameCatalogClient(baseURL string, timeout time.Duration) *GameCatalogClient {
	return &GameCatalogClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// GetGameById 根据游戏ID获取游戏信息
func (c *GameCatalogClient) GetGameById(ctx context.Context, gameId string) (*GameInfo, error) {
	url := fmt.Sprintf("%s/games/%s", c.baseURL, gameId)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("请求失败，状态码: %d, 响应: %s", resp.StatusCode, string(body))
	}

	var payload responsePayload
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if payload.Code != nil && *payload.Code != http.StatusOK {
		return nil, fmt.Errorf("获取游戏信息失败: %s", payload.Message)
	}

	if payload.Game != nil {
		return payload.Game, nil
	}
	if payload.Data != nil {
		return payload.Data, nil
	}

	return nil, fmt.Errorf("响应格式无法识别: %s", string(body))
}
