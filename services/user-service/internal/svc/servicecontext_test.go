package svc

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/user-service/internal/config"
	"github.com/tappi/tappi/services/user-service/utils"
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

// TestNewServiceContext_SqlOpenMalformedDSNPanic MySQL 畸形 DSN（tcp 地址后
// 缺 "/dbname" 分隔）在 sql.Open 阶段 ParseDSN 即失败 → panic，连 Ping 都到不了。
func TestNewServiceContext_SqlOpenMalformedDSNPanic(t *testing.T) {
	c := sqliteConfig(t)
	c.MySQL.DataSource = "user:pw@tcp(127.0.0.1:3306)x" // 无 file:/.db → mysql 分支

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

// TestNewServiceContext_CreateUsersTableFailsPanic 只读库且 users 表不存在：
// Ping 成功（读连接正常）→ CreateUsersTable 的 SQLite/MySQL 两种建表格式
// 均因只读/语法失败 → panic。
func TestNewServiceContext_CreateUsersTableFailsPanic(t *testing.T) {
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
	c.MySQL.DataSource = "file:" + path + "?mode=ro" // 含 file: → sqlite 分支

	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("readonly db without users table must panic at CreateUsersTable")
		}
		if msg, ok := r.(string); !ok || !strings.Contains(msg, "创建用户表失败") {
			t.Fatalf("panic = %v, want 创建用户表失败", r)
		}
	}()
	NewServiceContext(c)
}

// wantPanic 断言 NewServiceContext panic 且信息含 want。
func wantPanic(t *testing.T, name, want string, fn func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("%s: want panic", name)
		}
		msg, ok := r.(string)
		if !ok || !strings.Contains(msg, want) {
			t.Fatalf("%s: panic = %v, want containing %q", name, r, want)
		}
	}()
	fn()
}

// TestNewServiceContext_EmptyJWTPanic 空密钥启动期拒绝（不落库不装配）。
func TestNewServiceContext_EmptyJWTPanic(t *testing.T) {
	c := sqliteConfig(t)
	c.Auth.JWTSecret = ""
	wantPanic(t, "empty jwt", "JWT 密钥未配置", func() { NewServiceContext(c) })
}

// TestNewServiceContext_DevSecretWarns 内置演示密钥：大声告警后照常装配
// （本地开发默认形态），并顺带覆盖 InternalToken 非空分支与启用态
// analytics 客户端 → relay 接线。
func TestNewServiceContext_DevSecretWarns(t *testing.T) {
	c := sqliteConfig(t)
	c.Auth.JWTSecret = utils.WellKnownDevJWTSecret
	c.Services.GameCatalog.InternalToken = "svc-internal-tok"
	c.Analytics.BaseURL = "http://127.0.0.1:1" // 启用态 → relay 非空（noop 则 nil）

	ctx := NewServiceContext(c)
	if ctx.Auth == nil || ctx.GameCatalogClient == nil {
		t.Fatalf("dev-secret context incomplete: %+v", ctx)
	}
	if ctx.AnalyticsSvc == nil || ctx.AnalyticsRelay == nil || ctx.PromoAttrRelay == nil {
		t.Fatalf("relays not wired: %+v", ctx)
	}
}

// seedTables 预建只读库：seedDDL 落盘后以 ro 模式复用——已有表走
// CREATE IF NOT EXISTS no-op（不写库），缺表才真正建（只读必败），
// 驱动 outbox 建表 panic 臂。
func seedTables(t *testing.T, seedDDL ...string) config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "users.db")
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	for _, ddl := range seedDDL {
		if _, err := db.Exec(ddl); err != nil {
			t.Fatalf("exec seed %q: %v", ddl, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}

	c := sqliteConfig(t)
	c.MySQL.DataSource = "file:" + path + "?mode=ro"
	return c
}

// TestNewServiceContext_AnalyticsOutboxFailsPanic 只读库有 users 表（建表
// no-op 过）而无 analytics_outbox → GP-402 表建失败 → panic（表始终建，
// 失败不带病启动）。
func TestNewServiceContext_AnalyticsOutboxFailsPanic(t *testing.T) {
	c := seedTables(t, "CREATE TABLE users (id INTEGER PRIMARY KEY)")
	wantPanic(t, "analytics outbox", "create analytics outbox tables", func() { NewServiceContext(c) })
}

// TestNewServiceContext_PromoOutboxFailsPanic 只读库 users/analytics_outbox
// 皆备而缺 promo_attribution_outbox → GP-403 表建失败 → panic。
func TestNewServiceContext_PromoOutboxFailsPanic(t *testing.T) {
	c := seedTables(t, "CREATE TABLE users (id INTEGER PRIMARY KEY)",
		"CREATE TABLE analytics_outbox (event_id TEXT PRIMARY KEY)")
	wantPanic(t, "promo outbox", "create promo attr outbox tables", func() { NewServiceContext(c) })
}
