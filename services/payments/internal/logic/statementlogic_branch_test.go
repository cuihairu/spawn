package logic

// statementlogic.go 白盒分支：Generate 校验收口（厂商标识/商户/分成比例/周期）、
// 已有 confirmed/disputed 冲突、GetByKey 与 Upsert 失败、List 失败、Get/
// UpdateSettlement/Confirm/ResolveDispute/DisputeRounds/Export 的各错误与成功臂。
//
// 落库失败分两法：闭库（首个 SELECT 即败）与 ?mode=ro 只读连接（SELECT 成功、
// 写入失败——覆盖 Upsert/ClearDispute 这类「先读后写」的失败臂）。

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/payments/internal/notify"
	"github.com/tappi/tappi/services/payments/internal/svc"
	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"

	_ "github.com/mattn/go-sqlite3"
)

func openDB(t *testing.T, dsn string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		t.Fatalf("open %s: %v", dsn, err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func statementSvcAt(t *testing.T, role, dbPath string) *svc.ServiceContext {
	t.Helper()
	st := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"role":"` + role + `","partner_id":1}`))
	}))
	t.Cleanup(st.Close)
	sc := testSvc(st.URL)
	db := openDB(t, "file:"+dbPath)
	if err := model.NewStatementModel(db).CreateTables(); err != nil {
		t.Fatalf("statement tables: %v", err)
	}
	if err := model.NewOrderProjectionModel(db).CreateTables(); err != nil {
		t.Fatalf("projection tables: %v", err)
	}
	if err := model.NewCouponModel(db).CreateTables(); err != nil {
		t.Fatalf("coupon tables: %v", err)
	}
	sc.StatementDB = model.NewStatementModel(db)
	sc.ProjectionDB = model.NewOrderProjectionModel(db)
	sc.NotifySvc = notify.NewClient("http://127.0.0.1:1", "t", 500, nil) // fire-and-forget
	return sc
}

func statementSvc(t *testing.T, role string) *svc.ServiceContext {
	t.Helper()
	return statementSvcAt(t, role, filepath.Join(t.TempDir(), "st.db"))
}

func validGenReq() types.GenerateStatementRequest {
	return types.GenerateStatementRequest{
		Vendor: "v1", PartnerId: 2, PeriodStart: "2026-01-01", PeriodEnd: "2026-01-31", SplitRateBp: 7000,
	}
}

func genReq() *types.GenerateStatementRequest {
	r := validGenReq()
	return &r
}

func TestStatementLogic_GenerateValidationArms(t *testing.T) {
	sc := statementSvc(t, "platform_admin")
	l := NewStatementLogic(wbCtx(), sc)

	// 厂商标识为空 → 400
	r := validGenReq()
	r.Vendor = ""
	if _, err := l.Generate(&r, "Bearer t"); err == nil {
		t.Fatal("empty vendor = nil error, want 400")
	}
	// 商户非正 → 400
	r = validGenReq()
	r.PartnerId = 0
	if _, err := l.Generate(&r, "Bearer t"); err == nil {
		t.Fatal("bad partner = nil error, want 400")
	}
	// 分成比例越界 → 400
	r = validGenReq()
	r.SplitRateBp = 10001
	if _, err := l.Generate(&r, "Bearer t"); err == nil {
		t.Fatal("bad split = nil error, want 400")
	}
	// 周期不合法 → 400
	r = validGenReq()
	r.PeriodStart = ""
	if _, err := l.Generate(&r, "Bearer t"); err == nil {
		t.Fatal("empty period = nil error, want 400")
	}
	r = validGenReq()
	r.PeriodStart, r.PeriodEnd = "2026-02-01", "2026-01-01"
	if _, err := l.Generate(&r, "Bearer t"); err == nil {
		t.Fatal("inverted period = nil error, want 400")
	}
	// 角色守卫 → 403
	if _, err := NewStatementLogic(wbCtx(), statementSvc(t, "player")).Generate(genReq(), "Bearer t"); err == nil {
		t.Fatal("player generate = nil error, want 403")
	}
}

func TestStatementLogic_GenerateConflictAndErrorArms(t *testing.T) {
	sc := statementSvc(t, "platform_admin")
	l := NewStatementLogic(wbCtx(), sc)
	r := validGenReq()

	// 首次生成成功
	if _, err := l.Generate(&r, "Bearer t"); err != nil {
		t.Fatalf("generate = %v", err)
	}
	// 已确认 → 409
	sc.StatementDB.ConfirmStatement(1, 7)
	if _, err := l.Generate(&r, "Bearer t"); err == nil {
		t.Fatal("confirmed regenerate = nil error, want 409")
	}
	// 已异议 → 409
	sc.StatementDB.DisputeStatement(1, 7, "reason")
	if _, err := l.Generate(&r, "Bearer t"); err == nil {
		t.Fatal("disputed regenerate = nil error, want 409")
	}
}

func TestStatementLogic_GenerateGetByKeyError(t *testing.T) {
	// 闭库：GetByKey SELECT 即败 → 500
	closed := statementSvc(t, "platform_admin")
	closed.StatementDB = model.NewStatementModel(mustClosedDB(t))
	if _, err := NewStatementLogic(wbCtx(), closed).Generate(genReq(), "Bearer t"); err == nil {
		t.Fatal("get by key closed = nil error, want 500")
	}
}

func TestStatementLogic_GenerateUpsertError(t *testing.T) {
	// 只读连接：GetByKey/聚合 SELECT 成功，Upsert INSERT 败 → 500
	sc := statementSvc(t, "platform_admin")
	dbPath := filepath.Join(t.TempDir(), "st-ro.db")
	real := openDB(t, "file:"+dbPath)
	_ = model.NewStatementModel(real).CreateTables()
	_ = real.Close()
	ro := openDB(t, "file:"+dbPath+"?mode=ro")
	sc.StatementDB = model.NewStatementModel(ro)
	if _, err := NewStatementLogic(wbCtx(), sc).Generate(genReq(), "Bearer t"); err == nil {
		t.Fatal("upsert ro = nil error, want 500")
	}
}

func TestStatementLogic_ListGetArms(t *testing.T) {
	sc := statementSvc(t, "platform_admin")
	l := NewStatementLogic(wbCtx(), sc)
	_, _ = l.Generate(genReq(), "Bearer t")

	// List 成功 + 角色守卫 + 查询失败
	if _, err := l.List(&types.StatementListRequest{}, "Bearer t"); err != nil {
		t.Fatalf("list = %v", err)
	}
	if _, err := NewStatementLogic(wbCtx(), statementSvc(t, "player")).List(&types.StatementListRequest{}, "Bearer t"); err == nil {
		t.Fatal("player list = nil error, want 403")
	}
	closed := statementSvc(t, "platform_admin")
	closed.StatementDB = model.NewStatementModel(mustClosedDB(t))
	if _, err := NewStatementLogic(wbCtx(), closed).List(&types.StatementListRequest{}, "Bearer t"); err == nil {
		t.Fatal("list closed = nil error, want 500")
	}

	// Get 成功 / 不存在 / 查询失败
	if _, err := l.Get(&types.StatementPathRequest{Id: 1}, "Bearer t"); err != nil {
		t.Fatalf("get = %v", err)
	}
	if _, err := l.Get(&types.StatementPathRequest{Id: 999}, "Bearer t"); err == nil {
		t.Fatal("get not found = nil error, want 404")
	}
	if _, err := NewStatementLogic(wbCtx(), closed).Get(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("get closed = nil error, want 500")
	}
}

func TestStatementLogic_UpdateSettlementArms(t *testing.T) {
	sc := statementSvc(t, "platform_admin")
	l := NewStatementLogic(wbCtx(), sc)
	_, _ = l.Generate(genReq(), "Bearer t")

	// 负金额 → 400
	if _, err := l.UpdateSettlement(&types.UpdateSettlementRequest{Id: 1, Settlement: -5}, "Bearer t"); err == nil {
		t.Fatal("negative settlement = nil error, want 400")
	}
	// 不存在 → 404
	if _, err := l.UpdateSettlement(&types.UpdateSettlementRequest{Id: 999, Settlement: 1}, "Bearer t"); err == nil {
		t.Fatal("settlement not found = nil error, want 404")
	}
	// 成功
	if _, err := l.UpdateSettlement(&types.UpdateSettlementRequest{Id: 1, Settlement: 100}, "Bearer t"); err != nil {
		t.Fatalf("settlement = %v", err)
	}
	// 已确认 → 409
	sc.StatementDB.ConfirmStatement(1, 7)
	if _, err := l.UpdateSettlement(&types.UpdateSettlementRequest{Id: 1, Settlement: 50}, "Bearer t"); err == nil {
		t.Fatal("settlement confirmed = nil error, want 409")
	}
}

func TestStatementLogic_ConfirmArms(t *testing.T) {
	sc := statementSvc(t, "platform_admin")
	l := NewStatementLogic(wbCtx(), sc)
	_, _ = l.Generate(genReq(), "Bearer t")

	if _, err := l.Confirm(&types.StatementPathRequest{Id: 999}, "Bearer t"); err == nil {
		t.Fatal("confirm not found = nil error, want 404")
	}
	if _, err := l.Confirm(&types.StatementPathRequest{Id: 1}, "Bearer t"); err != nil {
		t.Fatalf("confirm = %v", err)
	}
	if _, err := l.Confirm(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("reconfirm = nil error, want 409")
	}
}

func TestStatementLogic_ResolveDisputeArms(t *testing.T) {
	sc := statementSvc(t, "platform_admin")
	l := NewStatementLogic(wbCtx(), sc)
	_, _ = l.Generate(genReq(), "Bearer t")

	// 不存在 → 404
	if _, err := l.ResolveDispute(&types.StatementPathRequest{Id: 999}, "Bearer t"); err == nil {
		t.Fatal("resolve not found = nil error, want 404")
	}
	// 无待处理异议（草稿）→ 409
	if _, err := l.ResolveDispute(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("resolve not disputed = nil error, want 409")
	}
	// 有未结轮次：DisputeStatement 落一轮 → ClearDispute + ResolveLatestDisputeRound ok
	sc.StatementDB.DisputeStatement(1, 7, "reason")
	if _, err := l.ResolveDispute(&types.StatementPathRequest{Id: 1}, "Bearer t"); err != nil {
		t.Fatalf("resolve = %v", err)
	}
}

func TestStatementLogic_ResolveDisputeNoRound(t *testing.T) {
	sc := statementSvc(t, "platform_admin")
	l := NewStatementLogic(wbCtx(), sc)
	_, _ = l.Generate(genReq(), "Bearer t")
	// 存量单据场景：置 disputed 但不落轮次 → ResolveLatestDisputeRound 返回
	// false（Infof 臂），不视为失败。
	sc.StatementDB.DisputeStatement(1, 7, "r")
	if _, err := l.ResolveDispute(&types.StatementPathRequest{Id: 1}, "Bearer t"); err != nil {
		t.Fatalf("resolve no round = %v", err)
	}
}

func TestStatementLogic_ResolveDisputeClearError(t *testing.T) {
	// 同一库文件先落 disputed 事实，再换只读连接：Get 读到 disputed，
	// ClearDispute UPDATE 败 → 500
	dbPath := filepath.Join(t.TempDir(), "st-clr.db")
	sc := statementSvcAt(t, "platform_admin", dbPath)
	l := NewStatementLogic(wbCtx(), sc)
	_, _ = l.Generate(genReq(), "Bearer t")
	sc.StatementDB.DisputeStatement(1, 7, "reason")
	ro := openDB(t, "file:"+dbPath+"?mode=ro")
	sc.StatementDB = model.NewStatementModel(ro)
	if _, err := l.ResolveDispute(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("clear dispute ro = nil error, want 500")
	}
}

func TestStatementLogic_DisputeRoundsArms(t *testing.T) {
	sc := statementSvc(t, "platform_admin")
	l := NewStatementLogic(wbCtx(), sc)
	_, _ = l.Generate(genReq(), "Bearer t")
	sc.StatementDB.DisputeStatement(1, 7, "why")
	// DisputeStatement 只改 vendor_statements 状态，轮次要单独落。
	sc.StatementDB.RecordDisputeRound(1, 7, "why", 100)

	resp, err := l.DisputeRounds(&types.StatementPathRequest{Id: 1}, "Bearer t")
	if err != nil || len(resp.Items) != 1 || resp.Items[0].DisputeReason != "why" {
		t.Fatalf("rounds = %+v err=%v", resp, err)
	}
	if _, err := l.DisputeRounds(&types.StatementPathRequest{Id: 999}, "Bearer t"); err == nil {
		t.Fatal("rounds not found = nil error, want 404")
	}
	closed := statementSvc(t, "platform_admin")
	closed.StatementDB = model.NewStatementModel(mustClosedDB(t))
	if _, err := NewStatementLogic(wbCtx(), closed).DisputeRounds(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("rounds closed = nil error, want 500")
	}
}

func TestStatementLogic_ExportArms(t *testing.T) {
	sc := statementSvc(t, "platform_admin")
	l := NewStatementLogic(wbCtx(), sc)
	_, _ = l.Generate(genReq(), "Bearer t")

	filename, data, err := l.Export(&types.StatementPathRequest{Id: 1}, "Bearer t")
	if err != nil || filename == "" || len(data) == 0 {
		t.Fatalf("export = (%q,%d,%v)", filename, len(data), err)
	}
	if _, _, err := l.Export(&types.StatementPathRequest{Id: 999}, "Bearer t"); err == nil {
		t.Fatal("export not found = nil error, want 404")
	}
}
