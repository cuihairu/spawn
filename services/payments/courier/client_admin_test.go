package courier

// client.go 管理面（v1.2 /v1/payments/admin/*）客户端测试：五方法成功路径 +
// 请求头/请求体/查询参数装配 + 空管理凭证 fail-closed（不设 Authorization 头，
// 网关 401 原样上抛）。

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

const adminTok = "admintok"

func TestAdminSkuRegistersAndHeaders(t *testing.T) {
	var gotGame, gotEnv, gotAuth, gotCT string
	var gotBody map[string]any
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotGame = r.Header.Get("X-Courier-Game-Id")
		gotEnv = r.Header.Get("X-Courier-Env")
		gotAuth = r.Header.Get("Authorization")
		gotCT = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"data":{"sku":{"id":"s1","productId":"gem","form":"DIRECT_PURCHASE","amountCents":600,"currency":"CNY"}}}`))
	})
	sku, err := c.AdminSku("game-a", "prod", adminTok, &AdminSkuReq{
		SkuId: "s1", ProductId: "gem", Form: "DIRECT_PURCHASE", AmountCents: 600, Currency: "CNY",
		GameID: "game-a", Env: "prod",
	})
	if err != nil {
		t.Fatalf("AdminSku: %v", err)
	}
	if gotGame != "game-a" || gotEnv != "prod" || gotAuth != "Bearer "+adminTok {
		t.Fatalf("headers game=%s env=%s auth=%s", gotGame, gotEnv, gotAuth)
	}
	if !strings.HasPrefix(gotCT, "application/json") {
		t.Fatalf("content-type = %q", gotCT)
	}
	if gotBody["skuId"] != "s1" || gotBody["amountCents"] != float64(600) ||
		gotBody["gameId"] != "game-a" || gotBody["env"] != "prod" {
		t.Fatalf("body = %v", gotBody)
	}
	if sku.Id != "s1" || sku.AmountCents != 600 || sku.Currency != "CNY" {
		t.Fatalf("sku = %+v", sku)
	}
}

func TestAdminListOrdersQueryParams(t *testing.T) {
	var gotQuery string
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":{"items":[],"nextCursor":""}}`))
	})
	page, err := c.AdminListOrders("game-a", "prod", adminTok, AdminOrderQuery{
		GameId: "game-a", Status: "PAID", AccountId: "acc_1", Limit: 50, Cursor: "curs",
	})
	if err != nil {
		t.Fatalf("AdminListOrders: %v", err)
	}
	if page == nil || page.NextCursor != "" {
		t.Fatalf("page = %+v", page)
	}
	for _, want := range []string{"gameId=game-a", "status=PAID", "accountId=acc_1", "limit=50", "cursor=curs"} {
		if !strings.Contains(gotQuery, want) {
			t.Fatalf("query %q 缺 %q", gotQuery, want)
		}
	}
}

func TestAdminListOrdersDefaultNoLimit(t *testing.T) {
	var gotQuery string
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		_, _ = w.Write([]byte(`{"data":{"items":[],"nextCursor":""}}`))
	})
	if _, err := c.AdminListOrders("game-a", "prod", adminTok, AdminOrderQuery{}); err != nil {
		t.Fatalf("AdminListOrders: %v", err)
	}
	if strings.Contains(gotQuery, "limit") {
		t.Fatalf("缺省不应带 limit, query = %q", gotQuery)
	}
}

func TestAdminGetOrderAndRedeliver(t *testing.T) {
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/payments/admin/orders/order_7":
			_, _ = w.Write([]byte(`{"data":{"id":"order_7","status":"PAID","accountId":"acc_1","gameId":"cg","amountCents":600,"currency":"CNY"}}`))
		case r.URL.Path == "/v1/payments/admin/orders/order_8/redeliver" && r.Method == http.MethodPost:
			_, _ = w.Write([]byte(`{"data":{"order":{"id":"order_8","status":"DELIVERED"},"redelivered":true}}`))
		default:
			http.NotFound(w, r)
		}
	})
	o, err := c.AdminGetOrder("game-a", "prod", adminTok, "order_7")
	if err != nil || o.Id != "order_7" || o.Status != "PAID" || o.AccountId != "acc_1" {
		t.Fatalf("AdminGetOrder = %+v err=%v", o, err)
	}
	rd, err := c.AdminRedeliver("game-a", "prod", adminTok, "order_8")
	if err != nil || !rd.Redelivered || rd.Order.Id != "order_8" {
		t.Fatalf("AdminRedeliver = %+v err=%v", rd, err)
	}
}

func TestAdminRefundBody(t *testing.T) {
	var gotBody map[string]any
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"data":{"orderId":"order_9","refund":{"action":"REFUND","amountCents":600,"reason":"bug","at":"2026-11-23T00:00:00Z"}}}`))
	})
	rf, err := c.AdminRefund("game-a", "prod", adminTok, "order_9", "REFUND", 600, "bug")
	if err != nil {
		t.Fatalf("AdminRefund: %v", err)
	}
	if gotBody["action"] != "REFUND" || gotBody["amountCents"] != float64(600) || gotBody["reason"] != "bug" {
		t.Fatalf("body = %v", gotBody)
	}
	if rf.OrderId != "order_9" || rf.Refund.AmountCents != 600 || rf.Refund.Action != "REFUND" {
		t.Fatalf("refund = %+v", rf)
	}
}

func TestAdminRefundOmitsEmptyReason(t *testing.T) {
	var gotBody map[string]any
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"data":{"orderId":"o","refund":{"action":"REFUND","amountCents":1,"at":"x"}}}`))
	})
	if _, err := c.AdminRefund("g", "prod", adminTok, "o", "REFUND", 1, ""); err != nil {
		t.Fatalf("AdminRefund: %v", err)
	}
	if _, ok := gotBody["reason"]; ok {
		t.Fatalf("空 reason 不应入体, body = %v", gotBody)
	}
}

func TestAdminEmptyTokenFailsClosed(t *testing.T) {
	var gotAuth string
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"COMMON_UNAUTHENTICATED","message":"管理凭证缺失"}}`))
	})
	// 空管理凭证 → 不设 Authorization 头（网关 fail-closed 401），APIError 上抛。
	_, err := c.AdminListOrders("g", "prod", "", AdminOrderQuery{})
	if err == nil {
		t.Fatal("空 token = nil, want error")
	}
	apiErr, ok := err.(*APIError)
	if !ok || !apiErr.IsCode("COMMON_UNAUTHENTICATED") || apiErr.HTTPStatus != 401 {
		t.Fatalf("err = %v, want 401 COMMON_UNAUTHENTICATED", err)
	}
	if gotAuth != "" {
		t.Fatalf("空 token 不应设 Authorization 头, got %q", gotAuth)
	}
}

func TestAdminRefundOrderState409(t *testing.T) {
	// 非 PAID 订单退款 → 409 PAYMENT_ORDER_STATE（执行器据此映射 REJECTED）。
	c := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		_, _ = w.Write([]byte(`{"error":{"code":"PAYMENT_ORDER_STATE","message":"订单未付款"}}`))
	})
	_, err := c.AdminRefund("g", "prod", adminTok, "order_1", "REFUND", 100, "")
	apiErr, ok := err.(*APIError)
	if !ok || !apiErr.IsCode("PAYMENT_ORDER_STATE") || apiErr.HTTPStatus != 409 {
		t.Fatalf("err = %v, want 409 PAYMENT_ORDER_STATE", err)
	}
}
