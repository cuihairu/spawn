package promotion

// bridge.go 剩余补漏分支：Enabled 的 nil 空 URL 与有 URL 两臂、Enqueue
// 成功入队臂、DeliverOnce 的投递成功（MarkDone）/失败记账（MarkFailed）/
// 记账失败（只读）三臂、post 的非法 URL（NewRequest）失败臂、Scan 的
// 未启用空转臂、ctx 取消退出臂、interval<=0 默认周期臂、ListRetry 失败
// （删队列表）续扫臂与 DeliverOnce 失败记日志续扫臂。
//
// 登记不可达（不测）：truncate 的 s[:n] 截断臂（post 错误消息恒短，除非
// 用超长 URL 触发慢 DNS，不实用）；Enqueue 的 json.Marshal 失败臂（Facts
// 为固定类型结构体）。

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/model"
)

// bridgeDB 建评估表并返回 (Bridge, db, 文件路径)（路径供 ?mode=ro 重开）。
func bridgeDB(t *testing.T, baseURL, secret string) (*Bridge, *sql.DB, string) {
	t.Helper()
	path := t.TempDir() + "/p.db"
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := model.NewPromotionEvalModel(db).CreateTables(); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	return NewBridge(model.NewPromotionEvalModel(db), baseURL, secret, 1000), db, path
}

// promotionsStub 返回一个可控状态码的 promotions 评估端点存根。
func promotionsStub(t *testing.T, status int) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestBridge_EnabledArms(t *testing.T) {
	// nil 桥 → 未启用
	var nilBridge *Bridge
	if nilBridge.Enabled() {
		t.Fatal("nil bridge Enabled = true")
	}
	// 空 URL → 未启用
	if NewBridge(nil, "", "", 0).Enabled() {
		t.Fatal("empty url Enabled = true")
	}
	// 有 URL → 启用
	if !NewBridge(nil, "http://x", "", 0).Enabled() {
		t.Fatal("configured bridge Enabled = false")
	}
}

func TestBridge_EnqueueArms(t *testing.T) {
	b, _, _ := bridgeDB(t, "http://x", "sec")
	// 入队成功 → ListRetry 查得到
	if err := b.Enqueue("order_1", "sandbox", Facts{OrderId: "order_1"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// 幂等：同订单重复入队跳过（UNIQUE）
	if err := b.Enqueue("order_1", "sandbox", Facts{OrderId: "order_1"}); err != nil {
		t.Fatalf("re-enqueue: %v", err)
	}
}

func TestBridge_DeliverOnceArms(t *testing.T) {
	b, _, _ := bridgeDB(t, promotionsStub(t, 200).URL, "sec")
	// 投递成功 → MarkDone（status=done）
	if err := b.Enqueue("order_1", "sandbox", Facts{OrderId: "order_1"}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	row := &model.PromotionEval{OrderId: "order_1", Payload: `{"order_id":"order_1"}`}
	if err := b.DeliverOnce(context.Background(), row); err != nil {
		t.Fatalf("deliver ok = %v, want nil", err)
	}

	// 投递失败 → MarkFailed（status=failed，attempts=1）
	b2, _, _ := bridgeDB(t, promotionsStub(t, 500).URL, "sec")
	if err := b2.Enqueue("order_2", "sandbox", Facts{}); err != nil {
		t.Fatalf("enqueue2: %v", err)
	}
	row2 := &model.PromotionEval{OrderId: "order_2", Payload: `{}`}
	if err := b2.DeliverOnce(context.Background(), row2); err == nil {
		t.Fatal("deliver fail = nil, want error")
	}

	// 记账失败（只读）→ 包装错误（post 先失败，MarkFailed 只读写败）
	b3, _, path3 := bridgeDB(t, "://bad", "sec")
	if err := b3.Enqueue("order_3", "sandbox", Facts{}); err != nil {
		t.Fatalf("enqueue3: %v", err)
	}
	ro := roBridgeDB(t, path3)
	roB := NewBridge(model.NewPromotionEvalModel(ro), "://bad", "sec", 1000)
	if err := roB.DeliverOnce(context.Background(), &model.PromotionEval{OrderId: "order_3", Payload: `{}`}); err == nil {
		t.Fatal("deliver ro = nil, want error")
	}

	// 重试封顶：attempts+1 > 6 → mult 钳到 6（仍走 MarkFailed 成功臂）
	if err := b2.Enqueue("order_4", "sandbox", Facts{}); err != nil {
		t.Fatalf("enqueue4: %v", err)
	}
	row4 := &model.PromotionEval{OrderId: "order_4", Payload: `{}`, Attempts: 10}
	if err := b2.DeliverOnce(context.Background(), row4); err == nil {
		t.Fatal("deliver capped = nil, want error")
	}

	// 服务器不可达（已关停端点）→ hc.Do 失败包装
	dead := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	b5, _, _ := bridgeDB(t, deadURL, "sec")
	if err := b5.DeliverOnce(context.Background(), &model.PromotionEval{OrderId: "order_5", Payload: `{}`}); err == nil {
		t.Fatal("deliver dead = nil, want error")
	}
}

func TestBridge_ScanArms(t *testing.T) {
	// 未启用 → 空转返回
	NewBridge(nil, "", "", 0).Scan(context.Background(), time.Millisecond)

	// 已启用 + 500 存根 + 短周期：ctx 取消退出；DeliverOnce 失败记日志续扫
	b, _, _ := bridgeDB(t, promotionsStub(t, 500).URL, "sec")
	if err := b.Enqueue("order_1", "sandbox", Facts{}); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		b.Scan(ctx, time.Millisecond) // interval>0 走 ticker
		close(done)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Scan did not stop after cancel")
	}

	// interval<=0 → 默认周期；队列表被删 → ListRetry 失败记日志续扫
	b2, db2, _ := bridgeDB(t, promotionsStub(t, 200).URL, "sec")
	if err := b2.Enqueue("order_1", "sandbox", Facts{}); err != nil {
		t.Fatalf("enqueue2: %v", err)
	}
	if _, err := db2.Exec(`DROP TABLE promotion_evaluations`); err != nil {
		t.Fatalf("drop: %v", err)
	}
	ctx2, cancel2 := context.WithCancel(context.Background())
	done2 := make(chan struct{})
	go func() {
		b2.Scan(ctx2, time.Millisecond) // ListRetry 失败 → 记日志 continue
		close(done2)
	}()
	time.Sleep(30 * time.Millisecond)
	cancel2()
	select {
	case <-done2:
	case <-time.After(2 * time.Second):
		t.Fatal("Scan(drop) did not stop after cancel")
	}

	// interval<=0 → 默认周期（130）：健康存根 + 立即取消（1 分钟 ticker 不触发）
	b3, _, _ := bridgeDB(t, promotionsStub(t, 200).URL, "sec")
	ctx3, cancel3 := context.WithCancel(context.Background())
	done3 := make(chan struct{})
	go func() {
		b3.Scan(ctx3, 0)
		close(done3)
	}()
	time.Sleep(20 * time.Millisecond)
	cancel3()
	select {
	case <-done3:
	case <-time.After(2 * time.Second):
		t.Fatal("Scan(interval 0) did not stop after cancel")
	}
}

// roBridgeDB 以 ?mode=ro 重开同一 sqlite 文件。
func roBridgeDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatalf("open ro: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}
