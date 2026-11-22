package logic

// adminorderlogic.go 剩余补漏分支：ListOrders/GetOrder/CreateRefund/ListRefunds/
// ListAudit 的角色守卫 403（requireAdmin）、ListByQuery 查询失败 500、GetOrder
// 查无 404、CreateRefund 的动作非法 400 / Get 失败 500 / 金额超实付 400 /
// CreateRefund 写入失败 500（只读连接：Get 成功、写入失败）。

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"
)

// projectionSvcAt 只建订单投影表的上下文（目录存根返回 role/partner 1）。
func projectionSvcAt(t *testing.T, role, dbPath string) (*svc.ServiceContext, *model.OrderProjectionModel) {
	t.Helper()
	st := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"role":"` + role + `","partner_id":1}`))
	}))
	t.Cleanup(st.Close)
	sc := testSvc(st.URL)
	db := openDB(t, "file:"+dbPath)
	if err := model.NewOrderProjectionModel(db).CreateTables(); err != nil {
		t.Fatalf("projection tables: %v", err)
	}
	m := model.NewOrderProjectionModel(db)
	sc.ProjectionDB = m
	return sc, m
}

// mustOrderRow 落一条已付款订单投影（退款守卫要求 PAID）。
func mustOrderRow(t *testing.T, m *model.OrderProjectionModel, orderID string, amount int64) {
	t.Helper()
	if err := m.UpsertFromOrder(&model.OrderProjection{
		OrderId: orderID, GameId: "game-a", PartnerId: 2, AccountId: "acc1",
		SkuId: "sk1", ProductId: "p1", Form: "form", AmountCents: amount,
		BaseQuantity: 1, Currency: "CNY", Status: "PAID",
	}); err != nil {
		t.Fatalf("upsert projection: %v", err)
	}
}

func TestAdminOrder_GuardArms(t *testing.T) {
	// 玩家角色 → requireAdmin 403（GetOrder/CreateRefund/ListRefunds/ListAudit）
	sc, _ := projectionSvcAt(t, "player", filepath.Join(t.TempDir(), "ao-guard.db"))
	l := NewAdminOrderLogic(wbCtx(), sc)
	if _, err := l.GetOrder(&types.OrderPathRequest{OrderId: "o1"}, "Bearer t"); err == nil {
		t.Fatal("player get order = nil error, want 403")
	}
	if _, err := l.CreateRefund(&types.CreateRefundRequest{OrderId: "o1"}, "Bearer t"); err == nil {
		t.Fatal("player refund = nil error, want 403")
	}
	if _, err := l.ListRefunds(&types.RefundListRequest{}, "Bearer t"); err == nil {
		t.Fatal("player list refunds = nil error, want 403")
	}
	if _, err := l.ListAudit(&types.AuditListRequest{}, "Bearer t"); err == nil {
		t.Fatal("player list audit = nil error, want 403")
	}
}

func TestAdminOrder_QueryErrorArms(t *testing.T) {
	// 闭库：ListOrders / CreateRefund 的首个查询即败 → 500
	sc, _ := projectionSvcAt(t, "platform_admin", filepath.Join(t.TempDir(), "ao-qe.db"))
	sc.ProjectionDB = model.NewOrderProjectionModel(mustClosedDB(t))
	l := NewAdminOrderLogic(wbCtx(), sc)
	if _, err := l.ListOrders(&types.AdminOrderQueryRequest{}, "Bearer t"); err == nil {
		t.Fatal("list orders closed = nil error, want 500")
	}
	if _, err := l.CreateRefund(&types.CreateRefundRequest{OrderId: "o1"}, "Bearer t"); err == nil {
		t.Fatal("refund closed = nil error, want 500")
	}
	// 动作非法 → 400（守卫过、校验先于查库）
	if _, err := l.CreateRefund(&types.CreateRefundRequest{OrderId: "o1", Action: "VOID"}, "Bearer t"); err == nil {
		t.Fatal("bad action = nil error, want 400")
	}
}

func TestAdminOrder_RequireAdminServiceDown(t *testing.T) {
	// requireAdmin：角色服务不可达 → 502（"角色服务不可用"）
	sc := testSvc("http://127.0.0.1:1")
	if _, err := NewAdminOrderLogic(wbCtx(), sc).GetOrder(&types.OrderPathRequest{OrderId: "o1"}, "Bearer t"); err == nil {
		t.Fatal("catalog down = nil error, want 502")
	}
}

func TestAdminOrder_RefundGuardArms(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "ao-rg.db")
	sc, m := projectionSvcAt(t, "platform_admin", dbPath)
	mustOrderRow(t, m, "o1", 100)
	l := NewAdminOrderLogic(wbCtx(), sc)

	// 查无订单 → 404
	if _, err := l.GetOrder(&types.OrderPathRequest{OrderId: "nope"}, "Bearer t"); err == nil {
		t.Fatal("get missing = nil error, want 404")
	}
	if _, err := l.CreateRefund(&types.CreateRefundRequest{OrderId: "nope"}, "Bearer t"); err == nil {
		t.Fatal("refund missing = nil error, want 404")
	}
	// 金额超实付 → 400
	if _, err := l.CreateRefund(&types.CreateRefundRequest{OrderId: "o1", AmountCents: 500}, "Bearer t"); err == nil {
		t.Fatal("overpay refund = nil error, want 400")
	}
	// 只读连接：Get ok（PAID 行在），CreateRefund 写入败 → 500
	ro := openDB(t, "file:"+dbPath+"?mode=ro")
	sc.ProjectionDB = model.NewOrderProjectionModel(ro)
	if _, err := l.CreateRefund(&types.CreateRefundRequest{OrderId: "o1", AmountCents: 100}, "Bearer t"); err == nil {
		t.Fatal("refund ro = nil error, want 500")
	}
}
