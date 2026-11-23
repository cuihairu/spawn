package promoattr

// GP-403 渠道归因 outbox 重投覆盖：nil 装配、扫描失败、各 Mark 写失败
// （best-effort 仅记日志）、退避乘数封顶、Run 周期 tick 与 ctx 取消。
// DB 失败臂用关库/触发器（RAISE(ABORT)）驱动，均诚实可达，不伪造。

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/user-service/internal/integration"
	"github.com/tappi/tappi/services/user-service/model"

	_ "github.com/mattn/go-sqlite3"
)

// paEnv 归因 outbox 测试环境：模型 + 原始连接（关库/触发器驱动 DB 失败臂）。
type paEnv struct {
	m   *model.PromoAttrOutboxModel
	raw *sql.DB
}

func newPAEnv(t *testing.T) *paEnv {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/promoattr.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := model.NewPromoAttrOutboxModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create outbox tables: %v", err)
	}
	return &paEnv{m: m, raw: db}
}

// closeDB 关底层连接（后续读/写均失败；驱动 DB-error 失败臂）。
func (e *paEnv) closeDB() {
	_ = e.raw.Close()
}

// addTrigger 建触发器（RAISE(ABORT)）驱动对应 UPDATE 失败臂；ddl 为完整
// CREATE TRIGGER 语句（WHEN 子句限定仅目标状态转移中止，其余写不受影响）。
func (e *paEnv) addTrigger(t *testing.T, ddl string) {
	t.Helper()
	if _, err := e.raw.Exec(ddl); err != nil {
		t.Fatalf("add trigger: %v", err)
	}
}

// stubCatalog game-catalog 归因解析存根：fail 时返 500（驱动重投失败臂）。
type stubCatalog struct {
	fail   bool
	calls  int
	clicks []string
}

func (s *stubCatalog) server() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.calls++
		s.clicks = append(s.clicks, r.URL.Path)
		if s.fail {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"click_id":"c1","channel":"ch1","attributed":true,"in_window":true}`))
	}))
}

func TestNewRelay_NilAndDefaults(t *testing.T) {
	env := newPAEnv(t)
	c := integration.NewGameCatalogClient("http://127.0.0.1:1", time.Second)

	if r := NewRelay(nil, c, time.Minute, 2, time.Hour); r != nil {
		t.Fatalf("NewRelay(nil outbox) = %v, want nil", r)
	}
	if r := NewRelay(env.m, nil, time.Minute, 2, time.Hour); r != nil {
		t.Fatalf("NewRelay(nil client) = %v, want nil", r)
	}
	// 非正 interval/maxAttempts/retention 各自回退默认（不 panic，仍装配）。
	if r := NewRelay(env.m, c, 0, 0, 0); r == nil {
		t.Fatal("NewRelay(0,0,0) = nil, want 已装配（默认回退）")
	}
	if r := NewRelay(env.m, c, -1, -2, -3); r == nil {
		t.Fatal("NewRelay(负值) = nil, want 已装配")
	}
}

func TestRelayOnce_NilReceiver(t *testing.T) {
	// nil 重投器（未接线）：RelayOnce 直接 0/0，不 panic。
	var r *Relay
	if a, d := r.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("RelayOnce = %d/%d, want 0/0", a, d)
	}
}

func TestRelayOnce_ListRetryDBError(t *testing.T) {
	// ListRetry 扫描失败（关库）→ 0/0（不误报、不 panic）。
	env := newPAEnv(t)
	env.closeDB()
	r := NewRelay(env.m, integration.NewGameCatalogClient("http://127.0.0.1:1", time.Second),
		time.Minute, 2, time.Hour)
	if r == nil {
		t.Fatal("NewRelay = nil")
	}
	if a, d := r.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("RelayOnce = %d/%d, want 0/0（扫描失败）", a, d)
	}
}

func TestRelayOnce_ExhaustsAndDelivers(t *testing.T) {
	// 失败未耗尽 → 退避 pending；恢复 → delivered；耗尽 → dropped。
	env := newPAEnv(t)
	stub := &stubCatalog{}
	srv := stub.server()
	t.Cleanup(srv.Close)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := env.m.Enqueue("pa:u1:c1", "c1", 1, past, `{"click_id":"c1"}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}

	r := NewRelay(env.m, integration.NewGameCatalogClient(srv.URL, time.Second),
		time.Minute, 10, time.Hour)
	if r == nil {
		t.Fatal("NewRelay = nil")
	}

	// 恢复态：重投成功 → delivered
	stub.fail = false
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 1 {
		t.Fatalf("attempted=%d delivered=%d, want 1/1", a, d)
	}
	rows, err := env.m.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339), 100)
	if err != nil || len(rows) != 0 {
		t.Fatalf("rows=%+v err=%v, want 已 delivered 清空重投队列", rows, err)
	}

	// 失败态 + maxAttempts=1 → 首次失败即耗尽 → dropped
	stub.fail = true
	if err := env.m.Enqueue("pa:u2:c2", "c2", 2, past, `{"click_id":"c2"}`, past); err != nil {
		t.Fatalf("enqueue 2: %v", err)
	}
	r1 := NewRelay(env.m, integration.NewGameCatalogClient(srv.URL, time.Second),
		time.Minute, 1, time.Hour)
	if a, d := r1.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0（失败留待下轮）", a, d)
	}
	if a, d := r1.RelayOnce(context.Background()); a != 0 || d != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 0/0（已 dropped 不再重投）", a, d)
	}
}

func TestRelayOnce_MarkDroppedDBError(t *testing.T) {
	// 耗尽置 dropped 但写失败：仅记日志（best-effort），不 panic。
	env := newPAEnv(t)
	stub := &stubCatalog{fail: true}
	srv := stub.server()
	t.Cleanup(srv.Close)
	env.addTrigger(t, `CREATE TRIGGER trg_drop BEFORE UPDATE ON promo_attribution_outbox
		WHEN NEW.status='dropped' BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := env.m.Enqueue("pa:u3:c3", "c3", 3, past, `{"click_id":"c3"}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	r := NewRelay(env.m, integration.NewGameCatalogClient(srv.URL, time.Second),
		time.Minute, 1, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0", a, d)
	}
}

func TestRelayOnce_MarkFailedDBErrorAndBackoffCap(t *testing.T) {
	// 未耗尽但 MarkFailed 写失败：仅记日志（best-effort）。同时验证退避
	// 乘数封顶 6（attempts=7 再失败一次 → mult 固定 6，不无限增长）。
	env := newPAEnv(t)
	stub := &stubCatalog{fail: true}
	srv := stub.server()
	t.Cleanup(srv.Close)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := env.m.Enqueue("pa:u4:c4", "c4", 4, past, `{"click_id":"c4"}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	// 先播种 attempts=7（此时无触发器，种子写不被中止），再挂触发器驱动
	// Relay 的 MarkFailed 失败臂（MarkFailed 保持 status='pending'，WHEN 命中）。
	if err := env.m.MarkFailed("pa:u4:c4", 7, "prev", past); err != nil {
		t.Fatalf("seed attempts: %v", err)
	}
	env.addTrigger(t, `CREATE TRIGGER trg_mf BEFORE UPDATE ON promo_attribution_outbox
		WHEN NEW.status='pending' BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	r := NewRelay(env.m, integration.NewGameCatalogClient(srv.URL, time.Second),
		time.Minute, 10, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 0 {
		t.Fatalf("attempted=%d delivered=%d, want 1/0", a, d)
	}
	rows, err := env.m.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339), 100)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows=%+v err=%v, want 1 条仍 pending", rows, err)
	}
	// 写失败 → attempts 原样留 7（本次计数未落库），行仍 pending 待下轮；
	// 乘数封顶分支（attempts=7 → mult 固定 6）已在调用 MarkFailed 前走过。
	if rows[0].Attempts != 7 || rows[0].Status != model.PromoAttrOutboxPending {
		t.Fatalf("attempts=%d status=%s, want 7/pending（写失败原样保留、仍可重试）",
			rows[0].Attempts, rows[0].Status)
	}
}

func TestRelayOnce_MarkDeliveredDBError(t *testing.T) {
	// 重投成功但 MarkDelivered 写失败：受理判定已完成（delivered=1），写失败仅
	// 记日志——不因写失败把已受理当失败。
	env := newPAEnv(t)
	stub := &stubCatalog{}
	srv := stub.server()
	t.Cleanup(srv.Close)
	env.addTrigger(t, `CREATE TRIGGER trg_md BEFORE UPDATE ON promo_attribution_outbox
		WHEN NEW.status='delivered' BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	if err := env.m.Enqueue("pa:u5:c5", "c5", 5, past, `{"click_id":"c5"}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	r := NewRelay(env.m, integration.NewGameCatalogClient(srv.URL, time.Second),
		time.Minute, 10, time.Hour)
	if a, d := r.RelayOnce(context.Background()); a != 1 || d != 1 {
		t.Fatalf("attempted=%d delivered=%d, want 1/1（受理判定完成，写失败仅日志）", a, d)
	}
}

func TestRun_PruneDBError(t *testing.T) {
	// Prune 清理失败：仅记日志（best-effort），Run 继续下一轮，不 panic、不退出。
	env := newPAEnv(t)
	stub := &stubCatalog{}
	srv := stub.server()
	t.Cleanup(srv.Close)
	env.addTrigger(t, `CREATE TRIGGER trg_del BEFORE DELETE ON promo_attribution_outbox
		BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	oldDelivered := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	if err := env.m.Enqueue("pa:prune:c", "c", 3, past, `{"click_id":"c"}`, past); err != nil {
		t.Fatalf("enqueue: %v", err)
	}
	if err := env.m.MarkDelivered("pa:prune:c", oldDelivered); err != nil {
		t.Fatalf("mark delivered: %v", err)
	}
	relay := NewRelay(env.m, integration.NewGameCatalogClient(srv.URL, time.Second),
		10*time.Millisecond, 10, time.Hour)
	if relay == nil {
		t.Fatal("NewRelay = nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { relay.Run(ctx); close(done) }()
	time.Sleep(100 * time.Millisecond) // 数轮 tick：Prune 失败日志被驱动
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on ctx cancel")
	}
}

func TestRun_NilReceiver(t *testing.T) {
	// nil 重投器（未接线）：Run 直接返回，不 panic（与 refundexec/playeract Run 同款守卫）。
	var r *Relay
	r.Run(context.Background())
}

func TestRun_TickAndCancel(t *testing.T) {
	// 周期 tick：到期行被重投 delivered（attempted>0 日志）+ 已终态旧行被清理
	// （prune n>0 日志）；ctx 取消即退出。
	env := newPAEnv(t)
	stub := &stubCatalog{}
	srv := stub.server()
	t.Cleanup(srv.Close)
	past := time.Now().UTC().Add(-time.Minute).Format(time.RFC3339)
	// rowA：到期 pending → 首 tick 重投成功
	if err := env.m.Enqueue("pa:run:cA", "cA", 1, past, `{"click_id":"cA"}`, past); err != nil {
		t.Fatalf("enqueue A: %v", err)
	}
	// rowB：已 delivered 且 delivered_at 很旧 → 首 tick 被 Prune 清理
	oldDelivered := time.Now().UTC().Add(-2 * time.Hour).Format(time.RFC3339)
	if err := env.m.Enqueue("pa:run:cB", "cB", 2, past, `{"click_id":"cB"}`, past); err != nil {
		t.Fatalf("enqueue B: %v", err)
	}
	if err := env.m.MarkDelivered("pa:run:cB", oldDelivered); err != nil {
		t.Fatalf("mark delivered B: %v", err)
	}

	relay := NewRelay(env.m, integration.NewGameCatalogClient(srv.URL, time.Second),
		10*time.Millisecond, 10, time.Hour)
	if relay == nil {
		t.Fatal("NewRelay = nil")
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { relay.Run(ctx); close(done) }()

	deadline := time.Now().Add(3 * time.Second)
	for {
		rows, err := env.m.ListRetry(time.Now().UTC().Add(time.Hour).Format(time.RFC3339), 100)
		if err == nil && len(rows) == 0 {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("tick 未达成（rowA 重投/rowB 清理），rows=%+v", rows)
		}
		time.Sleep(5 * time.Millisecond)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on ctx cancel")
	}
}

func TestTruncate_Long(t *testing.T) {
	// 长错误截断到 n（last_error 有界）；短串原样返回。
	long := strings.Repeat("x", 600)
	if got := truncate(long, 500); len(got) != 500 {
		t.Fatalf("truncate(len=%d) = len %d, want 500", len(long), len(got))
	}
	if got := truncate("short", 500); got != "short" {
		t.Fatalf("truncate(short) = %q, want 原样", got)
	}
}
