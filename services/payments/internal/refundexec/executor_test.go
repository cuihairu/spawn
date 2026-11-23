package refundexec

// 退款执行器单测（GP-204 管理适配）：PENDING 意图 → Courier 管理面退款端点
// （/v1/payments/admin/orders/{id}/refund）→ EXECUTED/REJECTED。覆盖：
//   - 接受登记 → EXECUTED，请求体/scope 头/管理凭证正确
//   - 瞬态失败（5xx/401/scope 未映射）→ 留 PENDING（下轮重试，不伪造成功）
//   - 确定性拒绝（409 PAYMENT_ORDER_STATE / 404）→ REJECTED + 理由留痕
//   - 已终态幂等（不重复调 Courier）
//   - NewExecutor nil 各缺失臂（fail-closed 未接线 = 不执行）
//   - RunOnce 扫 PENDING 批量

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/courier"
	"github.com/tappi/tappi/services/payments/internal/courierscope"
	"github.com/tappi/tappi/services/payments/model"

	_ "github.com/mattn/go-sqlite3"
)

const execAdminTok = "courier-admin-tok"

// execEnv 执行器测试环境：投影 DB + scope/courier 存根。
type execEnv struct {
	db         *model.OrderProjectionModel
	couponDB   *model.CouponModel
	rawDB      *sql.DB
	scopeSrv   *httptest.Server
	courierSrv *httptest.Server
	// 记录 Courier 收到的管理退款请求
	refundCalls int
	refundBody  map[string]any
	refundScope map[string]string
	refundAuth  string
	// 存根注入：refundStatus/refundCode 控制 Courier 响应；refundRejectOrder
	// 非空时仅该订单拒绝（其余成功，供批量扫描区分两笔）
	refundStatus      int
	refundCode        string
	refundRejectOrder string
}

func newExecEnv(t *testing.T) *execEnv {
	t.Helper()
	path := filepath.Join(t.TempDir(), "refundexec.db")
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := model.NewOrderProjectionModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	couponDB := model.NewCouponModel(db)
	if err := couponDB.CreateTables(); err != nil {
		t.Fatalf("create coupon tables: %v", err)
	}

	e := &execEnv{db: m, couponDB: couponDB, rawDB: db, refundStatus: http.StatusOK}

	e.scopeSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 仅 game-a 有映射；其余 404（未接入）。
		if r.URL.Path == "/internal/games/game-a/mapping" {
			_, _ = w.Write([]byte(`{"courierGameId":"courier-game-a","env":"dev"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"code":404,"message":"游戏未接入 Courier"}`))
	}))
	t.Cleanup(e.scopeSrv.Close)

	e.courierSrv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		orderID := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/v1/payments/admin/orders/"), "/refund")
		e.refundCalls++
		e.refundScope = map[string]string{
			"gameId": r.Header.Get("X-Courier-Game-Id"),
			"env":    r.Header.Get("X-Courier-Env"),
			"auth":   r.Header.Get("Authorization"),
		}
		_ = json.NewDecoder(r.Body).Decode(&e.refundBody)
		reject := e.refundStatus != http.StatusOK &&
			(e.refundRejectOrder == "" || orderID == e.refundRejectOrder)
		if !reject {
			_, _ = w.Write([]byte(`{"data":{"orderId":"` + orderID + `","refund":{"action":"REFUND","amountCents":600,"at":"2026-11-23T00:00:00Z"}}}`))
			return
		}
		w.WriteHeader(e.refundStatus)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]any{
			"code": e.refundCode, "message": "courier 拒绝详情",
		}})
	}))
	t.Cleanup(e.courierSrv.Close)
	return e
}

// executor 构造已接线执行器（scope/courier 指向存根）。
func (e *execEnv) executor() *Executor {
	return NewExecutor(e.db,
		courier.NewClient(e.courierSrv.URL, time.Second),
		courierscope.NewClient(e.scopeSrv.URL, "s2s-tok", time.Second),
		e.couponDB, execAdminTok, time.Minute)
}

// seedUsedCoupon 落一张已核销（used，关联 orderID）的券：定义 + 领取（claimed）
// + 锁（locked, orderID）+ 核销（used, orderID），即退款前「券已用在该单」事实。
func (e *execEnv) seedUsedCoupon(t *testing.T, orderID string) int64 {
	t.Helper()
	if err := e.couponDB.CreateCoupon(&model.Coupon{
		Code: "refund-return-1", GameId: "game-a", DiscountType: model.CouponDiscountFixed,
		DiscountValue: 100, ThresholdCents: 0, Status: model.CouponStatusPublished,
		StartAt: "2026-01-01T00:00:00Z", EndAt: "2099-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("create coupon: %v", err)
	}
	uc, _, err := e.couponDB.ClaimCoupon(&model.UserCoupon{
		CouponId: 1, UserId: 7, RequestId: "req-coupon-1",
	})
	if err != nil {
		t.Fatalf("claim coupon: %v", err)
	}
	if err := e.couponDB.LockUserCoupon(uc.Id, orderID); err != nil {
		t.Fatalf("lock coupon: %v", err)
	}
	if err := e.couponDB.MarkCouponUsedByOrder(orderID); err != nil {
		t.Fatalf("mark coupon used: %v", err)
	}
	return uc.Id
}

// seed 落一笔已付款订单投影 + PENDING 退款意图。
func (e *execEnv) seed(t *testing.T, orderID, gameID string, amount int64, action string) {
	t.Helper()
	if err := e.db.UpsertFromOrder(&model.OrderProjection{
		OrderId: orderID, GameId: gameID, PartnerId: 2, AccountId: "7",
		SkuId: "sku-1", ProductId: "gem", Form: "DIRECT_PURCHASE",
		AmountCents: amount, Currency: "CNY", Status: "PAID",
	}); err != nil {
		t.Fatalf("seed order: %v", err)
	}
	if err := e.db.CreateRefund(&model.RefundRequest{
		OrderId: orderID, Action: action, AmountCents: amount,
		Reason: "重复扣款", Status: model.RefundStatusPending, CreatedBy: 1,
	}); err != nil {
		t.Fatalf("seed refund: %v", err)
	}
}

// closeDB 关底层连接（SELECT/UPDATE 后续均失败；驱动 DB-error 失败臂）。
func (e *execEnv) closeDB() {
	_ = e.rawDB.Close()
}

// addTrigger 建触发器（RAISE(ABORT)）驱动对应 UPDATE 失败臂；ddl 为完整
// CREATE TRIGGER 语句（WHEN 子句限定仅目标状态转移中止，其余写不受影响）。
func (e *execEnv) addTrigger(t *testing.T, ddl string) {
	t.Helper()
	if _, err := e.rawDB.Exec(ddl); err != nil {
		t.Fatalf("add trigger: %v", err)
	}
}

func TestExecuteOne_Accepted(t *testing.T) {
	e := newExecEnv(t)
	e.seed(t, "order_1", "game-a", 600, "REFUND")

	got := e.executor().ExecuteOne("order_1", "REFUND")
	if got != Executed {
		t.Fatalf("execution = %q, want executed", got)
	}
	// 持久态 EXECUTED
	rf, err := e.db.GetRefund("order_1", "REFUND")
	if err != nil || rf.Status != model.RefundStatusExecuted {
		t.Fatalf("refund status = %+v err=%v, want EXECUTED", rf, err)
	}
	// 请求体与 scope 头：金额/动作/理由；scope 用映射后的 courier_game_id+env，
	// 绝不把 spawn game-a 当 Courier scope 发；管理凭证独立 Bearer。
	if e.refundBody["action"] != "REFUND" || e.refundBody["amountCents"] != float64(600) || e.refundBody["reason"] != "重复扣款" {
		t.Fatalf("refund body = %v", e.refundBody)
	}
	if e.refundScope["gameId"] != "courier-game-a" || e.refundScope["env"] != "dev" {
		t.Fatalf("scope headers = %v, want courier-game-a/dev", e.refundScope)
	}
	if e.refundScope["auth"] != "Bearer "+execAdminTok {
		t.Fatalf("auth = %q", e.refundScope["auth"])
	}
}

// TestExecuteOne_ReturnsOrderCoupons GP-305 退款返还：EXECUTED 后把订单关联的
// used 券回退 claimed（幂等）。确定性拒绝（REJECTED）不返券——券仍留 used，
// 退款未发生不应回退。
func TestExecuteOne_ReturnsOrderCoupons(t *testing.T) {
	// 1. 接受 → EXECUTED → 券回退 claimed
	e := newExecEnv(t)
	e.seed(t, "order_coupon", "game-a", 600, "REFUND")
	ucID := e.seedUsedCoupon(t, "order_coupon")
	if got := e.executor().ExecuteOne("order_coupon", "REFUND"); got != Executed {
		t.Fatalf("execution = %q, want executed", got)
	}
	uc, err := e.couponDB.GetUserCoupon(ucID)
	if err != nil || uc.Status != model.UserCouponStatusClaimed {
		t.Fatalf("coupon after executed refund = %+v err=%v, want claimed", uc, err)
	}

	// 2. 确定性拒绝 → REJECTED → 券保持 used（未退款不返券）
	e2 := newExecEnv(t)
	e2.seed(t, "order_rej_coupon", "game-a", 600, "REFUND")
	ucID2 := e2.seedUsedCoupon(t, "order_rej_coupon")
	e2.refundStatus, e2.refundCode = http.StatusConflict, "PAYMENT_ORDER_STATE"
	if got := e2.executor().ExecuteOne("order_rej_coupon", "REFUND"); got != Rejected {
		t.Fatalf("execution = %q, want rejected", got)
	}
	uc2, err := e2.couponDB.GetUserCoupon(ucID2)
	if err != nil || uc2.Status != model.UserCouponStatusUsed {
		t.Fatalf("coupon after rejected refund = %+v err=%v, want used", uc2, err)
	}
}

func TestExecuteOne_Transient5xxStaysPending(t *testing.T) {
	e := newExecEnv(t)
	e.seed(t, "order_1", "game-a", 600, "REFUND")
	e.refundStatus, e.refundCode = http.StatusServiceUnavailable, "COMMON_INTERNAL"

	if got := e.executor().ExecuteOne("order_1", "REFUND"); got != Pending {
		t.Fatalf("execution = %q, want pending", got)
	}
	rf, _ := e.db.GetRefund("order_1", "REFUND")
	if rf.Status != model.RefundStatusPending {
		t.Fatalf("status = %s, want PENDING（瞬态失败留 PENDING）", rf.Status)
	}
}

func TestExecuteOne_Unauthorized401StaysPending(t *testing.T) {
	// 401 凭证未配/无效：属配置未就绪，补配后可执行——留 PENDING，不永久拒绝。
	e := newExecEnv(t)
	e.seed(t, "order_1", "game-a", 600, "REFUND")
	e.refundStatus, e.refundCode = http.StatusUnauthorized, "COMMON_UNAUTHENTICATED"

	if got := e.executor().ExecuteOne("order_1", "REFUND"); got != Pending {
		t.Fatalf("execution = %q, want pending", got)
	}
	rf, _ := e.db.GetRefund("order_1", "REFUND")
	if rf.Status != model.RefundStatusPending {
		t.Fatalf("status = %s, want PENDING", rf.Status)
	}
}

func TestExecuteOne_PermanentStateRejected(t *testing.T) {
	// 未付款订单退款 → 409 PAYMENT_ORDER_STATE：确定性拒绝，重试无益。
	e := newExecEnv(t)
	e.seed(t, "order_1", "game-a", 600, "REFUND")
	e.refundStatus, e.refundCode = http.StatusConflict, "PAYMENT_ORDER_STATE"

	if got := e.executor().ExecuteOne("order_1", "REFUND"); got != Rejected {
		t.Fatalf("execution = %q, want rejected", got)
	}
	rf, _ := e.db.GetRefund("order_1", "REFUND")
	if rf.Status != model.RefundStatusRejected {
		t.Fatalf("status = %s, want REJECTED", rf.Status)
	}
	if rf.Note == "" || rf.Note == "执行完成" {
		t.Fatalf("note = %q, want courier 拒绝事实留痕", rf.Note)
	}
}

func TestExecuteOne_PermanentNotFoundRejected(t *testing.T) {
	e := newExecEnv(t)
	e.seed(t, "order_1", "game-a", 600, "REFUND")
	e.refundStatus, e.refundCode = http.StatusNotFound, "PAYMENT_ORDER_NOT_FOUND"

	if got := e.executor().ExecuteOne("order_1", "REFUND"); got != Rejected {
		t.Fatalf("execution = %q, want rejected", got)
	}
}

func TestExecuteOne_ScopeMissingStaysPending(t *testing.T) {
	// 游戏未映射 Courier（scope 404）→ 瞬态留 PENDING。
	e := newExecEnv(t)
	e.seed(t, "order_nomap", "game-x", 600, "REFUND")

	if got := e.executor().ExecuteOne("order_nomap", "REFUND"); got != Pending {
		t.Fatalf("execution = %q, want pending", got)
	}
	if e.refundCalls != 0 {
		t.Fatalf("courier refund calls = %d, want 0（未映射不调 Courier）", e.refundCalls)
	}
}

func TestExecuteOne_OrderMissingStaysPending(t *testing.T) {
	// 投影无该订单（不可读事实）→ 瞬态留 PENDING。
	e := newExecEnv(t)
	if err := e.db.CreateRefund(&model.RefundRequest{
		OrderId: "order_missing", Action: "REFUND", AmountCents: 100,
		Status: model.RefundStatusPending, CreatedBy: 1,
	}); err != nil {
		t.Fatalf("seed refund: %v", err)
	}
	if got := e.executor().ExecuteOne("order_missing", "REFUND"); got != Pending {
		t.Fatalf("execution = %q, want pending", got)
	}
}

func TestExecuteOne_IdempotentTerminal(t *testing.T) {
	// 已 EXECUTED：幂等回传现态，不重复调 Courier。
	e := newExecEnv(t)
	e.seed(t, "order_1", "game-a", 600, "REFUND")
	_ = e.db.MarkRefundExecuted("order_1", "REFUND")
	calls := e.refundCalls

	if got := e.executor().ExecuteOne("order_1", "REFUND"); got != Executed {
		t.Fatalf("execution = %q, want executed", got)
	}
	if e.refundCalls != calls {
		t.Fatalf("courier calls %d→%d, want unchanged（已终态不重复执行）", calls, e.refundCalls)
	}
}

func TestExecuteOne_NilReceiver(t *testing.T) {
	// nil 执行器（未接线）= pending，不 panic、不伪造成功。
	var e *Executor
	if got := e.ExecuteOne("order_1", "REFUND"); got != Pending {
		t.Fatalf("execution = %q, want pending", got)
	}
}

func TestNewExecutor_NilCases(t *testing.T) {
	env := newExecEnv(t)
	c := courier.NewClient(env.courierSrv.URL, time.Second)
	sc := courierscope.NewClient(env.scopeSrv.URL, "tok", time.Second)

	cases := []struct {
		name string
		db   *model.OrderProjectionModel
		c    *courier.Client
		sc   *courierscope.Client
		tok  string
	}{
		{"empty token", env.db, c, sc, ""},
		{"nil db", nil, c, sc, execAdminTok},
		{"nil courier", env.db, nil, sc, execAdminTok},
		{"nil scope", env.db, c, nil, execAdminTok},
	}
	for _, tc := range cases {
		if got := NewExecutor(tc.db, tc.c, tc.sc, env.couponDB, tc.tok, time.Minute); got != nil {
			t.Fatalf("%s: NewExecutor = %v, want nil", tc.name, got)
		}
	}
	// 全依赖在 → 非 nil。
	if got := NewExecutor(env.db, c, sc, env.couponDB, execAdminTok, time.Minute); got == nil {
		t.Fatal("full deps: NewExecutor = nil, want non-nil")
	}
	// interval <=0 回退 1 分钟（不 panic）。
	if got := NewExecutor(env.db, c, sc, env.couponDB, execAdminTok, 0); got == nil {
		t.Fatal("interval<=0: NewExecutor = nil, want non-nil")
	}
}

func TestRunOnce_ScansPending(t *testing.T) {
	e := newExecEnv(t)
	e.seed(t, "order_ok", "game-a", 600, "REFUND")      // → executed
	e.seed(t, "order_rej", "game-a", 600, "CORRECTION") // → rejected
	// 仅 order_rej 拒绝（per-order 控制）；order_ok 接受。
	e.refundRejectOrder = "order_rej"
	e.refundStatus, e.refundCode = http.StatusConflict, "PAYMENT_ORDER_STATE"

	attempted, executed := e.executor().RunOnce()
	if attempted != 2 {
		t.Fatalf("attempted = %d, want 2", attempted)
	}
	if executed != 1 {
		t.Fatalf("executed = %d, want 1", executed)
	}
	ok, _ := e.db.GetRefund("order_ok", "REFUND")
	rej, _ := e.db.GetRefund("order_rej", "CORRECTION")
	if ok.Status != model.RefundStatusExecuted || rej.Status != model.RefundStatusRejected {
		t.Fatalf("statuses ok=%s rej=%s", ok.Status, rej.Status)
	}
}

func TestRunOnce_NilExecutor(t *testing.T) {
	var e *Executor
	if a, x := e.RunOnce(); a != 0 || x != 0 {
		t.Fatalf("RunOnce = %d/%d, want 0/0", a, x)
	}
}

func TestRunOnce_ListRefundsDBError(t *testing.T) {
	// ListRefunds SELECT 失败（关库）→ 0/0（扫描失败不误报）。
	e := newExecEnv(t)
	e.closeDB()
	if a, x := e.executor().RunOnce(); a != 0 || x != 0 {
		t.Fatalf("RunOnce = %d/%d, want 0/0（扫描失败）", a, x)
	}
}

func TestExecuteOne_GetRefundDBError(t *testing.T) {
	// GetRefund 非 NotFound 错误（关库）→ 留 PENDING 重试，不伪造成功。
	e := newExecEnv(t)
	e.closeDB()
	if got := e.executor().ExecuteOne("order_1", "REFUND"); got != Pending {
		t.Fatalf("execution = %q, want pending（读失败留 PENDING）", got)
	}
}

func TestExecuteOne_RefundMissing(t *testing.T) {
	// 意图不存在（非 DB 故障，GetRefund 返 ErrRefundRequestNotFound）→ 无操作留
	// PENDING，不视为失败、不调 Courier。
	e := newExecEnv(t)
	if got := e.executor().ExecuteOne("order_no_such", "REFUND"); got != Pending {
		t.Fatalf("execution = %q, want pending（意图不存在无操作）", got)
	}
	if e.refundCalls != 0 {
		t.Fatalf("courier refund calls = %d, want 0（无意图不调 Courier）", e.refundCalls)
	}
}

func TestExecuteOne_MarkRejectedDBError(t *testing.T) {
	// 确定性拒绝但 MarkRefundRejected 写失败：仍返 Rejected（拒绝判定完成），
	// 写失败仅记日志（best-effort），拒绝原因留痕失败不重试。
	e := newExecEnv(t)
	e.seed(t, "order_1", "game-a", 600, "REFUND")
	e.refundStatus, e.refundCode = http.StatusConflict, "PAYMENT_ORDER_STATE"
	e.addTrigger(t, `CREATE TRIGGER trg_rej BEFORE UPDATE ON refund_requests
		WHEN NEW.status='REJECTED' BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	if got := e.executor().ExecuteOne("order_1", "REFUND"); got != Rejected {
		t.Fatalf("execution = %q, want rejected（写失败不影响拒绝判定）", got)
	}
}

func TestExecuteOne_MarkExecutedDBError(t *testing.T) {
	// Courier 接受但 MarkRefundExecuted 写失败：仍返 Executed（接受判定完成），
	// 写失败仅记日志（best-effort，不因写失败把已接受当失败）。
	e := newExecEnv(t)
	e.seed(t, "order_1", "game-a", 600, "REFUND")
	e.addTrigger(t, `CREATE TRIGGER trg_exec BEFORE UPDATE ON refund_requests
		WHEN NEW.status='EXECUTED' BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	if got := e.executor().ExecuteOne("order_1", "REFUND"); got != Executed {
		t.Fatalf("execution = %q, want executed（写失败不影响接受判定）", got)
	}
}

func TestExecuteOne_ReturnCouponsDBError(t *testing.T) {
	// EXECUTED 后券返还写失败：仅记日志（GP-305 best-effort），执行结果仍
	// EXECUTED——退款事实已定，券返还失败下轮/人工可复。
	e := newExecEnv(t)
	e.seed(t, "order_cp", "game-a", 600, "REFUND")
	e.seedUsedCoupon(t, "order_cp")
	e.addTrigger(t, `CREATE TRIGGER trg_coupon BEFORE UPDATE ON user_coupons
		BEGIN SELECT RAISE(ABORT,'boom'); END;`)
	if got := e.executor().ExecuteOne("order_cp", "REFUND"); got != Executed {
		t.Fatalf("execution = %q, want executed（返还失败不影响执行结果）", got)
	}
}

func TestRun_NilReceiver(t *testing.T) {
	// nil 执行器（未接线）：Run 直接返回，不 panic。
	var e *Executor
	e.Run(context.Background())
}

func TestRun_CancelStops(t *testing.T) {
	// ctx 取消即退出（进程信号面）。
	e := newExecEnv(t).executor()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { e.Run(ctx); close(done) }()
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop on ctx cancel")
	}
}

func TestRun_TickExecutes(t *testing.T) {
	// 周期 tick 执行 PENDING 退款（小间隔驱动），退出后已执行。
	e := newExecEnv(t)
	e.seed(t, "order_1", "game-a", 600, "REFUND")
	exec := NewExecutor(e.db,
		courier.NewClient(e.courierSrv.URL, time.Second),
		courierscope.NewClient(e.scopeSrv.URL, "s2s-tok", time.Second),
		e.couponDB, execAdminTok, 10*time.Millisecond)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { exec.Run(ctx); close(done) }()

	var rf *model.RefundRequest
	deadline := time.Now().Add(3 * time.Second)
	for {
		var err error
		rf, err = e.db.GetRefund("order_1", "REFUND")
		if err == nil && rf != nil && rf.Status == model.RefundStatusExecuted {
			break
		}
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("refund never executed, status=%v err=%v", rf, err)
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

func TestPermanent_NonAPIError(t *testing.T) {
	// 非 APIError（如网络/反序列化错误）不算确定性拒绝 → 瞬态留 PENDING。
	if permanent(errors.New("plain network error")) {
		t.Fatal("permanent(plain) = true, want false")
	}
}

func TestRejectNote_NonAPIError(t *testing.T) {
	// 非 APIError 时 note 用原始错误消息。
	if got := rejectNote(errors.New("boom")); got != "courier 拒绝: boom" {
		t.Fatalf("rejectNote = %q, want courier 拒绝: boom", got)
	}
}

func TestStatusOf(t *testing.T) {
	// 持久态 → 执行态映射（已终态幂等回传现态；其余一律 Pending）。
	if got := statusOf(model.RefundStatusExecuted); got != Executed {
		t.Fatalf("statusOf(EXECUTED) = %q, want executed", got)
	}
	if got := statusOf(model.RefundStatusRejected); got != Rejected {
		t.Fatalf("statusOf(REJECTED) = %q, want rejected", got)
	}
	if got := statusOf(model.RefundStatusPending); got != Pending {
		t.Fatalf("statusOf(PENDING) = %q, want pending", got)
	}
	if got := statusOf("UNKNOWN"); got != Pending {
		t.Fatalf("statusOf(UNKNOWN) = %q, want pending", got)
	}
}
