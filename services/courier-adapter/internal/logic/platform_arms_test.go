package logic

// platform.go 剩余补漏分支：newPlatformClient 的 timeout<=0 默认臂、
// platformRole 的 NewRequest/hc.Do/非 200/解码失败四臂、requireAdminRole
// 的上游不可达 502 臂、fetchUserEmail 的 NewRequest/hc.Do/非 200/解码失败
// 臂与 placeholderEmail 占位邮箱臂。
//
// 登记不可达（不测）：platformRole/fetchUserEmail 的 io.ReadAll 失败臂
// （io.LimitReader 对内存 body 恒不报错）。

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPlatformClient_TimeoutDefault(t *testing.T) {
	pc := newPlatformClient("http://x", 0)
	if pc.hc.Timeout != 5*time.Second {
		t.Fatalf("timeout = %v, want 5s default", pc.hc.Timeout)
	}
	if pc.base != "http://x" {
		t.Fatalf("base = %q, want http://x (trailing slash trimmed)", pc.base)
	}
}

func TestPlatformRole_Arms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()

	// NewRequest 失败（非法 base URL）
	e.base.Config.GameCatalog.BaseURL = "://bad"
	if _, _, err := platformRole(ctx, e.base, ""); err == nil {
		t.Fatal("bad catalog url = nil, want error")
	}

	// hc.Do 失败（已关停端点）
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	e.base.Config.GameCatalog.BaseURL = deadURL
	if _, _, err := platformRole(ctx, e.base, ""); err == nil {
		t.Fatal("dead catalog = nil, want error")
	}

	// 非 200 状态
	e2 := newArmsEnv(t)
	e2.catalog.set(func(r *http.Request) (int, string) { return http.StatusInternalServerError, "x" })
	if _, _, err := platformRole(ctx, e2.base, ""); err == nil {
		t.Fatal("catalog 500 = nil, want error")
	}

	// 解码失败（非 JSON body）
	e3 := newArmsEnv(t)
	e3.catalog.set(func(r *http.Request) (int, string) { return http.StatusOK, "not-json" })
	if _, _, err := platformRole(ctx, e3.base, ""); err == nil {
		t.Fatal("catalog non-json = nil, want error")
	}

	// requireAdminRole 上游不可达 → 502
	e4 := newArmsEnv(t)
	e4.base.Config.GameCatalog.BaseURL = deadURL
	if err := requireAdminRole(ctx, e4.base, ""); !isAPIErr(err, http.StatusBadGateway) {
		t.Fatalf("require admin unreachable = %v, want 502", err)
	}

	// 非管理员角色 → 403
	e5 := newArmsEnv(t)
	e5.catalog.set(func(r *http.Request) (int, string) { return http.StatusOK, `{"role":"player","partnerId":0}` })
	if err := requireAdminRole(ctx, e5.base, ""); !isAPIErr(err, http.StatusForbidden) {
		t.Fatalf("player role = %v, want 403", err)
	}
}

func TestFetchUserEmail_Arms(t *testing.T) {
	e := newArmsEnv(t)
	ctx := context.Background()

	// NewRequest 失败
	e.base.Config.UserService.BaseURL = "://bad"
	if got := fetchUserEmail(ctx, e.base, "", 77); got != "" {
		t.Fatalf("bad user url email = %q, want empty", got)
	}

	// hc.Do 失败
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	e.base.Config.UserService.BaseURL = deadURL
	if got := fetchUserEmail(ctx, e.base, "", 77); got != "" {
		t.Fatalf("dead user email = %q, want empty", got)
	}

	// code != 200 → 空串
	e2 := newArmsEnv(t)
	e2.user.set(func(r *http.Request) (int, string) { return http.StatusOK, `{"code":500,"data":{}}` })
	if got := fetchUserEmail(ctx, e2.base, "", 77); got != "" {
		t.Fatalf("code!=200 email = %q, want empty", got)
	}

	// 解码失败 → 空串
	e3 := newArmsEnv(t)
	e3.user.set(func(r *http.Request) (int, string) { return http.StatusOK, "not-json" })
	if got := fetchUserEmail(ctx, e3.base, "", 77); got != "" {
		t.Fatalf("non-json email = %q, want empty", got)
	}

	// 成功 → 邮箱透传（独立环境，避免前面改坏的 base URL 残留）
	e4 := newArmsEnv(t)
	if got := fetchUserEmail(ctx, e4.base, "Bearer t", 77); got != "p@example.com" {
		t.Fatalf("email = %q, want p@example.com", got)
	}
}

func TestPlaceholderEmail_Arms(t *testing.T) {
	if got := placeholderEmail(77); got != "spawn_77@platform.invalid" {
		t.Fatalf("placeholder = %q, want spawn_77@platform.invalid", got)
	}
}
