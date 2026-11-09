package utils

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims JWT 载荷（与 user-service/game-catalog 一致；令牌由 user-service
// 签发，本服务仅校验）。
type JWTClaims struct {
	UserId   int64  `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Auth JWT 校验工具
type Auth struct {
	jwtSecret []byte
}

func NewAuth(jwtSecret string) *Auth {
	return &Auth{jwtSecret: []byte(jwtSecret)}
}

func (a *Auth) ParseToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unsupported signing method: %v", token.Header["alg"])
		}
		return a.jwtSecret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}
	claims, ok := token.Claims.(*JWTClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}
	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("token expired")
	}
	return claims, nil
}

// WellKnownDevJWTSecret 仓库内置演示密钥（源码可见）。用它签出的令牌可伪造
// 任意用户：除本地开发外必须以 JWT_SECRET 覆盖，启动期命中此值大声告警。
const WellKnownDevJWTSecret = "tappi-user-service-jwt-secret-key-2024"

// WellKnownSandboxSecret 内置沙箱渠道 HMAC 密钥（源码可见）。仅限开发/沙箱
// teller 回调；真实渠道不得使用，且不得以沙箱 HMAC 冒充支付机构验签。
const WellKnownSandboxSecret = "tappi-payments-sandbox-channel-secret-2026"

// ValidateJWTSecret 启动期校验 JWT 密钥：空密钥直接拒绝。
func ValidateJWTSecret(secret string) error {
	if strings.TrimSpace(secret) == "" {
		return errors.New("JWT 密钥未配置：环境变量 JWT_SECRET")
	}
	return nil
}
