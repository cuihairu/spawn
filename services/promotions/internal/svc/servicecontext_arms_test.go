package svc

// servicecontext.go 补漏分支：空 JWT 密钥拒绝、内置演示密钥 WARN（不
// panic）、库打不开、各模型 CreateTables 失败（?mode=ro 预置前驱表后
// 目标表首建失败）panic 臂，以及装配成功路径。
//
// 登记不可达（不测）：无（全部 panic 臂经上述场景触达）。

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/promotions/internal/config"
	"github.com/tappi/tappi/services/promotions/model"
	"github.com/tappi/tappi/services/promotions/utils"

	_ "github.com/mattn/go-sqlite3"
)

// baseCfg 给一份可成功装配的配置（sqlite 文件库）。
func baseCfg(t *testing.T) (config.Config, string) {
	t.Helper()
	var c config.Config
	c.Auth.JWTSecret = "svc-test-secret"
	path := filepath.Join(t.TempDir(), "svc.db")
	c.MySQL.DataSource = "file:" + path
	return c, path
}

// assertPanic 断言 NewServiceContext panic 消息含片段。
func assertPanic(t *testing.T, c config.Config, want string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("want panic containing %q, got none", want)
		}
		if !strings.Contains(strings.ToLower(fmt.Sprintf("%v", r)), strings.ToLower(want)) {
			t.Fatalf("panic = %v, want contains %q", r, want)
		}
	}()
	NewServiceContext(c)
}

// roCfg 建可写库执行 precreate 后，以 ?mode=ro 重开同文件（只读库：
// 前驱表 IF NOT EXISTS 免建，目标表首建失败）。
func roCfg(t *testing.T, precreate func(*sql.DB)) config.Config {
	t.Helper()
	path := filepath.Join(t.TempDir(), "svc-ro.db")
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	// 先落一张表：0 字节 sqlite 文件以 ?mode=ro 打不开（只读模式不允许建页）
	if _, err := db.Exec(`CREATE TABLE _seed (x INTEGER)`); err != nil {
		t.Fatalf("seed table: %v", err)
	}
	if precreate != nil {
		precreate(db)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	c, _ := baseCfg(t)
	c.MySQL.DataSource = "file:" + path + "?mode=ro"
	return c
}

func TestNewServiceContext_Success(t *testing.T) {
	c, _ := baseCfg(t)
	// 配置 data-panel / community 基址，relay 也装配（非 noop）
	c.Analytics.BaseURL = "http://127.0.0.1:1"
	c.Notify.CommunityUrl = "http://127.0.0.1:1"
	sc := NewServiceContext(c)
	if sc == nil || sc.Auth == nil || sc.ActivityDB == nil || sc.RewardDB == nil ||
		sc.GiftPackDB == nil || sc.Notify == nil || sc.AnalyticsSvc == nil ||
		sc.AnalyticsRelay == nil || sc.AnalyticsOutbox == nil ||
		sc.NotifyRelay == nil || sc.NotifyOutbox == nil {
		t.Fatal("service context 装配不完整（存在空依赖）")
	}
}

func TestNewServiceContext_DemoSecretWarns(t *testing.T) {
	c, _ := baseCfg(t)
	c.Auth.JWTSecret = utils.WellKnownDevJWTSecret
	sc := NewServiceContext(c)
	if sc.Auth == nil {
		t.Fatal("demo-secret context Auth = nil")
	}
}

func TestNewServiceContext_PanicArms(t *testing.T) {
	// 空 JWT 密钥 → 拒绝
	c, _ := baseCfg(t)
	c.Auth.JWTSecret = " "
	assertPanic(t, c, "JWT 密钥")

	// 库打不开（父路径是文件）→ panic
	c2, path2 := baseCfg(t)
	blocker := path2 + ".blocker"
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("blocker: %v", err)
	}
	c2.MySQL.DataSource = "file:" + blocker + "/x.db"
	assertPanic(t, c2, "open db")

	// 活动表建失败（只读空库）
	assertPanic(t, roCfg(t, nil), "create activity tables")
	// 奖励表建失败（预置活动表）
	assertPanic(t, roCfg(t, func(db *sql.DB) {
		_ = model.NewActivityModel(db).CreateTables()
	}), "create reward grant tables")
	// 礼包表建失败（预置活动+奖励表）
	assertPanic(t, roCfg(t, func(db *sql.DB) {
		_ = model.NewActivityModel(db).CreateTables()
		_ = model.NewRewardGrantModel(db).CreateTables()
	}), "create gift pack tables")
	// analytics outbox 建失败（预置前四表）
	assertPanic(t, roCfg(t, func(db *sql.DB) {
		_ = model.NewActivityModel(db).CreateTables()
		_ = model.NewRewardGrantModel(db).CreateTables()
		_ = model.NewGiftPackModel(db).CreateTables()
	}), "create analytics outbox tables")
	// notify outbox 建失败（预置前五表）
	assertPanic(t, roCfg(t, func(db *sql.DB) {
		_ = model.NewActivityModel(db).CreateTables()
		_ = model.NewRewardGrantModel(db).CreateTables()
		_ = model.NewGiftPackModel(db).CreateTables()
		_ = model.NewAnalyticsOutboxModel(db).CreateTables()
	}), "create notify outbox tables")
}
