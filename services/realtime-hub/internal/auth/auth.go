// Package auth WS 握手令牌校验：与各服务同款 HS256 JWT（claims 结构与
// community/user-service 的 utils.JWTClaims 一致，共享 JWT_SECRET）。
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims 与各服务 utils.JWTClaims 保持一致的载荷结构。
type Claims struct {
	UserId   int64  `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Verifier 共享密钥校验器。
type Verifier struct {
	jwtSecret []byte
}

func NewVerifier(jwtSecret string) *Verifier {
	return &Verifier{jwtSecret: []byte(jwtSecret)}
}

// ParseToken 校验签名与有效期，返回用户 claims。
func (v *Verifier) ParseToken(tokenString string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unsupported signing method: %v", token.Header["alg"])
		}
		return v.jwtSecret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := token.Claims.(*Claims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token")
	}

	if claims.ExpiresAt != nil && claims.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("token expired")
	}

	return claims, nil
}
