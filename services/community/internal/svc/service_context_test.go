package svc

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/community/internal/config"
	"github.com/tappi/tappi/services/community/internal/model"

	_ "github.com/mattn/go-sqlite3"
)

// baseConfig 指向临时目录的文件 SQLite DSN（ensureSQLiteDir 会预创目录）。
func baseConfig(t *testing.T) config.Config {
	t.Helper()
	var c config.Config
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "community.db")
	c.Auth.JWTSecret = "svc-test-secret"
	return c
}

// TestNewServiceContext_SQLiteSeed 成功装配：建表 + 种子写入 + 三个仓库就绪。
func TestNewServiceContext_SQLiteSeed(t *testing.T) {
	ctx := NewServiceContext(baseConfig(t))

	if ctx.Auth == nil || ctx.Jwt == nil ||
		ctx.PostRepo == nil || ctx.TopicRepo == nil || ctx.FollowRepo == nil {
		t.Fatalf("incomplete context: %+v", ctx)
	}
	if ctx.Config.Auth.JWTSecret != "svc-test-secret" {
		t.Fatalf("config not retained: %+v", ctx.Config.Auth)
	}

	topic, err := ctx.TopicRepo.Get(1)
	if err != nil || topic.Name != "《黑神话：悟空》" {
		t.Fatalf("seeded topic = %+v, %v", topic, err)
	}
	post, err := ctx.PostRepo.Get(1)
	if err != nil || post.Title == "" {
		t.Fatalf("seeded post = %+v, %v", post, err)
	}
	if _, total := ctx.PostRepo.List(model.PostListFilter{Limit: 50}); total != 2 {
		t.Fatalf("seeded post total = %d, want 2", total)
	}
}

// TestNewServiceContext_SQLiteDirAndQueryParams DSN 带查询参数且父目录不存在：
// ensureSQLiteDir 截断 ?、预创目录后正常装配。
func TestNewServiceContext_SQLiteDirAndQueryParams(t *testing.T) {
	c := baseConfig(t)
	c.MySQL.DataSource += "?_busy_timeout=5000"

	ctx := NewServiceContext(c)
	if _, err := ctx.TopicRepo.Get(1); err != nil {
		t.Fatalf("seeded topic via param DSN: %v", err)
	}
}

// TestNewServiceContext_InMemoryDSN 内存库原样透传（ensureSQLiteDir 的
// ":memory:" 跳过分支），连接池钳制 1 保证表在同连接上存活，种子可读。
func TestNewServiceContext_InMemoryDSN(t *testing.T) {
	c := baseConfig(t)
	c.MySQL.DataSource = "file::memory:"

	ctx := NewServiceContext(c)
	topic, err := ctx.TopicRepo.Get(1)
	if err != nil || !topic.IsOfficial {
		t.Fatalf("in-memory seeded topic = %+v, %v", topic, err)
	}
}

// TestNewServiceContext_MemoryQueryParamsDSN "file:?..." 形式：截参数后路径为空，
// 走 ensureSQLiteDir 的空路径跳过分支，mode=memory 建共享内存库。
func TestNewServiceContext_MemoryQueryParamsDSN(t *testing.T) {
	c := baseConfig(t)
	c.MySQL.DataSource = "file:?mode=memory&cache=shared"

	ctx := NewServiceContext(c)
	if _, err := ctx.TopicRepo.Get(1); err != nil {
		t.Fatalf("shared-memory seeded topic: %v", err)
	}
}

// TestNewServiceContext_UnparsableDSN 非法 DSN → MySQL 驱动在 Open 期即报错。
func TestNewServiceContext_UnparsableDSN(t *testing.T) {
	c := baseConfig(t)
	c.MySQL.DataSource = "not-a-dsn"

	defer func() {
		v := recover()
		if v == nil || !strings.Contains(fmt.Sprint(v), "连接数据库失败") {
			t.Fatalf("panic = %v, want 连接数据库失败", v)
		}
	}()
	NewServiceContext(c)
}

// TestNewServiceContext_MySQLDialFail 可解析但不可达的 MySQL DSN → Ping 期
// 连接测试失败 panic。
func TestNewServiceContext_MySQLDialFail(t *testing.T) {
	c := baseConfig(t)
	c.MySQL.DataSource = "user:pw@tcp(127.0.0.1:1)/community"

	defer func() {
		v := recover()
		if v == nil || !strings.Contains(fmt.Sprint(v), "数据库连接测试失败") {
			t.Fatalf("panic = %v, want 数据库连接测试失败", v)
		}
	}()
	NewServiceContext(c)
}

// precreateDB 按粒度预建只读测试库：Ping 先行（建文件），再按需建表/种子。
func precreateDB(t *testing.T, path string, withTopics, topicsSeeded, withPosts, postsSeeded bool) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("precreate open: %v", err)
	}
	defer db.Close()
	if err := db.Ping(); err != nil {
		t.Fatalf("precreate ping: %v", err)
	}

	tm := model.NewTopicModel(db)
	pm := model.NewPostModel(db)
	if withTopics {
		if err := tm.CreateTopicsTable(); err != nil {
			t.Fatalf("precreate topics table: %v", err)
		}
	}
	if topicsSeeded {
		if err := tm.SeedIfEmpty(); err != nil {
			t.Fatalf("precreate topics seed: %v", err)
		}
	}
	if withPosts {
		if err := pm.CreatePostsTable(); err != nil {
			t.Fatalf("precreate posts table: %v", err)
		}
	}
	if postsSeeded {
		if err := pm.SeedIfEmpty(); err != nil {
			t.Fatalf("precreate posts seed: %v", err)
		}
	}
}

// roNewServiceContext 以只读 DSN 打开预建库（写路径全部失败）。
func roNewServiceContext(t *testing.T, path string) {
	t.Helper()
	c := baseConfig(t)
	c.MySQL.DataSource = "file:" + path + "?mode=ro"
	NewServiceContext(c)
}

// wantPanic 断言 recover 值为包含 want 的字符串。
func wantPanic(t *testing.T, want string, fn func()) {
	t.Helper()
	defer func() {
		v := recover()
		if v == nil || !strings.Contains(fmt.Sprint(v), want) {
			t.Fatalf("panic = %v, want containing %q", v, want)
		}
	}()
	fn()
}

// 以下五例沿启动序列逐段推进，覆盖五处建表/种子 panic 分支。

func TestNewServiceContext_RO_NoTablesPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "community.db")
	precreateDB(t, path, false, false, false, false)

	wantPanic(t, "创建话题表失败", func() { roNewServiceContext(t, path) })
}

func TestNewServiceContext_RO_TopicSeedPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "community.db")
	precreateDB(t, path, true, false, false, false)

	wantPanic(t, "初始化话题种子数据失败", func() { roNewServiceContext(t, path) })
}

func TestNewServiceContext_RO_PostsTablePanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "community.db")
	precreateDB(t, path, true, true, false, false)

	wantPanic(t, "创建帖子表失败", func() { roNewServiceContext(t, path) })
}

func TestNewServiceContext_RO_PostSeedPanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "community.db")
	precreateDB(t, path, true, true, true, false)

	wantPanic(t, "初始化帖子种子数据失败", func() { roNewServiceContext(t, path) })
}

func TestNewServiceContext_RO_FollowsTablePanic(t *testing.T) {
	path := filepath.Join(t.TempDir(), "community.db")
	precreateDB(t, path, true, true, true, true)

	wantPanic(t, "创建关注关系表失败", func() { roNewServiceContext(t, path) })
}
