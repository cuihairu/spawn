package utils

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims JWT 载荷（与 user-service/content-service 保持一致）
type JWTClaims struct {
	UserId   int64  `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Auth JWT 认证工具
type Auth struct {
	jwtSecret []byte
}

func NewAuth(jwtSecret string) *Auth {
	return &Auth{jwtSecret: []byte(jwtSecret)}
}

// WellKnownDevJWTSecret 仓库内置演示密钥（etc/*.yaml 与 docker-compose 的
// 默认值，源码可见）。用它签出的令牌可伪造任意用户：除本地开发外必须以
// JWT_SECRET 环境变量覆盖，启动期命中此值会大声告警。
const WellKnownDevJWTSecret = "tappi-user-service-jwt-secret-key-2024"

// ValidateJWTSecret 启动期校验 JWT 密钥：空密钥（未配置）直接拒绝，
// 避免以空密钥静默接受可被任何人复算的令牌。
func ValidateJWTSecret(secret string) error {
	if strings.TrimSpace(secret) == "" {
		return errors.New("JWT 密钥未配置：校验令牌需要显式密钥（环境变量 JWT_SECRET）")
	}
	return nil
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
