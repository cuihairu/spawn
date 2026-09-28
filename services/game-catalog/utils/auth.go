package utils

import (
	"errors"
	"fmt"
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

// NewAuth 创建认证工具
func NewAuth(jwtSecret string) *Auth {
	return &Auth{jwtSecret: []byte(jwtSecret)}
}

// ParseToken 解析JWT令牌
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

// ValidateToken 验证令牌有效性
func (a *Auth) ValidateToken(tokenString string) (bool, error) {
	_, err := a.ParseToken(tokenString)
	if err != nil {
		return false, err
	}
	return true, nil
}

// GetUserIdFromToken 从令牌中获取用户ID
func (a *Auth) GetUserIdFromToken(tokenString string) (int64, error) {
	claims, err := a.ParseToken(tokenString)
	if err != nil {
		return 0, err
	}
	return claims.UserId, nil
}
