package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/cuihairu/courier/gateway/providers/teller"
)

// TestSkusFromEnv_Valid 解析合法 COURIER_TELLER_SKUS（对齐 spawn
// payment_products 的 id/金额/币种）。
func TestSkusFromEnv_Valid(t *testing.T) {
	t.Setenv("COURIER_TELLER_SKUS", `[{"id":"p_sandbox_1","productId":"p_sandbox_1","form":"DIRECT_PURCHASE","amountCents":1000,"currency":"CNY"}]`)
	got, err := skusFromEnv()
	if err != nil {
		t.Fatalf("skusFromEnv() error = %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	s := got[0]
	if s.ID != "p_sandbox_1" || s.ProductId != "p_sandbox_1" ||
		s.Form != "DIRECT_PURCHASE" || s.AmountCents != 1000 || s.Currency != "CNY" {
		t.Fatalf("unexpected sku: %+v", s)
	}
}

// TestSkusFromEnv_Empty 未设置 = nil（不登记，与冻结网关一致）。
func TestSkusFromEnv_Empty(t *testing.T) {
	os.Unsetenv("COURIER_TELLER_SKUS")
	got, err := skusFromEnv()
	if err != nil {
		t.Fatalf("skusFromEnv() error = %v", err)
	}
	if got != nil {
		t.Fatalf("got %+v, want nil", got)
	}
}

// TestSkusFromEnv_Invalid 语法错误必须返回 error（fail-fast，不静默空表）。
func TestSkusFromEnv_Invalid(t *testing.T) {
	t.Setenv("COURIER_TELLER_SKUS", `{not-json`)
	if _, err := skusFromEnv(); err == nil {
		t.Fatal("skusFromEnv() = nil error, want error")
	}
}

// TestDeliverHook_PostsSignedPayload 验证 Deliver 钩子：POST 到 webhook、
// 请求体为 delivery.Instruction 同构 camelCase JSON、X-Delivery-Signature =
// hex(HMAC-SHA256(secret, rawBody))、成功返 nil。
func TestDeliverHook_PostsSignedPayload(t *testing.T) {
	const secret = "delivery-secret-2026"
	var gotBody []byte
	var gotSig string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("Content-Type = %q, want application/json", r.Header.Get("Content-Type"))
		}
		gotSig = r.Header.Get("X-Delivery-Signature")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()
	t.Setenv("COURIER_PAYMENTS_DELIVER_WEBHOOK", ts.URL)
	t.Setenv("COURIER_PAYMENTS_DELIVER_SECRET", secret)

	hook := deliverHook()
	if hook == nil {
		t.Fatal("deliverHook() = nil, want non-nil")
	}
	d := teller.Delivery{
		OrderID: "ord-1", AccountID: "acc-1", SkuID: "p_sandbox_1",
		ProductID: "p_sandbox_1", Form: "DIRECT_PURCHASE",
		AmountCents: 1000, Currency: "CNY",
	}
	if err := hook(d); err != nil {
		t.Fatalf("hook(d) error = %v", err)
	}

	// 载荷形状：camelCase 字段名（对齐 spawn delivery.Instruction）。
	var payload map[string]any
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	want := map[string]any{
		"orderId": "ord-1", "accountId": "acc-1", "skuId": "p_sandbox_1",
		"productId": "p_sandbox_1", "form": "DIRECT_PURCHASE",
		"amountCents": float64(1000), "currency": "CNY",
	}
	for k, v := range want {
		if payload[k] != v {
			t.Errorf("body[%q] = %v, want %v", k, payload[k], v)
		}
	}

	// 签名：hex(HMAC-SHA256(secret, rawBody))。
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(gotBody)
	if gotSig != hex.EncodeToString(mac.Sum(nil)) {
		t.Errorf("signature = %q, want hmac-sha256", gotSig)
	}
}

// TestDeliverHook_Non200 非 2xx 返回 error（teller 按契约留 PAID 待对账）。
func TestDeliverHook_Non200(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer ts.Close()
	t.Setenv("COURIER_PAYMENTS_DELIVER_WEBHOOK", ts.URL)
	t.Setenv("COURIER_PAYMENTS_DELIVER_SECRET", "s")
	hook := deliverHook()
	if err := hook(teller.Delivery{OrderID: "ord-1"}); err == nil {
		t.Fatal("hook() = nil error, want error on non-200")
	}
}

// TestDeliverHook_Unconfigured 未配置 webhook / 密钥 = nil（沙箱裸跑语义，
// 与冻结网关 nil Deliver 一致；部分配置也不静默降级）。
func TestDeliverHook_Unconfigured(t *testing.T) {
	os.Unsetenv("COURIER_PAYMENTS_DELIVER_WEBHOOK")
	os.Unsetenv("COURIER_PAYMENTS_DELIVER_SECRET")
	if h := deliverHook(); h != nil {
		t.Fatal("deliverHook() with no webhook = non-nil, want nil")
	}
	t.Setenv("COURIER_PAYMENTS_DELIVER_WEBHOOK", "http://x")
	os.Unsetenv("COURIER_PAYMENTS_DELIVER_SECRET")
	if h := deliverHook(); h != nil {
		t.Fatal("deliverHook() with webhook but no secret = non-nil, want nil")
	}
}

// TestAssemble_AuthFailClosed 装配的玩家支付端点对缺失 Bearer 恒 401
// （RequireAuth fail-closed 接线生效，不会裸奔）。
func TestAssemble_AuthFailClosed(t *testing.T) {
	h, err := assemble(nil)
	if err != nil {
		t.Fatalf("assemble() error = %v", err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	req, err := http.NewRequest(http.MethodGet, srv.URL+"/v1/payments/orders", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	// scope 只走 header（冻结契约），且先于 auth 校验：不给 scope 头会
	// 先命中 400 COMMON_INVALID_ARGUMENT，故补上合法 scope 再断言 401。
	req.Header.Set("X-Courier-Game-Id", "game_demo")
	req.Header.Set("X-Courier-Env", "dev")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET orders: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body=%s, want 401", resp.StatusCode, body)
	}
}

// adminGet 向装配发起管理面检索（GET /v1/payments/admin/orders，可带 Bearer）。
func adminGet(t *testing.T, srvURL, bearer string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srvURL+"/v1/payments/admin/orders", nil)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	// 管理面 S2S 仍过 scope 中间件（先于管理凭证校验）：补合法 scope 头，
	// 与 spawn payments courier 客户端同口径（经映射解析后的 scope）。
	req.Header.Set("X-Courier-Game-Id", "game_demo")
	req.Header.Set("X-Courier-Env", "dev")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET admin orders: %v", err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	return resp
}

// TestAssemble_AdminFailClosed 管理凭证未配置：管理端点一律 401（忘注入
// 不能变成开放管理面，GP-204 部署激活的 fail-closed 口径）。
func TestAssemble_AdminFailClosed(t *testing.T) {
	t.Setenv("COURIER_PAYMENTS_ADMIN_TOKEN", "")
	h, err := assemble(nil)
	if err != nil {
		t.Fatalf("assemble() error = %v", err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()
	if resp := adminGet(t, srv.URL, ""); resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("no token: status = %d body=%s, want 401", resp.StatusCode, body)
	}
	if resp := adminGet(t, srv.URL, "guess"); resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong token: status = %d, want 401", resp.StatusCode)
	}
}

// TestAssemble_AdminTokenWired 管理凭证接线生效：对 token 检索 200（空表
// 信封 data.items），错 token 仍 401。
func TestAssemble_AdminTokenWired(t *testing.T) {
	t.Setenv("COURIER_PAYMENTS_ADMIN_TOKEN", "admin-tok")
	h, err := assemble(nil)
	if err != nil {
		t.Fatalf("assemble() error = %v", err)
	}
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp := adminGet(t, srv.URL, "admin-tok")
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d body=%s, want 200", resp.StatusCode, body)
	}
	var payload struct {
		Data struct {
			Items []any `json:"items"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if payload.Data.Items == nil {
		t.Fatalf("items = null, want 空数组信封")
	}
	if resp2 := adminGet(t, srv.URL, "wrong"); resp2.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong token: status = %d, want 401", resp2.StatusCode)
	}
}
