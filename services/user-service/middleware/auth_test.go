package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tappi/tappi/services/user-service/internal/svc"
	"github.com/tappi/tappi/services/user-service/utils"
)

const testSecret = "user-service-middleware-test-secret"

func newTestSvcCtx() *svc.ServiceContext {
	return &svc.ServiceContext{
		Auth: utils.NewAuth(testSecret, 24*time.Hour),
	}
}

// runAuth 穿透被测中间件；path 为空时不设置路径默认 "/"。
func runAuth(t *testing.T, svcCtx *svc.ServiceContext, path, authHeader string) (*httptest.ResponseRecorder, *int64, *string, bool) {
	t.Helper()

	var gotUserID int64
	var gotUsername string
	reached := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		if v, ok := r.Context().Value("user_id").(int64); ok {
			gotUserID = v
		}
		if v, ok := r.Context().Value("username").(string); ok {
			gotUsername = v
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := AuthMiddleware(svcCtx)(next)

	req := httptest.NewRequest(http.MethodGet, path, nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	return rr, &gotUserID, &gotUsername, reached
}

func token(t *testing.T, auth *utils.Auth, userId int64, username string) string {
	t.Helper()
	signed, err := auth.GenerateToken(userId, username)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return signed
}

func TestSkipAuth(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/auth/register", true},
		{"/auth/login", true},
		{"/ping", true},
		{"/health", true},
		{"/from/", true}, // 前缀匹配（如 /from/xxx）
		{"/from/open", true},
		{"/auth/register/extra", true}, // 前缀命中也跳过
		{"/users/1", false},
		{"/auth/refresh", false}, // 不在白名单，需认证
		{"/", false},
		// 注：skipAuth 为纯前缀匹配（HasPrefix），/healthcheck 会命中 /health 前缀被放行。
		// 这是既有语义，白名单均为服务自有路径，本测试按实际行为记录。
		{"/healthcheck", true},
	}
	for _, tc := range cases {
		if got := skipAuth(tc.path); got != tc.want {
			t.Errorf("skipAuth(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}

func TestAuthMiddleware_SkipPathsPassThrough(t *testing.T) {
	for _, path := range []string{"/auth/login", "/auth/register", "/ping", "/health", "/from/public"} {
		rr, _, _, reached := runAuth(t, newTestSvcCtx(), path, "")
		if rr.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200 (skip list)", path, rr.Code)
		}
		if !reached {
			t.Fatalf("%s: next handler must be reached without a token", path)
		}
	}
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	rr, _, _, reached := runAuth(t, newTestSvcCtx(), "/users/1", "")

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if got := rr.Body.String(); got != `{"code":401,"message":"缺少认证令牌"}` {
		t.Fatalf("body = %q", got)
	}
	if reached {
		t.Fatal("next handler must not be reached without a token")
	}
}

func TestAuthMiddleware_InvalidPrefix(t *testing.T) {
	rr, _, _, reached := runAuth(t, newTestSvcCtx(), "/users/1", "Basic dXNlcjpwYXNz")

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if got := rr.Body.String(); got != `{"code":401,"message":"无效的令牌格式"}` {
		t.Fatalf("body = %q", got)
	}
	if reached {
		t.Fatal("next handler must not be reached with a malformed scheme")
	}
}

func TestAuthMiddleware_EmptyToken(t *testing.T) {
	rr, _, _, reached := runAuth(t, newTestSvcCtx(), "/users/1", "Bearer ")

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if got := rr.Body.String(); got != `{"code":401,"message":"令牌不能为空"}` {
		t.Fatalf("body = %q", got)
	}
	if reached {
		t.Fatal("next handler must not be reached with an empty token")
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	// 用不同密钥签出的令牌必然校验失败
	other := utils.NewAuth("other-secret", 24*time.Hour)
	rr, _, _, reached := runAuth(t, newTestSvcCtx(), "/users/1", "Bearer "+token(t, other, 1, "eve"))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if got := rr.Body.String(); got != `{"code":401,"message":"无效的令牌"}` {
		t.Fatalf("body = %q", got)
	}
	if reached {
		t.Fatal("next handler must not be reached with an invalid token")
	}
}

func TestAuthMiddleware_ExpiredToken(t *testing.T) {
	// 签发即过期的令牌（tokenExpire 为负）
	expiredAuth := utils.NewAuth(testSecret, -time.Minute)
	rr, _, _, reached := runAuth(t, newTestSvcCtx(), "/users/1",
		"Bearer "+token(t, expiredAuth, 1, "ghost"))

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if got := rr.Body.String(); got != `{"code":401,"message":"无效的令牌"}` {
		t.Fatalf("body = %q", got)
	}
	if reached {
		t.Fatal("next handler must not be reached with an expired token")
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	svcCtx := newTestSvcCtx()
	rr, userID, username, reached := runAuth(t, svcCtx, "/users/1",
		"Bearer "+token(t, svcCtx.Auth, 99, "carol"))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !reached {
		t.Fatal("next handler must be reached with a valid token")
	}
	if *userID != 99 {
		t.Fatalf("ctx user_id = %d, want 99", *userID)
	}
	if *username != "carol" {
		t.Fatalf("ctx username = %q, want carol", *username)
	}
}
