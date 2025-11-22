package utils

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// JWTClaims JWT 载荷
type JWTClaims struct {
	UserId   int64  `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Auth 认证工具
type Auth struct {
	jwtSecret     []byte
	tokenExpire   time.Duration
}

// NewAuth 创建认证工具
func NewAuth(jwtSecret string, tokenExpire time.Duration) *Auth {
	return &Auth{
		jwtSecret:   []byte(jwtSecret),
		tokenExpire: tokenExpire,
	}
}

// GenerateToken 生成JWT令牌
func (a *Auth) GenerateToken(userId int64, username string) (string, error) {
	now := time.Now()
	claims := JWTClaims{
		UserId:   userId,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(a.tokenExpire)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			Issuer:    "tappi-user-service",
			Subject:   fmt.Sprintf("%d", userId),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(a.jwtSecret)
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

// HashPassword 密码加密
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", fmt.Errorf("密码加密失败: %w", err)
	}
	return string(bytes), nil
}

// CheckPassword 验证密码
func CheckPassword(password, hash string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// ValidatePassword 密码强度验证
func ValidatePassword(password string) error {
	if len(password) < 6 {
		return errors.New("密码长度不能少于6位")
	}
	if len(password) > 50 {
		return errors.New("密码长度不能超过50位")
	}
	return nil
}

// ValidateEmail 邮箱格式验证
func ValidateEmail(email string) error {
	if email == "" {
		return errors.New("邮箱不能为空")
	}
	if len(email) > 100 {
		return errors.New("邮箱长度不能超过100位")
	}
	// 简单的邮箱格式验证
	if email[len(email)-4:] != ".com" && email[len(email)-3:] != ".cn" {
		return errors.New("邮箱格式不正确")
	}
	return nil
}

// ValidateUsername 用户名验证
func ValidateUsername(username string) error {
	if len(username) < 3 {
		return errors.New("用户名长度不能少于3位")
	}
	if len(username) > 50 {
		return errors.New("用户名长度不能超过50位")
	}
	return nil
}