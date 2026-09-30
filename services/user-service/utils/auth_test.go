package utils

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSecret = "test-secret-jwt-key-2024"

var testTokenExpire = 7 * 24 * time.Hour

func signTestToken(t *testing.T, secret string, claims JWTClaims) string {
	t.Helper()

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func testClaims(userId int64, username string, expiresAt time.Time) JWTClaims {
	return JWTClaims{
		UserId:   userId,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}
}

func TestNewAuth(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret, testTokenExpire)
	if auth == nil {
		t.Fatal("expected non-nil Auth")
	}
	if string(auth.jwtSecret) != testSecret {
		t.Fatal("expected jwtSecret to match")
	}
	if auth.tokenExpire != testTokenExpire {
		t.Fatal("expected tokenExpire to be 7 days")
	}
}

func TestGenerateTokenAndParse(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret, testTokenExpire)
	tokenString, err := auth.GenerateToken(42, "alice")
	if err != nil {
		t.Fatalf("GenerateToken error = %v", err)
	}
	if tokenString == "" {
		t.Fatal("expected non-empty token")
	}

	claims, parseErr := auth.ParseToken(tokenString)
	if parseErr != nil {
		t.Fatalf("ParseToken() error = %v", parseErr)
	}
	if claims.UserId != 42 {
		t.Fatalf("expected user_id 42, got %d", claims.UserId)
	}
	if claims.Username != "alice" {
		t.Fatalf("expected username alice, got %s", claims.Username)
	}
}

func TestParseToken_Valid(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret, testTokenExpire)
	claims := testClaims(42, "alice", time.Now().Add(time.Hour))
	tokenString := signTestToken(t, testSecret, claims)

	parsed, parseErr := auth.ParseToken(tokenString)
	if parseErr != nil {
		t.Fatalf("ParseToken() error = %v", parseErr)
	}
	if parsed.UserId != 42 || parsed.Username != "alice" {
		t.Fatalf("unexpected claims: %+v", parsed)
	}
}

func TestParseToken_Expired(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret, testTokenExpire)
	claims := testClaims(1, "bob", time.Now().Add(-time.Hour))
	tokenString := signTestToken(t, testSecret, claims)

	_, parseErr := auth.ParseToken(tokenString)
	if parseErr == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestParseToken_WrongSecret(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret, testTokenExpire)
	claims := testClaims(1, "carol", time.Now().Add(time.Hour))
	tokenString := signTestToken(t, "another-secret", claims)

	_, parseErr := auth.ParseToken(tokenString)
	if parseErr == nil {
		t.Fatal("expected error for wrong secret")
	}
}

func TestParseToken_UnsupportedSigningMethod(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret, testTokenExpire)

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"user_id":1,"username":"dave"}`))
	tokenString := header + "." + payload + ".c2lnbmF0dXJl"

	_, parseErr := auth.ParseToken(tokenString)
	if parseErr == nil {
		t.Fatal("expected error for non-HMAC token")
	}
	if !strings.Contains(parseErr.Error(), "不支持的签名方法") {
		t.Fatalf("expected unsupported-method error, got %q", parseErr.Error())
	}
}

func TestParseToken_Malformed(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret, testTokenExpire)

	_, parseErr := auth.ParseToken("not-a-token")
	if parseErr == nil {
		t.Fatal("expected error for malformed token")
	}
}

func TestValidateToken(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret, testTokenExpire)

	validClaims := testClaims(2, "eve", time.Now().Add(time.Hour))
	validTokenString := signTestToken(t, testSecret, validClaims)

	ok, validateErr := auth.ValidateToken(validTokenString)
	if validateErr != nil || !ok {
		t.Fatalf("ValidateToken(valid) = %v, %v; want true, nil", ok, validateErr)
	}

	expiredClaims := testClaims(2, "eve", time.Now().Add(-time.Hour))
	expiredTokenString := signTestToken(t, testSecret, expiredClaims)

	ok, validateErr = auth.ValidateToken(expiredTokenString)
	if validateErr == nil || ok {
		t.Fatalf("ValidateToken(expired) = %v, %v; want false, error", ok, validateErr)
	}
}

func TestGetUserIdFromToken(t *testing.T) {
	t.Parallel()

	auth := NewAuth(testSecret, testTokenExpire)

	validClaims := testClaims(99, "frank", time.Now().Add(time.Hour))
	validTokenString := signTestToken(t, testSecret, validClaims)

	userId, getUserErr := auth.GetUserIdFromToken(validTokenString)
	if getUserErr != nil || userId != 99 {
		t.Fatalf("GetUserIdFromToken(valid) = %d, %v; want 99, nil", userId, getUserErr)
	}

	_, getUserErr = auth.GetUserIdFromToken("garbage")
	if getUserErr == nil {
		t.Fatal("expected error for invalid token")
	}
}

func TestHashPassword(t *testing.T) {
	t.Parallel()

	password := "secure-password-123"
	hash, hashErr := HashPassword(password)
	if hashErr != nil {
		t.Fatalf("HashPassword error = %v", hashErr)
	}
	if hash == "" {
		t.Fatal("expected non-empty hash")
	}

	if !CheckPassword(password, hash) {
		t.Fatal("expected CheckPassword to return true for correct password")
	}

	if CheckPassword("wrong-password", hash) {
		t.Fatal("expected CheckPassword to return false for wrong password")
	}

	if CheckPassword("", hash) {
		t.Fatal("expected CheckPassword to return false for empty password")
	}
}

func TestCheckPassword(t *testing.T) {
	t.Parallel()

	password := "test-password"
	hash, hashErr := HashPassword(password)
	if hashErr != nil {
		t.Fatalf("HashPassword error = %v", hashErr)
	}

	if !CheckPassword(password, hash) {
		t.Fatal("expected CheckPassword to return true")
	}

	if CheckPassword("different-password", hash) {
		t.Fatal("expected CheckPassword to return false")
	}
}

func TestValidatePassword(t *testing.T) {
	t.Parallel()

	err := ValidatePassword("abcdef")
	if err != nil {
		t.Fatalf("ValidatePassword('abcdef') error = %v", err)
	}

	err = ValidatePassword("ab")
	if err == nil {
		t.Fatal("expected error for password < 6 chars")
	}

	err = ValidatePassword("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err == nil {
		t.Fatal("expected error for password > 50 chars")
	}
}

func TestValidateEmail(t *testing.T) {
	t.Parallel()

	err := ValidateEmail("user@example.com")
	if err != nil {
		t.Fatalf("ValidateEmail('user@example.com') error = %v", err)
	}

	err = ValidateEmail("user@domain.cn")
	if err != nil {
		t.Fatalf("ValidateEmail('user@domain.cn') error = %v", err)
	}

	err = ValidateEmail("")
	if err == nil {
		t.Fatal("expected error for empty email")
	}

	err = ValidateEmail("user@example.org")
	if err == nil {
		t.Fatal("expected error for non-.com/.cn email")
	}
}

func TestValidateUsername(t *testing.T) {
	t.Parallel()

	err := ValidateUsername("admin")
	if err != nil {
		t.Fatalf("ValidateUsername('admin') error = %v", err)
	}

	err = ValidateUsername("ab")
	if err == nil {
		t.Fatal("expected error for username < 3 chars")
	}

	err = ValidateUsername("aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if err == nil {
		t.Fatal("expected error for username > 50 chars")
	}
}

func TestParseToken_NewAuthWithDifferentSecretIsolation(t *testing.T) {
	t.Parallel()

	authA := NewAuth(testSecret, testTokenExpire)
	authB := NewAuth("other-secret", testTokenExpire)
	tokenString := signTestToken(t, testSecret, testClaims(7, "grace", time.Now().Add(time.Hour)))

	if _, parseErr := authA.ParseToken(tokenString); parseErr != nil {
		t.Fatalf("authA should accept its own tokens: %v", parseErr)
	}
	if _, parseErr := authB.ParseToken(tokenString); parseErr == nil {
		t.Fatal("authB should reject tokens signed with a different secret")
	}
}

// TestHashPassword_TooLong bcrypt 72 字节上限：超长密码返回包装错误而非 panic。
func TestHashPassword_TooLong(t *testing.T) {
	if _, err := HashPassword(strings.Repeat("x", 73)); err == nil || !strings.Contains(err.Error(), "密码加密失败") {
		t.Fatalf("oversized password err = %v", err)
	}
}

// TestValidateEmail_TooLong 超长邮箱在格式校验前被长度上限拦截。
func TestValidateEmail_TooLong(t *testing.T) {
	if err := ValidateEmail(strings.Repeat("a", 97) + "@b.com"); err == nil || !strings.Contains(err.Error(), "超过100位") {
		t.Fatalf("oversized email err = %v", err)
	}
}
