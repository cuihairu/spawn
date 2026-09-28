package utils

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "game-catalog-utils-test-secret"

func newTestAuth() *Auth { return NewAuth(testSecret) }

func signedClaims(t *testing.T, secret string, mutate func(*JWTClaims)) string {
	t.Helper()
	claims := JWTClaims{
		UserId:   7,
		Username: "alice",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	if mutate != nil {
		mutate(&claims)
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func TestParseToken_Valid(t *testing.T) {
	auth := newTestAuth()
	claims, err := auth.ParseToken(signedClaims(t, testSecret, nil))
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.UserId != 7 || claims.Username != "alice" {
		t.Fatalf("claims = %+v, want user 7 / alice", claims)
	}
}

func TestParseToken_Expired(t *testing.T) {
	auth := newTestAuth()
	token := signedClaims(t, testSecret, func(c *JWTClaims) {
		c.ExpiresAt = jwt.NewNumericDate(time.Now().Add(-time.Minute))
	})
	if _, err := auth.ParseToken(token); err == nil {
		t.Fatal("expired token must be rejected")
	}
}

func TestParseToken_WrongSecret(t *testing.T) {
	auth := newTestAuth()
	if _, err := auth.ParseToken(signedClaims(t, "other-secret", nil)); err == nil {
		t.Fatal("token signed with another secret must be rejected")
	}
}

// TestParseToken_UnsupportedSigningMethod 手工构造 RS256 令牌（签名必然无效），
// 确保在验签之前就因非 HMAC 签名方法被拒绝。
func TestParseToken_UnsupportedSigningMethod(t *testing.T) {
	header, err := json.Marshal(map[string]string{"alg": "RS256", "typ": "JWT"})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	payload, err := json.Marshal(map[string]interface{}{"user_id": 7, "username": "alice"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	forged := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(payload) + ".forged-signature"

	if _, err := newTestAuth().ParseToken(forged); err == nil {
		t.Fatal("RS256 token must be rejected")
	}
}

func TestParseToken_Malformed(t *testing.T) {
	for _, token := range []string{"", "not-a-jwt", "a.b"} {
		if _, err := newTestAuth().ParseToken(token); err == nil {
			t.Fatalf("malformed token %q must be rejected", token)
		}
	}
}

func TestValidateToken(t *testing.T) {
	auth := newTestAuth()
	ok, err := auth.ValidateToken(signedClaims(t, testSecret, nil))
	if err != nil || !ok {
		t.Fatalf("ValidateToken = %v, %v; want true, nil", ok, err)
	}
	if ok, err := auth.ValidateToken("garbage"); err == nil || ok {
		t.Fatalf("ValidateToken(garbage) = %v, %v; want false, err", ok, err)
	}
}

func TestGetUserIdFromToken(t *testing.T) {
	auth := newTestAuth()
	id, err := auth.GetUserIdFromToken(signedClaims(t, testSecret, nil))
	if err != nil {
		t.Fatalf("GetUserIdFromToken: %v", err)
	}
	if id != 7 {
		t.Fatalf("user id = %d, want 7", id)
	}
}
