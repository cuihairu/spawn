// Package upload 提供 api-gateway 统一上传入口的 JWT 校验与图片落盘。
// JWT 校验口径与 user-service 的 utils/auth.go 对齐：HS256、同一 secret、
// 同一字段结构；上传落本地盘（选型方案 A），业务服务只存返回的 URL。
package upload

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/golang-jwt/jwt/v5"
)

// UserClaims 与 user-service utils.JWTClaims 的字段口径保持一致，
// 改动需两侧同步。
type UserClaims struct {
	UserId   int64  `json:"user_id"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

// Validator 用与 user-service 一致的 secret 本地解析并校验 JWT。
type Validator struct {
	jwtSecret []byte
}

func NewValidator(secret string) *Validator {
	return &Validator{jwtSecret: []byte(secret)}
}

func (v *Validator) ParseToken(tokenString string) (*UserClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &UserClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("不支持的签名方法: %v", token.Header["alg"])
		}
		return v.jwtSecret, nil
	})
	if err != nil {
		return nil, fmt.Errorf("解析令牌失败: %w", err)
	}
	if claims, ok := token.Claims.(*UserClaims); ok && token.Valid {
		return claims, nil
	}
	return nil, errors.New("无效的令牌")
}

// allowedTypes 魔数检测放行的图片类型及其扩展名映射。
var allowedTypes = map[string]string{
	"image/png":  ".png",
	"image/jpeg": ".jpg",
	"image/webp": ".webp",
	"image/gif":  ".gif",
}

// MaxUploadError 标记请求体超出大小上限。
type MaxUploadError struct{ Limit int64 }

func (e *MaxUploadError) Error() string {
	return fmt.Sprintf("上传内容超过大小上限 %d 字节", e.Limit)
}

// Store 图片落盘：目录 + 大小上限。
type Store struct {
	Dir      string
	MaxBytes int64
}

// SaveImage 从 multipart 表单取出 file 字段，魔数校验通过后以随机文件名
// 落入 Store.Dir，返回可对外引用的 "/uploads/<name>" URL。
func (s *Store) SaveImage(r *http.Request, field string) (string, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, s.MaxBytes)
	file, _, err := r.FormFile(field)
	if err != nil {
		var mbe *http.MaxBytesError
		if errors.As(err, &mbe) {
			return "", &MaxUploadError{Limit: s.MaxBytes}
		}
		return "", fmt.Errorf("读取上传内容失败: %w", err)
	}
	defer file.Close()

	sniff := make([]byte, 512)
	n, err := file.Read(sniff)
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("读取上传内容失败: %w", err)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return "", fmt.Errorf("读取上传内容失败: %w", err)
	}
	ext, ok := allowedTypes[http.DetectContentType(sniff[:n])]
	if !ok {
		return "", fmt.Errorf("不支持的图片类型，仅允许 png/jpeg/webp/gif")
	}

	name := make([]byte, 16)
	if _, err := rand.Read(name); err != nil {
		return "", fmt.Errorf("生成文件名失败: %w", err)
	}
	target := filepath.Join(s.Dir, hex.EncodeToString(name)+ext)
	if err := os.MkdirAll(s.Dir, 0o755); err != nil {
		return "", fmt.Errorf("创建上传目录失败: %w", err)
	}
	out, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return "", fmt.Errorf("写入上传文件失败: %w", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, file); err != nil {
		os.Remove(target)
		return "", fmt.Errorf("写入上传文件失败: %w", err)
	}
	return "/uploads/" + filepath.Base(target), nil
}

// fileNameRe 上传文件名的白名单形态：32 位十六进制 + 受支持扩展名，
// 静态托管按此校验以杜绝路径遍历。
var fileNameRe = regexp.MustCompile(`^[0-9a-f]{32}\.(png|jpg|webp|gif)$`)

// SafeResolve 校验请求文件名并返回 uploads 目录下的绝对路径；不匹配
// 白名单时返回空串。静态托管端点据此拒绝路径遍历与任意文件读取。
func SafeResolve(dir, name string) string {
	name = strings.TrimPrefix(filepath.ToSlash(name), "/")
	if strings.Contains(name, "/") || !fileNameRe.MatchString(name) {
		return ""
	}
	return filepath.Join(dir, name)
}
