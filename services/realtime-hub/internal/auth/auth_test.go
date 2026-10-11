package auth

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "hub-test-secret"

func sign(t *testing.T, claims Claims) string {
	t.Helper()
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return token
}

func claimsFor(userId int64, ttl time.Duration) Claims {
	return Claims{
		UserId: userId, Username: "作者甲",
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl))},
	}
}

// TestParseToken_Valid 有效 HS256 令牌解析出用户 claims。
func TestParseToken_Valid(t *testing.T) {
	v := NewVerifier(testSecret)
	claims, err := v.ParseToken(sign(t, claimsFor(7, time.Hour)))
	if err != nil || claims.UserId != 7 || claims.Username != "作者甲" {
		t.Fatalf("claims=%+v err=%v, want user 7", claims, err)
	}
}

// TestParseToken_WrongSecret 密钥不符 → 拒绝。
func TestParseToken_WrongSecret(t *testing.T) {
	v := NewVerifier("other-secret")
	if _, err := v.ParseToken(sign(t, claimsFor(7, time.Hour))); err == nil {
		t.Fatal("wrong secret must reject")
	}
}

// TestParseToken_Expired 过期令牌 → 拒绝。
func TestParseToken_Expired(t *testing.T) {
	v := NewVerifier(testSecret)
	if _, err := v.ParseToken(sign(t, claimsFor(7, -time.Minute))); err == nil {
		t.Fatal("expired token must reject")
	}
}

// TestParseToken_RejectsNonHMAC 非 HMAC 签名算法 → 拒绝。
func TestParseToken_RejectsNonHMAC(t *testing.T) {
	v := NewVerifier(testSecret)
	token := jwt.NewWithClaims(jwt.SigningMethodNone, claimsFor(7, time.Hour))
	signed, err := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none: %v", err)
	}
	if _, err := v.ParseToken(signed); err == nil {
		t.Fatal("none algorithm must reject")
	}
}

// TestParseToken_Garbage 非法字符串 → 拒绝。
func TestParseToken_Garbage(t *testing.T) {
	if _, err := NewVerifier(testSecret).ParseToken("not-a-token"); err == nil {
		t.Fatal("garbage token must reject")
	}
}
