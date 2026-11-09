package handler

// GP-201 联合接入全流程集成测试：
// 映射管理（角色）→ 游戏会话（自动建号+代登录）→ 票据一次性兑换 →
// 注销同步（PENDING_DELETE/UNBOUND）→ 宽限期撤销（ACTIVE）→
// scope 只走 header（URL/body 一律不携带，对应 Courier scope.md 传输节）。
// Courier / game-catalog / user-service 均以 httptest 桩替身。

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tappi/tappi/services/courier-adapter/internal/config"
	"github.com/tappi/tappi/services/courier-adapter/internal/logic"
	"github.com/tappi/tappi/services/courier-adapter/internal/svc"
	"github.com/tappi/tappi/services/courier-adapter/utils"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

const testJWTSecret = "adapter-test-jwt-secret"

// courierStub Courier 网关替身：记录每次调用并校验 scope 传输方式。
type courierStub struct {
	mu         sync.Mutex
	registerN  int
	loginN     int
	deleteN    int
	cancelN    int
	violations []string // scope 出现在 URL/body 的违规记录
	deleteNow  bool     // true → account/delete 返回立即 DELETED
}

func (s *courierStub) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		gameID := r.Header.Get("X-Courier-Game-Id")
		env := r.Header.Get("X-Courier-Env")
		if gameID == "" || env == "" {
			s.violate("missing scope header")
		}
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(r.URL.Path, gameID) || strings.Contains(r.URL.RawQuery, gameID) {
			s.violate("game id leaked into URL: " + r.URL.String())
		}
		if strings.Contains(string(body), gameID) || strings.Contains(string(body), `"env"`) {
			s.violate("scope leaked into body: " + string(body))
		}

		switch {
		case strings.HasSuffix(r.URL.Path, "/identity/register"):
			s.mu.Lock()
			s.registerN++
			s.mu.Unlock()
			s.writeSession(w, "acc_reg_1")
		case strings.HasSuffix(r.URL.Path, "/identity/login"):
			s.mu.Lock()
			s.loginN++
			s.mu.Unlock()
			s.writeSession(w, "acc_reg_1")
		case strings.HasSuffix(r.URL.Path, "/identity/account/delete/cancel"):
			s.mu.Lock()
			s.cancelN++
			s.mu.Unlock()
			s.writeJSON(w, `{"data":{}}`)
		case strings.HasSuffix(r.URL.Path, "/identity/account/delete"):
			s.mu.Lock()
			s.deleteN++
			now := s.deleteNow
			s.mu.Unlock()
			if now {
				s.writeJSON(w, `{"data":{"status":"DELETED","deletedAt":"2026-11-09T00:00:00.000Z"}}`)
			} else {
				s.writeJSON(w, `{"data":{"status":"PENDING_DELETE","cancelToken":"ctok-1","pendingDelete":true}}`)
			}
		default:
			s.writeJSON(w, `{"error":{"code":"COMMON_NOT_FOUND","message":"no route"}}`)
		}
	}
}

func (s *courierStub) writeSession(w http.ResponseWriter, accountID string) {
	s.writeJSON(w, fmt.Sprintf(`{"data":{"account":{"id":%q,"type":"EMAIL","status":"ACTIVE","createdAt":"2026-11-09T00:00:00.000Z"},"accessToken":"atk-1","accessExpiresAt":"2026-11-09T00:15:00.000Z","refreshToken":"rtk-1","refreshExpiresAt":"2026-12-09T00:00:00.000Z"}}`, accountID))
}

func (s *courierStub) writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func (s *courierStub) violate(msg string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.violations = append(s.violations, msg)
}

func (s *courierStub) counts() (reg, login, del, cancel int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.registerN, s.loginN, s.deleteN, s.cancelN
}

// catalogStub game-catalog /me 替身：适配器转发的是玩家本人 JWT（透传），
// 桩不解签（测试替身），仅按 claims.user_id 映射角色回包。
func catalogStub() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		role := roleForAuthHeader(r.Header.Get("Authorization"))
		if role == "" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`"无效的令牌"`))
			return
		}
		_, _ = fmt.Fprintf(w, `{"userId":1,"username":"u","role":%q,"partnerId":0}`, role)
	}
}

// roleForAuthHeader 从转发 JWT 中取 user_id 对应角色（仅测试桩用）。
func roleForAuthHeader(authHeader string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(authHeader, prefix) {
		return ""
	}
	parts := strings.Split(authHeader[len(prefix):], ".")
	if len(parts) != 3 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		UserId int64 `json:"user_id"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	if claims.UserId == 42 {
		return "operator"
	}
	if claims.UserId == 77 {
		return "player"
	}
	return ""
}

// userStub user-service /users/:id 替身：适配器透传本人 JWT 即回邮箱，
// 无令牌回空邮箱（走占位邮箱分支）。
func userStub() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			_, _ = w.Write([]byte(`{"code":200,"message":"获取成功","data":{"id":77,"username":"player","email":"player@example.com","nickname":"player"}}`))
			return
		}
		_, _ = w.Write([]byte(`{"code":200,"message":"获取成功","data":{"id":1,"username":"u","email":"","nickname":"u"}}`))
	}
}

type testEnv struct {
	base    string
	courier *courierStub
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	cour := &courierStub{}
	// 预占空闲端口再释放（go-zero 不暴露 listener 句柄，与 game-catalog 同款）
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab free port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("release port: %v", err)
	}

	var cfg config.Config
	cfg.Host = "127.0.0.1"
	cfg.Port = port
	cfg.RestConf.Timeout = 5000
	cfg.Auth.JWTSecret = testJWTSecret
	cfg.MySQL.DataSource = "file:" + t.TempDir() + "/adapter.db"
	cfg.Courier.BaseURL = httptest.NewServer(cour.handler()).URL
	cfg.Courier.Timeout = 2000
	cfg.Credential.Key = "adapter-test-credential-key"
	cfg.GameCatalog.BaseURL = httptest.NewServer(catalogStub()).URL
	cfg.GameCatalog.Timeout = 2000
	cfg.UserService.BaseURL = httptest.NewServer(userStub()).URL
	cfg.UserService.Timeout = 2000
	cfg.Ticket.TTLSeconds = 120

	svcCtx := svc.NewServiceContext(cfg)
	// 与 adapter.go 生产装配一致：注册 logic 层错误处理器（状态码生效）
	httpx.SetErrorHandlerCtx(logic.ErrorHandler)
	server := rest.MustNewServer(cfg.RestConf)
	RegisterHandlers(server, svcCtx)
	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("test server did not become ready in time")
		}
		time.Sleep(20 * time.Millisecond)
	}

	return &testEnv{base: base, courier: cour}
}

func signToken(t *testing.T, userID int64, username string) string {
	t.Helper()
	claims := utils.JWTClaims{
		UserId:   userID,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testJWTSecret))
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return tok
}

func doJSON(t *testing.T, method, url, token, body string) (int, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do %s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return resp.StatusCode, string(b)
}

func TestRoutes_CourierAdapterFlow(t *testing.T) {
	env := newTestEnv(t)
	base := env.base
	player := signToken(t, 77, "player")
	operator := signToken(t, 42, "ops")

	// 1. 映射管理：player 403，operator 200，读回一致
	if code, body := doJSON(t, http.MethodPut, base+"/admin/games/g1/mapping", player,
		`{"courierGameId":"cg1","env":"prod","partnerId":1,"capabilities":{"login":true}}`); code != http.StatusForbidden {
		t.Fatalf("player upsert mapping status = %d, want 403 body=%s", code, body)
	}
	if code, body := doJSON(t, http.MethodPut, base+"/admin/games/g1/mapping", operator,
		`{"courierGameId":"cg1","env":"prod","partnerId":1,"capabilities":{"login":true}}`); code != http.StatusOK {
		t.Fatalf("operator upsert mapping status = %d body=%s", code, body)
	}
	code, body := doJSON(t, http.MethodGet, base+"/admin/games/g1/mapping", operator, "")
	if code != http.StatusOK {
		t.Fatalf("get mapping status = %d body=%s", code, body)
	}
	var mapping struct {
		CourierGameId string `json:"courierGameId"`
		Env           string `json:"env"`
		Capabilities  struct {
			Login bool `json:"login"`
		} `json:"capabilities"`
	}
	if err := json.Unmarshal([]byte(body), &mapping); err != nil {
		t.Fatalf("decode mapping: %v body=%s", err, body)
	}
	if mapping.CourierGameId != "cg1" || mapping.Env != "prod" || !mapping.Capabilities.Login {
		t.Fatalf("mapping echo mismatch: %+v", mapping)
	}

	// 2. 会话入口：未登录 401；未接入 404；未开通能力 403
	if code, _ := doJSON(t, http.MethodPost, base+"/games/g1/session", "", ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous session status = %d, want 401", code)
	}
	if code, _ := doJSON(t, http.MethodPost, base+"/games/missing/session", player, ""); code != http.StatusNotFound {
		t.Fatalf("unmapped game session status = %d, want 404", code)
	}
	if code, body := doJSON(t, http.MethodPut, base+"/admin/games/g2/mapping", operator,
		`{"courierGameId":"cg2","env":"dev","partnerId":1,"capabilities":{"login":false}}`); code != http.StatusOK {
		t.Fatalf("upsert g2 mapping status = %d body=%s", code, body)
	}
	if code, body := doJSON(t, http.MethodPost, base+"/games/g2/session", player, ""); code != http.StatusForbidden {
		t.Fatalf("login-disabled game session status = %d body=%s, want 403", code, body)
	}

	// 3. 游戏会话：自动建号（register）+ 票据
	code, body = doJSON(t, http.MethodPost, base+"/games/g1/session", player, "")
	if code != http.StatusOK {
		t.Fatalf("game session status = %d body=%s", code, body)
	}
	var session struct {
		Ticket    string `json:"ticket"`
		ExpiresAt string `json:"expiresAt"`
		GameId    string `json:"gameId"`
		Env       string `json:"env"`
	}
	if err := json.Unmarshal([]byte(body), &session); err != nil {
		t.Fatalf("decode session: %v body=%s", err, body)
	}
	if session.Ticket == "" || session.GameId != "cg1" || session.Env != "prod" {
		t.Fatalf("session response mismatch: %+v", session)
	}
	if reg, _, _, _ := env.courier.counts(); reg != 1 {
		t.Fatalf("register calls = %d, want 1 (auto-bind)", reg)
	}

	// 4. 兑换：错票 404；好票一次性
	if code, _ := doJSON(t, http.MethodPost, base+"/session/redeem", "", `{"ticket":"nope"}`); code != http.StatusNotFound {
		t.Fatalf("bad ticket redeem status = %d, want 404", code)
	}
	code, body = doJSON(t, http.MethodPost, base+"/session/redeem", "",
		fmt.Sprintf(`{"ticket":%q}`, session.Ticket))
	if code != http.StatusOK {
		t.Fatalf("redeem status = %d body=%s", code, body)
	}
	var redeemed struct {
		AccessToken string `json:"accessToken"`
		Account     struct {
			Id string `json:"id"`
		} `json:"account"`
	}
	if err := json.Unmarshal([]byte(body), &redeemed); err != nil {
		t.Fatalf("decode redeem: %v body=%s", err, body)
	}
	if redeemed.AccessToken != "atk-1" || redeemed.Account.Id != "acc_reg_1" {
		t.Fatalf("redeem payload mismatch: %+v", redeemed)
	}
	if code, _ := doJSON(t, http.MethodPost, base+"/session/redeem", "",
		fmt.Sprintf(`{"ticket":%q}`, session.Ticket)); code != http.StatusNotFound {
		t.Fatalf("second redeem status = %d, want 404 (one-time)", code)
	}

	// 5. 二次进游戏：复用绑定（login，不重复 register）
	if code, _ := doJSON(t, http.MethodPost, base+"/games/g1/session", player, ""); code != http.StatusOK {
		t.Fatalf("second session status = %d", code)
	}
	reg, login, _, _ := env.courier.counts()
	if reg != 1 || login != 1 {
		t.Fatalf("after second session: register=%d login=%d, want 1/1", reg, login)
	}

	// 6. 注销同步：受理 → 绑定 PENDING_DELETE；撤销 → ACTIVE
	code, body = doJSON(t, http.MethodPost, base+"/games/g1/account/delete", player, "")
	if code != http.StatusOK {
		t.Fatalf("account delete status = %d body=%s", code, body)
	}
	var del struct {
		Status      string `json:"status"`
		CancelToken string `json:"cancelToken"`
	}
	if err := json.Unmarshal([]byte(body), &del); err != nil {
		t.Fatalf("decode delete: %v body=%s", err, body)
	}
	if del.Status != "PENDING_DELETE" || del.CancelToken != "ctok-1" {
		t.Fatalf("delete payload mismatch: %+v", del)
	}
	code, body = doJSON(t, http.MethodPost, base+"/games/g1/account/delete/cancel", player,
		`{"cancelToken":"ctok-1"}`)
	if code != http.StatusOK {
		t.Fatalf("delete cancel status = %d body=%s", code, body)
	}
	if _, _, _, cancel := env.courier.counts(); cancel != 1 {
		t.Fatalf("cancel calls = %d, want 1", cancel)
	}

	// 7. 立即删除形态（GUEST 口径/宽限期=0）：绑定直接 UNBOUND
	if code, _ := doJSON(t, http.MethodPost, base+"/games/g1/account/delete", player, ""); code != http.StatusOK {
		t.Fatalf("immediate delete status = %d", code)
	}

	// 8. scope 只走 header：全程无违规
	if len(env.courier.violations) > 0 {
		t.Fatalf("scope transport violations: %v", env.courier.violations)
	}
}
