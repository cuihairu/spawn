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

	"github.com/tappi/tappi/services/api-gateway/internal/community"
	"github.com/tappi/tappi/services/api-gateway/internal/config"
	"github.com/tappi/tappi/services/api-gateway/internal/content"
	"github.com/tappi/tappi/services/api-gateway/internal/datapanel"
	"github.com/tappi/tappi/services/api-gateway/internal/games"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/tappi/tappi/services/api-gateway/internal/users"

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

// upstreamStubs 持有真实上游 stub 及其调用记录（content 仅作地址占位，无调用记录）。
type upstreamStubs struct {
	user         *callLog
	game         *callLog
	community    *callLog
	dataPanel    *callLog
	userURL      string
	gameURL      string
	communityURL string
	contentURL   string
	dataPanelURL string
}

// newUpstreamStubs 启动上游 stub，故障开关只依赖请求本身（无共享可变状态，-race 安全）：
//   - user-service：username=ghost → 401；userId=99 → 500
//   - game-catalog：limit=9 → 502
//   - community：POST /api/v1/posts/:id/share → 201；GET /api/v1/posts/0 → 404
func newUpstreamStubs(t *testing.T) *upstreamStubs {
	t.Helper()

	stubs := &upstreamStubs{user: &callLog{}, game: &callLog{}, community: &callLog{}, dataPanel: &callLog{}}

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
		case r.Method == http.MethodPost && r.URL.Path == "/auth/register":
			if strings.Contains(string(body), `"username":"taken"`) {
				// 业务失败走 HTTP 200 + 信封 code 409，客户端语义不变
				_, _ = w.Write([]byte(`{"code":409,"message":"用户名已被占用"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"user_id":7}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/users/99/recommendations":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("boom"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/recommendations"):
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"games":[{"id":"g-1","title":"Alpha RPG"}]}}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/users/"):
			// 资料详情（BFF 反代）：放在 recommendations 之后避免遮蔽
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"code":401,"message":"missing authorization token"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":{"id":7,"username":"alice","email":"a@x.dev"}}`))
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
		switch {
		case r.URL.Path == "/games/featured":
			if r.URL.Query().Get("limit") == "9" {
				w.WriteHeader(http.StatusBadGateway)
				return
			}
			// description 等大字段故意带上：HomeFeed 聚合须裁剪掉，不进响应
			_, _ = w.Write([]byte(`{"games":[{"id":"g-1","title":"Alpha RPG","description":"不该下发","score":9.5,"genres":["RPG"],"platforms":["PC"],"cover_image":"c.png"},{"id":"g-2","title":"Beta FPS","description":"不该下发"}],"total":2}`))
		case r.URL.Path == "/games":
			_, _ = w.Write([]byte(`{"games":[{"id":"g-1","title":"Alpha RPG"}],"total":1,"limit":20,"offset":0}`))
		case strings.HasPrefix(r.URL.Path, "/games/"):
			if strings.HasSuffix(r.URL.Path, "/nope") {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"code":404,"message":"游戏不存在"}`))
				return
			}
			_, _ = w.Write([]byte(`{"game":{"id":"g-1","title":"Alpha RPG"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("no such upstream endpoint"))
		}
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
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/posts/hot":
			// 必须放在 /posts/ prefix 之前（prefix 会匹配 /posts/hot）；正文 >60 字用于断言截断摘要
			_, _ = w.Write([]byte(`{"posts":[{"id":11,"topic_id":2,"author_id":7,"author_name":"alice",` +
				`"title":"热帖甲","content":"` + strings.Repeat("正文内容", 30) + `","images":["x.png"],` +
				`"like_count":12,"comment_count":3,"created_at":"2026-10-04T10:00:00Z"}],"total":1}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/topics":
			_, _ = w.Write([]byte(`{"topics":[{"id":2,"name":"星陨圈","description":"不应下发",` +
				`"post_count":30,"follower_count":88,"is_official":true}],"total":1}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/posts/"):
			_, _ = w.Write([]byte(`{"post":{"id":1,"title":"分享测试帖","content":"这是一段用于分享卡摘要的正文","author_name":"alice"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("no such upstream endpoint"))
		}
	}))
	t.Cleanup(communityStub.Close)

	contentStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// content 反代不在本文件断言范围（有独立包测试）；HomeFeed 聚合会真实打到
		// /api/v1/guides，其余路径 404 保证偶发误触立刻暴露。
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/api/v1/guides":
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","data":[{"id":5,"game_id":"g-1",` +
				`"game_title":"Alpha RPG","title":"全收集攻略","summary":"官方摘要","content":"长正文不应下发",` +
				`"author_name":"bob","likes":9,"views":120,"created_at":"2026-10-01T08:00:00Z"}],"total":1,"page":1}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("not stubbed"))
		}
	}))
	t.Cleanup(contentStub.Close)

	dataPanelStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		stubs.dataPanel.add(upstreamCall{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			auth:   r.Header.Get("Authorization"),
			body: func() string {
				b, _ := io.ReadAll(r.Body)
				return string(b)
			}(),
		})
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/stats/users/1001/summary":
			_, _ = w.Write([]byte(`{"summary":{"user_id":1001,"total_matches":42,"total_wins":25,` +
				`"win_rate":0.5952,"total_kills":310,"total_deaths":180,"total_assists":95,` +
				`"kd":1.7222,"total_score":12040,"total_rank_points":860,"game_count":2,` +
				`"last_played_at":"2026-10-05T21:30:00Z"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/stats/users/1001/games":
			_, _ = w.Write([]byte(`{"games":[{"game_id":"game-valorant","game_title":"Valorant",` +
				`"matches":87,"wins":44,"win_rate":0.5057,"kills":620,"deaths":510,"assists":240,` +
				`"kd":1.2157,"score":18900,"rank_points":1120,"last_played_at":"2026-10-05T21:30:00Z",` +
				`"created_at":"2026-10-09T00:00:00Z","updated_at":"2026-10-09T00:00:00Z"}],"total":1}`))
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/stats/users/1001/games/"):
			if strings.HasSuffix(r.URL.Path, "/nope") {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"code":404,"message":"stat not found"}`))
				return
			}
			_, _ = w.Write([]byte(`{"stat":{"game_id":"game-valorant","game_title":"Valorant","matches":87}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/stats/records":
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				_, _ = w.Write([]byte(`{"message":"missing or invalid token"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":200,"message":"ok","stat":{"game_id":"game-valorant","matches":88}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte("no such upstream endpoint"))
		}
	}))
	t.Cleanup(dataPanelStub.Close)

	stubs.userURL = userStub.URL
	stubs.gameURL = gameStub.URL
	stubs.communityURL = communityStub.URL
	stubs.contentURL = contentStub.URL
	stubs.dataPanelURL = dataPanelStub.URL
	return stubs
}

// newGatewayServer 按生产装配启动真实网关：配置里的上游地址指向 stub，
// 超时给 2s；就绪探测用的 /games/featured 调用会在返回后清空调用记录。
// 反代路由（content/community/users/games）与生产 main.go 同批注册，
// 保证自有 handler 与反代共存的路由冲突在此暴露。
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
	c.Upstreams.Content.BaseURL = stubs.contentURL
	c.Upstreams.Content.Timeout = 2000
	c.Upstreams.DataPanel.BaseURL = stubs.dataPanelURL
	c.Upstreams.DataPanel.Timeout = 2000

	svcCtx := svc.NewServiceContext(c)
	server := rest.MustNewServer(c.RestConf)
	RegisterHandlers(server, svcCtx)
	content.RegisterContentProxyRoutes(server, svcCtx)
	community.RegisterCommunityProxyRoutes(server, svcCtx)
	users.RegisterUserProxyRoutes(server, svcCtx)
	games.RegisterGameProxyRoutes(server, svcCtx)
	datapanel.RegisterDataPanelProxyRoutes(server, svcCtx)
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
	stubs.dataPanel.reset()
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

// --- GET /home/feed（BFF 第二阶段：列表聚合 + 字段裁剪） ---

func TestRoutes_HomeFeed(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodGet, base+"/home/feed?limit=4", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("home feed status = %d, body = %s", got.status, got.body)
	}

	// 精选游戏：裁剪后的字段在、大字段不在。
	games := list(t, got.json, "featured_games")
	if len(games) != 2 {
		t.Fatalf("featured_games = %#v", games)
	}
	first, _ := games[0].(map[string]interface{})
	if first["id"] != "g-1" || first["title"] != "Alpha RPG" || first["score"] != float64(9.5) {
		t.Fatalf("first game = %#v", first)
	}
	for _, dropped := range []string{"description", "developer", "publisher", "tags"} {
		if _, keep := first[dropped]; keep {
			t.Fatalf("featured_games 裁剪失败：%q 不应下发", dropped)
		}
	}

	// 热帖：正文不下发，摘要截断到 60 rune + …；计数类字段保留。
	posts := list(t, got.json, "hot_posts")
	if len(posts) != 1 {
		t.Fatalf("hot_posts = %#v", posts)
	}
	post, _ := posts[0].(map[string]interface{})
	if _, keep := post["content"]; keep {
		t.Fatal("hot_posts 裁剪失败：content 不应下发")
	}
	summary, _ := post["summary"].(string)
	if got := []rune(summary); len(got) != 61 || string(got[len(got)-1]) != "…" {
		t.Fatalf("summary = %q (%d runes), want 60 runes + …", summary, len(got))
	}
	if post["id"] != float64(11) || post["like_count"] != float64(12) || post["comment_count"] != float64(3) {
		t.Fatalf("post = %#v", post)
	}

	// 话题：description 不下发。
	topics := list(t, got.json, "topics")
	if len(topics) != 1 {
		t.Fatalf("topics = %#v", topics)
	}
	topic, _ := topics[0].(map[string]interface{})
	if _, keep := topic["description"]; keep {
		t.Fatal("topics 裁剪失败：description 不应下发")
	}
	if topic["name"] != "星陨圈" || topic["is_official"] != true {
		t.Fatalf("topic = %#v", topic)
	}

	// 攻略：content 不下发，summary 用官方摘要。
	guides := list(t, got.json, "guides")
	if len(guides) != 1 {
		t.Fatalf("guides = %#v", guides)
	}
	guide, _ := guides[0].(map[string]interface{})
	if _, keep := guide["content"]; keep {
		t.Fatal("guides 裁剪失败：content 不应下发")
	}
	if guide["summary"] != "官方摘要" || guide["title"] != "全收集攻略" {
		t.Fatalf("guide = %#v", guide)
	}

	// 正常路径 degraded 必须是空数组。
	if degraded := list(t, got.json, "degraded"); len(degraded) != 0 {
		t.Fatalf("degraded = %#v, want empty", degraded)
	}

	// 上游契约：game featured 带 limit=4；community hot+topics 各一次。
	sent := stubs.game.last(t)
	if sent.path != "/games/featured" || sent.query != "limit=4" {
		t.Fatalf("game upstream = %+v", sent)
	}
	seen := map[string]int{}
	for _, c := range stubs.community.calls {
		seen[c.path]++
	}
	if seen["/api/v1/posts/hot"] != 1 || seen["/api/v1/topics"] != 1 {
		t.Fatalf("community upstream calls = %v", seen)
	}
}

func TestRoutes_HomeFeedDegradesPerGroup(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// limit=9 触发 game 上游 502 → 仅 featured_games 降级，其余组照常返回。
	got := call(t, http.MethodGet, base+"/home/feed?limit=9", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("degraded status = %d, want 200 (body=%s)", got.status, got.body)
	}
	if games := list(t, got.json, "featured_games"); len(games) != 0 {
		t.Fatalf("featured_games = %#v, want empty", games)
	}
	degraded := list(t, got.json, "degraded")
	if len(degraded) != 1 || degraded[0] != "featured_games" {
		t.Fatalf("degraded = %#v, want [featured_games]", degraded)
	}
	// 其余三组仍有数据。
	if posts := list(t, got.json, "hot_posts"); len(posts) != 1 {
		t.Fatalf("hot_posts = %#v", posts)
	}
	if topics := list(t, got.json, "topics"); len(topics) != 1 {
		t.Fatalf("topics = %#v", topics)
	}
	if guides := list(t, got.json, "guides"); len(guides) != 1 {
		t.Fatalf("guides = %#v", guides)
	}
}

func TestRoutes_HomeFeedRouteBoundary(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 只注册 GET：POST → 405 且不触达上游；limit 非法 → 400。
	got := call(t, http.MethodPost, base+"/home/feed", "", `{}`)
	if got.status != http.StatusMethodNotAllowed {
		t.Fatalf("POST /home/feed status = %d, want 405", got.status)
	}
	got = call(t, http.MethodGet, base+"/home/feed?limit=abc", "", "")
	if got.status != http.StatusBadRequest {
		t.Fatalf("bad limit status = %d, want 400 (body=%s)", got.status, got.body)
	}
}

// --- BFF 第一阶段：user/game 反代（mobile-app 统一入口） ---

func TestRoutes_Register(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 成功：信封 data.user_id 透传；上游收到注册体。
	got := call(t, http.MethodPost, base+"/auth/register", "",
		`{"username":"alice","email":"a@x.dev","password":"pw"}`)
	if got.status != http.StatusOK {
		t.Fatalf("register status = %d, body = %s", got.status, got.body)
	}
	if got.json["code"] != float64(200) {
		t.Fatalf("envelope = %#v", got.json)
	}
	if data := object(t, got.json, "data"); data["user_id"] != float64(7) {
		t.Fatalf("data = %#v", data)
	}
	sent := stubs.user.last(t)
	if sent.method != http.MethodPost || sent.path != "/auth/register" {
		t.Fatalf("upstream request = %s %s", sent.method, sent.path)
	}
	if !strings.Contains(sent.body, `"username":"alice"`) {
		t.Fatalf("upstream body = %q", sent.body)
	}

	// 业务失败：HTTP 200 + 信封 code 409 原样透传（客户端 requestEnvelopeFull 语义不变）。
	got = call(t, http.MethodPost, base+"/auth/register", "", `{"username":"taken","email":"t@x.dev","password":"pw"}`)
	if got.status != http.StatusOK {
		t.Fatalf("taken status = %d, want 200 信封", got.status)
	}
	if got.json["code"] != float64(409) || got.json["message"] != "用户名已被占用" {
		t.Fatalf("taken envelope = %#v", got.json)
	}
}

func TestRoutes_UserProfile(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// Bearer 透传 + 信封 data 透传。
	got := call(t, http.MethodGet, base+"/users/7", "Bearer gw-token", "")
	if got.status != http.StatusOK {
		t.Fatalf("profile status = %d, body = %s", got.status, got.body)
	}
	if data := object(t, got.json, "data"); data["username"] != "alice" {
		t.Fatalf("data = %#v", data)
	}
	sent := stubs.user.last(t)
	if sent.path != "/users/7" || sent.auth != "Bearer gw-token" {
		t.Fatalf("upstream request = %+v", sent)
	}

	// 匿名 → 上游 401 原样透传（App 401 统一跳登录依赖状态码）。
	got = call(t, http.MethodGet, base+"/users/7", "", "")
	if got.status != http.StatusUnauthorized {
		t.Fatalf("anon status = %d, want 401", got.status)
	}
}

func TestRoutes_GamesListAndDetail(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 列表 + 搜索/分页 query 透传。
	got := call(t, http.MethodGet, base+"/games?keyword=rpg&limit=20&offset=0", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("games list status = %d, body = %s", got.status, got.body)
	}
	if games := list(t, got.json, "games"); len(games) != 1 {
		t.Fatalf("games = %#v", games)
	}
	sent := stubs.game.last(t)
	if sent.path != "/games" || sent.query != "keyword=rpg&limit=20&offset=0" {
		t.Fatalf("upstream request = %+v", sent)
	}

	// 详情路径参数透传。
	got = call(t, http.MethodGet, base+"/games/g-1", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("game detail status = %d, body = %s", got.status, got.body)
	}
	if game := object(t, got.json, "game"); game["title"] != "Alpha RPG" {
		t.Fatalf("game = %#v", game)
	}
	sent = stubs.game.last(t)
	if sent.path != "/games/g-1" {
		t.Fatalf("upstream request = %+v", sent)
	}

	// 缺失 → 上游 404 原样透传。
	got = call(t, http.MethodGet, base+"/games/nope", "", "")
	if got.status != http.StatusNotFound {
		t.Fatalf("missing status = %d, want 404", got.status)
	}
}

// --- data-panel 反代：/api/v1/stats（战绩面板） ---

func TestRoutes_StatsSummaryAndGames(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 汇总公开透传。
	got := call(t, http.MethodGet, base+"/api/v1/stats/users/1001/summary", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("summary status = %d, body = %s", got.status, got.body)
	}
	if summary := object(t, got.json, "summary"); summary["total_matches"] != float64(42) ||
		summary["game_count"] != float64(2) {
		t.Fatalf("summary = %#v", summary)
	}
	sent := stubs.dataPanel.last(t)
	if sent.method != http.MethodGet || sent.path != "/api/v1/stats/users/1001/summary" {
		t.Fatalf("upstream request = %s %s", sent.method, sent.path)
	}

	// 明细列表：query 分页透传。
	got = call(t, http.MethodGet, base+"/api/v1/stats/users/1001/games?limit=10&offset=0", "", "")
	if got.status != http.StatusOK {
		t.Fatalf("games status = %d, body = %s", got.status, got.body)
	}
	if games := list(t, got.json, "games"); len(games) != 1 {
		t.Fatalf("games = %#v", games)
	}
	if q := stubs.dataPanel.last(t).query; q != "limit=10&offset=0" {
		t.Fatalf("upstream query = %q, want limit=10&offset=0", q)
	}

	// 缺失游戏 → 上游 404 原样透传。
	got = call(t, http.MethodGet, base+"/api/v1/stats/users/1001/games/nope", "", "")
	if got.status != http.StatusNotFound {
		t.Fatalf("missing game status = %d, want 404", got.status)
	}
}

func TestRoutes_StatsRecordIngest(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 摄入：Bearer 透传，body 原样转发（归属由 data-panel 从令牌取）。
	got := call(t, http.MethodPost, base+"/api/v1/stats/records", "Bearer gw-token",
		`{"game_id":"game-valorant","matches":1,"wins":1}`)
	if got.status != http.StatusOK {
		t.Fatalf("record status = %d, body = %s", got.status, got.body)
	}
	if got.json["code"] != float64(200) {
		t.Fatalf("envelope = %#v", got.json)
	}
	if stat := object(t, got.json, "stat"); stat["matches"] != float64(88) {
		t.Fatalf("stat = %#v", stat)
	}
	sent := stubs.dataPanel.last(t)
	if sent.method != http.MethodPost || sent.path != "/api/v1/stats/records" {
		t.Fatalf("upstream request = %s %s", sent.method, sent.path)
	}
	if sent.auth != "Bearer gw-token" {
		t.Fatalf("upstream authorization = %q", sent.auth)
	}
	if !strings.Contains(sent.body, `"game_id":"game-valorant"`) {
		t.Fatalf("upstream body = %q", sent.body)
	}

	// 匿名摄入 → 上游 401 原样透传。
	got = call(t, http.MethodPost, base+"/api/v1/stats/records", "",
		`{"game_id":"game-valorant","matches":1}`)
	if got.status != http.StatusUnauthorized {
		t.Fatalf("anon record status = %d, want 401", got.status)
	}
}

// --- 路由注册本身 ---

// TestRoutes_RegistrationIsExact 校验网关自有 handler 的路由边界：
// 路径存在但方法不符返回 405，路径不存在返回 404，都不会被某条路由兜住，
// 也不会触达上游，证明没有隐式兜底路由。（反代路由由各 proxy 包单测覆盖。）
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
		// go-zero 对参数路由的未注册方法返回 404：关键断言是不触达上游
		{"资料只注册 GET", http.MethodPost, "/users/7", http.StatusMethodNotAllowed},
		{"未注册路径", http.MethodGet, "/api/v1/games", http.StatusNotFound},
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
