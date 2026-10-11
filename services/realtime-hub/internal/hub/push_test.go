package hub

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// TestPushOfflineUsersOnly 在线用户不推、离线用户推送（去重合并单请求）、
// 帧原样携带、token 头正确。
func TestPushOfflineUsersOnly(t *testing.T) {
	envCh := make(chan PushEnvelope, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Push-Token") != "push-tok" {
			t.Errorf("push token header = %q", r.Header.Get("X-Push-Token"))
		}
		var env PushEnvelope
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			t.Errorf("decode envelope: %v", err)
			return
		}
		envCh <- env
	}))
	t.Cleanup(srv.Close)

	h := New(Options{QueueSize: 8, WriteWait: time.Second, PingPeriod: 2 * time.Second})
	h.SetNotifier(NewWebhookNotifier(srv.URL, "push-tok"))
	online := &Client{UserId: 7}
	h.Register(online)

	frame := mustFrame(t, "notification", map[string]string{"k": "v"})
	// 7 在线、8 离线、7 重复入参：只推 8 一次
	if n := h.SendToUsers([]int64{7, 8, 7}, frame); n != 1 {
		t.Fatalf("delivered = %d, want 1 (online user)", n)
	}
	select {
	case env := <-envCh:
		if len(env.UserIds) != 1 || env.UserIds[0] != 8 {
			t.Fatalf("pushed users = %v, want [8]", env.UserIds)
		}
		if string(env.Frame) != string(frame) {
			t.Fatalf("pushed frame = %s, want %s", env.Frame, frame)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no push for offline user")
	}
	select {
	case extra := <-envCh:
		t.Fatalf("unexpected extra push %+v", extra)
	case <-time.After(300 * time.Millisecond):
	}
}

// TestPushDisabledWithRelay 多实例模式下本地零投递 ≠ 离线，不推。
func TestPushDisabledWithRelay(t *testing.T) {
	mr := miniredisRunT(t)
	pushed := make(chan PushEnvelope, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env PushEnvelope
		_ = json.NewDecoder(r.Body).Decode(&env)
		pushed <- env
	}))
	t.Cleanup(srv.Close)

	h, _ := newRelayedHub(t, mr.Addr())
	h.SetNotifier(NewWebhookNotifier(srv.URL, "tok"))

	if n := h.SendToUsers([]int64{99}, mustFrame(t, "notification", nil)); n != 0 {
		t.Fatalf("delivered = %d, want 0", n)
	}
	select {
	case extra := <-pushed:
		t.Fatalf("unexpected push %+v in relay mode", extra)
	case <-time.After(300 * time.Millisecond):
	}
}

// TestPushAllOnlineNoWebhook 全员在线零 webhook 调用。
func TestPushAllOnlineNoWebhook(t *testing.T) {
	calls := make(chan struct{}, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls <- struct{}{}
	}))
	t.Cleanup(srv.Close)

	h := New(Options{QueueSize: 8, WriteWait: time.Second, PingPeriod: 2 * time.Second})
	h.SetNotifier(NewWebhookNotifier(srv.URL, "tok"))
	online := &Client{UserId: 7}
	h.Register(online)

	if n := h.SendToUsers([]int64{7}, mustFrame(t, "notification", nil)); n != 1 {
		t.Fatalf("delivered = %d, want 1", n)
	}
	select {
	case <-calls:
		t.Fatal("unexpected push for online user")
	case <-time.After(300 * time.Millisecond):
	}
}
