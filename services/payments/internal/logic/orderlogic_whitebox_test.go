package logic

// orderlogic.go 白盒剩余分支：envOr 非空透传、CreateOrder 的 区服目录错误
// （502）与 Courier 下单失败（502）、GetOrder 的 Courier 成功/失败臂、
// ListMyOrders 的 Courier 失败臂、ListSkus 的查询失败臂、releaseOrderCoupons
// 的回滚失败记日志臂。
//
// 通过构造最小 svcCtx（按需注入真实/闭库 model 与 courier client）逐臂覆盖，
// 不经 HTTP 层（handler 已集成验证装配，这里只补逻辑臂）。

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/courier"
	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"

	_ "github.com/mattn/go-sqlite3"
)

func TestEnvOr_Passthrough(t *testing.T) {
	if got := envOr("staging"); got != "staging" {
		t.Fatalf("envOr(staging) = %q", got)
	}
}

func newWhiteboxProductDB(t *testing.T) *model.ProductModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "wb.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := model.NewProductModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("tables: %v", err)
	}
	return m
}

func wbCtx() context.Context {
	return context.WithValue(context.Background(), "user_id", int64(7))
}

// CreateOrder：区服目录不可达 → 502（ServerId>0 触发 catalogServerOpen）。
func TestCreateOrder_CatalogUnreachable(t *testing.T) {
	sc := &svc.ServiceContext{}
	sc.ProductDB = newWhiteboxProductDB(t)
	sc.Config.GameCatalog.BaseURL = "http://127.0.0.1:1" // 连接拒绝
	sc.Courier = courier.NewClient("http://127.0.0.1:1", time.Second)
	if _, err := sc.ProductDB.Create(&model.Product{
		Id: "sku-cat", ProductId: "p", Form: model.FormDirectPurchase,
		AmountCents: 100, Currency: "CNY", GameId: "game-a", PartnerId: 2,
	}); err != nil {
		t.Fatalf("create product: %v", err)
	}
	_, err := NewOrderLogic(wbCtx(), sc).CreateOrder(&types.CreateOrderRequest{
		GameId: "game-a", SkuId: "sku-cat", ServerId: 9,
	}, "Bearer t")
	apiErr, ok := err.(*apiError)
	if !ok || apiErr.StatusCode() != http.StatusBadGateway {
		t.Fatalf("catalog unreachable err = %v, want 502", err)
	}
}

// CreateOrder：Courier 下单失败 → 502（ServerId=0 跳过目录校验）。
func TestCreateOrder_CourierError(t *testing.T) {
	sc := &svc.ServiceContext{}
	sc.ProductDB = newWhiteboxProductDB(t)
	sc.Courier = courier.NewClient("http://127.0.0.1:1", time.Second) // 下单必败
	if _, err := sc.ProductDB.Create(&model.Product{
		Id: "sku-courier", ProductId: "p", Form: model.FormDirectPurchase,
		AmountCents: 100, Currency: "CNY", GameId: "game-a", PartnerId: 2,
	}); err != nil {
		t.Fatalf("create product: %v", err)
	}
	_, err := NewOrderLogic(wbCtx(), sc).CreateOrder(&types.CreateOrderRequest{
		GameId: "game-a", SkuId: "sku-courier",
	}, "Bearer t")
	apiErr, ok := err.(*apiError)
	if !ok || apiErr.StatusCode() != http.StatusBadGateway {
		t.Fatalf("courier err = %v, want 502", err)
	}
}

// GetOrder：Courier 成功（返回订单）与失败（非信封 → courierError）。
func TestGetOrder_BothArms(t *testing.T) {
	okStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"id":"o1","status":"PAID","amountCents":100,"currency":"CNY"}}`))
	}))
	t.Cleanup(okStub.Close)
	sc := &svc.ServiceContext{Courier: courier.NewClient(okStub.URL, time.Second)}
	resp, err := NewOrderLogic(wbCtx(), sc).GetOrder(&types.OrderQueryRequest{GameId: "game-a", OrderId: "o1"}, "Bearer t")
	if err != nil || resp.Order.Id != "o1" || resp.Order.Status != "PAID" {
		t.Fatalf("get ok = %+v err=%v", resp, err)
	}

	badStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`not-json`))
	}))
	t.Cleanup(badStub.Close)
	sc2 := &svc.ServiceContext{Courier: courier.NewClient(badStub.URL, time.Second)}
	if _, err := NewOrderLogic(wbCtx(), sc2).GetOrder(&types.OrderQueryRequest{GameId: "game-a", OrderId: "o1"}, "Bearer t"); err == nil {
		t.Fatal("get fail = nil error, want error")
	}
}

// ListMyOrders：Courier 失败 → courierError。
func TestListMyOrders_CourierError(t *testing.T) {
	stub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":"COMMON_UNAUTHORIZED","message":"no"}}`))
	}))
	t.Cleanup(stub.Close)
	sc := &svc.ServiceContext{Courier: courier.NewClient(stub.URL, time.Second)}
	if _, err := NewOrderLogic(wbCtx(), sc).ListMyOrders("game-a", "", "Bearer bad"); err == nil {
		t.Fatal("list my orders = nil error, want error")
	}
}

// ListSkus：商品查询失败 → 500。
func TestListSkus_QueryError(t *testing.T) {
	sc := &svc.ServiceContext{}
	sc.ProductDB = model.NewProductModel(mustClosedDB(t))
	if _, err := NewOrderLogic(wbCtx(), sc).ListSkus("game-a"); err == nil {
		t.Fatal("list skus = nil error, want error")
	}
}

// releaseOrderCoupons：回滚失败仅记日志（不 panic）。
func TestReleaseOrderCoupons_Error(t *testing.T) {
	sc := &svc.ServiceContext{CouponDB: model.NewCouponModel(mustClosedDB(t))}
	NewOrderLogic(wbCtx(), sc).releaseOrderCoupons("order_x", []int64{1})
}

func mustClosedDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "closed.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return db
}
