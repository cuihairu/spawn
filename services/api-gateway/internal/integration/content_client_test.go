package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestContentClient_ListGuides 验证对 content GET /api/v1/guides 的信封契约。
func TestContentClient_ListGuides(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":[` +
			`{"id":5,"game_id":"g1","game_title":"星陨物语","title":"全收集攻略","summary":"摘要",` +
			`"content":"不该进结构的长文","author_name":"bob","likes":9,"views":120,"created_at":"2026-10-01T08:00:00Z"}],` +
			`"total":1,"page":1}`))
	}))
	defer server.Close()

	client := NewContentClient(server.URL, time.Second)
	guides, err := client.ListGuides(context.Background(), 10)
	if err != nil {
		t.Fatalf("ListGuides: %v", err)
	}
	if gotPath != "/api/v1/guides" || gotQuery != "page=1&page_size=10" {
		t.Fatalf("request = %s?%s, want /api/v1/guides?page=1&page_size=10", gotPath, gotQuery)
	}
	if len(guides) != 1 {
		t.Fatalf("guides = %+v", guides)
	}
	g := guides[0]
	if g.Id != 5 || g.GameId != "g1" || g.GameTitle != "星陨物语" || g.Title != "全收集攻略" ||
		g.Summary != "摘要" || g.AuthorName != "bob" || g.Likes != 9 || g.Views != 120 {
		t.Fatalf("guide = %+v", g)
	}
}

func TestContentClient_ListGuides_BusinessCode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":500,"message":"内部错误"}`))
	}))
	defer server.Close()

	client := NewContentClient(server.URL, time.Second)
	_, err := client.ListGuides(context.Background(), 5)
	if err == nil || !strings.Contains(err.Error(), "content code 500") {
		t.Fatalf("err = %v, want content code 500", err)
	}
}

func TestContentClient_ListGuides_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer server.Close()

	client := NewContentClient(server.URL, time.Second)
	if _, err := client.ListGuides(context.Background(), 5); err == nil ||
		!strings.Contains(err.Error(), "content status 502") {
		t.Fatalf("err = %v, want content status 502", err)
	}
}

func TestContentClient_ListGuides_ConnectionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	base := server.URL
	server.Close()

	client := NewContentClient(base, time.Second)
	if _, err := client.ListGuides(context.Background(), 5); err == nil {
		t.Fatal("connection failure must error")
	}
}

func TestContentClient_Defaults(t *testing.T) {
	c := NewContentClient("", 0)
	if c.httpClient.Timeout != 5*time.Second {
		t.Fatalf("default timeout = %v, want 5s", c.httpClient.Timeout)
	}
}

func TestContentClient_ListGuides_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>not json</html>`))
	}))
	defer server.Close()

	client := NewContentClient(server.URL, time.Second)
	if _, err := client.ListGuides(context.Background(), 5); err == nil {
		t.Fatal("invalid JSON body must error")
	}
}
