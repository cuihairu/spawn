package middleware

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/tappi/tappi/services/data-panel/internal/httperr"
	"github.com/tappi/tappi/services/data-panel/internal/logic/common"
	"github.com/tappi/tappi/services/data-panel/utils"

	"github.com/zeromicro/go-zero/rest/httpx"
)

const testSecret = "community-middleware-test-secret"

func init() {
	// 与 community.go 生产装配一致：错误响应体由 httperr.ErrorHandler 塑形
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)
}

func signToken(t *testing.T, userId int64, username string) string {
	t.Helper()
	now := time.Now()
	claims := utils.JWTClaims{
		UserId:   userId,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

// result 记录中间件是否放行，以及放行时透传到 next 的身份信息。
type result struct {
	called     bool
	status     int
	body       string
	userId     int64
	username   string
	ctxErrText string
}

func run(t *testing.T, m *AuthMiddleware, method, authHeader string) result {
	t.Helper()
	var res result
	next := func(w http.ResponseWriter, r *http.Request) {
		res.called = true
		id, name, err := common.UserFromContext(r.Context())
		if err != nil {
			res.ctxErrText = err.Error()
		}
		res.userId, res.username = id, name
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("next"))
	}

	req := httptest.NewRequest(method, "/api/v1/posts", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	m.Handle(next)(rec, req)

	res.status = rec.Code
	res.body = strings.TrimSpace(rec.Body.String())
	return res
}

func decodeError(t *testing.T, body string) httperr.Response {
	t.Helper()
	var payload httperr.Response
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		t.Fatalf("decode error body %q: %v", body, err)
	}
	return payload
}

func assertRejected(t *testing.T, res result, wantStatus int, wantMessage string) {
	t.Helper()
	if res.called {
		t.Fatal("next handler must not be invoked on rejection")
	}
	if res.status != wantStatus {
		t.Fatalf("status = %d, want %d (body=%s)", res.status, wantStatus, res.body)
	}
	payload := decodeError(t, res.body)
	if int(payload.Code) != wantStatus || payload.Message != wantMessage {
		t.Fatalf("payload = %+v, want code=%d message=%q", payload, wantStatus, wantMessage)
	}
}

func TestNewAuthMiddleware(t *testing.T) {
	if m := NewAuthMiddleware(utils.NewAuth(testSecret)); m == nil || m.auth == nil {
		t.Fatalf("NewAuthMiddleware = %#v", m)
	}
}

// TestHandleAllowsPreflight OPTIONS 直通（浏览器 CORS 预检不带 Authorization）。
func TestHandleAllowsPreflight(t *testing.T) {
	m := NewAuthMiddleware(utils.NewAuth(testSecret))
	res := run(t, m, http.MethodOptions, "")
	if !res.called || res.status != http.StatusOK {
		t.Fatalf("preflight: called=%v status=%d body=%s", res.called, res.status, res.body)
	}
}

func TestHandleRejectsMissingToken(t *testing.T) {
	m := NewAuthMiddleware(utils.NewAuth(testSecret))
	assertRejected(t, run(t, m, http.MethodGet, ""), http.StatusUnauthorized, "missing authorization token")
}

func TestHandleRejectsBadPrefix(t *testing.T) {
	m := NewAuthMiddleware(utils.NewAuth(testSecret))
	for _, header := range []string{"Token abc", "bearer abc", "Bearer", "abc"} {
		t.Run(header, func(t *testing.T) {
			assertRejected(t, run(t, m, http.MethodGet, header), http.StatusUnauthorized, "invalid token format")
		})
	}
}

func TestHandleRejectsEmptyToken(t *testing.T) {
	m := NewAuthMiddleware(utils.NewAuth(testSecret))
	for _, header := range []string{"Bearer ", "Bearer      ", "Bearer  \t "} {
		t.Run(header, func(t *testing.T) {
			assertRejected(t, run(t, m, http.MethodGet, header), http.StatusUnauthorized, "empty token")
		})
	}
}

// TestHandleAuthNotConfigured m.auth == nil 的防御分支：返回 500 而非 panic。
func TestHandleAuthNotConfigured(t *testing.T) {
	m := NewAuthMiddleware(nil)
	assertRejected(t, run(t, m, http.MethodGet, "Bearer whatever"), http.StatusInternalServerError, "auth not configured")
}

func TestHandleRejectsInvalidToken(t *testing.T) {
	m := NewAuthMiddleware(utils.NewAuth(testSecret))
	for _, c := range []struct{ name, header string }{
		{"garbage", "Bearer garbage"},
		{"three segments", "Bearer a.b.c"},
		{"appended junk", "Bearer " + signToken(t, 42, "alice") + "tampered"},
		{"truncated", "Bearer " + signToken(t, 42, "alice")[:20]},
		{"payload tampered", "Bearer " + tamperPayload(t, signToken(t, 42, "alice"))},
	} {
		t.Run(c.name, func(t *testing.T) {
			assertRejected(t, run(t, m, http.MethodGet, c.header), http.StatusUnauthorized, "invalid token")
		})
	}
}

// tamperPayload 翻转 payload 段的一个字节并重新编码：签名必然失配，
// 行为确定（不依赖 base64 末位 padding 位是否有效）。
func tamperPayload(t *testing.T, token string) string {
	t.Helper()
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		t.Fatalf("unexpected token shape: %q", token)
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(raw) == 0 {
		t.Fatalf("decode payload: %v", err)
	}
	raw[0] ^= 0x01
	parts[1] = base64.RawURLEncoding.EncodeToString(raw)
	return strings.Join(parts, ".")
}

// TestHandleRejectsTokenFromAnotherSecret 密钥不匹配的令牌。
func TestHandleRejectsTokenFromAnotherSecret(t *testing.T) {
	other, err := jwt.NewWithClaims(jwt.SigningMethodHS256, utils.JWTClaims{UserId: 1, Username: "eve"}).
		SignedString([]byte("attacker-secret"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	m := NewAuthMiddleware(utils.NewAuth(testSecret))
	assertRejected(t, run(t, m, http.MethodGet, "Bearer "+other), http.StatusUnauthorized, "invalid token")
}

// TestHandleInjectsIdentity 放行时把 user_id / username 注入请求上下文。
func TestHandleInjectsIdentity(t *testing.T) {
	m := NewAuthMiddleware(utils.NewAuth(testSecret))

	res := run(t, m, http.MethodGet, "Bearer "+signToken(t, 42, "alice"))
	if !res.called || res.status != http.StatusOK {
		t.Fatalf("called=%v status=%d body=%s", res.called, res.status, res.body)
	}
	if res.userId != 42 || res.username != "alice" {
		t.Fatalf("injected identity = %d/%q, want 42/alice", res.userId, res.username)
	}
	if res.ctxErrText != "" {
		t.Fatalf("UserFromContext err = %q", res.ctxErrText)
	}

	// 令牌前后空白会被 TrimSpace 清理，不影响解析
	if res = run(t, m, http.MethodGet, "Bearer   "+signToken(t, 7, "bob")+"  "); !res.called ||
		res.userId != 7 || res.username != "bob" {
		t.Fatalf("padded token: called=%v identity=%d/%q", res.called, res.userId, res.username)
	}
}
