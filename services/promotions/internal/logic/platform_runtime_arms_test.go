package logic

// errors.go / platform.go / runtimelogic.go 补漏分支：ErrorHandler 的
// apiError 与兜底臂、platformRole 的请求构建/Do/非 200/解码失败/成功臂、
// requireAdminRole 的 502 与 403 臂、newPlatformClient 默认超时与去尾斜杠、
// ListRuntimeActivities/ListRuntimeSnapshots 的库错误/快照解析失败/窗口外/
// 定向人群过滤臂与 runtimeWindowCovers 的解析失败臂。

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tappi/tappi/services/promotions/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

func TestErrorHandler_Arms(t *testing.T) {
	code, body := ErrorHandler(context.Background(), newAPIError(http.StatusConflict, "冲突"))
	if code != http.StatusConflict || body != "冲突" {
		t.Fatalf("apiError = %d/%v", code, body)
	}
	// 非 apiError → 默认 400 + 原文
	code, body = ErrorHandler(context.Background(), errors.New("boom"))
	if code != http.StatusBadRequest || body != "boom" {
		t.Fatalf("plain = %d/%v", code, body)
	}
}

func TestPlatformClient_Arms(t *testing.T) {
	if c := newPlatformClient("http://h", 0); c.hc.Timeout != 5000*time.Millisecond {
		t.Fatalf("default timeout = %v", c.hc.Timeout)
	}
	if c := newPlatformClient("http://h/", 100); c.base != "http://h" {
		t.Fatalf("trim = %q", c.base)
	}
}

func TestPlatformRole_Arms(t *testing.T) {
	env := newLogicEnv(t, "")
	sc := &svc.ServiceContext{Config: env.sc.Config}

	// 请求构建失败（base 含非法字符）
	sc.Config.GameCatalog.BaseURL = "http://\nbad"
	if _, err := platformRole(context.Background(), sc, "Bearer x"); err == nil {
		t.Fatal("bad url = nil err")
	}

	// Do 失败（不可达）
	sc.Config.GameCatalog.BaseURL = "http://127.0.0.1:1"
	if _, err := platformRole(context.Background(), sc, ""); err == nil {
		t.Fatal("unreachable = nil err")
	}

	// 非 200
	env.withCatalog(t, "operator", http.StatusForbidden)
	sc.Config.GameCatalog.BaseURL = env.sc.Config.GameCatalog.BaseURL
	if _, err := platformRole(context.Background(), sc, ""); err == nil {
		t.Fatal("403 = nil err")
	}

	// 解码失败（junk body）
	sc.Config.GameCatalog.BaseURL = junkServer(t)
	if _, err := platformRole(context.Background(), sc, "Bearer x"); err == nil {
		t.Fatal("junk = nil err")
	}

	// 成功
	env.withCatalog(t, "operator", http.StatusOK)
	sc.Config.GameCatalog.BaseURL = env.sc.Config.GameCatalog.BaseURL
	role, err := platformRole(context.Background(), sc, "Bearer x")
	if err != nil || role != "operator" {
		t.Fatalf("ok = %q/%v", role, err)
	}
}

func TestRequireAdminRole_Arms(t *testing.T) {
	env := newLogicEnv(t, "")
	ctx := adminCtx(1, "1.2.3.4")

	// 角色服务不可用（不可达）→ 502
	env.sc.Config.GameCatalog.BaseURL = "http://127.0.0.1:1"
	if _, err := requireAdminRole(ctx, env.sc, "Bearer x"); err == nil {
		t.Fatal("unreachable = nil err")
	} else {
		mustAPIError(t, err, http.StatusBadGateway)
	}

	// 非管理角色 → 403
	env.withCatalog(t, "player", http.StatusOK)
	if _, err := requireAdminRole(ctx, env.sc, "Bearer x"); err == nil {
		t.Fatal("player role = nil err")
	} else {
		mustAPIError(t, err, http.StatusForbidden)
	}

	// operator → 放行
	env.withCatalog(t, "operator", http.StatusOK)
	if role, err := requireAdminRole(ctx, env.sc, "Bearer x"); err != nil || role != "operator" {
		t.Fatalf("operator = %q/%v", role, err)
	}

	// platform_admin → 放行
	env.withCatalog(t, "platform_admin", http.StatusOK)
	if _, err := requireAdminRole(ctx, env.sc, "Bearer x"); err != nil {
		t.Fatalf("platform_admin = %v", err)
	}
}

// junkServer 起一个返回非 JSON 正文的服务器，返回其地址。
func junkServer(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not-json"))
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestRuntimeActivities_Arms(t *testing.T) {
	env := newLogicEnv(t, "")
	ctx := adminCtx(1, "1.2.3.4")
	l := logx.WithContext(ctx)

	// 库错误（闭库）→ 空列表
	if err := env.db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if items := ListRuntimeActivities(env.sc, l, "g1", time.Now()); items == nil || len(items) != 0 {
		t.Fatalf("closed = %v", items)
	}
	if snaps := ListRuntimeSnapshots(env.sc, l, "g1", time.Now()); snaps == nil || len(snaps) != 0 {
		t.Fatalf("closed snaps = %v", snaps)
	}
}

func TestRuntimeActivities_WindowAndAudienceArms(t *testing.T) {
	env := newLogicEnv(t, "")
	l := logx.WithContext(context.Background())

	// 快照解析失败（原文非法）
	seedPublishedActivity(t, env.sc.ActivityDB, "g1", "not-json")

	// 窗口外（已过期）
	seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		`{"name":"past","start_at":"2020-01-01T00:00:00Z","end_at":"2020-06-01T00:00:00Z"}`)

	// 定向人群（type=new_user）→ 玩家列表不展示
	seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		`{"name":"targeted","start_at":"2020-01-01T00:00:00Z","end_at":"2099-01-01T00:00:00Z","audience":{"type":"new_user"}}`)

	// 合法窗口内 → 展示（budget 不下发）
	seedPublishedActivity(t, env.sc.ActivityDB, "g1",
		`{"name":"live","start_at":"2020-01-01T00:00:00Z","end_at":"2099-01-01T00:00:00Z","audience":{"type":"all"},"limits":{"budget_cents":1000},"stackable":true}`)

	now := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	items := ListRuntimeActivities(env.sc, l, "g1", now)
	if len(items) != 1 || items[0].Name != "live" {
		t.Fatalf("player items = %+v", items)
	}
	if _, hasBudget := items[0].Limits["budget_cents"]; hasBudget {
		t.Fatalf("budget leaked to player: %v", items[0].Limits)
	}
	// 游戏服快照：同窗口判定，非法快照被跳过
	snaps := ListRuntimeSnapshots(env.sc, l, "g1", now)
	if len(snaps) != 2 { // targeted + live
		t.Fatalf("game snaps = %d, want 2", len(snaps))
	}
}

func TestRuntimeWindowCovers_Arms(t *testing.T) {
	if runtimeWindowCovers(map[string]any{"start_at": "bad", "end_at": "2026-12-31T00:00:00Z"}, time.Now()) {
		t.Fatal("bad start = true")
	}
	if runtimeWindowCovers(map[string]any{"start_at": "2026-01-01T00:00:00Z", "end_at": "bad"}, time.Now()) {
		t.Fatal("bad end = true")
	}
	if !runtimeWindowCovers(map[string]any{"start_at": "2026-01-01T00:00:00Z", "end_at": "2026-12-31T00:00:00Z"},
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("within = false")
	}
	if runtimeWindowCovers(map[string]any{"start_at": "2027-01-01T00:00:00Z", "end_at": "2027-12-31T00:00:00Z"},
		time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("before start = true")
	}
}
