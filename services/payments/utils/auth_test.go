package utils

// auth.go 剩余补漏分支：ParseToken 的成功/过期/非法签名算法（非 HMAC）/
// 解析失败臂，ValidateJWTSecret 的空与非空臂。
//
// 登记不可达（不测）：claims 类型断言失败与 !token.Valid 臂（jwt v5
// 解析成功后 Claims 恒为 JWTClaims 且 Valid 恒 true，err==nil 时该臂不触发）；
// 过期复查臂（43-44）：jwt v5 默认校验 exp，过期令牌在解析期即报
// ErrTokenExpired，err 在 37 返回，43 的二次复查恒不触发。

import (
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestAuth_ParseTokenArms(t *testing.T) {
	a := NewAuth("test-secret")
	// 合法令牌 → 载荷正确
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, &JWTClaims{
		UserId: 42, Username: "alice",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	})
	raw, err := tok.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	claims, err := a.ParseToken(raw)
	if err != nil || claims.UserId != 42 || claims.Username != "alice" {
		t.Fatalf("valid token = %+v/%v", claims, err)
	}

	// 过期令牌 → "token expired"
	exp := jwt.NewWithClaims(jwt.SigningMethodHS256, &JWTClaims{
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour))},
	})
	expRaw, _ := exp.SignedString([]byte("test-secret"))
	if _, err := a.ParseToken(expRaw); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expired token = %v, want expired", err)
	}

	// 非 HMAC 签名算法（ES256 头）→ 密钥函数拒绝
	notHMAC := "eyJhbGciOiJFUzI1NiIsInR5cCI6IkpXVCJ9." + "eyJzdWIiOiIxIn0." + "aaaa"
	if _, err := a.ParseToken(notHMAC); err == nil || !strings.Contains(err.Error(), "unsupported signing method") {
		t.Fatalf("non-HMAC token = %v, want unsupported method", err)
	}

	// 解析失败（非令牌形状）→ parse token 包装错误
	if _, err := a.ParseToken("not.a.token"); err == nil {
		t.Fatal("garbage token = nil, want error")
	}
}

func TestValidateJWTSecretArms(t *testing.T) {
	if err := ValidateJWTSecret("  "); err == nil {
		t.Fatal("empty secret = nil, want error")
	}
	if err := ValidateJWTSecret("x"); err != nil {
		t.Fatalf("non-empty secret = %v, want nil", err)
	}
}
