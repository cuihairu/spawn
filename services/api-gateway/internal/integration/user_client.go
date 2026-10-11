package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type UserServiceClient struct {
	baseURL    string
	httpClient *http.Client
}

type RecommendationResponse struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func NewUserServiceClient(baseURL string, timeout time.Duration) *UserServiceClient {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &UserServiceClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *UserServiceClient) GetRecommendations(ctx context.Context, userID int64, limit int64, genres string, token string) (*RecommendationResponse, error) {
	endpoint := fmt.Sprintf("%s/users/%d/recommendations", c.baseURL, userID)
	query := url.Values{}
	if limit > 0 {
		query.Set("limit", fmt.Sprintf("%d", limit))
	}
	if genres != "" {
		query.Set("genres", genres)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.URL.RawQuery = query.Encode()
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		msg := readError(resp.Body)
		return nil, fmt.Errorf("recommendation failed: %s", msg)
	}

	var payload RecommendationResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func readError(r io.Reader) string {
	b, _ := io.ReadAll(io.LimitReader(r, 512))
	if len(b) == 0 {
		return "upstream error"
	}
	return string(b)
}
