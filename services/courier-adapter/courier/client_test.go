package courier

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// scope 只走 header：URL path/query 与 body 一律不携带（scope.md 传输节）。
func TestScopeTransportHeaderOnly(t *testing.T) {
	var lastURL, lastQuery, lastBody string
	var gameHeader, envHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		lastURL, lastQuery, lastBody = r.URL.Path, r.URL.RawQuery, string(b)
		gameHeader = r.Header.Get("X-Courier-Game-Id")
		envHeader = r.Header.Get("X-Courier-Env")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"data":{"account":{"id":"acc_1","type":"EMAIL","status":"ACTIVE"},"accessToken":"atk","accessExpiresAt":"e","refreshToken":"rtk","refreshExpiresAt":"re"}}`))
	}))
	defer srv.Close()

	c, err := NewClient(srv.URL, time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	sess, err := c.Login(context.Background(), "cg-secret", "prod", "a@b.invalid", "password123")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	if sess.AccountID != "acc_1" || sess.AccessToken != "atk" || sess.RefreshToken != "rtk" {
		t.Fatalf("session mismatch: %+v", sess)
	}
	if gameHeader != "cg-secret" || envHeader != "prod" {
		t.Fatalf("scope headers = %s/%s, want cg-secret/prod", gameHeader, envHeader)
	}
	if lastURL != "/v1/identity/login" {
		t.Fatalf("url path = %s, want /v1/identity/login", lastURL)
	}
	if lastQuery != "" {
		t.Fatalf("query = %q, scope must not ride query", lastQuery)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(lastBody), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	for key := range body {
		if key == "gameId" || key == "game_id" || key == "env" {
			t.Fatalf("scope key %q leaked into body", key)
		}
	}
}

func TestErrorEnvelopeMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"AUTH_INVALID_CREDENTIALS","message":"invalid email or password","retryable":false},"traceID":"t1"}`))
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL, time.Second)
	_, err := c.Login(context.Background(), "cg", "prod", "a@b.invalid", "wrong")
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err = %v, want *APIError", err)
	}
	if apiErr.HTTPStatus != http.StatusUnauthorized || apiErr.Code != "AUTH_INVALID_CREDENTIALS" {
		t.Fatalf("api error = %d/%s", apiErr.HTTPStatus, apiErr.Code)
	}
}

func TestDeleteAndCancel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/identity/account/delete":
			// Bearer 保护：注销受理需会话令牌（auth.md 端点表）
			if r.Header.Get("Authorization") != "Bearer atk-9" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"data":{"status":"PENDING_DELETE","cancelToken":"ct","pendingDelete":true}}`))
		case "/v1/identity/account/delete/cancel":
			// 匿名 + cancelToken（v1.1）
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body["cancelToken"] != "ct-1" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			_, _ = w.Write([]byte(`{"data":{}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c, _ := NewClient(srv.URL, time.Second)
	res, err := c.DeleteAccount(context.Background(), "cg", "prod", "atk-9")
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	if res.Status != "PENDING_DELETE" || res.CancelToken != "ct" || !res.PendingDelete {
		t.Fatalf("delete result mismatch: %+v", res)
	}
	if err := c.CancelDelete(context.Background(), "cg", "prod", "ct-1"); err != nil {
		t.Fatalf("cancel: %v", err)
	}
}

func TestNewClientRejectsEmpty(t *testing.T) {
	if _, err := NewClient("", time.Second); err == nil {
		t.Fatal("empty base url should error")
	}
}
