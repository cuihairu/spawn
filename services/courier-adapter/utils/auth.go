package utils

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims JWT 载荷（与 user-service/game-catalog 保持一致；令牌由
// user-service 签发，本服务仅校验）。
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

// WellKnownDevJWTSecret 仓库内置演示密钥（与 user-service 等共用默认值，
// 源码可见）。用它签出的令牌可伪造任意用户：除本地开发外必须以 JWT_SECRET
// 环境变量覆盖，启动期命中此值会大声告警（GP-004 同款口径）。
const WellKnownDevJWTSecret = "tappi-user-service-jwt-secret-key-2024"

// WellKnownDevCredentialKey 内置演示绑定凭据加密密钥（源码可见）。加密的
// 是代登录随机密码，非用户口令；除本地开发外必须以 CREDENTIAL_KEY 覆盖。
const WellKnownDevCredentialKey = "tappi-courier-adapter-credential-key-2026"

// ValidateJWTSecret 启动期校验 JWT 密钥：空密钥（未配置）直接拒绝。
func ValidateJWTSecret(secret string) error {
	if strings.TrimSpace(secret) == "" {
		return errors.New("JWT 密钥未配置：校验平台令牌需要显式密钥（环境变量 JWT_SECRET）")
	}
	return nil
}

// ValidateCredentialKey 启动期校验绑定凭据加密密钥：空密钥无法安全代管
// Courier 随机密码，直接拒绝启动。
func ValidateCredentialKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return errors.New("凭据加密密钥未配置：自动绑定需加密保存 Courier 随机密码（环境变量 CREDENTIAL_KEY）")
	}
	return nil
}
