package delivery

// GP-203 分发器单测：幂等（同订单不二次发货）、并发同订单、失败记账与
// 退避恢复、重启恢复（SQLite 落盘 → 关库重开 → 新实例继续投）。

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/model"
)

// gameStub 游戏服发货接口存根：可翻转失败，记录命中与最近签名/载荷。
type gameStub struct {
	srv    *httptest.Server
	secret string
	mu     sync.Mutex
	calls  int
	hits   int
	last   []byte
	sig    string
	fail   atomic.Bool
}

func newGameStub(t *testing.T, secret string) *gameStub {
	t.Helper()
	g := &gameStub{secret: secret}
	g.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		g.mu.Lock()
		g.sig = r.Header.Get(gameSignatureHeader)
		g.last = raw
		g.calls++
		defer g.mu.Unlock()
		if g.fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"error":"boom"}`))
			return
		}
		g.hits++
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(g.srv.Close)
	return g
}

func (g *gameStub) Hits() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.hits
}

// Calls 游戏服收到的总调用数（含失败尝试），用于断言重投确实重触达。
func (g *gameStub) Calls() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.calls
}

func (g *gameStub) Snapshot() (body []byte, sig string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.last, g.sig
}

// newStack 建库/表/商品/分发器（backoff 可注入，测试不等长退避）；返回 db
// 供重启恢复用例显式关库（模拟进程退出）。
func newStack(t *testing.T, dsn string, gameURL string, backoff time.Duration) (*sql.DB, *model.DeliveryModel, *model.ProductModel, *Dispatcher) {
	t.Helper()
	db, err := model.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if dsn == "" {
		t.Fatal("empty dsn")
	}
	products := model.NewProductModel(db)
	if err := products.CreateTables(); err != nil {
		t.Fatalf("create product tables: %v", err)
	}
	deliveries := model.NewDeliveryModel(db)
	if err := deliveries.CreateTables(); err != nil {
		t.Fatalf("create delivery tables: %v", err)
	}
	if gameURL != "" {
		if err := deliveries.UpsertEndpoint(&model.DeliveryEndpoint{GameId: "game-a", Endpoint: gameURL, Secret: "gs-secret", Enabled: true}); err != nil {
			t.Fatalf("upsert endpoint: %v", err)
		}
	}
	if _, err := products.Create(&model.Product{
		Id: "sku-1", ProductId: "gem-100", Form: model.FormDirectPurchase,
		AmountCents: 1000, Currency: "CNY", GameId: "game-a", PartnerId: 2,
	}); err != nil && !errors.Is(err, model.ErrProductExists) {
		t.Fatalf("create product: %v", err)
	}
	dispatcher := NewDispatcher(deliveries, products, 2000, backoff, nil)
	t.Cleanup(func() { _ = db.Close() })
	return db, deliveries, products, dispatcher
}

func seedTarget(t *testing.T, deliveries *model.DeliveryModel, orderId string) {
	t.Helper()
	err := deliveries.CreateTarget(&model.DeliveryOrder{
		OrderId: orderId, GameId: "game-a", ServerId: 1, RoleId: "role-9",
		AccountId: "acc-1", SkuId: "sku-1", ProductId: "gem-100",
		Form: model.FormDirectPurchase, AmountCents: 1000, Currency: "CNY",
	})
	if err != nil && !errors.Is(err, model.ErrDeliveryOrderExists) {
		t.Fatalf("seed target: %v", err)
	}
}

func instruct(orderId string) Instruction {
	return Instruction{
		OrderId: orderId, AccountId: "acc-1", SkuId: "sku-1", ProductId: "gem-100",
		Form: model.FormDirectPurchase, AmountCents: 1000, Currency: "CNY",
	}
}

func TestDispatch_IdempotentSignedDelivery(t *testing.T) {
	g := newGameStub(t, "gs-secret")
	_, deliveries, _, d := newStack(t, "file:"+t.TempDir()+"/d.db", g.srv.URL, time.Minute)
	seedTarget(t, deliveries, "order_1")

	ctx := context.Background()
	if err := d.Dispatch(ctx, instruct("order_1")); err != nil {
		t.Fatalf("dispatch = %v, want nil", err)
	}
	if g.Hits() != 1 {
		t.Fatalf("game hits = %d, want 1", g.Hits())
	}
	body, sig := g.Snapshot()
	mac := hmac.New(sha256.New, []byte("gs-secret"))
	mac.Write(body)
	if want := hex.EncodeToString(mac.Sum(nil)); sig != want {
		t.Fatalf("game signature = %q, want %q", sig, want)
	}
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	for k, want := range map[string]any{
		"orderId": "order_1", "accountId": "acc-1", "serverId": float64(1), "roleId": "role-9",
		"productId": "gem-100", "amountCents": float64(1000), "currency": "CNY",
	} {
		if payload[k] != want {
			t.Fatalf("payload[%s] = %v, want %v (body=%s)", k, payload[k], want, body)
		}
	}

	// 幂等：重复投递同一订单 → 直接成功，游戏服不二次收到
	if err := d.Dispatch(ctx, instruct("order_1")); err != nil {
		t.Fatalf("re-dispatch = %v, want nil", err)
	}
	if g.Hits() != 1 {
		t.Fatalf("game hits after re-dispatch = %d, want 1 (同订单不二次发货)", g.Hits())
	}
}

func TestDispatch_ConcurrentSameOrderSingleHit(t *testing.T) {
	g := newGameStub(t, "gs-secret")
	_, deliveries, _, d := newStack(t, "file:"+t.TempDir()+"/d.db", g.srv.URL, time.Minute)
	seedTarget(t, deliveries, "order_1")

	var wg sync.WaitGroup
	errs := make([]error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = d.Dispatch(context.Background(), instruct("order_1"))
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("concurrent dispatch[%d] = %v, want nil", i, err)
		}
	}
	if g.Hits() != 1 {
		t.Fatalf("game hits = %d, want 1 (并发同订单只投一次)", g.Hits())
	}
}

func TestDispatch_FailureRecordedThenRecovered(t *testing.T) {
	g := newGameStub(t, "gs-secret")
	_, deliveries, _, d := newStack(t, "file:"+t.TempDir()+"/d.db", g.srv.URL, 200*time.Millisecond)
	seedTarget(t, deliveries, "order_1")

	g.fail.Store(true)
	if err := d.Dispatch(context.Background(), instruct("order_1")); err == nil {
		t.Fatal("dispatch against failing game server = nil, want error")
	}
	row, err := deliveries.Get("order_1")
	if err != nil {
		t.Fatalf("get row: %v", err)
	}
	if row.Status != model.DeliveryStatusFailed || row.Attempts != 1 {
		t.Fatalf("row status=%s attempts=%d, want failed/1", row.Status, row.Attempts)
	}

	// 退避到期后定时恢复重投（backoff=200ms × attempts+1）
	g.fail.Store(false)
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, delivered := (&Recovery{db: deliveries, dispatch: d, interval: time.Second}).RecoverOnce(context.Background()); delivered > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("recovery never delivered the failed order")
		}
		time.Sleep(100 * time.Millisecond)
	}
	if g.Calls() != 2 {
		t.Fatalf("game calls = %d, want 2 (失败重投应重触达)", g.Calls())
	}
	if g.Hits() != 1 {
		t.Fatalf("game successful hits = %d, want 1", g.Hits())
	}
	row, _ = deliveries.Get("order_1")
	if row.Status != model.DeliveryStatusDelivered {
		t.Fatalf("row status = %s, want delivered", row.Status)
	}
}

func TestDispatch_GuardRejections(t *testing.T) {
	g := newGameStub(t, "gs-secret")
	_, deliveries, _, d := newStack(t, "file:"+t.TempDir()+"/d.db", g.srv.URL, time.Minute)
	seedTarget(t, deliveries, "order_1")
	ctx := context.Background()

	// 金额与商品配置漂移 → 拒绝且不触达游戏服
	bad := instruct("order_1")
	bad.AmountCents = 999
	if err := d.Dispatch(ctx, bad); err == nil {
		t.Fatal("mismatched amount dispatch = nil, want error")
	}
	// 商品不存在 → 拒绝
	if err := d.Dispatch(ctx, Instruction{OrderId: "order_1", SkuId: "nope", ProductId: "x", AmountCents: 1, Currency: "CNY"}); err == nil {
		t.Fatal("unknown sku dispatch = nil, want error")
	}
	// 缺少充值目标 → 拒绝（未回填前不发货）
	if err := d.Dispatch(ctx, Instruction{OrderId: "order_no_target", AccountId: "a", SkuId: "sku-1", ProductId: "gem-100", AmountCents: 1000, Currency: "CNY"}); err == nil {
		t.Fatal("missing target dispatch = nil, want error")
	}
	if g.Hits() != 0 {
		t.Fatalf("game hits = %d, want 0 (被拒指令不得触达游戏服)", g.Hits())
	}
}

func TestDispatch_LocksMapCleanedAfterUse(t *testing.T) {
	// P1-2 回归：原实现 locks sync.Map 只存不删，每订单一个 Mutex 永久驻留；
	// 修复后引用计数归零即删除，dispatch 完成 map 应为空。
	g := newGameStub(t, "gs-secret")
	_, deliveries, _, d := newStack(t, "file:"+t.TempDir()+"/d.db", g.srv.URL, time.Minute)
	ctx := context.Background()
	for i := 0; i < 5; i++ {
		order := fmt.Sprintf("order_%d", i)
		seedTarget(t, deliveries, order)
		if err := d.Dispatch(ctx, instruct(order)); err != nil {
			t.Fatalf("dispatch %s = %v", order, err)
		}
	}
	n := 0
	d.locks.Range(func(_, _ any) bool { n++; return true })
	if n != 0 {
		t.Fatalf("locks entries after dispatch = %d, want 0（防 sync.Map 无限增长）", n)
	}
}

func TestRestart_RecoveryAfterProcessReopen(t *testing.T) {
	// 重启恢复：落盘 SQLite → 关库（模拟进程退出）→ 重开（新实例）→
	// 恢复循环把未投递订单投出去。
	dsn := "file:" + t.TempDir() + "/restart.db"
	g := newGameStub(t, "gs-secret")

	db, deliveries, _, _ := newStack(t, dsn, g.srv.URL, time.Minute)
	seedTarget(t, deliveries, "order_1")
	// 模拟旧进程退出（账本在 SQLite 落盘）
	if err := db.Close(); err != nil {
		t.Fatalf("close db: %v", err)
	}

	_, reopened, _, d2 := newStack(t, dsn, g.srv.URL, time.Minute)
	rec := NewRecovery(reopened, d2, time.Minute)
	attempted, delivered := rec.RecoverOnce(context.Background())
	if attempted != 1 || delivered != 1 {
		t.Fatalf("recover after restart = (%d, %d), want (1, 1)", attempted, delivered)
	}
	if g.Hits() != 1 {
		t.Fatalf("game hits = %d, want 1", g.Hits())
	}
	// 再扫一轮：无到期行，不重发
	_, delivered = rec.RecoverOnce(context.Background())
	if delivered != 0 {
		t.Fatalf("second recovery delivered = %d, want 0", delivered)
	}
}
