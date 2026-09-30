package svc

import (
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/user-service/internal/config"
)

func sqliteConfig(t *testing.T) config.Config {
	t.Helper()
	var c config.Config
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "users.db")
	c.Auth.JWTSecret = "svc-test-secret"
	c.Auth.TokenExpire = 7
	c.Services.GameCatalog.BaseURL = "http://localhost:8890/"
	c.Services.GameCatalog.Timeout = 1000
	return c
}

// TestNewServiceContext_SQLite 成功装配：SQLite DSN 命中 file: 分支，
// 建表成功，各组件就绪；BaseURL 尾斜杠被客户端裁剪。
func TestNewServiceContext_SQLite(t *testing.T) {
	ctx := NewServiceContext(sqliteConfig(t))

	if ctx.DB == nil || ctx.UserModel == nil || ctx.Auth == nil || ctx.GameCatalogClient == nil {
		t.Fatalf("incomplete context: %+v", ctx)
	}
	if ctx.Config.Auth.JWTSecret != "svc-test-secret" {
		t.Fatalf("config not retained: %+v", ctx.Config.Auth)
	}
	// 表已建好：查询不存在的用户名 → false, nil
	exists, err := ctx.UserModel.CheckUsernameExists("nobody-here")
	if err != nil || exists {
		t.Fatalf("CheckUsernameExists = %v, %v; want false, nil", exists, err)
	}
}

// TestNewServiceContext_MySQLBranchPanic 非 SQLite DSN 走 mysql 驱动分支，
// 连接不可达 → Ping 失败 → panic 快速失败（与生产行为一致）。
func TestNewServiceContext_MySQLBranchPanic(t *testing.T) {
	c := sqliteConfig(t)
	c.MySQL.DataSource = "root:pw@tcp(127.0.0.1:1)/none" // 无 file:/.db → mysql 分支；端口 1 立即拒绝

	defer func() {
		if recover() == nil {
			t.Fatal("unreachable database must panic in NewServiceContext")
		}
	}()
	NewServiceContext(c)
}

// TestNewServiceContext_BadSQLitePathPanic SQLite 文件路径所在目录不存在 →
// Open 成功但 Ping 失败 → panic。
func TestNewServiceContext_BadSQLitePathPanic(t *testing.T) {
	c := sqliteConfig(t)
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "no-such-dir", "users.db")

	defer func() {
		if recover() == nil {
			t.Fatal("unopenable sqlite file must panic in NewServiceContext")
		}
	}()
	NewServiceContext(c)
}
