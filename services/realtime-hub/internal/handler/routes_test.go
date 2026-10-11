package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/websocket"

	"github.com/tappi/tappi/services/realtime-hub/internal/auth"
	"github.com/tappi/tappi/services/realtime-hub/internal/hub"
)

const (
	testSecret  = "routes-test-secret"
	testTokenID = "internal-token-2024"
)

func newTestServer(t *testing.T) (*httptest.Server, *hub.Hub) {
	t.Helper()
	h := hub.New(hub.Options{QueueSize: 8, WriteWait: time.Second, PingPeriod: 2 * time.Second})
	srv := httptest.NewServer(NewServer(h, auth.NewVerifier(testSecret), testTokenID))
	t.Cleanup(srv.Close)
	return srv, h
}

// waitConnections 轮询等连接入表（客户端 Dial 返回不代表服务端 Join 已执行）。
func waitConnections(t *testing.T, h *hub.Hub, want map[int64]int) {
	t.Helper()
	for i := 0; i < 250; i++ {
		ok := true
		for id, n := range want {
			if h.Connections(id) != n {
				ok = false
				break
			}
		}
		if ok {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("connections not reached within 5s")
}

func signToken(t *testing.T, userId int64, ttl time.Duration) string {
	t.Helper()
	claims := auth.Claims{
		UserId: userId, Username: "作者甲",
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl))},
	}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString([]byte(testSecret))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return token
}

func dialWS(t *testing.T, srvURL, token string) (*websocket.Conn, *http.Response, error) {
	t.Helper()
	url := "ws" + strings.TrimPrefix(srvURL, "http") + "/ws?token=" + token
	d := websocket.DefaultDialer
	d.HandshakeTimeout = 3 * time.Second
	return websocket.DefaultDialer.Dial(url, nil)
}

func publish(t *testing.T, srvURL, token, body string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srvURL+"/internal/publish", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("X-Internal-Token", token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// TestHealthReturnsOK 探活 200。
func TestHealthReturnsOK(t *testing.T) {
	srv, _ := newTestServer(t)
	resp, err := http.Get(srv.URL + "/health")
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d, want 200", resp.StatusCode)
	}
}

// TestWSRejectsBadTokens 无 token / 垃圾 token / 过期 token / 错误密钥签名
// 均 401 且不升级。
func TestWSRejectsBadTokens(t *testing.T) {
	srv, _ := newTestServer(t)

	// 用错误密钥签一个结构有效的令牌
	wrongClaims := auth.Claims{UserId: 7, RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))}}
	wrongSecretToken, err := jwt.NewWithClaims(jwt.SigningMethodHS256, wrongClaims).SignedString([]byte("other"))
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	cases := map[string]string{
		"no token":     "",
		"garbage":      "not-a-token",
		"expired":      signToken(t, 7, -time.Minute),
		"wrong secret": wrongSecretToken,
	}
	for name, token := range cases {
		conn, resp, err := dialWS(t, srv.URL, token)
		if err == nil {
			_ = conn.Close()
			t.Fatalf("%s: dial must fail", name)
		}
		if resp == nil || resp.StatusCode != http.StatusUnauthorized {
			t.Fatalf("%s: status = %v, want 401", name, resp)
		}
	}
}

// TestPublishDeliversFrameToConnectedClient 端到端：连接 → 投递 → 客户端收帧。
func TestPublishDeliversFrameToConnectedClient(t *testing.T) {
	srv, h := newTestServer(t)
	conn, _, err := dialWS(t, srv.URL, signToken(t, 7, time.Hour))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	waitConnections(t, h, map[int64]int{7: 1})

	resp := publish(t, srv.URL, testTokenID,
		`{"user_ids":[7],"event":{"type":"notification","payload":{"n":1}}}`)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("publish status = %d, want 200", resp.StatusCode)
	}
	var got publishResp
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil || got.Delivered != 1 {
		t.Fatalf("delivered = %+v err=%v, want 1", got, err)
	}

	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	var frame hub.Frame
	if err := json.Unmarshal(data, &frame); err != nil || frame.Type != "notification" {
		t.Fatalf("frame = %s (%v), want type notification", data, err)
	}
	if !bytes.Contains(frame.Data, []byte(`"n":1`)) {
		t.Fatalf("frame data = %s, want payload passthrough", frame.Data)
	}
}

// TestPublishFanoutMultipleDevicesAndUsers 同用户双端与多用户广播。
func TestPublishFanoutMultipleDevicesAndUsers(t *testing.T) {
	srv, h := newTestServer(t)
	dev1, _, err := dialWS(t, srv.URL, signToken(t, 7, time.Hour))
	if err != nil {
		t.Fatalf("dial dev1: %v", err)
	}
	defer dev1.Close()
	dev2, _, err := dialWS(t, srv.URL, signToken(t, 7, time.Hour))
	if err != nil {
		t.Fatalf("dial dev2: %v", err)
	}
	defer dev2.Close()
	other, _, err := dialWS(t, srv.URL, signToken(t, 8, time.Hour))
	if err != nil {
		t.Fatalf("dial other: %v", err)
	}
	defer other.Close()
	waitConnections(t, h, map[int64]int{7: 2, 8: 1})

	resp := publish(t, srv.URL, testTokenID,
		`{"user_ids":[7,8],"event":{"type":"notification","payload":{}}}`)
	var got publishResp
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil || got.Delivered != 3 {
		t.Fatalf("delivered = %+v err=%v, want 3", got, err)
	}
	for name, conn := range map[string]*websocket.Conn{"dev1": dev1, "dev2": dev2, "other": other} {
		_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
		if _, _, err := conn.ReadMessage(); err != nil {
			t.Fatalf("%s read: %v", name, err)
		}
	}
}

// TestPublishGuards 内部令牌缺失/错误 403；空 user_ids 与空 type 400；离线用户 delivered=0。
func TestPublishGuards(t *testing.T) {
	srv, _ := newTestServer(t)

	cases := []struct {
		name   string
		token  string
		body   string
		status int
	}{
		{"no token", "", `{"user_ids":[7],"event":{"type":"n"}}`, http.StatusForbidden},
		{"wrong token", "nope", `{"user_ids":[7],"event":{"type":"n"}}`, http.StatusForbidden},
		{"empty user_ids", testTokenID, `{"user_ids":[],"event":{"type":"n"}}`, http.StatusBadRequest},
		{"empty type", testTokenID, `{"user_ids":[7],"event":{"type":""}}`, http.StatusBadRequest},
		{"bad json", testTokenID, `{"user_ids":`, http.StatusBadRequest},
		{"unknown users", testTokenID, `{"user_ids":[999],"event":{"type":"n"}}`, http.StatusOK},
	}
	for _, tc := range cases {
		resp := publish(t, srv.URL, tc.token, tc.body)
		if resp.StatusCode != tc.status {
			t.Fatalf("%s: status = %d, want %d", tc.name, resp.StatusCode, tc.status)
		}
		if tc.name == "unknown users" {
			var got publishResp
			if err := json.NewDecoder(resp.Body).Decode(&got); err != nil || got.Delivered != 0 {
				t.Fatalf("unknown users delivered = %+v, want 0", got)
			}
		}
	}
}
