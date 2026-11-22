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
				_, _ = w.Write([]byte(`{"code":401,"message":"用户名或密码错误"}`))
				return
			}
			_, _ = w.Write([]byte(`{"code":200,"message":"登录成功","data":{"token":"gw-token","user_info":{"id":7,"username":"alice"}}}`))
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
		body, _ := io.ReadAll(r.Body)
		stubs.community.add(upstreamCall{
			method: r.Method,
			path:   r.URL.Path,
			query:  r.URL.RawQuery,
			auth:   r.Header.Get("Authorization"),
			body:   string(body),
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
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/posts/search":
			_, _ = w.Write([]byte(`{"posts":[{"id":12,"topic_id":2,"author_id":7,"author_name":"alice",` +
				`"title":"搜到的帖子","content":"命中","like_count":0,"comment_count":0,` +
				`"created_at":"2026-10-12T10:00:00Z"}],"total":1}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/topics":
			_, _ = w.Write([]byte(`{"topics":[{"id":2,"name":"星陨圈","description":"不应下发",` +
				`"post_count":30,"follower_count":88,"is_official":true}],"total":1}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/notifications":
			_, _ = w.Write([]byte(`{"notifications":[{"id":2,"user_id":7,"actor_id":8,` +
				`"actor_name":"读者乙","type":"like_post","target_id":101,"content":"点赞了你的帖子《好帖》",` +
				`"is_read":false,"created_at":"2026-10-10T00:00:00Z"}],"total":1}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/notifications/unread-count":
			_, _ = w.Write([]byte(`{"count":3}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/notifications/read-all":
			_, _ = w.Write([]byte(`{"code":0,"message":"ok"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/posts/1/report":
			_, _ = w.Write([]byte(`{"code":0,"message":"report submitted"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/moderation/reports":
			_, _ = w.Write([]byte(`{"reports":[{"id":1,"reporter_id":8,"target_type":"post","target_id":1,` +
				`"reason":"垃圾广告","status":"pending","created_at":"2026-10-10T00:00:00Z"}],"total":1}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/moderation/reports/1/handle":
			_, _ = w.Write([]byte(`{"code":0,"message":"report handled"}`))
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

// --- POST /auth/login（user-service 反代，信封原样透传） ---
// RegistrationIsExact 为网关级边界（跨代理族），随 harness 留在本文件。
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
