package logic

// orderlogic.go 白盒分支：courierError 把 Courier 契约错误码映射为语义化
// 状态码（非 APIError 一律 502，未知码 502 携带原始消息）。

import (
	"errors"
	"net/http"
	"testing"

	"github.com/tappi/tappi/services/payments/courier"
)

func TestCourierErrorArms(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"non-api-error", errors.New("boom"), http.StatusBadGateway},
		{"sku-not-found", &courier.APIError{Code: "PAYMENT_SKU_NOT_FOUND", Message: "x"}, http.StatusNotFound},
		{"order-not-found", &courier.APIError{Code: "PAYMENT_ORDER_NOT_FOUND", Message: "x"}, http.StatusNotFound},
		{"risk-rejected", &courier.APIError{Code: "PAYMENT_RISK_REJECTED", Message: "x"}, http.StatusForbidden},
		{"invalid-signature", &courier.APIError{Code: "PAYMENT_INVALID_SIGNATURE", Message: "x"}, http.StatusForbidden},
		{"order-state", &courier.APIError{Code: "PAYMENT_ORDER_STATE", Message: "x"}, http.StatusConflict},
		{"unknown-code", &courier.APIError{Code: "SOMETHING_ELSE", Message: "boom"}, http.StatusBadGateway},
	}
	for _, c := range cases {
		got := courierError(c.err)
		apiErr, ok := got.(*apiError)
		if !ok {
			t.Fatalf("%s: got %T, want *apiError", c.name, got)
		}
		if apiErr.StatusCode() != c.want {
			t.Fatalf("%s: status = %d, want %d", c.name, apiErr.StatusCode(), c.want)
		}
	}
	// 未知码 502 消息体应携带 Courier 原始 message（排障线索不丢）
	got := courierError(&courier.APIError{Code: "SOMETHING_ELSE", Message: "boom"})
	if got.Error() != "支付服务错误：boom" {
		t.Fatalf("default message = %q", got.Error())
	}
}
