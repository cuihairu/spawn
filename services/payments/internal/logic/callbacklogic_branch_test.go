package logic

// callbacklogic.go 分支：Handle 的签名无效 403、回调归属解析 fail-closed 502、
// 转发 Courier 失败 502，以及 projectPaid / redeemOrderCoupons / emitOrderPaid /
// enqueueRewardEvaluation 的解析失败臂、查询失败臂与入队失败臂（回调接受后
// fire-and-forget，各失败仅记日志不阻断）。
//
// 构造法：沙箱验签用 SandboxHMAC 存根（签名即 HMAC(secret, rawBody)）；回调
// 归属解析（orderId → 订单投影 game_id → 映射）用活投影库 + 映射存根，任一
// 环节取不到即 502（网关 scope 中间件对 callback 强制要 scope 头，无法解析
// 则绝不转发）；转发成功用返回 200 信封的 Courier 存根，失败用 127.0.0.1:1
// 死端口；券/发货换已关闭 sqlite 制造查询失败；analytics 与 PromotionBridge
// 指向死端口使 Enabled()=true。

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
	"github.com/tappi/tappi/services/payments/internal/courierscope"
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
	sc.Real = channel.NewRealAdapter(channel.RealConfig{}) // 未注入材料：渠道一律 501 占位
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

// cbProjection 活投影库（建表 + 订单行）：回调归属解析第一跳（Get）。
func cbProjection(t *testing.T, orderID, gameID string) *model.OrderProjectionModel {
	t.Helper()
	db := openDB(t, "file:"+filepath.Join(t.TempDir(), "cb-proj.db"))
	proj := model.NewOrderProjectionModel(db)
	if err := proj.CreateTables(); err != nil {
		t.Fatalf("projection tables: %v", err)
	}
	if err := proj.UpsertFromOrder(&model.OrderProjection{
		OrderId: orderID, GameId: gameID, AccountId: "7", SkuId: "sk1", ProductId: "p1",
		AmountCents: 1000, BaseQuantity: 1, Currency: "CNY", Status: "CREATED",
	}); err != nil {
		t.Fatalf("seed projection: %v", err)
	}
	return proj
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
	// 转发 Courier 不可达 → 502（归属解析已通过、转发失败）
	sc.Courier = courier.NewClient("http://127.0.0.1:1", 300*time.Millisecond)
	sc.ProjectionDB = cbProjection(t, "o1", "game-a")
	sc.ScopeRes = newWhiteboxScope(t)
	body := []byte(`{"orderId":"o1"}`)
	if err := NewCallbackLogic(wbCtx(), sc).Handle("sandbox", body, cbSig(body), nil); err == nil {
		t.Fatal("forward down = nil error, want 502")
	}
}

// 归属解析三跳任一取不到 → 502，且转发存根零调用（解析不到 scope 绝不转发）。
func TestCallback_ScopeUnresolvableArms(t *testing.T) {
	var forwarded int
	stub := cbCourierStub(t)
	stub = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		forwarded++
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{}})
	}))
	t.Cleanup(stub.Close)

	sc := cbSandboxSvc(t)
	sc.Courier = courier.NewClient(stub.URL, 2*time.Second)

	// 跳一：回调体缺 orderId（投影/映射未装配，nil 守卫先挡）。
	body := []byte(`{}`)
	if err := NewCallbackLogic(wbCtx(), sc).Handle("sandbox", body, cbSig(body), nil); err == nil {
		t.Fatal("no orderId = nil error, want 502")
	}

	// 跳二：订单投影缺失（库建表但无该单）
	sc.ProjectionDB = cbProjection(t, "other", "game-a")
	body = []byte(`{"orderId":"o1"}`)
	if err := NewCallbackLogic(wbCtx(), sc).Handle("sandbox", body, cbSig(body), nil); err == nil {
		t.Fatal("projection missing = nil error, want 502")
	}

	// 跳三：映射缺失（404 → ErrMappingNotFound）
	sc.ProjectionDB = cbProjection(t, "o1", "game-a")
	scopeStub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(scopeStub.Close)
	sc.ScopeRes = courierscope.NewClient(scopeStub.URL, "tok", 2*time.Second)
	if err := NewCallbackLogic(wbCtx(), sc).Handle("sandbox", body, cbSig(body), nil); err == nil {
		t.Fatal("mapping missing = nil error, want 502")
	}

	// 跳四（解析已装配后仍取不到 orderId）：表单体缺 orderId 与完全不可解析
	// 体（callbackOrderFields JSON 失败回退表单，两处皆空即 fail-closed）。
	formNoOrder := []byte(`paidAt=20261127T000000Z&sign_type=RSA2`)
	if err := NewCallbackLogic(wbCtx(), sc).Handle("sandbox", formNoOrder, cbSig(formNoOrder), nil); err == nil {
		t.Fatal("form body without orderId = nil error, want 502")
	}
	garbage := []byte("not-json and not-form %zz")
	if err := NewCallbackLogic(wbCtx(), sc).Handle("sandbox", garbage, cbSig(garbage), nil); err == nil {
		t.Fatal("unparseable body = nil error, want 502")
	}

	if forwarded != 0 {
		t.Fatalf("forward calls = %d, want 0 (scope 解析失败不应转发)", forwarded)
	}
}

func TestCallback_BackendErrorArms(t *testing.T) {
	// 归属解析三跳全通 + 转发 200：券核销失败 / 评估入队失败两臂各自
	// Errorf，回调仍 nil（fire-and-forget 不阻断）。
	sc := cbSandboxSvc(t)
	sc.Courier = courier.NewClient(cbCourierStub(t).URL, 2*time.Second)
	sc.ProjectionDB = cbProjection(t, "o1", "game-a")
	sc.ScopeRes = newWhiteboxScope(t)
	sc.AnalyticsSvc = analytics.NewClient("http://127.0.0.1:1", "t", 100, nil) // Enabled
	sc.PromotionBridge = promotion.NewBridge(model.NewPromotionEvalModel(mustClosedDB(t)), "http://127.0.0.1:1", "s", 100)
	sc.DeliveryDB = model.NewDeliveryModel(mustClosedDB(t))
	sc.CouponDB = model.NewCouponModel(mustClosedDB(t))
	body := []byte(`{"orderId":"o1","paidAt":"2026-11-22T00:00:00Z"}`)
	if err := NewCallbackLogic(wbCtx(), sc).Handle("sandbox", body, cbSig(body), nil); err != nil {
		t.Fatalf("backend down = %v, want nil", err)
	}
	// 表单体回退臂（GP-202）：支付宝异步通知为表单承载，orderId/paidAt 经
	// callbackOrderFields 表单解析取出，下游链路（归属解析/投影/核销/入队）同链可过。
	formBody := []byte(`orderId=o1&paidAt=20261122T000000Z&sign_type=RSA2`)
	if err := NewCallbackLogic(wbCtx(), sc).Handle("sandbox", formBody, cbSig(formBody), nil); err != nil {
		t.Fatalf("form body = %v, want nil", err)
	}
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
	sc.ScopeRes = newWhiteboxScope(t)
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

// 投影的解析失败臂与查询失败臂：归属解析前置后须先 Get 成功才进回调流程，
// 故直呼私有方法覆盖（已关闭 sqlite 制造查询失败；各失败仅记日志）。
func TestCallback_DownstreamProjectionFailArms(t *testing.T) {
	sc := cbSandboxSvc(t)
	sc.AnalyticsSvc = analytics.NewClient("http://127.0.0.1:1", "t", 100, nil)
	sc.PromotionBridge = promotion.NewBridge(model.NewPromotionEvalModel(mustClosedDB(t)), "http://127.0.0.1:1", "s", 100)
	sc.ProjectionDB = model.NewOrderProjectionModel(mustClosedDB(t))
	sc.CouponDB = model.NewCouponModel(mustClosedDB(t))
	l := NewCallbackLogic(wbCtx(), sc)
	// 解析失败臂（无 orderId）：各提前返回
	l.projectPaid([]byte(`{}`))
	l.redeemOrderCoupons([]byte(`{}`))
	l.emitOrderPaid([]byte(`{}`))
	l.enqueueRewardEvaluation([]byte(`{}`), "sandbox")
	// 查询失败臂（有 orderId 但投影库已关）：各 Errorf 致残臂
	body := []byte(`{"orderId":"o1","paidAt":"2026-11-22T00:00:00Z"}`)
	l.projectPaid(body)
	l.emitOrderPaid(body)
	l.enqueueRewardEvaluation(body, "sandbox")
}
