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

func TestForwardCallbackSetsSignature(t *testing.T) {
	var gotSig string
	var gotBody string
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotSig = r.Header.Get("X-Payment-Signature")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		gotBody = string(buf)
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
	})
	body := []byte(`{"orderId":"order_9"}`)
	if err := c.ForwardCallback(body, "abc123"); err != nil {
		t.Fatalf("forward: %v", err)
	}
	if gotSig != "abc123" {
		t.Fatalf("sig = %s", gotSig)
	}
	if !strings.Contains(gotBody, "order_9") {
		t.Fatalf("body = %s", gotBody)
	}
}
