package logic

// callbacklogic.go 剩余补漏分支：Handle 的签名无效 403、转发 Courier 失败 502，
// 以及 projectPaid / redeemOrderCoupons / emitOrderPaid / enqueueRewardEvaluation
// 的解析失败臂、查询失败臂与入队失败臂（各失败仅记日志，不阻断回调）。
//
// 构造法：沙箱验签用 SandboxHMAC 存根（签名即 HMAC(secret, rawBody)）；转发
// 成功用返回 200 信封的 Courier 存根，失败用 127.0.0.1:1 死端口；投影/券/发货
// 换已关闭 sqlite 制造查询失败；analytics 与 PromotionBridge 指向死端口使
// Enabled()=true（parse 失败臂需先过 Enabled 守卫）。

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/courier"
	"github.com/tappi/tappi/services/payments/internal/analytics"
	"github.com/tappi/tappi/services/payments/internal/channel"
	"github.com/tappi/tappi/services/payments/internal/promotion"
	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/model"
)

const cbSecret = "cb-test-secret"

// cbSandboxSvc 只配沙箱适配器的最小上下文。
func cbSandboxSvc(t *testing.T) *svc.ServiceContext {
	t.Helper()
	sc := testSvc("http://127.0.0.1:1")
	sc.Sandbox = &channel.SandboxHMAC{Secret: []byte(cbSecret)}
	sc.Real = channel.NewRealChannel("wechat")
	sc.Courier = courier.NewClient("http://127.0.0.1:1", 300*time.Millisecond)
	return sc
}

// cbCourierStub 返回 200 信封的 Courier 回调存根（转发成功面）。
func cbCourierStub(t *testing.T) *httptest.Server {
	t.Helper()
	st := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
	}))
	t.Cleanup(st.Close)
	return st
}

// cbSig 沙箱 HMAC 签名（与 SandboxHMAC 镜像）。
func cbSig(raw []byte) string {
	mac := hmac.New(sha256.New, []byte(cbSecret))
	mac.Write(raw)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestCallback_HandleVerifyAndForwardArms(t *testing.T) {
	// 签名无效 → 403（验签先于解析，fail-closed）
	sc := cbSandboxSvc(t)
	if err := NewCallbackLogic(wbCtx(), sc).Handle("sandbox", []byte(`{"orderId":"o1"}`), "bad-sig", nil); err == nil {
		t.Fatal("bad signature = nil error, want 403")
	}
	// 转发 Courier 不可达 → 502（验签过、转发失败）
	sc.Courier = courier.NewClient("http://127.0.0.1:1", 300*time.Millisecond)
	body := []byte(`{"orderId":"o1"}`)
	if err := NewCallbackLogic(wbCtx(), sc).Handle("sandbox", body, cbSig(body), nil); err == nil {
		t.Fatal("forward down = nil error, want 502")
	}
}

func TestCallback_ParseFailArms(t *testing.T) {
	// 无 orderId 的空对象：四个解析失败臂各自提前返回（不阻断回调，仍 nil）
	sc := cbSandboxSvc(t)
	sc.Courier = courier.NewClient(cbCourierStub(t).URL, 2*time.Second)
	sc.AnalyticsSvc = analytics.NewClient("http://127.0.0.1:1", "t", 100, nil) // Enabled
	sc.PromotionBridge = promotion.NewBridge(model.NewPromotionEvalModel(mustClosedDB(t)), "http://127.0.0.1:1", "s", 100)
	body := []byte(`{}`)
	if err := NewCallbackLogic(wbCtx(), sc).Handle("sandbox", body, cbSig(body), nil); err != nil {
		t.Fatalf("empty body = %v, want nil (parse-fail arms logged)", err)
	}
}

func TestCallback_BackendErrorArms(t *testing.T) {
	// 有 orderId：投影 MarkPaid 失败 / 投影 Get 失败 / 券核销失败 / 评估 Get 失败
	// → 各自 Errorf 臂，回调仍 nil（fire-and-forget 不阻断）
	dbPath := filepath.Join(t.TempDir(), "cb-be.db")
	db := openDB(t, "file:"+dbPath)
	sc := cbSandboxSvc(t)
	sc.Courier = courier.NewClient(cbCourierStub(t).URL, 2*time.Second)
	sc.AnalyticsSvc = analytics.NewClient("http://127.0.0.1:1", "t", 100, nil)
	sc.PromotionBridge = promotion.NewBridge(model.NewPromotionEvalModel(mustClosedDB(t)), "http://127.0.0.1:1", "s", 100)
	sc.DeliveryDB = model.NewDeliveryModel(mustClosedDB(t))
	sc.CouponDB = model.NewCouponModel(mustClosedDB(t))
	sc.ProjectionDB = model.NewOrderProjectionModel(mustClosedDB(t))
	body := []byte(`{"orderId":"o1","paidAt":"2026-11-22T00:00:00Z"}`)
	if err := NewCallbackLogic(wbCtx(), sc).Handle("sandbox", body, cbSig(body), nil); err != nil {
		t.Fatalf("backend down = %v, want nil", err)
	}
	_ = db
}

func TestCallback_EnqueueErrorArm(t *testing.T) {
	// 投影/发货/券全通，仅评估队列入队失败 → Errorf 臂（回调仍 nil）
	dbPath := filepath.Join(t.TempDir(), "cb-eq.db")
	db := openDB(t, "file:"+dbPath)
	if err := model.NewOrderProjectionModel(db).CreateTables(); err != nil {
		t.Fatalf("projection tables: %v", err)
	}
	if err := model.NewCouponModel(db).CreateTables(); err != nil {
		t.Fatalf("coupon tables: %v", err)
	}
	proj := model.NewOrderProjectionModel(db)
	if err := proj.UpsertFromOrder(&model.OrderProjection{
		OrderId: "o-eq", GameId: "game-a", AccountId: "7", SkuId: "sk1", ProductId: "p1",
		AmountCents: 1000, BaseQuantity: 1, Currency: "CNY", Status: "PAID", PaidAt: "2026-11-22T00:00:00Z",
	}); err != nil {
		t.Fatalf("seed projection: %v", err)
	}
	sc := cbSandboxSvc(t)
	sc.Courier = courier.NewClient(cbCourierStub(t).URL, 2*time.Second)
	sc.AnalyticsSvc = analytics.NewClient("http://127.0.0.1:1", "t", 100, nil)
	sc.PromotionBridge = promotion.NewBridge(model.NewPromotionEvalModel(mustClosedDB(t)), "http://127.0.0.1:1", "s", 100)
	sc.ProjectionDB = proj
	sc.CouponDB = model.NewCouponModel(db)
	sc.DeliveryDB = model.NewDeliveryModel(db)
	body := []byte(`{"orderId":"o-eq","paidAt":"2026-11-22T00:00:00Z"}`)
	if err := NewCallbackLogic(wbCtx(), sc).Handle("sandbox", body, cbSig(body), nil); err != nil {
		t.Fatalf("enqueue fail = %v, want nil", err)
	}
}
