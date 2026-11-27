package main

// 参考实现自检：契约三纪律逐条锁定——验签先于解析（坏签名配非法体仍 403
// 且不解析）、按 orderId 幂等（重放 200 duplicate=true 不重复发放）、
// 密钥未配置 fail-closed（合法签名也 403）。

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func sign(t *testing.T, key string, body string) string {
	t.Helper()
	mac := hmac.New(sha256.New, []byte(key))
	mac.Write([]byte(body))
	return hex.EncodeToString(mac.Sum(nil))
}

func deliver(t *testing.T, h http.Handler, body, sig string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/deliver", strings.NewReader(body))
	if sig != "" {
		req.Header.Set("X-Game-Delivery-Signature", sig)
	}
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func grantedCount(t *testing.T, h http.Handler) int {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/granted", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /granted = %d", rec.Code)
	}
	var out struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode /granted: %v body=%s", err, rec.Body.String())
	}
	return out.Count
}

func resetGranted() {
	granted.mu.Lock()
	granted.orders = make(map[string]bool)
	granted.mu.Unlock()
}

func TestMockGameServerContract(t *testing.T) {
	t.Setenv("MOCK_GAME_SECRET", "mock-game-secret-test")
	resetGranted()

	const goodBody = `{"orderId":"o_mock_1","accountId":"acc-7","serverId":4,"roleId":"r1","skuId":"sku-1"}`
	sig := sign(t, "mock-game-secret-test", goodBody)

	// 1. 合法签名 → 200 首次发放（duplicate=false）。
	rec := deliver(t, http.HandlerFunc(handleDeliver), goodBody, sig)
	if rec.Code != http.StatusOK {
		t.Fatalf("valid delivery = %d body=%s", rec.Code, rec.Body.String())
	}
	var out struct {
		Duplicate bool `json:"duplicate"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil || out.Duplicate {
		t.Fatalf("first delivery duplicate=%v err=%v body=%s", out.Duplicate, err, rec.Body.String())
	}
	if got := grantedCount(t, http.HandlerFunc(handleGranted)); got != 1 {
		t.Fatalf("granted count = %d, want 1", got)
	}

	// 2. 同 orderId 重放（幂等兜底：payments 重投/断单重发）→ 200
	// duplicate=true，不重复发放。
	rec2 := deliver(t, http.HandlerFunc(handleDeliver), goodBody, sig)
	if rec2.Code != http.StatusOK {
		t.Fatalf("replay = %d, want 200（幂等重放不得 5xx 触发无意义重投）", rec2.Code)
	}
	var out2 struct {
		Duplicate bool `json:"duplicate"`
	}
	if err := json.Unmarshal(rec2.Body.Bytes(), &out2); err != nil || !out2.Duplicate {
		t.Fatalf("replay duplicate=%v err=%v body=%s", out2.Duplicate, err, rec2.Body.String())
	}
	if got := grantedCount(t, http.HandlerFunc(handleGranted)); got != 1 {
		t.Fatalf("granted count after replay = %d, want 1（不重复发放）", got)
	}

	// 3. 不同订单照常发放。
	otherBody := strings.Replace(goodBody, "o_mock_1", "o_mock_2", 1)
	rec3 := deliver(t, http.HandlerFunc(handleDeliver), otherBody, sign(t, "mock-game-secret-test", otherBody))
	if rec3.Code != http.StatusOK || grantedCount(t, http.HandlerFunc(handleGranted)) != 2 {
		t.Fatalf("second order = %d granted=%d, want 200/2", rec3.Code, grantedCount(t, http.HandlerFunc(handleGranted)))
	}

	// 4. 篡改体配合法旧签名 → 403 且不发放。
	tampered := strings.Replace(goodBody, "acc-7", "acc-hack", 1)
	rec4 := deliver(t, http.HandlerFunc(handleDeliver), tampered, sig)
	if rec4.Code != http.StatusForbidden {
		t.Fatalf("tampered body = %d, want 403", rec4.Code)
	}

	// 5. 验签先于解析：坏签名配非法 JSON 体仍 403（绝不因解析错误改判 400）。
	rec5 := deliver(t, http.HandlerFunc(handleDeliver), `{"orderId":`, sign(t, "mock-game-secret-test", `other`))
	if rec5.Code != http.StatusForbidden {
		t.Fatalf("bad sig malformed body = %d, want 403（验签先于解析）", rec5.Code)
	}

	// 6. 缺签名头 → 403。
	rec6 := deliver(t, http.HandlerFunc(handleDeliver), goodBody, "")
	if rec6.Code != http.StatusForbidden {
		t.Fatalf("missing signature = %d, want 403", rec6.Code)
	}

	// 7. 非 POST → 405。
	rec7 := httptest.NewRequest(http.MethodGet, "/deliver", nil)
	rec7r := httptest.NewRecorder()
	handleDeliver(rec7r, rec7)
	if rec7r.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /deliver = %d, want 405", rec7r.Code)
	}

	// 8. 密钥未配置 → fail-closed：合法签名也 403。
	t.Setenv("MOCK_GAME_SECRET", "")
	rec8 := deliver(t, http.HandlerFunc(handleDeliver), goodBody, sig)
	if rec8.Code != http.StatusForbidden {
		t.Fatalf("unconfigured secret = %d, want 403（fail-closed）", rec8.Code)
	}
}
