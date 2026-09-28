package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestGameCatalogClient_GetFeatured 验证对 game-catalog
// GET /games/featured?limit=N 的线上契约。
func TestGameCatalogClient_GetFeatured(t *testing.T) {
	var gotPath, gotLimit string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotLimit = r.URL.Query().Get("limit")
		w.Write([]byte(`{"games":[{"id":"g-rpg-1","title":"Alpha RPG","trending_score":90}],"total":8,"limit":3,"offset":0}`))
	}))
	defer server.Close()

	client := NewGameCatalogClient(server.URL+"/", time.Second)
	result, err := client.GetFeatured(context.Background(), 3)
	if err != nil {
		t.Fatalf("GetFeatured: %v", err)
	}

	if gotPath != "/games/featured" {
		t.Fatalf("path = %q, want /games/featured", gotPath)
	}
	if gotLimit != "3" {
		t.Fatalf("limit query = %q, want 3", gotLimit)
	}

	games, ok := result["games"].([]interface{})
	if !ok || len(games) != 1 {
		t.Fatalf("games payload = %#v", result["games"])
	}
	first, _ := games[0].(map[string]interface{})
	if first["id"] != "g-rpg-1" || first["title"] != "Alpha RPG" {
		t.Fatalf("first game = %#v", first)
	}
	if total, _ := result["total"].(float64); total != 8 {
		t.Fatalf("total = %#v, want 8", result["total"])
	}
}

func TestGameCatalogClient_GetFeatured_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewGameCatalogClient(server.URL, time.Second)
	_, err := client.GetFeatured(context.Background(), 3)
	if err == nil || !strings.Contains(err.Error(), "game catalog status 502") {
		t.Fatalf("err = %v, want status message", err)
	}
}

func TestGameCatalogClient_GetFeatured_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>not json</html>`))
	}))
	defer server.Close()

	client := NewGameCatalogClient(server.URL, time.Second)
	if _, err := client.GetFeatured(context.Background(), 3); err == nil {
		t.Fatal("invalid JSON must fail")
	}
}

func TestGameCatalogClient_Defaults(t *testing.T) {
	c := NewGameCatalogClient("", 0)
	if c.httpClient.Timeout != 5*time.Second {
		t.Fatalf("default timeout = %v, want 5s", c.httpClient.Timeout)
	}
	if c.baseURL != "" { // 注意：与 user-service 版不同，空 baseURL 原样保留
		t.Fatalf("baseURL = %q", c.baseURL)
	}

	c2 := NewGameCatalogClient("http://example.com/", time.Second)
	if c2.baseURL != "http://example.com" {
		t.Fatalf("trimmed baseURL = %q", c2.baseURL)
	}
}

func TestGameCatalogClient_ContextCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		w.Write([]byte(`{}`))
	}))
	defer server.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	client := NewGameCatalogClient(server.URL, time.Second)
	if _, err := client.GetFeatured(ctx, 3); err == nil {
		t.Fatal("canceled context must fail the call")
	}
}
