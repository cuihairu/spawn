package upload

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "upload-test-secret"

func signToken(t *testing.T, secret string, userId int64, username string, ttl time.Duration) string {
	t.Helper()
	claims := UserClaims{
		UserId:   userId,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

// --- Validator.ParseToken ---

func TestValidator_ParseToken_Valid(t *testing.T) {
	v := NewValidator(testSecret)
	token := signToken(t, testSecret, 42, "alice", time.Hour)

	claims, err := v.ParseToken(token)
	if err != nil {
		t.Fatalf("ParseToken: %v", err)
	}
	if claims.UserId != 42 || claims.Username != "alice" {
		t.Fatalf("claims = %+v, want (42, alice)", claims)
	}
}

func TestValidator_ParseToken_Invalid(t *testing.T) {
	v := NewValidator(testSecret)

	if _, err := v.ParseToken("not-a-token"); err == nil {
		t.Fatal("ParseToken with garbage must error")
	}
	// 签名密钥不匹配
	other := signToken(t, "wrong-secret", 1, "x", time.Hour)
	if _, err := v.ParseToken(other); err == nil {
		t.Fatal("ParseToken with wrong secret must error")
	}
	// 过期令牌
	expired := signToken(t, testSecret, 1, "x", -time.Hour)
	if _, err := v.ParseToken(expired); err == nil {
		t.Fatal("ParseToken with expired token must error")
	}
}

// TestValidator_ParseToken_RejectsNonHMAC RSA 签名的令牌：密钥类型选择器
// 拒绝非 HMAC 方法（与 user-service 口径一致的算法混淆防护）。
func TestValidator_ParseToken_RejectsNonHMAC(t *testing.T) {
	v := NewValidator(testSecret)

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}
	claims := UserClaims{
		UserId:   7,
		Username: "rsa",
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodRS256, claims).SignedString(key)
	if err != nil {
		t.Fatalf("sign rsa token: %v", err)
	}

	_, err = v.ParseToken(token)
	if err == nil || !strings.Contains(err.Error(), "不支持的签名方法") {
		t.Fatalf("err = %v, want 不支持的签名方法", err)
	}
}

// --- Store.SaveImage ---

func pngBody() []byte {
	// 1x1 PNG 魔数 + 最小 IHDR（魔数检测只需前 8 字节）
	return []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
	}
}

func multipartRequest(t *testing.T, field, filename string, body []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile(field, filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := fw.Write(body); err != nil {
		t.Fatalf("write body: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	r := httptest.NewRequest(http.MethodPost, "/upload", &buf)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	return r
}

func TestStore_SaveImage_Png(t *testing.T) {
	dir := t.TempDir()
	s := &Store{Dir: dir, MaxBytes: 1 << 20}

	url, err := s.SaveImage(multipartRequest(t, "file", "pic.png", pngBody()), "file")
	if err != nil {
		t.Fatalf("SaveImage: %v", err)
	}
	if !strings.HasPrefix(url, "/uploads/") {
		t.Fatalf("url = %q, want /uploads/ prefix", url)
	}
	name := strings.TrimPrefix(url, "/uploads/")
	if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
		t.Fatalf("saved file missing: %v", err)
	}
}

func TestStore_SaveImage_NoField(t *testing.T) {
	s := &Store{Dir: t.TempDir(), MaxBytes: 1 << 20}

	if _, err := s.SaveImage(multipartRequest(t, "other", "pic.png", pngBody()), "file"); err == nil {
		t.Fatal("SaveImage with missing field must error")
	}
}

func TestStore_SaveImage_TooLarge(t *testing.T) {
	s := &Store{Dir: t.TempDir(), MaxBytes: 8}

	_, err := s.SaveImage(multipartRequest(t, "file", "pic.png", pngBody()), "file")
	if err == nil {
		t.Fatal("SaveImage over limit must error")
	}
	var mbe *MaxUploadError
	if !errors.As(err, &mbe) {
		t.Fatalf("err = %v (%T), want *MaxUploadError", err, err)
	}
	if mbe.Limit != 8 {
		t.Fatalf("Limit = %d, want 8", mbe.Limit)
	}
}

func TestStore_SaveImage_UnsupportedType(t *testing.T) {
	s := &Store{Dir: t.TempDir(), MaxBytes: 1 << 20}
	// 纯文本魔数不在白名单
	body := []byte("this is plain text, not an image at all")

	if _, err := s.SaveImage(multipartRequest(t, "file", "pic.png", body), "file"); err == nil {
		t.Fatal("SaveImage with non-image body must error")
	}
}

// TestStore_SaveImage_MkdirFails Dir 路径中间存在同名普通文件：
// MkdirAll 无法建目录 → 「创建上传目录失败」。
func TestStore_SaveImage_MkdirFails(t *testing.T) {
	base := t.TempDir()
	blocker := filepath.Join(base, "f")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker file: %v", err)
	}
	s := &Store{Dir: filepath.Join(blocker, "sub"), MaxBytes: 1 << 20}

	_, err := s.SaveImage(multipartRequest(t, "file", "pic.png", pngBody()), "file")
	if err == nil || !strings.Contains(err.Error(), "创建上传目录失败") {
		t.Fatalf("err = %v, want 创建上传目录失败", err)
	}
}

// TestStore_SaveImage_WriteFails 目录存在但只读：魔数检测通过后
// OpenFile O_CREATE 失败 → 「写入上传文件失败」。
func TestStore_SaveImage_WriteFails(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatalf("chmod dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) }) // 便于 TempDir 清理

	s := &Store{Dir: dir, MaxBytes: 1 << 20}
	_, err := s.SaveImage(multipartRequest(t, "file", "pic.png", pngBody()), "file")
	if err == nil || !strings.Contains(err.Error(), "写入上传文件失败") {
		t.Fatalf("err = %v, want 写入上传文件失败", err)
	}
}

// --- SafeResolve ---

func TestSafeResolve(t *testing.T) {
	dir := t.TempDir()
	name := strings.Repeat("a", 32) + ".png"

	if got := SafeResolve(dir, name); got != filepath.Join(dir, name) {
		t.Fatalf("valid name = %q, want %q", got, filepath.Join(dir, name))
	}
	// 路径遍历
	if got := SafeResolve(dir, "../"+name); got != "" {
		t.Fatalf("traversal = %q, want empty", got)
	}
	if got := SafeResolve(dir, "/etc/passwd"); got != "" {
		t.Fatalf("absolute = %q, want empty", got)
	}
	// 非法扩展名
	if got := SafeResolve(dir, strings.Repeat("a", 32)+".exe"); got != "" {
		t.Fatalf("bad ext = %q, want empty", got)
	}
	// 非 32 位十六进制
	if got := SafeResolve(dir, "short.png"); got != "" {
		t.Fatalf("short name = %q, want empty", got)
	}
}
