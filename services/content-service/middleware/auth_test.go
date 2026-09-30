package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/tappi/tappi/services/content-service/internal/svc"
	"github.com/tappi/tappi/services/content-service/utils"
)

func signToken(t *testing.T, secret string, userId int64, username string) string {
	t.Helper()

	now := time.Now()
	claims := utils.JWTClaims{
		UserId:   userId,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func TestAuthMiddleware_ReadOnlyOptionalAuth(t *testing.T) {
	t.Parallel()

	secret := "test-secret"
	svcCtx := &svc.ServiceContext{Auth: utils.NewAuth(secret)}
	mw := AuthMiddleware(svcCtx)

	t.Run("valid token populates context", func(t *testing.T) {
		t.Parallel()

		token := signToken(t, secret, 123, "alice")
		req := httptest.NewRequest(http.MethodGet, "/api/v1/guides", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()

		var gotUserId int64
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotUserId, _ = r.Context().Value("user_id").(int64)
			w.WriteHeader(http.StatusOK)
		})

		mw(next).ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}
		if gotUserId != 123 {
			t.Fatalf("unexpected user_id: %d", gotUserId)
		}
	})

	t.Run("invalid token does not block read endpoints", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest(http.MethodGet, "/api/v1/guides", nil)
		req.Header.Set("Authorization", "Bearer invalid.token.value")
		rr := httptest.NewRecorder()

		called := false
		next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		})

		mw(next).ServeHTTP(rr, req)
		if !called {
			t.Fatalf("next handler not called")
		}
		if rr.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d", rr.Code)
		}
	})
}

func TestAuthMiddleware_WriteRequiresAuth(t *testing.T) {
	t.Parallel()

	secret := "test-secret"
	svcCtx := &svc.ServiceContext{Auth: utils.NewAuth(secret)}
	mw := AuthMiddleware(svcCtx)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/guides", nil)
	rr := httptest.NewRecorder()

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw(next).ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("unexpected status: %d", rr.Code)
	}
}

func TestAuthMiddleware_InvalidAuthScheme401(t *testing.T) {
	t.Parallel()

	svcCtx := &svc.ServiceContext{Auth: utils.NewAuth("test-secret")}
	mw := AuthMiddleware(svcCtx)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/guides", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	rr := httptest.NewRecorder()

	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next must not run for non-Bearer scheme")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "无效的令牌格式") {
		t.Fatalf("body = %s, want 无效的令牌格式", rr.Body.String())
	}
}

func TestAuthMiddleware_EmptyBearerToken401(t *testing.T) {
	t.Parallel()

	svcCtx := &svc.ServiceContext{Auth: utils.NewAuth("test-secret")}
	mw := AuthMiddleware(svcCtx)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/guides", nil)
	req.Header.Set("Authorization", "Bearer ")
	rr := httptest.NewRecorder()

	mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next must not run for empty bearer token")
	})).ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "令牌不能为空") {
		t.Fatalf("body = %s, want 令牌不能为空", rr.Body.String())
	}
}

// TestSkipAuth_ReadOnlyFallback GET 且路径不在 /guides、/comments 前缀内 →
// 走到最后的 return（false）；guides/comments 前缀放行。
func TestSkipAuth_ReadOnlyFallback(t *testing.T) {
	if !skipAuth(http.MethodGet, "/api/v1/guides/1") {
		t.Fatal("GET guides must be skipped")
	}
	if !skipAuth(http.MethodGet, "/api/v1/comments") {
		t.Fatal("GET comments must be skipped")
	}
	if skipAuth(http.MethodGet, "/api/v1/other") {
		t.Fatal("GET other paths must not be skipped")
	}
	if skipAuth(http.MethodPost, "/api/v1/guides") {
		t.Fatal("POST must not be skipped")
	}
}

// TestSkipAuth_HealthEndpoints /ping 与 /health 无条件放行。
func TestSkipAuth_HealthEndpoints(t *testing.T) {
	for _, path := range []string{"/ping", "/health"} {
		if !skipAuth(http.MethodGet, path) || !skipAuth(http.MethodPost, path) {
			t.Fatalf("%s must always be skipped", path)
		}
	}
}
