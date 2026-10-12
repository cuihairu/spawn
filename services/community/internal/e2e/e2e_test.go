// Package e2e 进程内全栈端到端测试：真实 rest.Server + 全量路由 + 真实
// SQLite + 真实 JWT 中间件，HTTP 客户端走完整用户旅程（README 待实现
// 「服务编排、数据回放」口径）。realtime-hub 以本地桩接住 /internal/publish，
// 断言编排面真的跨域发出事件（帖子/评论/关注/私信/回执）。
package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"

	"github.com/tappi/tappi/services/community/internal/config"
	"github.com/tappi/tappi/services/community/internal/handler"
	"github.com/tappi/tappi/services/community/internal/httperr"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/utils"
)

// ---- realtime-hub 发布桩 ----

type stubEvent struct {
	Type    string // 域级帧类型：notification / message / ...
	Subtype string // payload 内业务类型（like_post / comment_post / follow_user / ...）
	UserIds []int64
}

type publishStub struct {
	url    string
	mu     sync.Mutex
	events []stubEvent
}

func (p *publishStub) snapshot() []stubEvent {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]stubEvent(nil), p.events...)
}

// startPublishStub 接住 community → realtime-hub 的投递。
func startPublishStub(t *testing.T) *publishStub {
	t.Helper()
	stub := &publishStub{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/publish" {
			http.NotFound(w, r)
			return
		}
		var body struct {
			UserIds []int64 `json:"user_ids"`
			Event   struct {
				Type    string `json:"type"`
				Payload struct {
					Type string `json:"type"`
				} `json:"payload"`
			} `json:"event"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		stub.mu.Lock()
		stub.events = append(stub.events, stubEvent{Type: body.Event.Type, Subtype: body.Event.Payload.Type, UserIds: body.UserIds})
		stub.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	stub.url = srv.URL
	t.Cleanup(srv.Close)
	return stub
}

// waitForEvents 轮询直到谓词满足（发布为 fire-and-forget goroutine，需等待）。
func waitForEvents(t *testing.T, stub *publishStub, pred func([]stubEvent) bool) []stubEvent {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		events := stub.snapshot()
		if pred(events) {
			return events
		}
		if time.Now().After(deadline) {
			t.Fatalf("events not settled in time: %+v", events)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ---- 全栈服务启动 ----

func startServer(t *testing.T, secret, realtimeHubURL string) string {
	t.Helper()

	lst, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := lst.Addr().(*net.TCPAddr).Port
	_ = lst.Close()

	var c config.Config
	c.Name = "community-e2e"
	c.Host = "127.0.0.1"
	c.Port = port
	c.Timeout = 10000
	c.Auth.JWTSecret = secret
	c.MySQL.DataSource = "file:" + filepath.Join(t.TempDir(), "community-e2e.db")
	c.Realtime.Url = realtimeHubURL

	// main 入口同款全局错误处理（缺它 go-zero 默认把一切错误打成 400）
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	serverCtx := svc.NewServiceContext(c)
	server := rest.MustNewServer(c.RestConf)
	handler.RegisterHandlers(server, serverCtx)
	go server.Start()
	t.Cleanup(server.Stop)

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/api/v1/topics")
		if err == nil {
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
			return base
		}
		if time.Now().After(deadline) {
			t.Fatal("e2e server did not become ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// ---- 客户端助手 ----

func mintToken(t *testing.T, secret string, userId int64, username string) string {
	t.Helper()
	claims := utils.JWTClaims{
		UserId:   userId,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("mint token: %v", err)
	}
	return token
}

// do 走真实 HTTP 栈一跳；返回状态码与响应体。
func do(t *testing.T, method, base, path, token string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s: %v", path, err)
		}
		reader = strings.NewReader(string(raw))
	}
	req, err := http.NewRequest(method, base+path, reader)
	if err != nil {
		t.Fatalf("new request %s: %v", path, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return resp.StatusCode, out
}

func mustOK(t *testing.T, status int, resp map[string]any, what string) map[string]any {
	t.Helper()
	if status != http.StatusOK {
		t.Fatalf("%s: expected 200, got %d body=%v", what, status, resp)
	}
	return resp
}

// callOK = do + mustOK：一跳请求并断言 200，返回响应体。
func callOK(t *testing.T, method, base, path, token string, body any) map[string]any {
	t.Helper()
	st, resp := do(t, method, base, path, token, body)
	return mustOK(t, st, resp, method+" "+path)
}

func num(t *testing.T, resp map[string]any, keys ...string) float64 {
	t.Helper()
	cur := any(resp)
	for _, k := range keys {
		m, ok := cur.(map[string]any)
		if !ok {
			t.Fatalf("path %v not found in %v", keys, resp)
		}
		cur = m[k]
	}
	v, ok := cur.(float64)
	if !ok {
		t.Fatalf("path %v not numeric: %v", keys, resp)
	}
	return v
}

// ---- 旅程：服务编排（话题→帖子→关注→点赞→评论→关注人→通知→已读→私信→回执） ----

func TestCommunityEndToEndJourney(t *testing.T) {
	stub := startPublishStub(t)
	secret := "e2e-jwt-secret"
	base := startServer(t, secret, stub.url)

	alice := mintToken(t, secret, 101, "阿莉")
	bob := mintToken(t, secret, 202, "小波")

	// 1) 无 token 访问受保护端点 → 401
	if st, _ := do(t, http.MethodGet, base, "/api/v1/notifications", "", nil); st != http.StatusUnauthorized {
		t.Fatalf("bare notifications: expected 401, got %d", st)
	}

	// 2) Alice 建话题并发帖
	topic := callOK(t, http.MethodPost, base, "/api/v1/topics", alice,
		map[string]any{"name": "e2e话题", "description": "旅程起点"})["topic"].(map[string]any)
	topicId := int64(num(t, topic, "id"))
	post := callOK(t, http.MethodPost, base, "/api/v1/posts", alice,
		map[string]any{"topic_id": topicId, "title": "旅程第一帖", "content": "正文"})["post"].(map[string]any)
	postId := int64(num(t, post, "id"))

	// 2.5) 公开搜索：标题关键词命中、空白查询空结果（无 token 可用）
	if n := num(t, callOK(t, http.MethodGet, base, "/api/v1/posts/search?q=%E6%97%85%E7%A8%8B", "", nil),
		"total"); n != 1 {
		t.Fatalf("search total = %v, want 1", n)
	}
	if n := num(t, callOK(t, http.MethodGet, base, "/api/v1/posts/search?q=%20", "", nil),
		"total"); n != 0 {
		t.Fatalf("blank search total = %v, want 0", n)
	}

	// 3) Bob 关注话题 → 关注流含该帖
	callOK(t, http.MethodPost, base, fmt.Sprintf("/api/v1/topics/%d/follow", topicId), bob, nil)
	followed := callOK(t, http.MethodGet, base, "/api/v1/posts/followed", bob, nil)
	if n := num(t, followed, "total"); n < 1 {
		t.Fatalf("followed feed total = %v, want >= 1", n)
	}

	// 4) Bob 点赞 + 评论 + 关注 Alice
	callOK(t, http.MethodPost, base, fmt.Sprintf("/api/v1/posts/%d/like", postId), bob, nil)
	callOK(t, http.MethodPost, base, fmt.Sprintf("/api/v1/posts/%d/comments", postId), bob,
		map[string]any{"content": "好帖！"})
	callOK(t, http.MethodPost, base, "/api/v1/users/101/follow", bob, nil)

	// 5) Alice 收到 3 条通知（like_post/comment_post/follow_user），未读 3
	list := callOK(t, http.MethodGet, base, "/api/v1/notifications", alice, nil)
	if n := num(t, list, "total"); n != 3 {
		t.Fatalf("notifications total = %v, want 3", n)
	}
	if n := num(t, callOK(t, http.MethodGet, base, "/api/v1/notifications/unread-count", alice, nil),
		"count"); n != 3 {
		t.Fatalf("unread = %v, want 3", n)
	}

	// 6) Alice 全部已读 → 归零
	callOK(t, http.MethodPost, base, "/api/v1/notifications/read-all", alice, nil)
	if n := num(t, callOK(t, http.MethodGet, base, "/api/v1/notifications/unread-count", alice, nil),
		"count"); n != 0 {
		t.Fatalf("unread after read-all = %v, want 0", n)
	}

	// 7) Bob → Alice 私信两条；Alice 会话/线程/已读链路
	for _, text := range []string{"嗨", "在吗"} {
		callOK(t, http.MethodPost, base, "/api/v1/messages", bob,
			map[string]any{"recipient_id": 101, "content": text})
	}
	if n := num(t, callOK(t, http.MethodGet, base, "/api/v1/messages/unread-count", alice, nil),
		"count"); n != 2 {
		t.Fatalf("dm unread = %v, want 2", n)
	}
	convs := callOK(t, http.MethodGet, base, "/api/v1/messages/conversations", alice, nil)
	if c := convs["conversations"].([]any); len(c) != 1 ||
		num(t, c[0].(map[string]any), "peer_id") != 202 || num(t, c[0].(map[string]any), "unread") != 2 {
		t.Fatalf("conversations = %v", convs)
	}
	if n := num(t, callOK(t, http.MethodGet, base, "/api/v1/messages/thread/202", alice, nil),
		"total"); n != 2 {
		t.Fatalf("thread total = %v, want 2", n)
	}
	if n := num(t, callOK(t, http.MethodPost, base, "/api/v1/messages/thread/202/read", alice, nil),
		"count"); n != 0 {
		t.Fatalf("remaining after mark read = %v, want 0", n)
	}

	// 8) 编排面事件：点赞/评论/关注/私信帧都真实发出（payload 子类型 + 收件人断言）
	waitForEvents(t, stub, func(ev []stubEvent) bool {
		have := map[string]bool{}
		msgs := 0
		for _, e := range ev {
			if len(e.UserIds) != 1 || e.UserIds[0] != 101 {
				continue
			}
			if e.Type == "notification" {
				switch e.Subtype {
				case "like_post", "comment_post", "follow_user":
					have[e.Subtype] = true
				}
			}
			if e.Type == "message" {
				msgs++
			}
		}
		return have["like_post"] && have["comment_post"] && have["follow_user"] && msgs >= 2
	})
}

// ---- 数据回放：幂等契约（README 各端点「幂等」口径钉进测试） ----

func TestCommunityDataReplayIdempotent(t *testing.T) {
	stub := startPublishStub(t) // 挂上桩：发布可达，测试输出干净
	secret := "replay-jwt-secret"
	base := startServer(t, secret, stub.url)

	alice := mintToken(t, secret, 301, "回放甲")
	bob := mintToken(t, secret, 402, "回放乙")

	topic := callOK(t, http.MethodPost, base, "/api/v1/topics", alice,
		map[string]any{"name": "回放话题", "description": "幂等契约"})["topic"].(map[string]any)
	post := callOK(t, http.MethodPost, base, "/api/v1/posts", alice,
		map[string]any{"topic_id": int64(num(t, topic, "id")), "title": "回放目标帖", "content": "正文"})["post"].(map[string]any)
	postId := int64(num(t, post, "id"))

	// 1) 点赞回放：计数不重计（唯一键幂等），通知不刷屏
	callOK(t, http.MethodPost, base, fmt.Sprintf("/api/v1/posts/%d/like", postId), bob, nil)
	callOK(t, http.MethodPost, base, fmt.Sprintf("/api/v1/posts/%d/like", postId), bob, nil)
	if n := num(t, callOK(t, http.MethodGet, base, fmt.Sprintf("/api/v1/posts/%d", postId), bob, nil),
		"post", "like_count"); n != 1 {
		t.Fatalf("like_count after replay = %v, want 1", n)
	}

	// 2) 评论回放：评论为追加型设计（两条都在），通知去重仅一条
	callOK(t, http.MethodPost, base, fmt.Sprintf("/api/v1/posts/%d/comments", postId), bob,
		map[string]any{"content": "同一句话"})
	callOK(t, http.MethodPost, base, fmt.Sprintf("/api/v1/posts/%d/comments", postId), bob,
		map[string]any{"content": "同一句话"})
	if n := num(t, callOK(t, http.MethodGet, base, fmt.Sprintf("/api/v1/posts/%d/comments", postId), bob, nil),
		"total"); n != 2 {
		t.Fatalf("comments after replay = %v, want 2 (append-only by design)", n)
	}

	// 3) 关注回放：关系唯一，重复关注不报错不加行
	callOK(t, http.MethodPost, base, "/api/v1/users/301/follow", bob, nil)
	callOK(t, http.MethodPost, base, "/api/v1/users/301/follow", bob, nil)

	// 4) 私信回放：消息为追加型设计（聊天记录保留两条），未读 2
	callOK(t, http.MethodPost, base, "/api/v1/messages", bob,
		map[string]any{"recipient_id": 301, "content": "重复的话"})
	callOK(t, http.MethodPost, base, "/api/v1/messages", bob,
		map[string]any{"recipient_id": 301, "content": "重复的话"})
	if n := num(t, callOK(t, http.MethodGet, base, "/api/v1/messages/thread/402", alice, nil),
		"total"); n != 2 {
		t.Fatalf("dm thread after replay = %v, want 2 (append-only by design)", n)
	}
	if n := num(t, callOK(t, http.MethodGet, base, "/api/v1/messages/unread-count", alice, nil),
		"count"); n != 2 {
		t.Fatalf("dm unread after replay = %v, want 2", n)
	}

	// 5) 通知侧收口：三类通知各恰一条（like/comment/follow），未读 3
	list := callOK(t, http.MethodGet, base, "/api/v1/notifications", alice, nil)
	counts := map[string]int{}
	items, _ := list["notifications"].([]any)
	for _, it := range items {
		m, _ := it.(map[string]any)
		if s, ok := m["type"].(string); ok {
			counts[s]++
		}
	}
	if counts["like_post"] != 1 || counts["comment_post"] != 1 || counts["follow_user"] != 1 {
		t.Fatalf("notification dedup broken: %v (list=%v)", counts, list)
	}
	if n := num(t, list, "total"); n != 3 {
		t.Fatalf("notifications total = %v, want 3", n)
	}
	if n := num(t, callOK(t, http.MethodGet, base, "/api/v1/notifications/unread-count", alice, nil),
		"count"); n != 3 {
		t.Fatalf("unread after replay = %v, want 3", n)
	}
}
