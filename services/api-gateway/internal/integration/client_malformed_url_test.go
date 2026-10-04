package integration

import (
	"context"
	"strings"
	"testing"
	"time"
)

// 各 client 构造器都不校验 baseURL；配置值携带控制字符时
// http.NewRequestWithContext 内部 url.Parse 失败，各方法的
// 构造请求错误分支由此触达（不经网络）。

func TestGameCatalogClient_MalformedBaseURL(t *testing.T) {
	c := NewGameCatalogClient("http://bad\x7fhost", time.Second)
	_, err := c.GetFeatured(context.Background(), 5)
	if err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatalf("err = %v, want url.Parse control-character failure", err)
	}
}

func TestUserServiceClient_Login_MalformedBaseURL(t *testing.T) {
	c := NewUserServiceClient("http://bad\nhost", time.Second)
	_, err := c.Login(context.Background(), LoginPayload{Username: "a", Password: "b"})
	if err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatalf("err = %v, want url.Parse control-character failure", err)
	}
}

func TestUserServiceClient_GetRecommendations_MalformedBaseURL(t *testing.T) {
	c := NewUserServiceClient("http://bad\nhost", time.Second)
	_, err := c.GetRecommendations(context.Background(), 1, 5, "RPG", "tok")
	if err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatalf("err = %v, want url.Parse control-character failure", err)
	}
}

func TestCommunityClient_ListHotPosts_MalformedBaseURL(t *testing.T) {
	c := NewCommunityClient("http://bad\x7fhost", time.Second)
	_, err := c.ListHotPosts(context.Background(), 5)
	if err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatalf("err = %v, want url.Parse control-character failure", err)
	}
}

func TestCommunityClient_ListTopics_MalformedBaseURL(t *testing.T) {
	c := NewCommunityClient("http://bad\x7fhost", time.Second)
	_, err := c.ListTopics(context.Background(), 5)
	if err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatalf("err = %v, want url.Parse control-character failure", err)
	}
}

func TestContentClient_ListGuides_MalformedBaseURL(t *testing.T) {
	c := NewContentClient("http://bad\x7fhost", time.Second)
	_, err := c.ListGuides(context.Background(), 5)
	if err == nil || !strings.Contains(err.Error(), "control character") {
		t.Fatalf("err = %v, want url.Parse control-character failure", err)
	}
}
