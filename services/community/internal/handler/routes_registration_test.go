package handler

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/tappi/tappi/services/community/internal/httperr"

	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// startRouteTestServer 启动真实 go-zero rest 服务器并走生产 RegisterHandlers
// 路由注册（含各路由组的 Auth 中间件），验证路由表本身可达。
func startRouteTestServer(t *testing.T) (base string, alice, bob string) {
	t.Helper()
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	svcCtx, secret := newTestServiceContext(t)
	alice = signToken(t, secret, 100, "alice")
	bob = signToken(t, secret, 200, "bob")

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
	go server.Start()
	t.Cleanup(server.Stop)

	base = "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp, err := http.Get(base + "/api/v1/topics")
		if err == nil {
			_ = resp.Body.Close()
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("test server did not become ready")
		}
		time.Sleep(20 * time.Millisecond)
	}
	return base, alice, bob
}

// call 发起请求并做底线断言：状态码必须 2xx/4xx（路由可达性契约禁止 500）。
func call(t *testing.T, method, url, token, body string) (int, []byte) {
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
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, url, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	status := resp.StatusCode
	if status < 200 || status >= 500 {
		t.Fatalf("%s %s out of 2xx/4xx contract: status=%d body=%s", method, url, status, payload)
	}
	return status, payload
}

func decode(t *testing.T, payload []byte) map[string]interface{} {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal(payload, &m); err != nil {
		t.Fatalf("decode %s: %v", payload, err)
	}
	return m
}

// TestAllRoutesReachable 遍历 RegisterHandlers 注册的全部 18 条路由，
// 每条至少一条 2xx/4xx 用例，断言无任何 500。
func TestAllRoutesReachable(t *testing.T) {
	base, alice, bob := startRouteTestServer(t)

	// --- 公开读：topics ---
	status, payload := call(t, http.MethodGet, base+"/api/v1/topics", "", "")
	if status != 200 {
		t.Fatalf("GET /topics status=%d body=%s", status, payload)
	}
	if total, _ := decode(t, payload)["total"].(float64); total < 1 {
		t.Fatalf("expected seeded topics, body=%s", payload)
	}

	status, payload = call(t, http.MethodGet, base+"/api/v1/topics/1", "", "")
	if status != 200 {
		t.Fatalf("GET /topics/1 status=%d body=%s", status, payload)
	}
	if status, payload = call(t, http.MethodGet, base+"/api/v1/topics/999", "", ""); status != 404 {
		t.Fatalf("GET /topics/999 status=%d, want 404 (ErrTopicNotFound)", status)
	}

	// --- 认证读/写：topics ---
	// GET /topics/following：静态段须优先于 GET /topics/:id 命中 following 路由
	status, _ = call(t, http.MethodGet, base+"/api/v1/topics/following", alice, "")
	if status != 200 {
		t.Fatalf("GET /topics/following status=%d, want 200 (static beats :id)", status)
	}

	status, payload = call(t, http.MethodPost, base+"/api/v1/topics", alice,
		`{"name":"route-test","description":"d"}`)
	if status != 200 {
		t.Fatalf("POST /topics status=%d body=%s", status, payload)
	}
	created := decode(t, payload)["topic"].(map[string]interface{})
	createdTopicId := strconv.Itoa(int(created["id"].(float64)))

	status, _ = call(t, http.MethodPost, base+"/api/v1/topics/"+createdTopicId+"/follow", alice, "")
	if status != 200 {
		t.Fatalf("POST /topics/:id/follow status=%d", status)
	}
	status, _ = call(t, http.MethodDelete, base+"/api/v1/topics/"+createdTopicId+"/follow", alice, "")
	if status != 200 {
		t.Fatalf("DELETE /topics/:id/follow status=%d", status)
	}

	// --- 公开读：posts ---
	status, _ = call(t, http.MethodGet, base+"/api/v1/posts?topic_id=1&limit=5", "", "")
	if status != 200 {
		t.Fatalf("GET /posts status=%d", status)
	}

	status, payload = call(t, http.MethodPost, base+"/api/v1/posts", alice,
		`{"topic_id":1,"title":"route post","content":"body"}`)
	if status != 200 {
		t.Fatalf("POST /posts status=%d body=%s", status, payload)
	}
	postId := strconv.Itoa(int(decode(t, payload)["post"].(map[string]interface{})["id"].(float64)))

	// 空字段走 logic 层 BadRequest → 400（经真实路由 + ErrorHandler）
	if status, _ = call(t, http.MethodPost, base+"/api/v1/posts", alice, `{}`); status != 400 {
		t.Fatalf("POST /posts empty body status=%d, want 400", status)
	}

	status, payload = call(t, http.MethodGet, base+"/api/v1/posts/"+postId, "", "")
	if status != 200 {
		t.Fatalf("GET /posts/:id status=%d body=%s", status, payload)
	}
	if status, _ = call(t, http.MethodGet, base+"/api/v1/posts/999", "", ""); status != 404 {
		t.Fatalf("GET /posts/999 status=%d, want 404", status)
	}

	// GET /posts/hot：静态段须优先于 GET /posts/:id 命中 hot 路由
	status, _ = call(t, http.MethodGet, base+"/api/v1/posts/hot", "", "")
	if status != 200 {
		t.Fatalf("GET /posts/hot status=%d, want 200 (static beats :id)", status)
	}

	// GET /posts/followed（M3 关注流）：匿名 401 —— 若误落到公开组
	// GET /posts/:id 会回 404，401 同时证明静态段优先与认证组归属
	if status, _ = call(t, http.MethodGet, base+"/api/v1/posts/followed", "", ""); status != 401 {
		t.Fatalf("GET /posts/followed anonymous status=%d, want 401 (auth group + static beats :id)", status)
	}

	// --- 认证写：posts（作者/非作者）---
	if status, _ = call(t, http.MethodPut, base+"/api/v1/posts/"+postId, bob,
		`{"title":"hacked"}`); status != 403 {
		t.Fatalf("PUT /posts/:id non-author status=%d, want 403", status)
	}
	if status, _ = call(t, http.MethodPut, base+"/api/v1/posts/"+postId, alice,
		`{"title":"renamed"}`); status != 200 {
		t.Fatalf("PUT /posts/:id author status=%d, want 200", status)
	}

	status, payload = call(t, http.MethodPost, base+"/api/v1/posts/"+postId+"/like", alice, "")
	if status != 200 {
		t.Fatalf("POST /posts/:id/like status=%d body=%s", status, payload)
	}
	status, _ = call(t, http.MethodPost, base+"/api/v1/posts/"+postId+"/share", alice, "")
	if status != 200 {
		t.Fatalf("POST /posts/:id/share status=%d", status)
	}

	if status, _ = call(t, http.MethodDelete, base+"/api/v1/posts/"+postId, bob, ""); status != 403 {
		t.Fatalf("DELETE /posts/:id non-author status=%d, want 403", status)
	}

	// --- 认证写：users follow ---
	if status, payload = call(t, http.MethodPost, base+"/api/v1/users/100/follow", alice, ""); status != 400 {
		t.Fatalf("self follow status=%d body=%s, want 400", status, payload)
	}
	if status, _ = call(t, http.MethodPost, base+"/api/v1/users/200/follow", alice, ""); status != 200 {
		t.Fatalf("POST /users/:id/follow status=%d, want 200", status)
	}
	if status, _ = call(t, http.MethodDelete, base+"/api/v1/users/200/follow", alice, ""); status != 200 {
		t.Fatalf("DELETE /users/:id/follow status=%d, want 200", status)
	}

	// --- M3 关注流 + 已关注用户列表 ---
	if status, _ = call(t, http.MethodGet, base+"/api/v1/users/following", "", ""); status != 401 {
		t.Fatalf("GET /users/following anonymous status=%d, want 401", status)
	}
	if status, payload = call(t, http.MethodGet, base+"/api/v1/users/following", alice, ""); status != 200 {
		t.Fatalf("GET /users/following status=%d body=%s", status, payload)
	} else if _, ok := decode(t, payload)["user_ids"]; !ok {
		t.Fatalf("GET /users/following body must carry user_ids, got %s", payload)
	}

	// alice 关注 bob + 话题 1（种子帖所在）→ 已关注列表含 200、关注流非空且分页可用
	if status, _ = call(t, http.MethodPost, base+"/api/v1/users/200/follow", alice, ""); status != 200 {
		t.Fatalf("POST /users/:id/follow (re-follow) status=%d", status)
	}
	if status, payload = call(t, http.MethodGet, base+"/api/v1/users/following", alice, ""); status != 200 {
		t.Fatalf("GET /users/following status=%d", status)
	} else {
		ids := decode(t, payload)["user_ids"].([]interface{})
		if len(ids) != 1 || ids[0].(float64) != 200 {
			t.Fatalf("user_ids want [200], got %s", payload)
		}
	}
	if status, _ = call(t, http.MethodPost, base+"/api/v1/topics/1/follow", alice, ""); status != 200 {
		t.Fatalf("POST /topics/1/follow status=%d", status)
	}
	if status, payload = call(t, http.MethodGet, base+"/api/v1/posts/followed?limit=1&offset=0", alice, ""); status != 200 {
		t.Fatalf("GET /posts/followed status=%d body=%s", status, payload)
	} else {
		body := decode(t, payload)
		if total, _ := body["total"].(float64); total < 1 {
			t.Fatalf("followed feed of topic-1 follower must be non-empty, got %s", payload)
		}
		if posts, _ := body["posts"].([]interface{}); len(posts) != 1 {
			t.Fatalf("limit=1 must cap page at 1 item, got %s", payload)
		}
	}
	// 空态：bob 未关注任何 → 空列表 + total 0
	if status, payload = call(t, http.MethodGet, base+"/api/v1/posts/followed", bob, ""); status != 200 {
		t.Fatalf("GET /posts/followed (empty) status=%d", status)
	} else {
		body := decode(t, payload)
		if total, _ := body["total"].(float64); total != 0 {
			t.Fatalf("empty follow set must yield total 0, got %s", payload)
		}
		if posts, _ := body["posts"].([]interface{}); len(posts) != 0 {
			t.Fatalf("empty follow set must yield [] posts, got %s", payload)
		}
	}
	// 收尾取关，恢复现场
	if status, _ = call(t, http.MethodDelete, base+"/api/v1/users/200/follow", alice, ""); status != 200 {
		t.Fatalf("DELETE /users/:id/follow cleanup status=%d", status)
	}
	if status, _ = call(t, http.MethodDelete, base+"/api/v1/topics/1/follow", alice, ""); status != 200 {
		t.Fatalf("DELETE /topics/:id/follow cleanup status=%d", status)
	}

	// --- 认证写：delete post（作者收尾）---
	if status, _ = call(t, http.MethodDelete, base+"/api/v1/posts/"+postId, alice, ""); status != 200 {
		t.Fatalf("DELETE /posts/:id author status=%d, want 200", status)
	}

	// 认证中间件拒收：受保护路由无令牌/坏令牌 → 401（真 HTTP 状态）
	if status, _ = call(t, http.MethodPost, base+"/api/v1/posts", "", `{"topic_id":1}`); status != 401 {
		t.Fatalf("POST /posts anonymous status=%d, want 401", status)
	}
	if status, _ = call(t, http.MethodPost, base+"/api/v1/topics", bob+"x", `{"name":"n"}`); status != 401 {
		t.Fatalf("POST /topics bad token status=%d, want 401", status)
	}

	// --- 参数解析失败矩阵：坏路径参数 / 坏 query / 坏 JSON → 400 ---
	parseErrs := []struct{ name, method, url, body string }{
		{"follow bad user_id", http.MethodPost, "/api/v1/users/abc/follow", ""},
		{"unfollow bad user_id", http.MethodDelete, "/api/v1/users/abc/follow", ""},
		{"get posts bad limit", http.MethodGet, "/api/v1/posts?limit=abc", ""},
		{"get post bad id", http.MethodGet, "/api/v1/posts/abc", ""},
		{"hot posts bad limit", http.MethodGet, "/api/v1/posts/hot?limit=abc", ""},
		{"update post bad id", http.MethodPut, "/api/v1/posts/abc", `{}`},
		{"delete post bad id", http.MethodDelete, "/api/v1/posts/abc", ""},
		{"like post bad id", http.MethodPost, "/api/v1/posts/abc/like", ""},
		{"share post bad id", http.MethodPost, "/api/v1/posts/abc/share", ""},
		{"get topics bad limit", http.MethodGet, "/api/v1/topics?limit=abc", ""},
		{"get topic bad id", http.MethodGet, "/api/v1/topics/abc", ""},
		{"follow topic bad id", http.MethodPost, "/api/v1/topics/abc/follow", ""},
		{"unfollow topic bad id", http.MethodDelete, "/api/v1/topics/abc/follow", ""},
		{"create topic bad json", http.MethodPost, "/api/v1/topics", `{bad`},
	}
	for _, tc := range parseErrs {
		if status, _ = call(t, tc.method, base+tc.url, alice, tc.body); status != 400 {
			t.Fatalf("%s: %s %s status=%d, want 400", tc.name, tc.method, tc.url, status)
		}
	}

	// --- logic 层真 error 分支：目标不存在 → 404（ErrPostNotFound/ErrTopicNotFound）---
	notFounds := []struct{ name, method, url, body string }{
		{"create post missing topic", http.MethodPost, "/api/v1/posts", `{"topic_id":999,"title":"t","content":"c"}`},
		{"like missing post", http.MethodPost, "/api/v1/posts/999/like", ""},
		{"share missing post", http.MethodPost, "/api/v1/posts/999/share", ""},
		{"follow missing topic", http.MethodPost, "/api/v1/topics/999/follow", ""},
		{"unfollow missing topic", http.MethodDelete, "/api/v1/topics/999/follow", ""},
		{"update missing post", http.MethodPut, "/api/v1/posts/999", `{"title":"t"}`},
		{"delete missing post", http.MethodDelete, "/api/v1/posts/999", ""},
	}
	for _, tc := range notFounds {
		if status, _ = call(t, tc.method, base+tc.url, alice, tc.body); status != 404 {
			t.Fatalf("%s: %s %s status=%d, want 404", tc.name, tc.method, tc.url, status)
		}
	}
	// 注：UnfollowUser/GetFollowingTopics/GetTopics/GetHotPosts/CreateTopic 等
	// handler 的 err 分支仅剩仓储落盘失败或中间件前 ctx 缺失——无 HTTP 可达路径。
}

// TestMiddleware_401_nonJWT_authHeader 中间件对非 Bearer 认证头的 401 响应分支。
func TestMiddleware_401_nonJWT_authHeader(t *testing.T) {
	base, _, _ := startRouteTestServer(t)

	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/posts", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", "Basic xyz")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s /api/v1/posts: %v", http.MethodPost, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("non-JWT auth header status = %d, want 401", resp.StatusCode)
	}
}

// TestMiddleware_401_malformedBearerToken 中间件对 Bearer 头但令牌非法时的 401 响应分支。
func TestMiddleware_401_malformedBearerToken(t *testing.T) {
	base, _, _ := startRouteTestServer(t)

	// 使用无效的 Bearer 令牌（随机字符串，未 HMAC 签名）
	token := "Bearer random-junk-token-string"
	req, err := http.NewRequest(http.MethodPost, base+"/api/v1/posts", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	req.Header.Set("Authorization", token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s /api/v1/posts: %v", http.MethodPost, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("malformed Bearer token status = %d, want 401", resp.StatusCode)
	}
}
