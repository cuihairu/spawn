package svc

// servicecontext.go 剩余补漏分支：构造成功路径（依赖装配齐全 + 内置演示密钥
// 告警不 panic）、启动期 panic 臂（空 JWT 密钥、空凭据密钥、库打不开/建表
// 失败、Courier 客户端建不起来），以及 msDuration 的 <=0 默认臂。
//
// 登记不可达（不测）：无（全部 panic 臂经上述场景触达）。

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/courier-adapter/courier"
	"github.com/tappi/tappi/services/courier-adapter/internal/config"
	"github.com/tappi/tappi/services/courier-adapter/model"
	"github.com/tappi/tappi/services/courier-adapter/utils"
)

// svcCfg 给一份可成功装配的配置（sqlite 文件库 + 健康 Courier 存根）。
func svcCfg(t *testing.T) (config.Config, string) {
	t.Helper()
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(stub.Close)
	var c config.Config
	c.Auth.JWTSecret = "svc-test-secret"
	c.MySQL.DataSource = "file:" + t.TempDir() + "/svc.db"
	c.Credential.Key = "svc-test-cred-key"
	c.Courier.BaseURL = stub.URL
	c.Courier.Timeout = 2000
	c.Ticket.TTLSeconds = 120
	return c, c.MySQL.DataSource
}

func TestNewServiceContext_Success(t *testing.T) {
	c, _ := svcCfg(t)
	sc := NewServiceContext(c)
	defer func() { _ = sc.DB.Close() }()
	if sc.Auth == nil || sc.Courier == nil || sc.Mappings == nil ||
		sc.Bindings == nil || sc.Audits == nil || sc.Tickets == nil || sc.DB == nil {
		t.Fatal("service context 装配不完整（存在空依赖）")
	}
}

func TestNewServiceContext_TicketTTLIsSeconds(t *testing.T) {
	// 0a986b5 回归：Ticket.TTLSeconds 是秒，装配后票据有效期必须约等于
	// TTLSeconds×1s（曾误按毫秒装配，票据出票即过期、兑票恒 404）。
	c, _ := svcCfg(t)
	sc := NewServiceContext(c)
	defer func() { _ = sc.DB.Close() }()
	tk, err := sc.Tickets.Issue(7, "courier-game-a", "dev", &courier.Session{})
	if err != nil {
		t.Fatalf("issue ticket: %v", err)
	}
	want := 120 * time.Second
	delta := tk.ExpiresAt.Sub(time.Now())
	if delta < want-time.Minute || delta > want+time.Minute {
		t.Fatalf("ticket TTL = %v, want ~%v（TTLSeconds 装配口径回归）", delta, want)
	}
	if _, err := sc.Tickets.Redeem(tk.Token); err != nil {
		t.Fatalf("redeem within TTL failed: %v（票据出票即过期 = 毫秒误用回归）", err)
	}
}

func TestNewServiceContext_DemoSecretWarns(t *testing.T) {
	// 内置演示 JWT 密钥 + 演示凭据密钥 → 两处 WARN（不 panic）
	c, _ := svcCfg(t)
	c.Auth.JWTSecret = utils.WellKnownDevJWTSecret
	c.Credential.Key = utils.WellKnownDevCredentialKey
	sc := NewServiceContext(c)
	defer func() { _ = sc.DB.Close() }()
	if sc.Auth == nil {
		t.Fatal("demo-secret context Auth = nil")
	}
}

func TestNewServiceContext_PanicArms(t *testing.T) {
	// 空 JWT 密钥 → 拒绝
	c, _ := svcCfg(t)
	c.Auth.JWTSecret = "  "
	assertSvcPanic(t, c, "JWT 密钥")

	// 空凭据加密密钥 → 拒绝
	c2, _ := svcCfg(t)
	c2.Credential.Key = ""
	assertSvcPanic(t, c2, "凭据加密密钥")

	// 库打不开（父路径是文件）→ panic
	c3, _ := svcCfg(t)
	blocker := strings.TrimPrefix(c3.MySQL.DataSource, "file:") + ".blocker"
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("blocker: %v", err)
	}
	c3.MySQL.DataSource = "file:" + blocker + "/x.db"
	assertSvcPanic(t, c3, "连接数据库失败")

	// 建表失败（只读空库：有页无业务表）→ panic
	c4, _ := svcCfg(t)
	roPath := strings.TrimPrefix(c4.MySQL.DataSource, "file:") + ".ro"
	db, err := model.Open("file:" + roPath)
	if err != nil {
		t.Fatalf("open ro seed: %v", err)
	}
	if _, err := db.Exec(`CREATE TABLE _seed (x INTEGER)`); err != nil {
		t.Fatalf("seed table: %v", err)
	}
	_ = db.Close()
	c5 := c4
	c5.MySQL.DataSource = "file:" + roPath + "?mode=ro"
	assertSvcPanic(t, c5, "建表失败")

	// 打开成功但 Ping 失败（垃圾字节文件）→ panic
	c7, _ := svcCfg(t)
	garbage := strings.TrimPrefix(c7.MySQL.DataSource, "file:") + ".garbage"
	if err := os.WriteFile(garbage, []byte("not a sqlite database"), 0o644); err != nil {
		t.Fatalf("garbage: %v", err)
	}
	c7.MySQL.DataSource = "file:" + garbage
	assertSvcPanic(t, c7, "数据库连接测试失败")

	// Courier 客户端建不起来（空 base URL）→ panic
	c6, _ := svcCfg(t)
	c6.Courier.BaseURL = ""
	assertSvcPanic(t, c6, "初始化 Courier 客户端失败")
}

// assertSvcPanic 断言 panic 消息含片段。
func assertSvcPanic(t *testing.T, c config.Config, want string) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("want panic containing %q, got none", want)
		}
		if !strings.Contains(fmt.Sprintf("%v", r), want) {
			t.Fatalf("panic = %v, want contains %q", r, want)
		}
	}()
	sc := NewServiceContext(c)
	if sc != nil {
		_ = sc.DB.Close()
	}
}

func TestMsDurationArms(t *testing.T) {
	// <=0 → 默认 5s；正值 → 原样
	if got := msDuration(0); got != 5*time.Second {
		t.Fatalf("msDuration(0) = %v, want 5s", got)
	}
	if got := msDuration(-1); got != 5*time.Second {
		t.Fatalf("msDuration(-1) = %v, want 5s", got)
	}
	if got := msDuration(1500); got != 1500*time.Millisecond {
		t.Fatalf("msDuration(1500) = %v, want 1.5s", got)
	}
}
