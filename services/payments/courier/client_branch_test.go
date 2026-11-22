package courier

// client.go 剩余补漏分支：GetOrder/ListSkus 成功路径、ListOrders 错误臂、
// call() 的非信封/HTTP 状态/data 反序列化臂、ForwardCallback 三类错误臂，
// 以及 APIError 的 Error()/IsCode() 方法。

import (
	"errors"
	"net/http"
	"strings"
	"testing"
)

func TestClientGetOrderAndListSkus(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/payments/skus":
			_, _ = w.Write([]byte(`{"data":[{"id":"sku-1","productId":"gem","form":"DIRECT_PURCHASE","amountCents":100,"currency":"CNY"}]}`))
		case "/v1/payments/orders/order_7":
			_, _ = w.Write([]byte(`{"data":{"id":"order_7","status":"PAID","amountCents":100,"currency":"CNY"}}`))
		default:
			http.NotFound(w, r)
		}
	})
	skus, err := c.ListSkus("game-a", "prod", "Bearer tok")
	if err != nil || len(skus) != 1 || skus[0].Id != "sku-1" || skus[0].AmountCents != 100 {
		t.Fatalf("ListSkus = %+v err=%v", skus, err)
	}
	order, err := c.GetOrder("game-a", "prod", "Bearer tok", "order_7")
	if err != nil || order.Id != "order_7" || order.Status != "PAID" {
		t.Fatalf("GetOrder = %+v err=%v", order, err)
	}
}

func TestClientListOrdersErrorArm(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"COMMON_UNAUTHORIZED","message":"no"}}`))
	})
	if _, err := c.ListOrders("g", "prod", "Bearer bad"); err == nil {
		t.Fatal("ListOrders unauthorized = nil, want error")
	}
}

func TestClientCallHTTPStatusArm(t *testing.T) {
	// 信封无 error 键且状态 >= 300 → HTTP_ 错误码
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"data":{}}`))
	})
	_, err := c.CreateOrder("g", "prod", "Bearer t", "s")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Code != "HTTP_Bad Gateway" {
		t.Fatalf("err = %v, want HTTP_ APIError", err)
	}
}

func TestClientCallNonEnvelopeArm(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>not json</html>`))
	})
	_, err := c.ListSkus("g", "prod", "Bearer t")
	if err == nil || !strings.Contains(err.Error(), "非信封") {
		t.Fatalf("err = %v, want non-envelope", err)
	}
}

func TestClientCallDataUnmarshalArm(t *testing.T) {
	// data 形状与目标类型不符 → 反序列化失败
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"unexpected":"object"}}`))
	})
	if _, err := c.ListSkus("g", "prod", "Bearer t"); err == nil {
		t.Fatal("ListSkus mismatched data = nil, want error")
	}
}

func TestForwardCallbackErrorArms(t *testing.T) {
	// 信封 error（契约错误码）
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":{"code":"PAYMENT_INVALID_SIGNATURE","message":"bad"}}`))
	})
	err := c.ForwardCallback([]byte(`{}`), "deadbeef")
	apiErr, ok := err.(*APIError)
	if !ok || !apiErr.IsCode("PAYMENT_INVALID_SIGNATURE") {
		t.Fatalf("forward envelope err = %v", err)
	}

	// 纯 HTTP 状态（无 error 键）
	c2 := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"data":{}}`))
	})
	if err := c2.ForwardCallback([]byte(`{}`), "x"); err == nil {
		t.Fatal("forward http err = nil, want error")
	}

	// 非信封响应
	c3 := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	})
	if err := c3.ForwardCallback([]byte(`{}`), "x"); err == nil {
		t.Fatal("forward non-envelope = nil, want error")
	}
}

func TestAPIErrorErrorString(t *testing.T) {
	e := &APIError{HTTPStatus: 404, Code: "X", Message: "boom"}
	if got := e.Error(); got != "courier X: boom" {
		t.Fatalf("Error() = %q", got)
	}
	var target *APIError
	if !errors.As(e, &target) || !target.IsCode("X") {
		t.Fatal("errors.As chain broken")
	}
}
