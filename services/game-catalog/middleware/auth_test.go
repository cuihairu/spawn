package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/utils"
)

const testSecret = "unit-test-secret"

// newTestSvcCtx 仅构造中间件用到的 Auth 字段，避免依赖数据文件加载。
func newTestSvcCtx() *svc.ServiceContext {
	return &svc.ServiceContext{Auth: utils.NewAuth(testSecret)}
}

func signToken(t *testing.T, secret string, claims utils.JWTClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func validClaims() utils.JWTClaims {
	return utils.JWTClaims{
		UserId:   42,
		Username: "alice",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
}

// runMiddleware 穿透被测中间件；next 记录透传后的上下文值。
func runMiddleware(t *testing.T, svcCtx *svc.ServiceContext, header string) (*httptest.ResponseRecorder, *int64, *string, bool) {
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

	req := httptest.NewRequest(http.MethodPost, "/games", nil)
	if header != "" {
		req.Header.Set("Authorization", header)
	}
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	return rr, &gotUserID, &gotUsername, reached
}

func TestAuthMiddleware_MissingHeader(t *testing.T) {
	rr, _, _, reached := runMiddleware(t, newTestSvcCtx(), "")

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
	rr, _, _, reached := runMiddleware(t, newTestSvcCtx(), "Basic dXNlcjpwYXNz")

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
	rr, _, _, reached := runMiddleware(t, newTestSvcCtx(), "Bearer   ")

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
	svcCtx := newTestSvcCtx()
	rr, _, _, reached := runMiddleware(t, svcCtx, "Bearer "+signToken(t, "wrong-secret", validClaims()))

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
	expired := validClaims()
	expired.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))

	rr, _, _, reached := runMiddleware(t, newTestSvcCtx(), "Bearer "+signToken(t, testSecret, expired))

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
	rr, userID, username, reached := runMiddleware(t, newTestSvcCtx(), "Bearer "+signToken(t, testSecret, validClaims()))

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if !reached {
		t.Fatal("next handler must be reached with a valid token")
	}
	if *userID != 42 {
		t.Fatalf("ctx user_id = %d, want 42", *userID)
	}
	if *username != "alice" {
		t.Fatalf("ctx username = %q, want alice", *username)
	}
}
