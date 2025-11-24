package integration

import (
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
