package svc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/content-service/internal/config"
)

// baseConfig 指向临时目录的绝对路径数据文件（缺失 → 仓库种子回退）。
func baseConfig(t *testing.T) config.Config {
	t.Helper()
	var c config.Config
	c.DataSource.GuidesFile = filepath.Join(t.TempDir(), "guides.json")
	c.DataSource.CommentsFile = filepath.Join(t.TempDir(), "comments.json")
	c.Auth.JWTSecret = "svc-test-secret"
	c.Services.GameCatalog.BaseURL = "http://localhost:18890/"
	c.Services.GameCatalog.Timeout = 1000
	return c
}

// TestNewServiceContext_AbsolutePath 成功装配：组件齐全，配置保留，
// 数据文件缺失时仓库回退到种子数据。
func TestNewServiceContext_AbsolutePath(t *testing.T) {
	ctx := NewServiceContext(baseConfig(t))

	if ctx.GuideRepository == nil || ctx.CommentRepository == nil ||
		ctx.Auth == nil || ctx.GameCatalogClient == nil {
		t.Fatalf("incomplete context: %+v", ctx)
	}
	if ctx.Config.Auth.JWTSecret != "svc-test-secret" {
		t.Fatalf("config not retained: %+v", ctx.Config.Auth)
	}
}

// TestNewServiceContext_RelativePath 相对路径走 filepath.Clean 分支；
// 相对文件不存在同样回退种子数据（与工作目录无关）。
func TestNewServiceContext_RelativePath(t *testing.T) {
	c := baseConfig(t)
	c.DataSource.GuidesFile = "data/nonexistent-guides.json"
	c.DataSource.CommentsFile = "data/nonexistent-comments.json"

	ctx := NewServiceContext(c)
	if ctx.GuideRepository == nil || ctx.CommentRepository == nil {
		t.Fatal("relative path must resolve to seeded repositories")
	}
}

// TestNewServiceContext_BadGuidesPanic 攻略数据文件非法 JSON → 加载即 panic。
func TestNewServiceContext_BadGuidesPanic(t *testing.T) {
	c := baseConfig(t)
	if err := os.WriteFile(c.DataSource.GuidesFile, []byte("{invalid"), 0o644); err != nil {
		t.Fatalf("write bad guides: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("corrupt guides file must panic in NewServiceContext")
		}
	}()
	NewServiceContext(c)
}

// TestNewServiceContext_BadCommentsPanic 评论数据文件非法 JSON → 加载即 panic
// （攻略文件正常，触达第二处 panic 分支）。
func TestNewServiceContext_BadCommentsPanic(t *testing.T) {
	c := baseConfig(t)
	if err := os.WriteFile(c.DataSource.CommentsFile, []byte("[not json"), 0o644); err != nil {
		t.Fatalf("write bad comments: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("corrupt comments file must panic in NewServiceContext")
		}
	}()
	NewServiceContext(c)
}
