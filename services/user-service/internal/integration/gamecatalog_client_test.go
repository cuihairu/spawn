package integration

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestGameCatalogClient_GetRecommendations 验证对 game-catalog
// GET /games/recommendations 的真实线上契约（查询参数与 {"games": [...]} 响应）。
func TestGameCatalogClient_GetRecommendations(t *testing.T) {
	var gotPath string
	var gotQuery map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"games":[
			{"id":"g-rpg-1","title":"Alpha RPG","cover_image":"c1.png","genres":["RPG"],"platforms":["PC"],"score":9.0,"tags":["story"]},
			{"id":"g-fps-1","title":"Gamma Strike","cover_image":"c2.png","genres":["FPS"],"platforms":["PC"],"score":8.8,"tags":["pvp"]}
		]}`))
	}))
	defer server.Close()

	client := NewGameCatalogClient(server.URL, time.Second)
	got, err := client.GetRecommendations(context.Background(), RecommendationParams{
		UserID: "user-42",
		Genres: []string{"RPG", "FPS"},
		Limit:  5,
	})
	if err != nil {
		t.Fatalf("GetRecommendations: %v", err)
	}

	if gotPath != "/games/recommendations" {
		t.Fatalf("path = %q, want /games/recommendations", gotPath)
	}
	if q := gotQuery["userId"]; len(q) != 1 || q[0] != "user-42" {
		t.Fatalf("userId query = %v", gotQuery["userId"])
	}
	// genres 以逗号拼接（game-catalog 端按逗号切分再 TrimSpace）
	if q := gotQuery["genres"]; len(q) != 1 || q[0] != "RPG,FPS" {
		t.Fatalf("genres query = %v", gotQuery["genres"])
	}
	if q := gotQuery["limit"]; len(q) != 1 || q[0] != "5" {
		t.Fatalf("limit query = %v", gotQuery["limit"])
	}

	if len(got) != 2 {
		t.Fatalf("games = %d, want 2", len(got))
	}
	if got[0].Id != "g-rpg-1" || got[0].Title != "Alpha RPG" || got[0].Score != 9.0 {
		t.Fatalf("first game = %+v", got[0])
	}
	if !equalStrings(got[1].Genres, []string{"FPS"}) {
		t.Fatalf("second game genres = %v", got[1].Genres)
	}
}

// TestGameCatalogClient_OmitsEmptyParams 空参数不得出现在查询串中。
func TestGameCatalogClient_OmitsEmptyParams(t *testing.T) {
	var gotQuery map[string][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		w.Write([]byte(`{"games":[]}`))
	}))
	defer server.Close()

	client := NewGameCatalogClient(server.URL, time.Second)
	if _, err := client.GetRecommendations(context.Background(), RecommendationParams{}); err != nil {
		t.Fatalf("GetRecommendations: %v", err)
	}
	for _, key := range []string{"userId", "genres", "limit"} {
		if _, ok := gotQuery[key]; ok {
			t.Fatalf("param %q must be omitted when empty", key)
		}
	}
}

func TestGameCatalogClient_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("boom"))
	}))
	defer server.Close()

	client := NewGameCatalogClient(server.URL, time.Second)
	_, err := client.GetRecommendations(context.Background(), RecommendationParams{Limit: 1})
	if err == nil || !strings.Contains(err.Error(), "game catalog 500") || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("err = %v, want status+body message", err)
	}
}

func TestGameCatalogClient_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not-json`))
	}))
	defer server.Close()

	client := NewGameCatalogClient(server.URL, time.Second)
	if _, err := client.GetRecommendations(context.Background(), RecommendationParams{}); !isJSONError(err) {
		t.Fatalf("err = %v, want JSON decode failure", err)
	}
}

func TestGameCatalogClient_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{"games":[]}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewGameCatalogClient(server.URL, time.Second)
	if _, err := client.GetRecommendations(ctx, RecommendationParams{}); err == nil {
		t.Fatal("canceled context must fail the call")
	}
}

func TestGameCatalogClient_ConstructionDefaults(t *testing.T) {
	// timeout<=0 → 默认 5s；baseURL 尾斜杠裁剪；空 baseURL → 默认 game-catalog 地址
	c := NewGameCatalogClient("", 0)
	if c.httpClient.Timeout != 5*time.Second {
		t.Fatalf("default timeout = %v, want 5s", c.httpClient.Timeout)
	}
	if c.baseURL != "http://localhost:8890" {
		t.Fatalf("default baseURL = %q", c.baseURL)
	}

	c2 := NewGameCatalogClient("http://example.com///", time.Second)
	if c2.baseURL != "http://example.com" {
		t.Fatalf("trimmed baseURL = %q", c2.baseURL)
	}
}

func isJSONError(err error) bool {
	if err == nil {
		return false
	}
	var syn *json.SyntaxError
	return errors.As(err, &syn)
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestGameCatalogClient_MalformedBaseURL 非法 baseURL（未闭合 IPv6）→
// http.NewRequestWithContext 构造失败，错误原样返回。
func TestGameCatalogClient_MalformedBaseURL(t *testing.T) {
	client := NewGameCatalogClient("http://[::1", time.Second)
	if _, err := client.GetRecommendations(context.Background(), RecommendationParams{UserID: "u1"}); err == nil {
		t.Fatal("malformed baseURL must fail request construction")
	}
}
