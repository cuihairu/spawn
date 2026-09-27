package utils

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret"

// signTestToken 使用指定密钥和载荷签发 JWT。
func signTestToken(t *testing.T, secret string, claims JWTClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func testClaims(userId int64, username string, expiresAt time.Time) JWTClaims {
	return JWTClaims{
		UserId:   userId,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}
}

func TestParseToken_Valid(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret)
	tokenString := signTestToken(t, testSecret, testClaims(42, "alice", time.Now().Add(time.Hour)))

	claims, err := auth.ParseToken(tokenString)
	if err != nil {
		t.Fatalf("ParseToken() error = %v", err)
	}
	if claims.UserId != 42 || claims.Username != "alice" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
}

func TestParseToken_Expired(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret)
	tokenString := signTestToken(t, testSecret, testClaims(1, "bob", time.Now().Add(-time.Hour)))

	claims, err := auth.ParseToken(tokenString)
	if err == nil {
		t.Fatalf("expected error for expired token, got claims %+v", claims)
	}
}

func TestParseToken_WrongSecret(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret)
	tokenString := signTestToken(t, "another-secret", testClaims(1, "carol", time.Now().Add(time.Hour)))

	claims, err := auth.ParseToken(tokenString)
	if err == nil {
		t.Fatalf("expected error for wrong secret, got claims %+v", claims)
	}
}

func TestParseToken_UnsupportedSigningMethod(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret)

	// 手工构造 alg=RS256 的令牌，验证 keyfunc 拒绝非 HMAC 签名方法。
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"user_id":1,"username":"dave"}`))
	tokenString := header + "." + payload + ".c2lnbmF0dXJl"

	claims, err := auth.ParseToken(tokenString)
	if err == nil {
		t.Fatalf("expected error for non-HMAC token, got claims %+v", claims)
	}
	if !strings.Contains(err.Error(), "不支持的签名方法") {
		t.Fatalf("expected unsupported-method error, got %q", err.Error())
	}
}

func TestParseToken_Malformed(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret)

	claims, err := auth.ParseToken("not-a-token")
	if err == nil {
		t.Fatalf("expected error for malformed token, got claims %+v", claims)
	}
	if claims != nil {
		t.Fatalf("expected nil claims on error, got %+v", claims)
	}
}

func TestValidateToken(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret)

	valid := signTestToken(t, testSecret, testClaims(2, "eve", time.Now().Add(time.Hour)))
	ok, err := auth.ValidateToken(valid)
	if err != nil || !ok {
		t.Fatalf("ValidateToken(valid) = %v, %v; want true, nil", ok, err)
	}

	expired := signTestToken(t, testSecret, testClaims(2, "eve", time.Now().Add(-time.Hour)))
	ok, err = auth.ValidateToken(expired)
	if err == nil || ok {
		t.Fatalf("ValidateToken(expired) = %v, %v; want false, error", ok, err)
	}
}

func TestGetUserIdFromToken(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret)

	valid := signTestToken(t, testSecret, testClaims(99, "frank", time.Now().Add(time.Hour)))
	userId, err := auth.GetUserIdFromToken(valid)
	if err != nil || userId != 99 {
		t.Fatalf("GetUserIdFromToken(valid) = %d, %v; want 99, nil", userId, err)
	}

	userId, err = auth.GetUserIdFromToken("garbage")
	if err == nil || userId != 0 {
		t.Fatalf("GetUserIdFromToken(invalid) = %d, %v; want 0, error", userId, err)
	}
}

func TestParseToken_NewAuthWithDifferentSecretIsolation(t *testing.T) {
	t.Parallel()

	authA := NewAuth(testSecret)
	authB := NewAuth("other-secret")
	tokenString := signTestToken(t, testSecret, testClaims(7, "grace", time.Now().Add(time.Hour)))

	if _, err := authA.ParseToken(tokenString); err != nil {
		t.Fatalf("authA should accept its own tokens: %v", err)
	}
	if _, err := authB.ParseToken(tokenString); err == nil {
		t.Fatal("authB should reject tokens signed with a different secret")
	}
}