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

// TestCommunityClient_ListHotPosts 验证对 community GET /api/v1/posts/hot 的裸列表契约。
func TestCommunityClient_ListHotPosts(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"posts":[{"id":11,"topic_id":2,"author_id":7,"author_name":"alice",` +
			`"title":"通关心得","content":"很长很长","images":["x.png"],"like_count":12,"comment_count":3,` +
			`"created_at":"2026-10-04T10:00:00Z"}],"total":1}`))
	}))
	defer server.Close()

	client := NewCommunityClient(server.URL, time.Second)
	posts, err := client.ListHotPosts(context.Background(), 5)
	if err != nil {
		t.Fatalf("ListHotPosts: %v", err)
	}
	if gotPath != "/api/v1/posts/hot" || gotQuery != "limit=5" {
		t.Fatalf("request = %s?%s, want /api/v1/posts/hot?limit=5", gotPath, gotQuery)
	}
	if len(posts) != 1 {
		t.Fatalf("posts = %+v", posts)
	}
	p := posts[0]
	if p.Id != 11 || p.TopicId != 2 || p.AuthorId != 7 || p.AuthorName != "alice" ||
		p.Title != "通关心得" || p.Content != "很长很长" || p.LikeCount != 12 || p.CommentCount != 3 {
		t.Fatalf("post = %+v", p)
	}
}

// TestCommunityClient_ListTopics 验证对 community GET /api/v1/topics 的裸列表契约。
func TestCommunityClient_ListTopics(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"topics":[{"id":2,"name":"星陨圈","description":"不该进结构",` +
			`"post_count":30,"follower_count":88,"is_official":true}],"total":1}`))
	}))
	defer server.Close()

	client := NewCommunityClient(server.URL, time.Second)
	topics, err := client.ListTopics(context.Background(), 8)
	if err != nil {
		t.Fatalf("ListTopics: %v", err)
	}
	if gotPath != "/api/v1/topics" || gotQuery != "limit=8&offset=0" {
		t.Fatalf("request = %s?%s, want /api/v1/topics?limit=8&offset=0", gotPath, gotQuery)
	}
	if len(topics) != 1 || topics[0].Id != 2 || topics[0].Name != "星陨圈" ||
		!topics[0].IsOfficial || topics[0].PostCount != 30 || topics[0].FollowerCount != 88 {
		t.Fatalf("topics = %+v", topics)
	}
}

func TestCommunityClient_ListHotPosts_Non200(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewCommunityClient(server.URL, time.Second)
	if _, err := client.ListHotPosts(context.Background(), 5); err == nil ||
		!strings.Contains(err.Error(), "community status 503") {
		t.Fatalf("err = %v, want community status 503", err)
	}
}

func TestCommunityClient_ListTopics_ConnectionError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	base := server.URL
	server.Close()

	client := NewCommunityClient(base, time.Second)
	if _, err := client.ListTopics(context.Background(), 5); err == nil {
		t.Fatal("connection failure must error")
	}
}
