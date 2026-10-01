package svc

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/content-service/internal/config"

	_ "github.com/mattn/go-sqlite3"
)

// TestNewServiceContext_SQLiteSeed SQLite 文件 DSN：建库建表 + 空表写入内嵌
// 种子；两仓储经缓存可查种子攻略与评论。
func TestNewServiceContext_SQLiteSeed(t *testing.T) {
	var c config.Config
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "content.db")
	c.Auth.JWTSecret = "svc-test-secret"
	c.Services.GameCatalog.BaseURL = "http://localhost:18890/"
	c.Services.GameCatalog.Timeout = 1000

	ctx := NewServiceContext(c)

	if ctx.GuideRepository == nil || ctx.CommentRepository == nil ||
		ctx.Auth == nil || ctx.GameCatalogClient == nil {
		t.Fatalf("incomplete context: %+v", ctx)
	}
	guide, err := ctx.GuideRepository.Get(1)
	if err != nil || guide == nil || guide.Title != "新手入门指南：如何在交界地存活下来" {
		t.Fatalf("expected embedded guide seed, got guide=%+v err=%v", guide, err)
	}
	comment, err := ctx.CommentRepository.Get(1)
	if err != nil || comment == nil || comment.Content != "这个攻略写得太好了！帮了我大忙！" {
		t.Fatalf("expected embedded comment seed, got comment=%+v err=%v", comment, err)
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
	if _, err := ctx.GuideRepository.Get(1); err != nil {
		t.Fatalf("seeded guide: %v", err)
	}
	if _, err := ctx.CommentRepository.Get(1); err != nil {
		t.Fatalf("seeded comment: %v", err)
	}
}

// --- 只读库快速失败：建表/种子四条 panic 分支 ---
// mode=ro 打开预建好的库文件：Ping 可读通过，任何写入（建表/种子）即失败，
// 逐项裁剪预建内容即可精确触达 NewServiceContext 的四条启动期 panic。

// precreateDB 预建库文件与可选的表（mode=ro 打开不存在的文件会在 Ping 阶段失败，
// 故无条件 Ping 以落盘空库文件）。
func precreateDB(t *testing.T, path string, withGuides, withComments bool) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("precreate open: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Fatalf("precreate ping: %v", err)
	}
	if withGuides {
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS guides (
			id INTEGER PRIMARY KEY AUTOINCREMENT, game_id VARCHAR(64) NOT NULL,
			game_title VARCHAR(256) NOT NULL, title VARCHAR(256) NOT NULL, content TEXT NOT NULL,
			summary VARCHAR(1024) NOT NULL, cover_image VARCHAR(512) NOT NULL,
			author_id INTEGER NOT NULL DEFAULT 0, author_name VARCHAR(64) NOT NULL,
			tags VARCHAR(512) NOT NULL, views INTEGER NOT NULL DEFAULT 0,
			likes INTEGER NOT NULL DEFAULT 0, is_published INTEGER NOT NULL DEFAULT 0,
			created_at VARCHAR(32) NOT NULL, updated_at VARCHAR(32) NOT NULL)`); err != nil {
			t.Fatalf("precreate guides table: %v", err)
		}
	}
	if withComments {
		if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS comments (
			id INTEGER PRIMARY KEY AUTOINCREMENT, target_type VARCHAR(32) NOT NULL,
			target_id INTEGER NOT NULL DEFAULT 0, user_id INTEGER NOT NULL DEFAULT 0,
			user_name VARCHAR(64) NOT NULL, content TEXT NOT NULL,
			parent_id INTEGER NOT NULL DEFAULT 0, reply_to_id INTEGER NOT NULL DEFAULT 0,
			likes INTEGER NOT NULL DEFAULT 0, created_at VARCHAR(32) NOT NULL,
			updated_at VARCHAR(32) NOT NULL)`); err != nil {
			t.Fatalf("precreate comments table: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("precreate close: %v", err)
	}
}

// seedGuideRow 向预建库写入一条攻略（攻略半边在只读库上须能无写入通过）。
func seedGuideRow(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("seed row open: %v", err)
	}
	defer db.Close()
	if _, err := db.Exec(`INSERT INTO guides (id, game_id, game_title, title, content,
		summary, cover_image, author_id, author_name, tags, views, likes, is_published,
		created_at, updated_at) VALUES (1, 'g', 'G', 't', 'c', 's', '', 1, 'a', '[]',
		0, 0, 1, '2024-01-01T00:00:00Z', '2024-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("seed guide row: %v", err)
	}
}

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

func TestNewServiceContext_ReadOnly_CreateGuidesTablePanics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.db")
	precreateDB(t, path, false, false) // 空库：建攻略表即触雷
	roNewServiceContext(t, path)
}

func TestNewServiceContext_ReadOnly_SeedGuidesPanics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.db")
	precreateDB(t, path, true, false) // 仅攻略表：建表 no-op，空表种子写入触雷
	roNewServiceContext(t, path)
}

func TestNewServiceContext_ReadOnly_CreateCommentsTablePanics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.db")
	precreateDB(t, path, true, false)
	seedGuideRow(t, path) // 攻略半边可只读通过，评论建表触雷
	roNewServiceContext(t, path)
}

func TestNewServiceContext_ReadOnly_SeedCommentsPanics(t *testing.T) {
	path := filepath.Join(t.TempDir(), "content.db")
	precreateDB(t, path, true, true)
	seedGuideRow(t, path) // 攻略全通过，评论表已建但为空：评论种子写入触雷
	roNewServiceContext(t, path)
}
