package logic

// statementlogic.go 剩余补漏分支：Get/UpdateSettlement/Confirm/ResolveDispute 的
// 角色守卫 403、Get 查询失败 500、写入失败 500（只读连接）、异议轮次表缺失
// 500、Export 明细表缺失 500、Generate 的优惠聚合失败 500（无 coupon 表）、
// Upsert 失败 500（全表只读）与重生成保留结算单臂。

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"
)

func TestStatement_GuardArms(t *testing.T) {
	// 非管理员角色 → requireAdmin 403（Get/UpdateSettlement/Confirm/ResolveDispute）
	sc := statementSvc(t, "player")
	l := NewStatementLogic(wbCtx(), sc)
	if _, err := l.Get(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("player get = nil error, want 403")
	}
	if _, err := l.UpdateSettlement(&types.UpdateSettlementRequest{Id: 1, Settlement: 1}, "Bearer t"); err == nil {
		t.Fatal("player settlement = nil error, want 403")
	}
	if _, err := l.Confirm(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("player confirm = nil error, want 403")
	}
	if _, err := l.ResolveDispute(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("player resolve = nil error, want 403")
	}
}

func TestStatement_GetErrorArms(t *testing.T) {
	// 首个查询即败 → 500（Get/UpdateSettlement/ResolveDispute/Export）
	closed := statementSvc(t, "platform_admin")
	closed.StatementDB = model.NewStatementModel(mustClosedDB(t))
	l := NewStatementLogic(wbCtx(), closed)
	if _, err := l.Get(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("get closed = nil error, want 500")
	}
	if _, err := l.UpdateSettlement(&types.UpdateSettlementRequest{Id: 1, Settlement: 1}, "Bearer t"); err == nil {
		t.Fatal("settlement closed = nil error, want 500")
	}
	if _, err := l.ResolveDispute(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("resolve closed = nil error, want 500")
	}
	if _, _, err := l.Export(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("export closed = nil error, want 500")
	}
}

func TestStatement_WriteErrorArms(t *testing.T) {
	// 先落一张草稿，再换只读连接：Get 读到，UpdateSettlement/Confirm 写入败 → 500
	dbPath := filepath.Join(t.TempDir(), "st-wr.db")
	sc := statementSvcAt(t, "platform_admin", dbPath)
	id := mustPartnerRow(t, sc.StatementDB, 1)
	ro := openDB(t, "file:"+dbPath+"?mode=ro")
	sc.StatementDB = model.NewStatementModel(ro)
	l := NewStatementLogic(wbCtx(), sc)

	if _, err := l.UpdateSettlement(&types.UpdateSettlementRequest{Id: id, Settlement: 100}, "Bearer t"); err == nil {
		t.Fatal("settlement ro = nil error, want 500")
	}
	if _, err := l.Confirm(&types.StatementPathRequest{Id: id}, "Bearer t"); err == nil {
		t.Fatal("confirm ro = nil error, want 500")
	}
}

func TestStatement_DisputeRoundTableMissing(t *testing.T) {
	// 置 disputed 后删异议轮次表：ClearDispute ok（vendor_statements 在），
	// ResolveLatestDisputeRound 败（无表）→ 500
	dbPath := filepath.Join(t.TempDir(), "st-rnd.db")
	sc := statementSvcAt(t, "platform_admin", dbPath)
	l := NewStatementLogic(wbCtx(), sc)
	id := mustPartnerRow(t, sc.StatementDB, 1)
	sc.StatementDB.DisputeStatement(id, 7, "why")
	// 另开一条同一文件的连接删表（模型不暴露底层 *sql.DB）
	db := openDB(t, "file:"+dbPath)
	db.Exec(`DROP TABLE statement_dispute_history`)
	// 结案轮次留痕失败不阻断主流程（仅日志）→ 返回 nil，但已触 Errorf 臂
	if _, err := l.ResolveDispute(&types.StatementPathRequest{Id: id}, "Bearer t"); err != nil {
		t.Fatalf("resolve round table missing = %v, want graceful nil", err)
	}
}

func TestStatement_DisputeRoundsTableMissing(t *testing.T) {
	// Get ok，ListDisputeRounds 败（无表）→ 500
	dbPath := filepath.Join(t.TempDir(), "st-rounds.db")
	sc := statementSvcAt(t, "platform_admin", dbPath)
	l := NewStatementLogic(wbCtx(), sc)
	id := mustPartnerRow(t, sc.StatementDB, 1)
	db := openDB(t, "file:"+dbPath)
	db.Exec(`DROP TABLE statement_dispute_history`)
	if _, err := l.DisputeRounds(&types.StatementPathRequest{Id: id}, "Bearer t"); err == nil {
		t.Fatal("rounds table missing = nil error, want 500")
	}
}

func TestStatement_ExportLineItemsMissing(t *testing.T) {
	// Get ok（vendor_statements 在），明细表 order_projection 缺失 → 500
	st := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"role":"platform_admin","partner_id":1}`))
	}))
	t.Cleanup(st.Close)
	sc := testSvc(st.URL)
	db := openDB(t, "file:"+filepath.Join(t.TempDir(), "st-li.db"))
	if err := model.NewStatementModel(db).CreateTables(); err != nil {
		t.Fatalf("statement tables: %v", err)
	}
	stmtDB := model.NewStatementModel(db)
	sc.StatementDB = stmtDB
	sc.ProjectionDB = model.NewOrderProjectionModel(db)
	id := mustPartnerRow(t, stmtDB, 1)
	if _, _, err := NewStatementLogic(wbCtx(), sc).Export(&types.StatementPathRequest{Id: id}, "Bearer t"); err == nil {
		t.Fatal("export line items missing = nil error, want 500")
	}
}

func TestStatement_GenerateDiscountAggregateError(t *testing.T) {
	// 无 coupon 表：paid/refund 聚合 ok，discount 聚合败 → 500
	st := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"role":"platform_admin","partner_id":1}`))
	}))
	t.Cleanup(st.Close)
	sc := testSvc(st.URL)
	db := openDB(t, "file:"+filepath.Join(t.TempDir(), "st-dc.db"))
	if err := model.NewStatementModel(db).CreateTables(); err != nil {
		t.Fatalf("statement tables: %v", err)
	}
	if err := model.NewOrderProjectionModel(db).CreateTables(); err != nil {
		t.Fatalf("projection tables: %v", err)
	}
	sc.StatementDB = model.NewStatementModel(db)
	sc.ProjectionDB = model.NewOrderProjectionModel(db)
	if _, err := NewStatementLogic(wbCtx(), sc).Generate(genReq(), "Bearer t"); err == nil {
		t.Fatal("discount aggregate missing = nil error, want 500")
	}
}

func TestStatement_GenerateUpsertErrorFull(t *testing.T) {
	// 全表只读：各聚合 SELECT ok，UpsertStatement 写入败 → 500
	dbPath := filepath.Join(t.TempDir(), "st-up.db")
	sc := statementSvcAt(t, "platform_admin", dbPath)
	ro := openDB(t, "file:"+dbPath+"?mode=ro")
	sc.StatementDB = model.NewStatementModel(ro)
	if _, err := NewStatementLogic(wbCtx(), sc).Generate(genReq(), "Bearer t"); err == nil {
		t.Fatal("upsert full ro = nil error, want 500")
	}
}

func TestStatement_GeneratePreserveSettlement(t *testing.T) {
	// 重生成保留已录入结算单：existing != nil → settlement 沿用
	sc := statementSvc(t, "platform_admin")
	l := NewStatementLogic(wbCtx(), sc)
	if _, err := l.Generate(genReq(), "Bearer t"); err != nil {
		t.Fatalf("generate #1 = %v", err)
	}
	row, err := sc.StatementDB.GetByKey("v1", 2, "2026-01-01", "2026-01-31")
	if err != nil {
		t.Fatalf("get by key: %v", err)
	}
	if _, err := l.UpdateSettlement(&types.UpdateSettlementRequest{Id: row.Id, Settlement: 50}, "Bearer t"); err != nil {
		t.Fatalf("settlement = %v", err)
	}
	resp, err := l.Generate(genReq(), "Bearer t")
	if err != nil {
		t.Fatalf("generate #2 = %v", err)
	}
	if resp.Statement.Settlement != 50 {
		t.Fatalf("regenerate settlement = %d, want 50", resp.Statement.Settlement)
	}
}
