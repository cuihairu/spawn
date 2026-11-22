package logic

// logic 补漏分支共享装配：真实 sqlite 库 + 可切换响应路由的 Courier /
// game-catalog / user-service / payments 四个上游存根。默认路由回合法响应，
// 子测试用 set() 覆盖指定路径/状态码，或用只读/关闭库变体触发写/读失败臂。

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tappi/tappi/services/courier-adapter/internal/config"
	"github.com/tappi/tappi/services/courier-adapter/internal/svc"
	"github.com/tappi/tappi/services/courier-adapter/internal/ticket"
	"github.com/tappi/tappi/services/courier-adapter/model"
	"github.com/tappi/tappi/services/courier-adapter/utils"
)

// stubHTTP 可切换响应的上游存根。
type stubHTTP struct {
	mu  sync.Mutex
	fn  func(*http.Request) (int, string)
	srv *httptest.Server
}

func newStubHTTP(t *testing.T, fn func(*http.Request) (int, string)) *stubHTTP {
	t.Helper()
	s := &stubHTTP{fn: fn}
	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		f := s.fn
		s.mu.Unlock()
		code, body := f(r)
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(s.srv.Close)
	return s
}

func (s *stubHTTP) url() string { return s.srv.URL }

func (s *stubHTTP) set(fn func(*http.Request) (int, string)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.fn = fn
}

// defaultCourier 合法响应的 Courier 路由。
func defaultCourier(r *http.Request) (int, string) {
	switch {
	case strings.HasSuffix(r.URL.Path, "/messages"):
		return http.StatusOK, `{"data":{"senderType":"PLAYER","body":"hi","createdAt":"c"}}`
	case strings.HasSuffix(r.URL.Path, "/identity/register"),
		strings.HasSuffix(r.URL.Path, "/identity/login"):
		return http.StatusOK, `{"data":{"account":{"id":"acc_1","type":"EMAIL","status":"ACTIVE"},"accessToken":"atk-1","accessExpiresAt":"e","refreshToken":"rtk-1","refreshExpiresAt":"re"}}`
	case strings.HasSuffix(r.URL.Path, "/identity/account/delete/cancel"):
		return http.StatusOK, `{"data":{}}`
	case strings.HasSuffix(r.URL.Path, "/identity/account/delete"):
		return http.StatusOK, `{"data":{"status":"PENDING_DELETE","cancelToken":"ct-1","pendingDelete":true}}`
	case strings.HasSuffix(r.URL.Path, "/v1/support/tickets") && r.Method == http.MethodPost:
		return http.StatusOK, `{"data":{"id":"tk1","title":"t","status":"OPEN","createdAt":"c","updatedAt":"u"}}`
	case strings.HasSuffix(r.URL.Path, "/v1/support/tickets") && r.Method == http.MethodGet:
		return http.StatusOK, `{"data":{"items":[],"nextCursor":""}}`
	case strings.Contains(r.URL.Path, "/v1/support/tickets/") && r.Method == http.MethodGet:
		return http.StatusOK, `{"data":{"ticket":{"id":"tk1","status":"OPEN"},"messages":[]}}`
	case strings.Contains(r.URL.Path, "/v1/support/faq"):
		return http.StatusOK, `{"data":{"items":[{"id":"f1","question":"q","answer":"a","keywords":["k"]}]}}`
	default:
		return http.StatusNotFound, `{"error":{"code":"COMMON_NOT_FOUND","message":"no"}}`
	}
}

// armsEnv 装配四存根 + 完整 ServiceContext（可写库）。
type armsEnv struct {
	t        *testing.T
	path     string
	cfg      config.Config
	courier  *stubHTTP
	catalog  *stubHTTP
	user     *stubHTTP
	payments *stubHTTP
	base     *svc.ServiceContext
}

func newArmsEnv(t *testing.T) *armsEnv {
	t.Helper()
	e := &armsEnv{
		t:       t,
		path:    t.TempDir() + "/logic.db",
		courier: newStubHTTP(t, defaultCourier),
		catalog: newStubHTTP(t, func(r *http.Request) (int, string) { return http.StatusOK, `{"role":"operator","partnerId":1}` }),
		user: newStubHTTP(t, func(r *http.Request) (int, string) {
			return http.StatusOK, `{"code":200,"data":{"email":"p@example.com"}}`
		}),
		payments: newStubHTTP(t, func(r *http.Request) (int, string) {
			return http.StatusOK, `{"order":{"game_id":"g1","account_id":"77"}}`
		}),
	}
	e.cfg.Auth.JWTSecret = "logic-test-secret"
	e.cfg.MySQL.DataSource = "file:" + e.path
	e.cfg.Credential.Key = "logic-test-cred-key"
	e.cfg.Courier.BaseURL = e.courier.url()
	e.cfg.Courier.Timeout = 2000
	e.cfg.GameCatalog.BaseURL = e.catalog.url()
	e.cfg.GameCatalog.Timeout = 2000
	e.cfg.UserService.BaseURL = e.user.url()
	e.cfg.UserService.Timeout = 2000
	e.cfg.Payments.BaseURL = e.payments.url()
	e.cfg.Payments.InternalToken = "internal-tok"
	e.cfg.Payments.Timeout = 2000
	e.cfg.Ticket.TTLSeconds = 120
	e.base = svc.NewServiceContext(e.cfg)
	t.Cleanup(func() { _ = e.base.DB.Close() })
	return e
}

// seedMapping 写入一张已接入映射（能力默认全开）。
func (e *armsEnv) seedMapping(id, courierGameID, env string, caps model.Capabilities) *model.GameMapping {
	e.t.Helper()
	m := &model.GameMapping{SpawnGameId: id, CourierGameId: courierGameID, Env: env, PartnerId: 1, Capabilities: caps}
	if err := e.base.Mappings.Upsert(m); err != nil {
		e.t.Fatalf("seed mapping: %v", err)
	}
	return m
}

// seedBinding 写入一条已绑定记录（密文由 key 加密）。
func (e *armsEnv) seedBinding(userID int64, courierAccountID string) *model.Binding {
	e.t.Helper()
	enc, err := model.EncryptSecret(e.cfg.Credential.Key, "pw-123456")
	if err != nil {
		e.t.Fatalf("encrypt: %v", err)
	}
	b := &model.Binding{SpawnUserID: userID, CourierAccountID: courierAccountID, CourierEmail: "p@example.com", SecretEnc: enc, Status: model.BindingActive, BindMethod: "auto"}
	if err := e.base.Bindings.Create(b); err != nil {
		e.t.Fatalf("seed binding: %v", err)
	}
	return b
}

// roSvc 同一文件只读重开（读成功、写失败）的 ServiceContext。
func (e *armsEnv) roSvc() *svc.ServiceContext {
	e.t.Helper()
	db, err := sql.Open("sqlite3", "file:"+e.path+"?mode=ro")
	if err != nil {
		e.t.Fatalf("open ro: %v", err)
	}
	e.t.Cleanup(func() { _ = db.Close() })
	return e.withDB(db)
}

// closedSvc 已关闭连接的 ServiceContext（首查即败）。
func (e *armsEnv) closedSvc() *svc.ServiceContext {
	e.t.Helper()
	db, err := sql.Open("sqlite3", "file:"+e.path)
	if err != nil {
		e.t.Fatalf("open closed: %v", err)
	}
	if err := db.Close(); err != nil {
		e.t.Fatalf("close: %v", err)
	}
	return e.withDB(db)
}

// courierLoginOkSupport500 登录/注册成功但工单接口 500 的 Courier 路由。
func courierLoginOkSupport500(r *http.Request) (int, string) {
	if containsPath(r, "/identity/login") || containsPath(r, "/identity/register") {
		return defaultCourier(r)
	}
	return http.StatusInternalServerError, `{"error":{"code":"E","message":"boom"}}`
}

// containsPath 判断请求路径是否含子串（存根路由分发用）。
func containsPath(r *http.Request, s string) bool { return strings.Contains(r.URL.Path, s) }

// restoreCourier 恢复健康 Courier 路由。
func (e *armsEnv) restoreCourier() { e.courier.set(defaultCourier) }

// dbExec 在基础可写库上执行原始 SQL（脏行/改列用）。
func dbExec(e *armsEnv, q string) (sql.Result, error) { return e.base.DB.Exec(q) }

// withDB 用给定库装配 ServiceContext（复用同一 Courier 客户端与配置）。
func (e *armsEnv) withDB(db *sql.DB) *svc.ServiceContext {
	return &svc.ServiceContext{
		Config:   e.cfg,
		Auth:     utils.NewAuth(e.cfg.Auth.JWTSecret),
		Courier:  e.base.Courier,
		Mappings: model.NewMappingModel(db),
		Bindings: model.NewBindingModel(db),
		Audits:   model.NewAuditModel(db),
		Tickets:  ticket.NewStore(time.Minute),
		DB:       db,
	}
}
