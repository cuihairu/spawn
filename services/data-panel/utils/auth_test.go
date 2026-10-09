package utils

import (
	"crypto/rand"
	"crypto/rsa"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "community-utils-auth-test-secret"

func signToken(t *testing.T, secret string, claims jwt.Claims, method jwt.SigningMethod) string {
	t.Helper()
	signed, err := jwt.NewWithClaims(method, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func claimsFor(username string) JWTClaims {
	now := time.Now()
	return JWTClaims{
		UserId:   42,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "community",
			Subject:   "42",
		},
	}
}

func TestNewAuth(t *testing.T) {
	if a := NewAuth(testSecret); a == nil || string(a.jwtSecret) != testSecret {
		t.Fatalf("NewAuth(%q) = %#v", testSecret, a)
	}
}

func TestParseTokenValid(t *testing.T) {
	auth := NewAuth(testSecret)
	claims, err := auth.ParseToken(signToken(t, testSecret, claimsFor("alice"), jwt.SigningMethodHS256))
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.UserId != 42 || claims.Username != "alice" {
		t.Fatalf("claims = %+v", claims)
	}
	if claims.Issuer != "community" || claims.Subject != "42" {
		t.Fatalf("registered claims = %+v", claims.RegisteredClaims)
	}
}

// TestParseTokenAcceptsAnyHMAC 校验中间层只要求 HMAC 家族，而非固定 HS256。
func TestParseTokenAcceptsAnyHMAC(t *testing.T) {
	auth := NewAuth(testSecret)
	claims, err := auth.ParseToken(signToken(t, testSecret, claimsFor("bob"), jwt.SigningMethodHS512))
	if err != nil {
		t.Fatalf("HS512 must be accepted: %v", err)
	}
	if claims.Username != "bob" {
		t.Fatalf("username = %q", claims.Username)
	}
}

// TestParseTokenRejectsNonHMAC 覆盖 keyfunc 的非 HMAC 签名方法拒绝分支。
func TestParseTokenRejectsNonHMAC(t *testing.T) {
	auth := NewAuth(testSecret)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claimsFor("mallory")).
		SignedString(key)
	if err != nil {
		t.Fatalf("sign RS256: %v", err)
	}
	_, err = auth.ParseToken(signed)
	if err == nil || !strings.Contains(err.Error(), "unsupported signing method") ||
		!strings.Contains(err.Error(), "RS256") {
		t.Fatalf("err = %v, want unsupported signing method RS256", err)
	}
}

func TestParseTokenWrongSecret(t *testing.T) {
	other := NewAuth("some-other-secret")
	_, err := other.ParseToken(signToken(t, testSecret, claimsFor("alice"), jwt.SigningMethodHS256))
	if err == nil || !strings.Contains(err.Error(), "signature is invalid") {
		t.Fatalf("err = %v, want signature validation failure", err)
	}
}

// TestParseTokenExpired jwt/v5 在 ParseWithClaims 阶段即校验 exp，
// 因此 auth.go 里手写的 "token expired" 分支实际不可达（未单独造用例）。
func TestParseTokenExpired(t *testing.T) {
	auth := NewAuth(testSecret)
	claims := claimsFor("alice")
	claims.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Hour))
	claims.NotBefore = jwt.NewNumericDate(time.Now().Add(-2 * time.Hour))
	_, err := auth.ParseToken(signToken(t, testSecret, claims, jwt.SigningMethodHS256))
	if err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("err = %v, want expiry rejection", err)
	}
}

func TestParseTokenNotYetValid(t *testing.T) {
	auth := NewAuth(testSecret)
	claims := claimsFor("alice")
	claims.NotBefore = jwt.NewNumericDate(time.Now().Add(time.Hour))
	_, err := auth.ParseToken(signToken(t, testSecret, claims, jwt.SigningMethodHS256))
	if err == nil || !strings.Contains(err.Error(), "not valid yet") {
		t.Fatalf("err = %v, want nbf rejection", err)
	}
}

func TestParseTokenMalformed(t *testing.T) {
	auth := NewAuth(testSecret)
	for _, bad := range []string{"", "not.a.token", "a.b", "....", "Bearer abc"} {
		if _, err := auth.ParseToken(bad); err == nil {
			t.Fatalf("ParseToken(%q) must fail", bad)
		}
	}
}

// TestParseTokenEmptyClaims 无 exp/无 subject 的令牌仍可解析，字段取零值。
func TestParseTokenEmptyClaims(t *testing.T) {
	auth := NewAuth(testSecret)
	claims, err := auth.ParseToken(signToken(t, testSecret, JWTClaims{}, jwt.SigningMethodHS256))
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.UserId != 0 || claims.Username != "" || claims.ExpiresAt != nil {
		t.Fatalf("claims = %+v, want zero values", claims)
	}
}
