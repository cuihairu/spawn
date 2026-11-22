package utils

// auth.go 补漏分支：ParseToken 的解析失败/非 HMAC 签名方法/claims 断言
// 失败/post-parse 过期臂（jwt v5 对手动签出的已过期令牌在 parse 期就
// 拒绝，post-parse 复查臂经时钟内过期难触发，登记不可达），以及
// ValidateJWTSecret 的空密钥拒绝臂。
//
// 登记不可达（不测）：39.3,41.1（`!ok || !token.Valid` — v5 解析成功后
// Valid 恒真、断言恒成功）；43.3,45.1（post-parse exp 复查 — 过期令牌
// 在解析期即被拒绝）。

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// signToken 用 HS256 签出测试令牌（可自定过期时刻）。
func signToken(t *testing.T, secret string, exp time.Time) string {
	t.Helper()
	claims := JWTClaims{
		UserId:           7,
		Username:         "player",
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(exp)},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tok
}

func TestParseToken_Arms(t *testing.T) {
	const secret = "promotions-test-secret"
	a := NewAuth(secret)

	// 合法令牌 → claims 回读
	claims, err := a.ParseToken(signToken(t, secret, time.Now().Add(time.Hour)))
	if err != nil || claims.UserId != 7 || claims.Username != "player" {
		t.Fatalf("valid token = %+v/%v", claims, err)
	}

	// 垃圾令牌 → 解析失败
	if _, err := a.ParseToken("garbage"); err == nil || !strings.Contains(err.Error(), "parse token") {
		t.Fatalf("garbage token = %v, want parse token error", err)
	}

	// 错误密钥签发 → 解析失败
	if _, err := a.ParseToken(signToken(t, "other-secret", time.Now().Add(time.Hour))); err == nil {
		t.Fatal("wrong-secret token = nil err, want error")
	}

	// 非 HMAC 签名方法（alg=none）→ keyfunc 拒绝
	none := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{"sub": "1"})
	bad, err := none.SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatalf("sign none token: %v", err)
	}
	if _, err := a.ParseToken(bad); err == nil {
		t.Fatal("none token = nil err, want unsupported signing method")
	} else if !strings.Contains(err.Error(), "unsupported signing method") {
		t.Fatalf("none token err = %v, want unsupported signing method", err)
	}

	// 已过期令牌（解析期 exp 校验）→ 解析失败
	if _, err := a.ParseToken(signToken(t, secret, time.Now().Add(-time.Hour))); err == nil {
		t.Fatal("expired token = nil err, want error")
	}
}

func TestValidateJWTSecret_Arms(t *testing.T) {
	if err := ValidateJWTSecret(""); err == nil {
		t.Fatal("empty secret = nil err, want error")
	} else if !strings.Contains(err.Error(), "JWT_SECRET") {
		t.Fatalf("empty secret err = %v, want JWT_SECRET hint", err)
	}
	if err := ValidateJWTSecret("   "); err == nil {
		t.Fatal("blank secret = nil err, want error")
	}
	if err := ValidateJWTSecret("real-secret"); err != nil {
		t.Fatalf("real secret = %v, want nil", err)
	}
}
