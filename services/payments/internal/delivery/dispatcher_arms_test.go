package delivery

// dispatcher.go 剩余补漏分支：Dispatch 的空订单号校验臂、账本 Get 非
// NotFound 失败臂（闭库）、NotFound 分支里 CreateTarget 失败臂（只读，非
// UNIQUE 错误）与补建后二次 Get 失败臂（AFTER INSERT DELETE 触发器把行删
// 掉）、商户归属漂移拒绝臂（target 的 GameId 与商品不一致）、无可用发货
// 接口拒绝臂（不配 endpoint）、http.NewRequestWithContext 失败臂（非法
// endpoint URL）、游戏服不可达臂（已关停的 endpoint）、MarkDelivered 失败
// 臂（只读）、onDelivered 回调触发臂、fail() 的退避封顶 6 倍臂（attempts
// ≥ 6）与 truncate 截断臂（>256 字节响应体）。
//
// 登记不可达（不测）：json.Marshal（固定类型 map 恒可序列化）失败臂。

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/model"
)

// roDeliveryDB 以 ?mode=ro 重开同一 sqlite 文件。
func roDeliveryDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

// bigFailStub 返回 500 且响应体超 256 字节（触发 truncate 截断臂）。
func bigFailStub(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(strings.Repeat("A", 300)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestDispatch_ValidationAndGetFailArms(t *testing.T) {
	// 空订单号（含空白）→ 校验拒绝
	_, _, _, d := newStack(t, "file:"+t.TempDir()+"/d.db", "", time.Minute)
	if err := d.Dispatch(context.Background(), Instruction{OrderId: "  "}); err == nil {
		t.Fatal("empty order id = nil, want error")
	}
	// 账本 Get 非 NotFound 失败（闭库）→ 137 return err
	db, deliveries, _, d2 := newStack(t, "file:"+t.TempDir()+"/d2.db", "", time.Minute)
	seedTarget(t, deliveries, "order_1")
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if err := d2.Dispatch(context.Background(), instruct("order_1")); err == nil {
		t.Fatal("closed-db dispatch = nil, want error")
	}
}

func TestDispatch_CreateTargetErrorArms(t *testing.T) {
	// 只读：Get 查无（NotFound）→ CreateTarget 只读写失败（非 UNIQUE）→ 131
	path := t.TempDir() + "/ro.db"
	db, _, _, _ := newStack(t, "file:"+path, "", time.Minute)
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	ro := roDeliveryDB(t, path)
	roD := NewDispatcher(model.NewDeliveryModel(ro), model.NewProductModel(ro), 2000, time.Minute, nil)
	if err := roD.Dispatch(context.Background(), instruct("order_1")); err == nil {
		t.Fatal("readonly create-target = nil, want error")
	}

	// AFTER INSERT DELETE 触发器：CreateTarget 成功后行即被删，二次 Get 查无 → 134
	db2, _, _, d2 := newStack(t, "file:"+t.TempDir()+"/trig.db", "", time.Minute)
	if _, err := db2.Exec(`CREATE TRIGGER trg_wipe AFTER INSERT ON delivery_orders FOR EACH ROW
		BEGIN DELETE FROM delivery_orders WHERE order_id = NEW.order_id; END`); err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	if err := d2.Dispatch(context.Background(), instruct("order_x")); err == nil {
		t.Fatal("trigger-wiped dispatch = nil, want error")
	}
}

func TestDispatch_GuardArms(t *testing.T) {
	g := newGameStub(t, "gs-secret")
	_, deliveries, _, d := newStack(t, "file:"+t.TempDir()+"/d.db", g.srv.URL, time.Minute)
	// 商户归属漂移：target.GameId 与商品不一致 → 151 拒绝
	if err := deliveries.CreateTarget(&model.DeliveryOrder{
		OrderId: "order_mismatch", GameId: "game-b", ServerId: 1, RoleId: "r",
		AccountId: "a", SkuId: "sku-1", ProductId: "gem-100",
		Form: model.FormDirectPurchase, AmountCents: 1000, Currency: "CNY",
	}); err != nil {
		t.Fatalf("create mismatch target: %v", err)
	}
	if err := d.Dispatch(context.Background(), instruct("order_mismatch")); err == nil {
		t.Fatal("game mismatch dispatch = nil, want error")
	}

	// 未配置发货接口：无 endpoint → 163 拒绝
	_, deliveries2, _, d2 := newStack(t, "file:"+t.TempDir()+"/d2.db", "", time.Minute)
	seedTarget(t, deliveries2, "order_noep")
	if err := d2.Dispatch(context.Background(), instruct("order_noep")); err == nil {
		t.Fatal("no endpoint dispatch = nil, want error")
	}
}

func TestDispatch_NetworkErrorArms(t *testing.T) {
	// 非法 endpoint URL → http.NewRequestWithContext 失败臂（184）
	_, deliveries, _, d := newStack(t, "file:"+t.TempDir()+"/d.db", "", time.Minute)
	seedTarget(t, deliveries, "order_badurl")
	if err := deliveries.UpsertEndpoint(&model.DeliveryEndpoint{GameId: "game-a", Endpoint: "://bad", Secret: "s", Enabled: true}); err != nil {
		t.Fatalf("upsert bad endpoint: %v", err)
	}
	if err := d.Dispatch(context.Background(), instruct("order_badurl")); err == nil {
		t.Fatal("bad endpoint url = nil, want error")
	}

	// 已关停的游戏服 → hc.Do 失败 → 190 不可达臂
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	_, deliveries2, _, d2 := newStack(t, "file:"+t.TempDir()+"/d2.db", "", time.Minute)
	seedTarget(t, deliveries2, "order_dead")
	if err := deliveries2.UpsertEndpoint(&model.DeliveryEndpoint{GameId: "game-a", Endpoint: deadURL, Secret: "s", Enabled: true}); err != nil {
		t.Fatalf("upsert dead endpoint: %v", err)
	}
	if err := d2.Dispatch(context.Background(), instruct("order_dead")); err == nil {
		t.Fatal("dead endpoint = nil, want error")
	}
}

func TestDispatch_WriteFailAndCallbackArms(t *testing.T) {
	// 只读账本 + 成功游戏服：MarkDelivered 失败臂（198）
	g := newGameStub(t, "gs-secret")
	path := t.TempDir() + "/ro.db"
	db, deliveries, _, _ := newStack(t, "file:"+path, g.srv.URL, time.Minute)
	seedTarget(t, deliveries, "order_ro")
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	ro := roDeliveryDB(t, path)
	roD := NewDispatcher(model.NewDeliveryModel(ro), model.NewProductModel(ro), 2000, time.Minute, nil)
	if err := roD.Dispatch(context.Background(), instruct("order_ro")); err == nil {
		t.Fatal("readonly mark-delivered = nil, want error")
	}

	// 只读账本 + 失败游戏服：fail() 的 MarkFailed 失败臂（214）
	path2 := t.TempDir() + "/ro2.db"
	db2, deliveries2, _, _ := newStack(t, "file:"+path2, bigFailStub(t).URL, time.Minute)
	seedTarget(t, deliveries2, "order_ro2")
	if err := db2.Close(); err != nil {
		t.Fatalf("close2: %v", err)
	}
	ro2 := roDeliveryDB(t, path2)
	roD2 := NewDispatcher(model.NewDeliveryModel(ro2), model.NewProductModel(ro2), 2000, time.Minute, nil)
	if err := roD2.Dispatch(context.Background(), instruct("order_ro2")); err == nil {
		t.Fatal("readonly mark-failed = nil, want error")
	}

	// onDelivered 回调触发臂（200）
	g2 := newGameStub(t, "gs-secret")
	got := ""
	db3, deliveries3, _, _ := newStack(t, "file:"+t.TempDir()+"/d3.db", g2.srv.URL, time.Minute)
	seedTarget(t, deliveries3, "order_cb")
	d3 := NewDispatcher(model.NewDeliveryModel(db3), model.NewProductModel(db3), 2000, time.Minute, func(orderID string) { got = orderID })
	if err := d3.Dispatch(context.Background(), instruct("order_cb")); err != nil {
		t.Fatalf("callback dispatch = %v", err)
	}
	if got != "order_cb" {
		t.Fatalf("onDelivered = %q, want order_cb", got)
	}
}

func TestDispatch_AttemptsCapAndTruncate(t *testing.T) {
	// 500 且响应体 300 字节 → 每次 fail() 的 truncate 截断臂（223）
	// 循环 8 次 → attempts 到 8，退避封顶 6 倍臂（210）触发
	_, deliveries, _, d := newStack(t, "file:"+t.TempDir()+"/d.db", bigFailStub(t).URL, 200*time.Millisecond)
	seedTarget(t, deliveries, "order_1")
	for i := 0; i < 8; i++ {
		if err := d.Dispatch(context.Background(), instruct("order_1")); err == nil {
			t.Fatalf("dispatch[%d] = nil, want error", i)
		}
	}
	row, err := deliveries.Get("order_1")
	if err != nil {
		t.Fatalf("get row: %v", err)
	}
	if row.Attempts < 6 {
		t.Fatalf("attempts = %d, want >= 6（退避封顶臂应触发）", row.Attempts)
	}
}
