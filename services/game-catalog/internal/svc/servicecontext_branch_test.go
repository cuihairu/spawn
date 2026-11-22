package svc

// NewServiceContext 启动期守卫臂补漏：JWT 密钥未配置 → panic；内置演示密钥 →
// 大声告警（仍继续装配）；包体目录不可创建 → panic。

import (
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
	"github.com/tappi/tappi/services/game-catalog/utils"
)

// JWT 密钥为空 → ValidateJWTSecret 拒绝 → panic（GP-004 启动期守卫）。
func TestNewServiceContext_BlankJWTSecretPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("blank JWT secret must panic in NewServiceContext")
		}
	}()
	var c config.Config
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "games.db")
	c.Auth.JWTSecret = "   "
	NewServiceContext(c)
}

// 内置演示密钥：大声告警后照常装配（非本地开发须以 JWT_SECRET 覆盖）。
func TestNewServiceContext_WellKnownDevSecretWarns(t *testing.T) {
	var c config.Config
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "games.db")
	c.Auth.JWTSecret = utils.WellKnownDevJWTSecret

	ctx := NewServiceContext(c)
	if ctx.GameRepository == nil {
		t.Fatal("incomplete context")
	}
}

// 包体目录不可创建（父路径是文件）→ NewLocalPackageStore 失败 → panic。
func TestNewServiceContext_PackageStoreInitPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("unusable package dir must panic in NewServiceContext")
		}
	}()
	var c config.Config
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "games.db")
	c.Auth.JWTSecret = "svc-test-secret"
	c.Package.Dir = "/dev/null/packages"
	NewServiceContext(c)
}
