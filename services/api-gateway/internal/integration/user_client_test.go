package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestUserServiceClient_GetRecommendations 验证对 user-service
// GET /users/:id/recommendations 的线上契约（query + Bearer 透传）。
func TestUserServiceClient_GetRecommendations(t *testing.T) {
	var gotPath, gotAuth, gotLimit, gotGenres string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotLimit = r.URL.Query().Get("limit")
		gotGenres = r.URL.Query().Get("genres")
		w.Write([]byte(`{"code":0,"message":"ok","data":{"games":[{"id":"g-1"}]}}`))
	}))
	defer server.Close()

	client := NewUserServiceClient(server.URL, time.Second)
	resp, err := client.GetRecommendations(context.Background(), 7, 5, "RPG,FPS", "jwt-token-1")
	if err != nil {
		t.Fatalf("GetRecommendations: %v", err)
	}

	if gotPath != "/users/7/recommendations" {
		t.Fatalf("path = %q", gotPath)
	}
	if gotAuth != "Bearer jwt-token-1" {
		t.Fatalf("authorization = %q", gotAuth)
	}
	if gotLimit != "5" || gotGenres != "RPG,FPS" {
		t.Fatalf("query limit=%q genres=%q", gotLimit, gotGenres)
	}
	if resp.Code != 0 || resp.Message != "ok" {
		t.Fatalf("payload = %+v", resp)
	}
	data, _ := resp.Data.(map[string]interface{})
	games, _ := data["games"].([]interface{})
	if len(games) != 1 {
		t.Fatalf("data.games = %#v", data)
	}
}

func TestUserServiceClient_GetRecommendations_OmitsEmptyAndToken(t *testing.T) {
	var gotQuery map[string][]string
	var gotAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query()
		gotAuth = r.Header.Get("Authorization")
		w.Write([]byte(`{"code":0,"message":"ok","data":null}`))
	}))
	defer server.Close()

	client := NewUserServiceClient(server.URL, time.Second)
	if _, err := client.GetRecommendations(context.Background(), 3, 0, "", ""); err != nil {
		t.Fatalf("GetRecommendations: %v", err)
	}
	if _, ok := gotQuery["limit"]; ok {
		t.Fatal("limit=0 must be omitted")
	}
	if _, ok := gotQuery["genres"]; ok {
		t.Fatal("empty genres must be omitted")
	}
	if gotAuth != "" {
		t.Fatal("empty token must not set Authorization header")
	}
}

func TestUserServiceClient_GetRecommendations_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte("denied"))
	}))
	defer server.Close()

	client := NewUserServiceClient(server.URL, time.Second)
	_, err := client.GetRecommendations(context.Background(), 1, 5, "", "")
	if err == nil || !strings.Contains(err.Error(), "recommendation failed: denied") {
		t.Fatalf("err = %v", err)
	}
}

func TestUserServiceClient_Defaults(t *testing.T) {
	c := NewUserServiceClient("", 0)
	if c.httpClient.Timeout != 5*time.Second {
		t.Fatalf("default timeout = %v, want 5s", c.httpClient.Timeout)
	}
}

func TestUserServiceClient_GetRecommendations_ConnectionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	client := NewUserServiceClient(server.URL, time.Second)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.GetRecommendations(ctx, 1, 5, "", ""); err == nil {
		t.Fatal("canceled context must error")
	}
}

func TestUserServiceClient_GetRecommendations_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{not-json`))
	}))
	defer server.Close()

	client := NewUserServiceClient(server.URL, time.Second)
	if _, err := client.GetRecommendations(context.Background(), 1, 5, "", ""); err == nil {
		t.Fatal("invalid JSON body must error")
	}
}
