package svc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/community/internal/config"
)

// baseConfig 指向临时目录的绝对路径数据文件（缺失 → 仓库种子回退）。
func baseConfig(t *testing.T) config.Config {
	t.Helper()
	dir := t.TempDir()
	var c config.Config
	c.DataSource.TopicsFile = filepath.Join(dir, "topics.json")
	c.DataSource.PostsFile = filepath.Join(dir, "posts.json")
	c.DataSource.FollowsFile = filepath.Join(dir, "follows.json")
	c.Auth.JWTSecret = "svc-test-secret"
	return c
}

// TestNewServiceContext_Success 成功装配：中间件、Jwt、三个仓库就绪。
func TestNewServiceContext_Success(t *testing.T) {
	ctx := NewServiceContext(baseConfig(t))

	if ctx.Auth == nil || ctx.Jwt == nil ||
		ctx.PostRepo == nil || ctx.TopicRepo == nil || ctx.FollowRepo == nil {
		t.Fatalf("incomplete context: %+v", ctx)
	}
	if ctx.Config.Auth.JWTSecret != "svc-test-secret" {
		t.Fatalf("config not retained: %+v", ctx.Config.Auth)
	}
}

// TestNewServiceContext_RelativePaths 相对路径走 filepath.Clean 分支；
// 相对文件不存在同样回退种子数据（与工作目录无关）。
func TestNewServiceContext_RelativePaths(t *testing.T) {
	c := baseConfig(t)
	c.DataSource.TopicsFile = "data/nonexistent-topics.json"
	c.DataSource.PostsFile = "data/nonexistent-posts.json"
	c.DataSource.FollowsFile = "data/nonexistent-follows.json"

	ctx := NewServiceContext(c)
	if ctx.TopicRepo == nil || ctx.PostRepo == nil || ctx.FollowRepo == nil {
		t.Fatal("relative paths must resolve to seeded repositories")
	}
}

// TestNewServiceContext_BadTopicsPanic 话题数据文件非法 JSON → 加载即 panic。
func TestNewServiceContext_BadTopicsPanic(t *testing.T) {
	c := baseConfig(t)
	if err := os.WriteFile(c.DataSource.TopicsFile, []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("write bad topics: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("corrupt topics file must panic in NewServiceContext")
		}
	}()
	NewServiceContext(c)
}

// TestNewServiceContext_BadPostsPanic 帖子数据文件非法 JSON → 加载即 panic。
func TestNewServiceContext_BadPostsPanic(t *testing.T) {
	c := baseConfig(t)
	if err := os.WriteFile(c.DataSource.PostsFile, []byte("[not json"), 0o644); err != nil {
		t.Fatalf("write bad posts: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("corrupt posts file must panic in NewServiceContext")
		}
	}()
	NewServiceContext(c)
}

// TestNewServiceContext_BadFollowsPanic 关注关系文件非法 JSON → 加载即 panic
// （前两个文件正常，触达第三处 panic 分支）。
func TestNewServiceContext_BadFollowsPanic(t *testing.T) {
	c := baseConfig(t)
	if err := os.WriteFile(c.DataSource.FollowsFile, []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("write bad follows: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("corrupt follows file must panic in NewServiceContext")
		}
	}()
	NewServiceContext(c)
}
