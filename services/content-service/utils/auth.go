package utils

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// JWTClaims JWT 载荷
type JWTClaims struct {
	UserId   int64  `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Auth 认证工具
type Auth struct {
	jwtSecret []byte
}

// NewAuth 创建认证工具
func NewAuth(jwtSecret string) *Auth {
	return &Auth{
		jwtSecret: []byte(jwtSecret),
	}
}

// ParseToken 解析JWT令牌
func (a *Auth) ParseToken(tokenString string) (*JWTClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &JWTClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("不支持的签名方法: %v", token.Header["alg"])
		}
		return a.jwtSecret, nil
	})

	if err != nil {
		return nil, fmt.Errorf("解析令牌失败: %w", err)
	}

	if claims, ok := token.Claims.(*JWTClaims); ok && token.Valid {
		// 检查令牌是否过期
		if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
			return nil, errors.New("令牌已过期")
		}
		return claims, nil
	}

	return nil, errors.New("无效的令牌")
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
