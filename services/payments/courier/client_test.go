package courier

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newTestServer(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewClient(srv.URL, time.Second)
}

func TestClientScopeHeadersAndEnvelope(t *testing.T) {
	var gotGameID, gotEnv, gotAuth string
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotGameID = r.Header.Get("X-Courier-Game-Id")
		gotEnv = r.Header.Get("X-Courier-Env")
		gotAuth = r.Header.Get("Authorization")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"id": "order_1", "status": "CREATED", "amountCents": 100, "currency": "CNY"},
		})
	})
	order, err := c.CreateOrder("game-a", "staging", "Bearer tok", "sku-1")
	if err != nil {
		t.Fatalf("create order: %v", err)
	}
	if gotGameID != "game-a" || gotEnv != "staging" || gotAuth != "Bearer tok" {
		t.Fatalf("scope headers: game=%s env=%s auth=%s", gotGameID, gotEnv, gotAuth)
	}
	if order.Id != "order_1" || order.AmountCents != 100 || order.Currency != "CNY" {
		t.Fatalf("order = %+v", order)
	}
}

func TestClientErrorCode(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error": map[string]any{"code": "PAYMENT_SKU_NOT_FOUND", "message": "no such sku"},
		})
	})
	_, err := c.CreateOrder("game-a", "prod", "Bearer tok", "sku-x")
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok || !apiErr.IsCode("PAYMENT_SKU_NOT_FOUND") {
		t.Fatalf("err = %v", err)
	}
}

func TestForwardCallbackSetsSignatureAndScope(t *testing.T) {
	var gotSig, gotGame, gotEnv, gotBody string
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Payment-Signature")
		gotGame = r.Header.Get("X-Courier-Game-Id")
		gotEnv = r.Header.Get("X-Courier-Env")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
	})
	body := []byte(`{"orderId":"order_9"}`)
	if err := c.ForwardCallback(body, "abc123", "cg9", "dev"); err != nil {
		t.Fatalf("forward: %v", err)
	}
	if gotSig != "abc123" {
		t.Fatalf("sig = %s", gotSig)
	}
	// 网关 scope 中间件对 callback 强制要 scope 头（缺则 400），转发必须携带
	if gotGame != "cg9" || gotEnv != "dev" {
		t.Fatalf("scope headers = %q/%q, want cg9/dev", gotGame, gotEnv)
	}
	if !strings.Contains(gotBody, "order_9") {
		t.Fatalf("body = %s", gotBody)
	}
}

func TestForwardCallbackMissingScopeFailsClosed(t *testing.T) {
	// scope 头缺任一个 → 本层拒绝，不发请求（防静默回退成必 400 的转发）。
	c := NewClient("http://127.0.0.1:1", time.Second)
	if err := c.ForwardCallback([]byte(`{}`), "x", "", "dev"); err == nil {
		t.Fatal("empty game = nil, want error")
	}
	if err := c.ForwardCallback([]byte(`{}`), "x", "cg", ""); err == nil {
		t.Fatal("empty env = nil, want error")
	}
}
