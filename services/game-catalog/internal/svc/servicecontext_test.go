package svc

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/game-catalog/internal/config"

	_ "github.com/mattn/go-sqlite3"
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

// --- 只读库启动梯子（与 content-service svc 同款口径） ---

// roNewServiceContext 以只读 DSN 装配服务上下文（预期 panic，由调用方 recover）。
func roNewServiceContext(t *testing.T, path string) {
	t.Helper()

	defer func() {
		if recover() == nil {
			t.Fatal("read-only database must fail startup in NewServiceContext")
		}
	}()

	var c config.Config
	c.MySQL.DataSource = "file:" + path + "?mode=ro"
	c.Auth.JWTSecret = "ro-secret"
	NewServiceContext(c)
}

// TestNewServiceContext_ReadOnly_CreateGamesTablePanics 已存在但无 games
// 表的只读库：Ping 通过，建游戏表触雷（梯子第一级）。文件必须先落盘，
// 否则 mode=ro 在打开阶段即失败，够不到建表分支。
func TestNewServiceContext_ReadOnly_CreateGamesTablePanics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "games.db")

	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE marker (x INTEGER)`); err != nil {
		t.Fatalf("create marker table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	roNewServiceContext(t, path)
}

// TestNewServiceContext_ReadOnly_SeedGamesPanics 预建空 games 表的只读库：
// CreateGamesTable 幂等 no-op，空表种子写入触雷（梯子第二级）。
func TestNewServiceContext_ReadOnly_SeedGamesPanics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "games.db")

	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE games (
		id VARCHAR(64) PRIMARY KEY,
		title VARCHAR(256) NOT NULL,
		description VARCHAR(2048) NOT NULL,
		genres VARCHAR(1024) NOT NULL,
		platforms VARCHAR(1024) NOT NULL,
		tags VARCHAR(1024) NOT NULL,
		release_date VARCHAR(32) NOT NULL,
		developer VARCHAR(256) NOT NULL,
		publisher VARCHAR(256) NOT NULL,
		score DOUBLE NOT NULL DEFAULT 0,
		trending_score INTEGER NOT NULL DEFAULT 0,
		cover_image VARCHAR(512) NOT NULL)`); err != nil {
		t.Fatalf("precreate games table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	roNewServiceContext(t, path)
}
