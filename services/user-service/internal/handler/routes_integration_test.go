package handler

import (
	"database/sql"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/user-service/internal/integration"
	"github.com/tappi/tappi/services/user-service/internal/svc"
	"github.com/tappi/tappi/services/user-service/middleware"
	"github.com/tappi/tappi/services/user-service/model"
	"github.com/tappi/tappi/services/user-service/utils"

	"github.com/zeromicro/go-zero/rest"
)

const testJWTSecret = "user-handler-integration-secret"

// gameStubHandler game-catalog 契约 stub：GET /games/recommendations →
// {"games":[...]}；genres 含 "err-trigger" 时回 500 以驱动 502 envelope。
func gameStubHandler(t *testing.T, seen *map[string]string) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		if seen != nil {
			*seen = map[string]string{
				"path":   r.URL.Path,
				"userId": r.URL.Query().Get("userId"),
				"genres": r.URL.Query().Get("genres"),
				"limit":  r.URL.Query().Get("limit"),
			}
		}
		if strings.Contains(r.URL.Query().Get("genres"), "err-trigger") {
			w.WriteHeader(http.StatusInternalServerError)
			io.WriteString(w, "catalog boom")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"games":[
			{"id":"g-1","title":"Alpha RPG","cover_image":"a.png","genres":["RPG"],"platforms":["PC"],"score":9.2,"tags":["story"]},
			{"id":"g-2","title":"Beta Sim","cover_image":"b.png","genres":["SIM"],"platforms":["PC"],"score":8.1,"tags":["chill"]}
		]}`)
	}
}

// newUserHandlerServer 启动真实 go-zero rest 服务器：生产 RegisterHandlers +
// user.go 同款 server.Use 认证中间件装配；SQLite 临时文件库，预置两个用户。
func newUserHandlerServer(t *testing.T, stub http.HandlerFunc) (base string, svcCtx *svc.ServiceContext) {
	t.Helper()

	games := httptest.NewServer(stub)
	t.Cleanup(games.Close)

	db, err := sql.Open("sqlite3", filepath.Join(t.TempDir(), "users.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	userModel := model.NewUserModel(db)
	if err := userModel.CreateUsersTable(); err != nil {
		t.Fatalf("create table: %v", err)
	}
	hashed, err := utils.HashPassword("pw-alice")
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	now := time.Now()
	if _, err := db.Exec(`INSERT INTO users (username, email, password, nickname, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"alice", "alice@example.com", hashed, "Alice", now, now); err != nil {
		t.Fatalf("seed alice: %v", err)
	}
	if _, err := db.Exec(`INSERT INTO users (username, email, password, nickname, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		"bob", "bob@example.com", hashed, "Bob", now, now); err != nil {
		t.Fatalf("seed bob: %v", err)
	}

	svcCtx = &svc.ServiceContext{
		DB:                db,
		UserModel:         userModel,
		Auth:              utils.NewAuth(testJWTSecret, time.Hour),
		GameCatalogClient: integration.NewGameCatalogClient(games.URL, 2*time.Second),
	}

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	var conf rest.RestConf
	conf.Port = port
	server := rest.MustNewServer(conf)
	RegisterHandlers(server, svcCtx)
	// 与 user.go 生产装配一致：Handler 风格中间件适配为 rest.Middleware
	server.Use(func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			middleware.AuthMiddleware(svcCtx)(next).ServeHTTP(w, r)
		}
	})
	go server.Start()
	t.Cleanup(server.Stop)

	base = "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/from/you")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("test server did not become ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	return base, svcCtx
}

// do 发起请求；返回状态码与响应体 JSON（解析失败时 raw 为空）。
func do(t *testing.T, method, url, token, body string) (int, map[string]interface{}, string) {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var payload map[string]interface{}
	_ = json.Unmarshal(raw, &payload)
	return resp.StatusCode, payload, string(raw)
}

func code(t *testing.T, body map[string]interface{}) float64 {
	t.Helper()
	v, ok := body["code"].(float64)
	if !ok {
		t.Fatalf("code missing or non-numeric: %#v", body)
	}
	return v
}

// 覆盖率注记：各 handler 90% 中未覆盖的分支是 logic 返回真 error →
// httpx.ErrorCtx。除 LoginLogic 外（LoginHandler 已 100%），register/
// getuserinfo/updateuserinfo/getuserrecommendations/user 五个 logic 的所有
// 错误路径均以 ApiResponse envelope + nil error 返回，该分支无 HTTP 可达路径。

// TestRoutes_AuthGate 认证中间件：/users/* 无令牌/坏格式/坏令牌 → 真 401；
// /auth/* 与 /from/* 免认证可达。
func TestRoutes_AuthGate(t *testing.T) {
	base, _ := newUserHandlerServer(t, gameStubHandler(t, nil))

	status, body, _ := do(t, http.MethodGet, base+"/users/1", "", "")
	if status != http.StatusUnauthorized || code(t, body) != 401 {
		t.Fatalf("no token: status=%d body=%v", status, body)
	}
	if body["message"] != "缺少认证令牌" {
		t.Fatalf("message = %v", body["message"])
	}

	status, body, _ = do(t, http.MethodGet, base+"/users/1", "abc.def.ghi", "")
	if status != http.StatusUnauthorized || body["message"] != "无效的令牌" {
		t.Fatalf("bad token: status=%d body=%v", status, body)
	}

	// 免认证路径：/from/you、/auth/register 均可达（后者在业务测试覆盖）
	status, body, raw := do(t, http.MethodGet, base+"/from/you", "", "")
	if status != http.StatusOK || body["message"] != "Hello, you!" {
		t.Fatalf("/from/you: status=%d raw=%s", status, raw)
	}
}

// TestRoutes_FromOptions go-zero 路径 options=you|me 约束。
func TestRoutes_FromOptions(t *testing.T) {
	base, _ := newUserHandlerServer(t, gameStubHandler(t, nil))

	if status, _, raw := do(t, http.MethodGet, base+"/from/me", "", ""); status != 200 {
		t.Fatalf("/from/me: status=%d raw=%s", status, raw)
	}
	if status, _, _ := do(t, http.MethodGet, base+"/from/badname", "", ""); status != http.StatusBadRequest {
		t.Fatalf("/from/badname status=%d, want 400", status)
	}
}

// TestRoutes_RegisterLoginFlow 注册→登录→携带令牌访问 /users/:id 全链路。
func TestRoutes_RegisterLoginFlow(t *testing.T) {
	base, svcCtx := newUserHandlerServer(t, gameStubHandler(t, nil))

	// 注册成功（envelope HTTP 200 + code 200）
	status, body, raw := do(t, http.MethodPost, base+"/auth/register", "",
		`{"username":"carol","email":"carol@example.com","password":"pw-carol","nickname":"Carol"}`)
	if status != http.StatusOK || code(t, body) != 200 {
		t.Fatalf("register: status=%d raw=%s", status, raw)
	}

	// 重复用户名 → envelope 409
	status, body, _ = do(t, http.MethodPost, base+"/auth/register", "",
		`{"username":"carol","email":"carol2@example.com","password":"pw-carol"}`)
	if status != http.StatusOK || code(t, body) != 409 {
		t.Fatalf("dup register: status=%d body=%v", status, body)
	}

	// 空 username（字段齐全）→ logic envelope 400（HTTP 200）
	status, body, _ = do(t, http.MethodPost, base+"/auth/register", "",
		`{"username":"","email":"x@y.com","password":"pw-long"}`)
	if status != http.StatusOK || code(t, body) != 400 {
		t.Fatalf("empty register: status=%d body=%v", status, body)
	}
	// 非法 JSON → go-zero Parse 400
	if status, _, _ = do(t, http.MethodPost, base+"/auth/register", "", `{bad`); status != 400 {
		t.Fatalf("bad json register status=%d, want 400", status)
	}

	// 登录失败：错误密码
	status, _, raw = do(t, http.MethodPost, base+"/auth/login", "",
		`{"username":"alice","password":"wrong"}`)
	if status != http.StatusBadRequest || !strings.Contains(raw, "用户名或密码错误") {
		t.Fatalf("wrong password: status=%d raw=%s", status, raw)
	}
	// 登录失败：空字段
	if status, _, _ = do(t, http.MethodPost, base+"/auth/login", "", `{"password":"x"}`); status != 400 {
		t.Fatalf("empty username login status=%d, want 400", status)
	}

	// 登录成功 → token + user_info
	status, body, raw = do(t, http.MethodPost, base+"/auth/login", "",
		`{"username":"alice","password":"pw-alice"}`)
	if status != http.StatusOK || body["token"] == "" {
		t.Fatalf("login: status=%d raw=%s", status, raw)
	}
	token := body["token"].(string)
	info := body["user_info"].(map[string]interface{})
	if info["username"] != "alice" || info["nickname"] != "Alice" {
		t.Fatalf("user_info = %v", info)
	}

	// 携带登录令牌访问受保护接口
	status, body, _ = do(t, http.MethodGet, base+"/users/1", token, "")
	if status != http.StatusOK || code(t, body) != 200 {
		t.Fatalf("get user 1: status=%d body=%v", status, body)
	}
	if data := body["data"].(map[string]interface{}); data["username"] != "alice" {
		t.Fatalf("data = %v", data)
	}

	// 不存在用户 → envelope 404
	status, body, _ = do(t, http.MethodGet, base+"/users/999", token, "")
	if status != http.StatusOK || code(t, body) != 404 {
		t.Fatalf("get user 999: status=%d body=%v", status, body)
	}

	// 坏路径参数 → Parse 400
	if status, _, _ = do(t, http.MethodGet, base+"/users/abc", token, ""); status != 400 {
		t.Fatalf("bad id status=%d, want 400", status)
	}

	// 中间件签发的 token 与本地 svcCtx 密钥一致（互验）
	if _, err := svcCtx.Auth.ParseToken(token); err != nil {
		t.Fatalf("issued token not parseable: %v", err)
	}
}

// TestRoutes_UpdateUserInfo PUT /users/:id 成功、404、坏邮箱、缺字段路径。
func TestRoutes_UpdateUserInfo(t *testing.T) {
	base, svcCtx := newUserHandlerServer(t, gameStubHandler(t, nil))
	token, err := svcCtx.Auth.GenerateToken(1, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	status, body, raw := do(t, http.MethodPut, base+"/users/1", token,
		`{"email":"alice@new.com","nickname":"Alicia"}`)
	if status != http.StatusOK || code(t, body) != 200 {
		t.Fatalf("update: status=%d raw=%s", status, raw)
	}
	data := body["data"].(map[string]interface{})
	if data["nickname"] != "Alicia" || data["email"] != "alice@new.com" {
		t.Fatalf("data = %v", data)
	}

	// 缺邮箱字段（required）→ Parse 400
	if status, _, _ = do(t, http.MethodPut, base+"/users/1", token, `{}`); status != 400 {
		t.Fatalf("missing email status=%d, want 400", status)
	}

	// 坏邮箱格式 → envelope 400
	status, body, _ = do(t, http.MethodPut, base+"/users/1", token,
		`{"email":"not-an-email","nickname":"x"}`)
	if status != http.StatusOK || code(t, body) != 400 {
		t.Fatalf("bad email: status=%d body=%v", status, body)
	}

	// 用户不存在 → envelope 404
	status, body, _ = do(t, http.MethodPut, base+"/users/999", token,
		`{"email":"a@b.com","nickname":"x"}`)
	if status != http.StatusOK || code(t, body) != 404 {
		t.Fatalf("update 999: status=%d body=%v", status, body)
	}

	// 坏路径参数 → Parse 400
	if status, _, _ = do(t, http.MethodPut, base+"/users/abc", token, `{"email":"a@b.com"}`); status != 400 {
		t.Fatalf("bad id status=%d, want 400", status)
	}
}

// TestRoutes_Recommendations 推荐接口：契约透传、404、502 envelope。
func TestRoutes_Recommendations(t *testing.T) {
	var seen map[string]string
	base, svcCtx := newUserHandlerServer(t, gameStubHandler(t, &seen))
	token, err := svcCtx.Auth.GenerateToken(1, "alice")
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}

	status, body, raw := do(t, http.MethodGet, base+"/users/1/recommendations?genres=RPG,SIM&limit=3", token, "")
	if status != http.StatusOK || code(t, body) != 200 {
		t.Fatalf("recommendations: status=%d raw=%s", status, raw)
	}
	// 透传给 game-catalog 的契约参数
	if seen["path"] != "/games/recommendations" || seen["userId"] != "1" || seen["limit"] != "3" {
		t.Fatalf("upstream call = %v", seen)
	}
	if !strings.Contains(seen["genres"], "RPG") {
		t.Fatalf("genres passthrough = %q", seen["genres"])
	}
	data := body["data"].(map[string]interface{})
	games := data["recommendations"].([]interface{})
	if len(games) != 2 {
		t.Fatalf("data = %v", body["data"])
	}
	first := games[0].(map[string]interface{})
	if first["title"] != "Alpha RPG" || first["score"].(float64) != 9.2 {
		t.Fatalf("first game = %v", first)
	}
	if data["user_nickname"] != "Alice" {
		t.Fatalf("user_nickname = %v", data["user_nickname"])
	}

	// 用户不存在 → envelope 404（不触发上游调用）
	seen = nil
	status, body, _ = do(t, http.MethodGet, base+"/users/999/recommendations", token, "")
	if status != http.StatusOK || code(t, body) != 404 {
		t.Fatalf("reco 999: status=%d body=%v", status, body)
	}
	if seen != nil {
		t.Fatal("must not call game-catalog for missing user")
	}

	// 上游失败 → envelope 502
	status, body, _ = do(t, http.MethodGet, base+"/users/1/recommendations?genres=err-trigger", token, "")
	if status != http.StatusOK || code(t, body) != 502 {
		t.Fatalf("reco upstream fail: status=%d body=%v", status, body)
	}

	// limit 溢出被钳制到 1..20（20 → 上游收到 20）
	status, _, _ = do(t, http.MethodGet, base+"/users/1/recommendations?limit=99", token, "")
	if status != http.StatusOK || seen["limit"] != "20" {
		t.Fatalf("clamp limit: status=%d seen=%v", status, seen)
	}

	// 坏 limit → Parse 400
	if status, _, _ = do(t, http.MethodGet, base+"/users/1/recommendations?limit=abc", token, ""); status != 400 {
		t.Fatalf("bad limit status=%d, want 400", status)
	}
	// 匿名 → 401（受保护路由）
	if status, _, _ = do(t, http.MethodGet, base+"/users/1/recommendations", "", ""); status != 401 {
		t.Fatalf("anonymous reco status=%d, want 401", status)
	}
}
