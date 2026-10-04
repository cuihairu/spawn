package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tappi/tappi/services/api-gateway/internal/config"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// 本文件在真实 go-zero rest server 上跑生产的 RegisterHandlers 装配
// （svc.NewServiceContext + 真实 integration 客户端），上游用 httptest stub，
// 断言的是网关与 user-service / game-catalog 的真实线上契约，而不是 mock 掉逻辑。
// content/community 反代路由不在此覆盖范围：它们走 internal/proxy，已有独立测试。

// upstreamCall 记录网关实际发往上游的请求（用于契约断言）。
type upstreamCall struct {
	method      string
	path        string
	query       string
	auth        string
	contentType string
	body        string
}

// callLog 记录上游收到的调用；互斥保护保证 -race 干净。
type callLog struct {
	mu    sync.Mutex
	calls []upstreamCall
}

func (l *callLog) add(c upstreamCall) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = append(l.calls, c)
}

func (l *callLog) last(t *testing.T) upstreamCall {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(l.calls) == 0 {
		t.Fatal("上游没有收到任何调用")
	}
	return l.calls[len(l.calls)-1]
}

func (l *callLog) count(t *testing.T) int {
	t.Helper()
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.calls)
}

func (l *callLog) reset() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.calls = nil
}

// upstreamStubs 持有三个真实上游 stub 及其调用记录。
type upstreamStubs struct {
	user         *callLog
	game         *callLog
	community    *callLog
	userURL      string
	gameURL      string
	communityURL string
}

// newUpstreamStubs 启动上游 stub，故障开关只依赖请求本身（无共享可变状态，-race 安全）：
//   - user-service：username=ghost → 401；userId=99 → 500
//   - game-catalog：limit=9 → 502
//   - community：POST /api/v1/posts/:id/share → 201；GET /api/v1/posts/0 → 404
func newUpstreamStubs(t *testing.T) *upstreamStubs {
	t.Helper()

	stubs := &upstreamStubs{user: &callLog{}, game: &callLog{}, community: &callLog{}}

	userStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		stubs.user.add(upstreamCall{
			method:      r.Method,
			path:        r.URL.Path,
			query:       r.URL.RawQuery,
			auth:        r.Header.Get("Authorization"),
			contentType: r.Header.Get("Content-Type"),
			body:        string(body),
		})
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/auth/login":
			if strings.Contains(string(body), `"username":"ghost"`) {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"用户名或密码错误"}`))
				return
			}
			_, _ = w.Write([]byte(`{"token":"gw-token","user_info":{"id":7,"username":"alice"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/users/99/recommendations":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/recommendations"):
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"games":[{"id":"g-1","title":"Alpha RPG"}]}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("no such upstream endpoint"))
		}
	}))
	t.Cleanup(userStub.Close)

	gameStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stubs.game.add(upstreamCall{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
		})
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path != "/games/featured" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.URL.Query().Get("limit") == "9" {
			w.WriteHeader(http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"games":[{"id":"g-1","title":"Alpha RPG"},{"id":"g-2","title":"Beta FPS"}],"total":2}`))
	}))
	t.Cleanup(gameStub.Close)

	communityStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stubs.community.add(upstreamCall{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
		})
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/posts/999":
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":404,"message":"帖子不存在"}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/posts/"):
			_, _ = w.Write([]byte(`{"post":{"id":1,"title":"分享测试帖","content":"这是一段用于分享卡摘要的正文","author_name":"alice"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("no such upstream endpoint"))
		}
	}))
	t.Cleanup(communityStub.Close)

	stubs.userURL = userStub.URL
	stubs.gameURL = gameStub.URL
	stubs.communityURL = communityStub.URL
	return stubs
}

// newGatewayServer 按生产装配启动真实网关：配置里的上游地址指向 stub，
// 超时给 2s；就绪探测用的 /games/featured 调用会在返回后清空调用记录。
func newGatewayServer(t *testing.T, stubs *upstreamStubs) string {
	t.Helper()

	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	var c config.Config
	c.Port = port
	c.Upstreams.UserService.BaseURL = stubs.userURL
	c.Upstreams.UserService.Timeout = 2000
	c.Upstreams.GameCatalog.BaseURL = stubs.gameURL
	c.Upstreams.GameCatalog.Timeout = 2000
	c.Upstreams.Community.BaseURL = stubs.communityURL
	c.Upstreams.Community.Timeout = 2000

	svcCtx := svc.NewServiceContext(c)
	server := rest.MustNewServer(c.RestConf)
	RegisterHandlers(server, svcCtx)
	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/games/featured")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("gateway test server did not become ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	stubs.user.reset()
	stubs.game.reset()
	stubs.community.reset()
	return base
}

// httpResp 是网关响应的统一视图：错误分支经 httpx.ErrorCtx 走 http.Error，
// 响应体是纯文本而非 JSON，所以两种都收。
type httpResp struct {
	status int
	body   string
	json   map[string]interface{}
}

func call(t *testing.T, method, url, token, body string) httpResp {
	t.Helper()
	var reader io.Reader
	if body != "" {
		reader = bytes.NewReader([]byte(body))
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", token)
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
	out := httpResp{status: resp.StatusCode, body: string(raw)}
	_ = json.Unmarshal(raw, &out.json)
	return out
}

// object 取响应 JSON 里的对象字段。
func object(t *testing.T, m map[string]interface{}, key string) map[string]interface{} {
	t.Helper()
	v, ok := m[key].(map[string]interface{})
	if !ok {
		t.Fatalf("field %q 不是对象: %#v", key, m[key])
	}
	return v
}

// list 取响应 JSON 里的数组字段。
func list(t *testing.T, m map[string]interface{}, key string) []interface{} {
	t.Helper()
	v, ok := m[key].([]interface{})
	if !ok {
		t.Fatalf("field %q 不是数组: %#v", key, m[key])
	}
	return v
}

// --- POST /auth/login ---

func TestRoutes_Login(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 成功：令牌与用户信息透传，同时校验网关→user-service 的线上契约。
	got := call(t, http.MethodPost, base+"/auth/login", "",
		`{"username":"alice","password":"pw"}`)
	if got.status != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", got.status, got.body)
	}
	if got.json["token"] != "gw-token" {
		t.Fatalf("token = %#v", got.json["token"])
	}
	if userInfo := object(t, got.json, "user_info"); userInfo["username"] != "alice" {
		t.Fatalf("user_info = %#v", userInfo)
	}

	sent := stubs.user.last(t)
	if sent.method != http.MethodPost || sent.path != "/auth/login" {
		t.Fatalf("upstream request = %s %s, want POST /auth/login", sent.method, sent.path)
	}
	if !strings.HasPrefix(sent.contentType, "application/json") {
		t.Fatalf("content-type = %q", sent.contentType)
	}
	var payload struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.Unmarshal([]byte(sent.body), &payload); err != nil {
		t.Fatalf("上游请求体不是 LoginPayload JSON: %q (%v)", sent.body, err)
	}
	if payload.Username != "alice" || payload.Password != "pw" {
		t.Fatalf("上游请求体 = %q", sent.body)
	}
}

func TestRoutes_LoginUpstreamFailure(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 上游 401 → 客户端报 login failed → handler 走 httpx.ErrorCtx（HTTP 400）
	got := call(t, http.MethodPost, base+"/auth/login", "",
		`{"username":"ghost","password":"pw"}`)
	if got.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", got.status)
	}
	if !strings.Contains(got.body, "login failed") ||
		!strings.Contains(got.body, "用户名或密码错误") {
		t.Fatalf("body = %q, want login failed + 上游原文", got.body)
	}
}

func TestRoutes_LoginParseErrors(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 非法 JSON → httpx.Parse 失败，且不得打到上游。
	got := call(t, http.MethodPost, base+"/auth/login", "", `{bad json`)
	if got.status != http.StatusBadRequest {
		t.Fatalf("malformed body status = %d, want 400", got.status)
	}
	if n := stubs.user.count(t); n != 0 {
		t.Fatalf("上游被调用 %d 次，parse 失败时不应有调用", n)
	}

	// 缺少必填字段（username/password 无 optional 标签）同样 400。
	got = call(t, http.MethodPost, base+"/auth/login", "", `{}`)
	if got.status != http.StatusBadRequest {
		t.Fatalf("empty object status = %d, want 400 (body=%s)", got.status, got.body)
	}
	if n := stubs.user.count(t); n != 0 {
		t.Fatalf("上游被调用 %d 次，缺字段时不应有调用", n)
	}
}

// --- GET /games/featured ---

func TestRoutes_FeaturedGames(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/games/featured", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("featured status = %d, body = %s", got.status, got.body)
	}
	games := list(t, got.json, "games")
	if len(games) != 2 {
		t.Fatalf("games = %#v", games)
	}
	first, _ := games[0].(map[string]interface{})
	if first["id"] != "g-1" || first["title"] != "Alpha RPG" {
		t.Fatalf("first game = %#v", first)
	}

	// FeaturedRequest 的 limit 带 default=6：未传时 logic 兜底为 6。
	if q := stubs.game.last(t).query; q != "limit=6" {
		t.Fatalf("upstream query = %q, want limit=6", q)
	}
}

func TestRoutes_FeaturedGamesLimitPassthrough(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/games/featured?limit=3", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("featured status = %d, body = %s", got.status, got.body)
	}
	sent := stubs.game.last(t)
	if sent.method != http.MethodGet || sent.path != "/games/featured" {
		t.Fatalf("upstream request = %s %s", sent.method, sent.path)
	}
	if sent.query != "limit=3" {
		t.Fatalf("upstream query = %q, want limit=3", sent.query)
	}
}

func TestRoutes_FeaturedGamesUpstreamFailure(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/games/featured?limit=9", "", "")
	if got.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", got.status)
	}
	if !strings.Contains(got.body, "game catalog status 502") {
		t.Fatalf("body = %q, want 上游 502 说明", got.body)
	}
}

func TestRoutes_FeaturedGamesParseError(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/games/featured?limit=abc", "", "")
	if got.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", got.status, got.body)
	}
	if n := stubs.game.count(t); n != 0 {
		t.Fatalf("上游被调用 %d 次，parse 失败时不应有调用", n)
	}
}

// --- GET /users/:id/recommendations ---

func TestRoutes_UserRecommendations(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/users/7/recommendations?limit=5&genres=RPG", "Bearer gw-token", "")
	if got.status != http.StatusOK {
		t.Fatalf("status = %d, body = %s", got.status, got.body)
	}
	if got.json["code"] != float64(200) || got.json["message"] != "ok" {
		t.Fatalf("envelope = %#v", got.json)
	}
	data := object(t, got.json, "data")
	if games := list(t, data, "games"); len(games) != 1 {
		t.Fatalf("data.games = %#v", games)
	}

	// 上游契约：路径含 path 参数 id，query 只带显式参数，Bearer 透传。
	sent := stubs.user.last(t)
	if sent.method != http.MethodGet || sent.path != "/users/7/recommendations" {
		t.Fatalf("upstream request = %s %s", sent.method, sent.path)
	}
	if sent.query != "genres=RPG&limit=5" {
		t.Fatalf("upstream query = %q", sent.query)
	}
	if sent.auth != "Bearer gw-token" {
		t.Fatalf("upstream authorization = %q", sent.auth)
	}
}

func TestRoutes_UserRecommendationsTokenHandling(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	cases := []struct {
		name string
		auth string
		want string
	}{
		{"无 Authorization 头时不下发", "", ""},
		{"Bearer 前缀后需去空白", "Bearer   padded  ", "Bearer padded"},
		{"无 Bearer 前缀时整段当令牌", "raw-token", "Bearer raw-token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := call(t, http.MethodGet, base+"/users/7/recommendations", tc.auth, ""); got.status != http.StatusOK {
				t.Fatalf("status = %d, body = %s", got.status, got.body)
			}
			if auth := stubs.user.last(t).auth; auth != tc.want {
				t.Fatalf("upstream authorization = %q, want %q", auth, tc.want)
			}
		})
	}

	// 无 query 时 limit/genres 不得出现在上游查询串里（0/空值不传）。
	call(t, http.MethodGet, base+"/users/7/recommendations", "", "")
	if q := stubs.user.last(t).query; q != "" {
		t.Fatalf("upstream query = %q, want empty", q)
	}
}

func TestRoutes_UserRecommendationsUpstreamFailure(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/users/99/recommendations", "", "")
	if got.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", got.status)
	}
	if !strings.Contains(got.body, "recommendation failed: boom") {
		t.Fatalf("body = %q, want 上游原文", got.body)
	}
}

func TestRoutes_UserRecommendationsParseError(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 路径参数 id 非数字 → httpx.Parse 失败，且不得打到上游。
	got := call(t, http.MethodGet, base+"/users/abc/recommendations", "", "")
	if got.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", got.status, got.body)
	}
	if n := stubs.user.count(t); n != 0 {
		t.Fatalf("上游被调用 %d 次，parse 失败时不应有调用", n)
	}
}

// --- GET /s/p/:id 分享卡 ---

func TestRoutes_ShareLink(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/s/p/1", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("share link status = %d, body = %s", got.status, got.body)
	}
	for _, want := range []string{
		"<!DOCTYPE html>", "分享测试帖", "og:title", "og:type", "og:url",
		"spawn:///post/1", "/community/posts/1", "在 spawn 中打开", "在浏览器中查看",
		// html/template 不做 Sprintf 转义：%% 会原样渲染成非法 CSS，锁死这个坑
		"width: 88%",
	} {
		if !strings.Contains(got.body, want) {
			t.Fatalf("share card 缺 %q, body = %s", want, got.body)
		}
	}
	if !strings.Contains(got.body, "这是一段用于分享卡摘要的正文") {
		t.Fatalf("share card 缺正文摘要, body = %s", got.body)
	}

	// 上游契约：只 GET /api/v1/posts/:id，不携带任何 query。
	sent := stubs.community.last(t)
	if sent.method != http.MethodGet || sent.path != "/api/v1/posts/1" {
		t.Fatalf("upstream request = %s %s", sent.method, sent.path)
	}
	if sent.query != "" {
		t.Fatalf("upstream query = %q, want empty", sent.query)
	}
}

func TestRoutes_ShareLinkFallback(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 帖子不存在 → 通用卡片兜底，但深链入口保持可用。
	got := call(t, http.MethodGet, base+"/s/p/999", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("fallback status = %d, body = %s", got.status, got.body)
	}
	if !strings.Contains(got.body, "在 spawn 里看看这篇帖子") {
		t.Fatalf("fallback title 缺失, body = %s", got.body)
	}
	if !strings.Contains(got.body, "spawn:///post/999") || !strings.Contains(got.body, "/community/posts/999") {
		t.Fatalf("fallback 深链缺失, body = %s", got.body)
	}
}

func TestRoutes_ShareLinkBadID(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 路径参数非法 → 400，且不得打到上游。
	got := call(t, http.MethodGet, base+"/s/p/abc", "", "")
	if got.status != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", got.status, got.body)
	}
	if n := stubs.community.count(t); n != 0 {
		t.Fatalf("上游被调用 %d 次，parse 失败时不应有调用", n)
	}
}

// --- 路由注册本身 ---

// TestRoutes_RegistrationIsExact 校验 RegisterHandlers 注册的路由集合恰好是四条：
// 路径存在但方法不符返回 405，路径不存在返回 404，都不会被某条路由兜住，
// 也不会触达上游，证明没有隐式兜底路由。
func TestRoutes_RegistrationIsExact(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	cases := []struct {
		name   string
		method string
		path   string
		want   int
	}{
		{"登录只注册 POST", http.MethodGet, "/auth/login", http.StatusMethodNotAllowed},
		{"精选只注册 GET", http.MethodPost, "/games/featured", http.StatusMethodNotAllowed},
		{"推荐只注册 GET", http.MethodPost, "/users/7/recommendations", http.StatusMethodNotAllowed},
		{"分享卡只注册 GET", http.MethodPost, "/s/p/1", http.StatusMethodNotAllowed},
		{"未注册路径", http.MethodGet, "/games", http.StatusNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := call(t, tc.method, base+tc.path, "", "")
			if got.status != tc.want {
				t.Fatalf("%s %s status = %d, want %d (body=%s)", tc.method, tc.path, got.status, tc.want, got.body)
			}
		})
	}
	if n := stubs.user.count(t) + stubs.game.count(t); n != 0 {
		t.Fatalf("404 路径不应触达上游，实际 %d 次", n)
	}
}
