package svc

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/data-panel/internal/config"

	_ "github.com/mattn/go-sqlite3"
)

func sqliteConfig(t *testing.T) config.Config {
	t.Helper()
	var c config.Config
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "stats.db")
	c.Auth.JWTSecret = "dp-svc-test-secret"
	return c
}

// TestNewServiceContext_SQLite 成功装配：SQLite DSN（带查询参数，命中
// ensureSQLiteDir 的问号截断分支）→ 建表 + 内嵌种子写入就绪。
func TestNewServiceContext_SQLite(t *testing.T) {
	c := sqliteConfig(t)
	c.MySQL.DataSource += "?mode=rwc"

	ctx := NewServiceContext(c)
	if ctx.StatsRepo == nil || ctx.Auth == nil || ctx.Jwt == nil {
		t.Fatalf("incomplete context: %+v", ctx)
	}
	if ctx.Config.Auth.JWTSecret != "dp-svc-test-secret" {
		t.Fatalf("config not retained: %q", ctx.Config.Auth.JWTSecret)
	}
	// 种子数据已写入：演示用户 1001 至少一局战绩
	_, total, err := ctx.StatsRepo.ListGames(1001, 10, 0)
	if err != nil || total == 0 {
		t.Fatalf("ListGames(1001) = %d, %v; want seeded rows", total, err)
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

// TestNewServiceContext_SqlOpenMalformedDSNPanic MySQL 畸形 DSN（tcp 地址后
// 缺 "/dbname" 分隔）在 sql.Open 阶段 ParseDSN 即失败 → panic，连 Ping 都到不了。
func TestNewServiceContext_SqlOpenMalformedDSNPanic(t *testing.T) {
	c := sqliteConfig(t)
	c.MySQL.DataSource = "user:pw@tcp(127.0.0.1:3306)x"

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("malformed DSN must panic at sql.Open")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "连接数据库失败") {
			t.Fatalf("panic = %v, want 连接数据库失败", r)
		}
	}()
	NewServiceContext(c)
}

// TestNewServiceContext_BadSQLitePathPanic SQLite DSN 指向已存在的目录
// （ensureSQLiteDir 会自建缺失父目录，故用目录路径使 Open 不可开）→
// Open 成功但 Ping 失败 → panic。
func TestNewServiceContext_BadSQLitePathPanic(t *testing.T) {
	c := sqliteConfig(t)
	c.MySQL.DataSource = "file:" + t.TempDir() // 目录本身不能作为数据库文件打开

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("unopenable sqlite file must panic in NewServiceContext")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "数据库连接测试失败") {
			t.Fatalf("panic = %v, want 数据库连接测试失败", r)
		}
	}()
	NewServiceContext(c)
}

// TestNewServiceContext_CreateTableFailsPanic 只读库且 player_stats 表不存在：
// Ping 成功（读连接正常）→ CreateTable 因只读失败 → panic。
func TestNewServiceContext_CreateTableFailsPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ro.db")
	writable, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open writable db: %v", err)
	}
	if _, err := writable.Exec(`CREATE TABLE seed(x)`); err != nil { // 落盘建文件
		t.Fatalf("create seed table: %v", err)
	}
	if err := writable.Close(); err != nil {
		t.Fatalf("close writable db: %v", err)
	}

	c := sqliteConfig(t)
	c.MySQL.DataSource = "file:" + path + "?mode=ro"

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("readonly db without player_stats table must panic at CreateTable")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "创建战绩表失败") {
			t.Fatalf("panic = %v, want 创建战绩表失败", r)
		}
	}()
	NewServiceContext(c)
}

// TestNewServiceContext_SeedIfEmptyFailsPanic 预建缺列的 player_stats 表：
// CREATE TABLE IF NOT EXISTS 成功跳过，SeedIfEmpty 行数统计正常（0 行），
// Seed 插入因缺列报错 → panic。
func TestNewServiceContext_SeedIfEmptyFailsPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "badseed.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE player_stats (id INTEGER PRIMARY KEY)`); err != nil {
		t.Fatalf("create malformed table: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	c := sqliteConfig(t)
	c.MySQL.DataSource = "file:" + path

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("malformed table must panic at SeedIfEmpty")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "初始化战绩种子数据失败") {
			t.Fatalf("panic = %v, want 初始化战绩种子数据失败", r)
		}
	}()
	NewServiceContext(c)
}

// TestEnsureSQLiteDir 直测三分支：内存库与空路径原样返回；带查询参数的
// 文件 DSN 先按问号截断再建父目录，DSN 本身不改写。
func TestEnsureSQLiteDir(t *testing.T) {
	if got := ensureSQLiteDir(":memory:"); got != ":memory:" {
		t.Fatalf(":memory: = %q, want unchanged", got)
	}
	if got := ensureSQLiteDir("file:"); got != "file:" {
		t.Fatalf("file: = %q, want unchanged", got)
	}

	dir := filepath.Join(t.TempDir(), "a", "b")
	dsn := "file:" + filepath.Join(dir, "stats.db") + "?mode=rwc"
	if got := ensureSQLiteDir(dsn); got != dsn {
		t.Fatalf("dsn = %q, want unchanged", got)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("parent dir not created: %v", err)
	}
}
