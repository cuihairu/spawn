package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestUserServiceClient_Login 验证对 user-service POST /auth/login 的线上契约。
func TestUserServiceClient_Login(t *testing.T) {
	var gotPath, gotContentType, gotBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		b, _ := io.ReadAll(r.Body)
		gotBody = string(b)
		w.Write([]byte(`{"token":"jwt-token-1","user_info":{"id":7,"username":"alice"}}`))
	}))
	defer server.Close()

	client := NewUserServiceClient(server.URL, time.Second)
	result, err := client.Login(context.Background(), LoginPayload{Username: "alice", Password: "pw"})
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	if gotPath != "/auth/login" {
		t.Fatalf("path = %q, want /auth/login", gotPath)
	}
	if !strings.HasPrefix(gotContentType, "application/json") {
		t.Fatalf("content-type = %q", gotContentType)
	}
	var sent LoginPayload
	if err := json.Unmarshal([]byte(gotBody), &sent); err != nil {
		t.Fatalf("request body not JSON LoginPayload: %q (%v)", gotBody, err)
	}
	if sent.Username != "alice" || sent.Password != "pw" {
		t.Fatalf("request body = %q", gotBody)
	}

	if result.Token != "jwt-token-1" {
		t.Fatalf("token = %q", result.Token)
	}
	info, _ := result.UserInfo.(map[string]interface{})
	if info["username"] != "alice" {
		t.Fatalf("user_info = %#v", result.UserInfo)
	}
}

func TestUserServiceClient_Login_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"用户名或密码错误"}`))
	}))
	defer server.Close()

	client := NewUserServiceClient(server.URL, time.Second)
	_, err := client.Login(context.Background(), LoginPayload{Username: "a", Password: "b"})
	if err == nil || !strings.Contains(err.Error(), "login failed") ||
		!strings.Contains(err.Error(), "用户名或密码错误") {
		t.Fatalf("err = %v, want login failed + upstream body", err)
	}
}

func TestUserServiceClient_Login_Non200EmptyBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewUserServiceClient(server.URL, time.Second)
	_, err := client.Login(context.Background(), LoginPayload{})
	if err == nil || !strings.Contains(err.Error(), "upstream error") {
		t.Fatalf("err = %v, want fallback message for empty body", err)
	}
}

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

func TestUserServiceClient_Login_ConnectionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	base := server.URL
	server.Close()

	client := NewUserServiceClient(base, time.Second)
	if _, err := client.Login(context.Background(), LoginPayload{Username: "a", Password: "b"}); err == nil {
		t.Fatal("connection failure must error")
	}
}

func TestUserServiceClient_Login_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`<html>not json</html>`))
	}))
	defer server.Close()

	client := NewUserServiceClient(server.URL, time.Second)
	if _, err := client.Login(context.Background(), LoginPayload{Username: "a", Password: "b"}); err == nil {
		t.Fatal("invalid JSON body must error")
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
