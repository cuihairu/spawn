package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
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

// GameResponse 游戏详情响应
type GameResponse struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Data    GameInfo `json:"data"`
}

// NewGameCatalogClient 创建游戏目录服务客户端
func NewGameCatalogClient(baseURL string, timeout time.Duration) *GameCatalogClient {
	return &GameCatalogClient{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

// GetGameById 根据游戏ID获取游戏信息
func (c *GameCatalogClient) GetGameById(ctx context.Context, gameId string) (*GameInfo, error) {
	url := fmt.Sprintf("%s/api/v1/games/%s", c.baseURL, gameId)

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

	var gameResp GameResponse
	if err := json.Unmarshal(body, &gameResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	if gameResp.Code != http.StatusOK {
		return nil, fmt.Errorf("获取游戏信息失败: %s", gameResp.Message)
	}

	return &gameResp.Data, nil
}
