package integration

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestCommunityClient_GetPost 验证对 community GET /api/v1/posts/:id 的线上契约。
func TestCommunityClient_GetPost(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"post":{"id":1,"title":"分享测试帖","content":"正文","author_name":"alice"}}`))
	}))
	defer server.Close()

	client := NewCommunityClient(server.URL, time.Second)
	post, err := client.GetPost(context.Background(), 1)
	if err != nil {
		t.Fatalf("GetPost: %v", err)
	}
	if gotPath != "/api/v1/posts/1" || gotQuery != "" {
		t.Fatalf("request = %s?%s, want /api/v1/posts/1 without query", gotPath, gotQuery)
	}
	if post.Id != 1 || post.Title != "分享测试帖" || post.Content != "正文" || post.AuthorName != "alice" {
		t.Fatalf("post = %+v", post)
	}
}

func TestCommunityClient_GetPost_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"message":"帖子不存在"}`))
	}))
	defer server.Close()

	client := NewCommunityClient(server.URL, time.Second)
	_, err := client.GetPost(context.Background(), 0)
	if err == nil || !strings.Contains(err.Error(), "community status 404") {
		t.Fatalf("err = %v, want community status 404", err)
	}
}

func TestCommunityClient_GetPost_InvalidJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>not json</html>`))
	}))
	defer server.Close()

	client := NewCommunityClient(server.URL, time.Second)
	if _, err := client.GetPost(context.Background(), 1); err == nil {
		t.Fatal("invalid JSON body must error")
	}
}

func TestCommunityClient_GetPost_ConnectionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	base := server.URL
	server.Close()

	client := NewCommunityClient(base, time.Second)
	if _, err := client.GetPost(context.Background(), 1); err == nil {
		t.Fatal("connection failure must error")
	}
}

func TestCommunityClient_GetPost_Defaults(t *testing.T) {
	c := NewCommunityClient("", 0)
	if c.httpClient.Timeout != 5*time.Second {
		t.Fatalf("default timeout = %v, want 5s", c.httpClient.Timeout)
	}
}

func TestCommunityClient_GetPost_MalformedBaseURL(t *testing.T) {
	c := NewCommunityClient("http://bad\x7fhost", time.Second)
	_, err := c.GetPost(context.Background(), 1)
	if err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatalf("err = %v, want url.Parse control-character failure", err)
	}
}
