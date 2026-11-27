package svc

// servicecontext.go 剩余补漏分支：构造成功路径（全部依赖装配 + 字段非空）
// 与启动期 panic 臂（空 JWT 密钥、空 DSN 打不开库、按模型顺序的建表失败
// panic）。建表 panic 阶梯用只读重开 + 逐模型预建的方式把失败定位到目标
// 模型：目标之前的模型表已存在（IF NOT EXISTS 只读空转），目标模型首个
// 缺失表在只读库上 CREATE 失败。
//
// 登记不可达（不测）：无（本函数全部 panic 臂均由上述阶梯触达）。

import (
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/tappi/tappi/services/payments/internal/config"
	"github.com/tappi/tappi/services/payments/model"
	"github.com/tappi/tappi/services/payments/utils"
)

// svcConfig 给一份不触发启动期 panic 的配置（sqlite 文件库）。
func svcConfig(dsn string) config.Config {
	var c config.Config
	c.MySQL.DataSource = dsn
	c.Auth.JWTSecret = "test-jwt-secret"
	c.Channel.SandboxSecret = "non-demo-sandbox"
	c.Delivery.Secret = "test-delivery-secret"
	c.Delivery.RetryIntervalSeconds = 60
	c.Delivery.TimeoutMs = 5000
	c.Courier.BaseURL = "http://127.0.0.1:1"
	c.Courier.Timeout = 5000
	c.Promotions.BaseURL = "http://127.0.0.1:1"
	c.Promotions.Secret = "s"
	c.Promotions.Timeout = 5000
	c.Analytics.BaseURL = "http://127.0.0.1:1"
	c.Analytics.Token = "t"
	c.Analytics.Timeout = 5000
	c.Analytics.OutboxIntervalSeconds = 30
	c.Analytics.OutboxMaxAttempts = 10
	c.Analytics.OutboxRetentionHours = 168
	c.Notify.BaseURL = "http://127.0.0.1:1"
	c.Notify.Token = "t"
	c.Notify.Timeout = 5000
	c.Notify.OutboxIntervalSeconds = 30
	c.Notify.OutboxMaxAttempts = 10
	c.Notify.OutboxRetentionHours = 168
	return c
}

func TestNewServiceContext_Success(t *testing.T) {
	sc := NewServiceContext(svcConfig("file:" + t.TempDir() + "/s.db"))
	// 关键依赖全部装配
	if sc.Auth == nil || sc.ProductDB == nil || sc.DeliveryDB == nil ||
		sc.ProjectionDB == nil || sc.StatementDB == nil || sc.EvalDB == nil ||
		sc.CouponDB == nil || sc.Courier == nil || sc.Sandbox == nil ||
		sc.Real == nil || sc.Dispatcher == nil || sc.PromotionBridge == nil ||
		sc.AnalyticsSvc == nil || sc.AnalyticsRelay == nil || sc.AnalyticsOutbox == nil ||
		sc.NotifySvc == nil || sc.NotifyRelay == nil || sc.NotifyOutbox == nil {
		t.Fatal("service context 装配不完整（存在空依赖）")
	}
	if !sc.PromotionBridge.Enabled() {
		t.Fatal("配置了 PROMOTIONS_URL 但桥未启用")
	}
}

// panicCreator 按装配顺序列出各模型的建表失败 panic（want = panic 消息片段）。
var panicCreator = []struct {
	name string
	want string
	fn   func(*sql.DB) error
}{
	{"product", "create tables", func(db *sql.DB) error { return model.NewProductModel(db).CreateTables() }},
	{"delivery", "create delivery tables", func(db *sql.DB) error { return model.NewDeliveryModel(db).CreateTables() }},
	{"projection", "create order projection tables", func(db *sql.DB) error { return model.NewOrderProjectionModel(db).CreateTables() }},
	{"statement", "create statement tables", func(db *sql.DB) error { return model.NewStatementModel(db).CreateTables() }},
	{"eval", "create promotion eval tables", func(db *sql.DB) error { return model.NewPromotionEvalModel(db).CreateTables() }},
	{"coupon", "create coupon tables", func(db *sql.DB) error { return model.NewCouponModel(db).CreateTables() }},
	{"analytics", "create analytics outbox tables", func(db *sql.DB) error { return model.NewAnalyticsOutboxModel(db).CreateTables() }},
	{"notify", "create notify outbox tables", func(db *sql.DB) error { return model.NewNotifyOutboxModel(db).CreateTables() }},
}

func TestNewServiceContext_CreateTablePanicLadder(t *testing.T) {
	for i, c := range panicCreator {
		i, c := i, c
		t.Run(c.name, func(t *testing.T) {
			// 目标模型之前的表预建好，目标模型首个缺失表在只读库上失败。
			// _seed 保证文件有页（空文件 + ?mode=ro 会打不开，无表页）。
			path := t.TempDir() + "/l.db"
			db, err := model.Open("file:" + path)
			if err != nil {
				t.Fatalf("open writable: %v", err)
			}
			if _, err := db.Exec(`CREATE TABLE _seed (x INTEGER)`); err != nil {
				t.Fatalf("seed table: %v", err)
			}
			for _, prev := range panicCreator[:i] {
				if err := prev.fn(db); err != nil {
					t.Fatalf("pre-create %s: %v", prev.name, err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatalf("close: %v", err)
			}
			cfg := svcConfig("file:" + path + "?mode=ro")
			assertPanic(t, func() { NewServiceContext(cfg) }, c.want)
		})
	}
}

func TestNewServiceContext_DemoSecretWarns(t *testing.T) {
	// 内置演示 JWT 密钥 + 空沙箱密钥（自动注入演示值）→ 两处 WARN 不 panic
	cfg := svcConfig("file:" + t.TempDir() + "/w.db")
	cfg.Auth.JWTSecret = utils.WellKnownDevJWTSecret
	cfg.Channel.SandboxSecret = ""
	cfg.Delivery.Secret = ""
	sc := NewServiceContext(cfg)
	if sc.Auth == nil {
		t.Fatal("dev-secret context Auth = nil")
	}
}

func TestNewServiceContext_OpenPanics(t *testing.T) {
	// 空 JWT 密钥 → 启动期拒绝
	assertPanic(t, func() {
		cfg := svcConfig("file:" + t.TempDir() + "/a.db")
		cfg.Auth.JWTSecret = "   "
		NewServiceContext(cfg)
	}, "JWT 密钥")

	// 空 DSN → 打不开库
	assertPanic(t, func() {
		cfg := svcConfig("file:" + t.TempDir() + "/a.db")
		cfg.MySQL.DataSource = ""
		NewServiceContext(cfg)
	}, "open db")
}

func assertPanic(t *testing.T, fn func(), want string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("want panic, got none")
		}
		if !strings.Contains(fmt.Sprintf("%v", r), want) {
			t.Fatalf("panic = %v, want contains %q", r, want)
		}
	}()
	fn()
}

// GP-202 验签材料装载：file: 前缀从挂载文件读 PEM（env 注入会在首个换行
// 截断）；其余原样透传，file: 文件缺失 fail-loud panic。
func TestLoadChannelMaterial(t *testing.T) {
	if got := loadChannelMaterial(""); got != "" {
		t.Fatalf("empty = %q, want \"\"（渠道未接入）", got)
	}
	const pem = "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"
	if got := loadChannelMaterial(pem); got != pem {
		t.Fatalf("inline passthrough = %q, want original PEM", got)
	}
	path := t.TempDir() + "/platform-cert.pem"
	if err := os.WriteFile(path, []byte(pem), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
	if got := loadChannelMaterial("file:" + path); got != pem {
		t.Fatalf("file load = %q, want full file content", got)
	}
	assertPanic(t, func() {
		loadChannelMaterial("file:" + t.TempDir() + "/missing.pem")
	}, "load channel material")
}
