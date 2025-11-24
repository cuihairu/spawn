package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type GameCatalogClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewGameCatalogClient(baseURL string, timeout time.Duration) *GameCatalogClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &GameCatalogClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *GameCatalogClient) GetFeatured(ctx context.Context, limit int64) (map[string]interface{}, error) {
	endpoint := fmt.Sprintf("%s/games/featured?limit=%d", c.baseURL, limit)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("game catalog status %d", resp.StatusCode)
	}

	result := make(map[string]interface{})
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}
