package svc

import (
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
)

// TestNewServiceContext_SQLiteSeed SQLite 文件 DSN：建库建表 + 空表写入内嵌
// 种子；GameRepository 经缓存可查种子游戏。
func TestNewServiceContext_SQLiteSeed(t *testing.T) {
	var c config.Config
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "games.db")
	c.Auth.JWTSecret = "svc-test-secret"

	ctx := NewServiceContext(c)

	if ctx.GameRepository == nil || ctx.Auth == nil {
		t.Fatalf("incomplete context: %+v", ctx)
	}
	game, err := ctx.GameRepository.Get("game-elden-ring")
	if err != nil || game == nil || game.Title != "Elden Ring" {
		t.Fatalf("expected embedded seed, got game=%+v err=%v", game, err)
	}
}

// TestNewServiceContext_MySQLDialFail 结构合法但目标不可达的 MySQL DSN →
// Ping 处 panic（「数据库连接测试失败」，启动期快速失败）。
func TestNewServiceContext_MySQLDialFail(t *testing.T) {
	var c config.Config
	c.MySQL.DataSource = "user:pw@tcp(127.0.0.1:3306)/db"
	c.Auth.JWTSecret = "panic-test-secret"

	defer func() {
		if recover() == nil {
			t.Fatal("unreachable MySQL must panic in NewServiceContext")
		}
	}()
	NewServiceContext(c)
}

// TestNewServiceContext_UnparsableDSN 结构不完整（缺库名分隔符）的 DSN
// 在 sql.Open 即报错（go-sql-driver 急切解析），走「连接数据库失败」panic 分支。
func TestNewServiceContext_UnparsableDSN(t *testing.T) {
	var c config.Config
	c.MySQL.DataSource = "not-a-dsn"
	c.Auth.JWTSecret = "panic-test-secret"

	defer func() {
		if recover() == nil {
			t.Fatal("unparsable DSN must panic in NewServiceContext")
		}
	}()
	NewServiceContext(c)
}

// TestNewServiceContext_InMemoryDSN 内存库 DSN（带 ? 参数）：ensureSQLiteDir
// 识别 :memory: 与参数截断分支，跳过目录创建，服务照常建表种子。
func TestNewServiceContext_InMemoryDSN(t *testing.T) {
	var c config.Config
	c.MySQL.DataSource = "file::memory:?cache=shared"
	c.Auth.JWTSecret = "mem-test-secret"

	ctx := NewServiceContext(c)
	if _, err := ctx.GameRepository.Get("game-elden-ring"); err != nil {
		t.Fatalf("seeded game: %v", err)
	}
}
