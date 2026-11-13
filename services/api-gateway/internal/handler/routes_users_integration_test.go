package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestRoutes_Login(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 成功：上游信封原样透传（code 200 + data.token/user_info），
	// 同时校验网关→user-service 的线上契约。
	got := call(t, http.MethodPost, base+"/auth/login", "",
		`{"username":"alice","password":"pw"}`)
	if got.status != http.StatusOK {
		t.Fatalf("login status = %d, body = %s", got.status, got.body)
	}
	if got.json["code"] != float64(200) {
		t.Fatalf("envelope = %#v", got.json)
	}
	data := object(t, got.json, "data")
	if data["token"] != "gw-token" {
		t.Fatalf("token = %#v", data["token"])
	}
	if userInfo := object(t, data, "user_info"); userInfo["username"] != "alice" {
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
		t.Fatalf("上游请求体不是登录 JSON: %q (%v)", sent.body, err)
	}
	if payload.Username != "alice" || payload.Password != "pw" {
		t.Fatalf("上游请求体 = %q", sent.body)
	}
}

func TestRoutes_LoginUpstreamFailure(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	// 上游 401 信封原样透传（客户端 request 走 401 分支），HTTP 状态不改写。
	got := call(t, http.MethodPost, base+"/auth/login", "",
		`{"username":"ghost","password":"pw"}`)
	if got.status != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", got.status)
	}
	if got.json["code"] != float64(401) || got.json["message"] != "用户名或密码错误" {
		t.Fatalf("envelope = %#v", got.json)
	}
}

// TestRoutes_LoginVerbatimBody 网关不做请求体校验：非法 JSON 原样转发，
// 上游响应原样透传（与 /auth/register 同款语义）。
func TestRoutes_LoginVerbatimBody(t *testing.T) {
	stubs := newUpstreamStubs(t)
	base := newGatewayServer(t, stubs)

	got := call(t, http.MethodPost, base+"/auth/login", "", `{bad json`)
	if got.status != http.StatusOK {
		t.Fatalf("malformed body status = %d, want 200 透传 (body=%s)", got.status, got.body)
	}
	if got.json["code"] != float64(200) {
		t.Fatalf("envelope = %#v", got.json)
	}
	if sent := stubs.user.last(t); sent.body != `{bad json` {
		t.Fatalf("upstream body = %q, want verbatim", sent.body)
	}
}

// --- GET /games/featured ---

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
