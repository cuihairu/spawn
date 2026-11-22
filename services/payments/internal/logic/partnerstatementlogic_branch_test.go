package logic

// partnerstatementlogic.go 剩余补漏分支：List/Get/Export/Confirm/Dispute/
// Differences 的角色守卫 403 臂（非 partner 角色）、GetForPartner 查询失败
// 500、Export 明细表缺失 500、Confirm/Dispute 落库失败 500（只读连接先读后写
// 败）、Dispute 理由过长 400 与 404。partner 作用域经 statementSvcAt 的目录
// 存根（role=partner, partner_id=1）进入逻辑层。

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/tappi/tappi/services/payments/internal/types"
	"github.com/tappi/tappi/services/payments/model"
)

func mustPartnerRow(t *testing.T, m *model.StatementModel, partnerID int64) int64 {
	t.Helper()
	s := &model.VendorStatement{
		Vendor: "sandbox", PartnerId: partnerID,
		PeriodStart: "2026-01-01", PeriodEnd: "2026-01-31",
		PaidTotal: 1000, DiscountTotal: 0, RefundTotal: 0,
		SplitBase: 1000, SplitRateBp: 7000, SplitAmount: 700,
		Settlement: 0, Diff: 700, OrderCount: 1, Status: model.StatementStatusDraft,
		CreatedBy: 1,
	}
	if err := m.UpsertStatement(s); err != nil {
		t.Fatalf("upsert statement: %v", err)
	}
	got, err := m.GetByKey(s.Vendor, partnerID, s.PeriodStart, s.PeriodEnd)
	if err != nil {
		t.Fatalf("get by key: %v", err)
	}
	return got.Id
}

func TestPartnerStatement_ForbiddenArms(t *testing.T) {
	// 非 partner 角色 → requirePartnerRole 403（List/Get/Export/Confirm/Dispute/Differences）
	sc := statementSvc(t, "player")
	l := NewPartnerStatementLogic(wbCtx(), sc)

	if _, err := l.List(&types.PartnerStatementListRequest{}, "Bearer t"); err == nil {
		t.Fatal("player list = nil error, want 403")
	}
	if _, err := l.Get(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("player get = nil error, want 403")
	}
	if _, _, err := l.Export(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("player export = nil error, want 403")
	}
	if _, err := l.Confirm(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("player confirm = nil error, want 403")
	}
	if _, err := l.Dispute(&types.DisputeStatementRequest{Id: 1, Reason: "ab"}, "Bearer t"); err == nil {
		t.Fatal("player dispute = nil error, want 403")
	}
	if _, err := l.Differences(&types.StatementListRequest{}, "Bearer t"); err == nil {
		t.Fatal("player differences = nil error, want 403")
	}
}

func TestPartnerStatement_QueryErrorArms(t *testing.T) {
	// GetForPartner 首个查询即败 → 500（Get/Export）
	closed := statementSvc(t, "partner")
	closed.StatementDB = model.NewStatementModel(mustClosedDB(t))
	if _, err := NewPartnerStatementLogic(wbCtx(), closed).Get(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("get closed = nil error, want 500")
	}
	if _, _, err := NewPartnerStatementLogic(wbCtx(), closed).Export(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("export closed = nil error, want 500")
	}
	if _, err := NewPartnerStatementLogic(wbCtx(), closed).Confirm(&types.StatementPathRequest{Id: 1}, "Bearer t"); err == nil {
		t.Fatal("confirm closed = nil error, want 500")
	}
	if _, err := NewPartnerStatementLogic(wbCtx(), closed).Dispute(&types.DisputeStatementRequest{Id: 1, Reason: "ab"}, "Bearer t"); err == nil {
		t.Fatal("dispute closed = nil error, want 500")
	}
}

func TestPartnerStatement_LineItemsMissingTable(t *testing.T) {
	// GetForPartner ok（vendor_statements 在），明细表 order_projection 缺失 →
	// StatementLineItems 失败 → 500。此库只建对账单表。
	st := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"role":"partner","partner_id":1}`))
	}))
	t.Cleanup(st.Close)
	sc := testSvc(st.URL)
	db := openDB(t, "file:"+filepath.Join(t.TempDir(), "ps-only.db"))
	if err := model.NewStatementModel(db).CreateTables(); err != nil {
		t.Fatalf("statement tables: %v", err)
	}
	stmtDB := model.NewStatementModel(db)
	sc.StatementDB = stmtDB
	sc.ProjectionDB = model.NewOrderProjectionModel(db) // 表未建；本用例不触审计
	id := mustPartnerRow(t, stmtDB, 1)
	if _, _, err := NewPartnerStatementLogic(wbCtx(), sc).Export(&types.StatementPathRequest{Id: id}, "Bearer t"); err == nil {
		t.Fatal("line items missing table = nil error, want 500")
	}
}

func TestPartnerStatement_WriteErrorArms(t *testing.T) {
	// 先落一张草稿单据（partner 1），再换只读连接：GetForPartner 读到，
	// ConfirmStatement / DisputeStatement 写入败 → 500
	dbPath := filepath.Join(t.TempDir(), "ps-ro.db")
	sc := statementSvcAt(t, "partner", dbPath)
	id := mustPartnerRow(t, sc.StatementDB, 1)
	ro := openDB(t, "file:"+dbPath+"?mode=ro")
	sc.StatementDB = model.NewStatementModel(ro)

	if _, err := NewPartnerStatementLogic(wbCtx(), sc).Confirm(&types.StatementPathRequest{Id: id}, "Bearer t"); err == nil {
		t.Fatal("confirm ro = nil error, want 500")
	}
	if _, err := NewPartnerStatementLogic(wbCtx(), sc).Dispute(&types.DisputeStatementRequest{Id: id, Reason: "ab"}, "Bearer t"); err == nil {
		t.Fatal("dispute ro = nil error, want 500")
	}
}

func TestPartnerStatement_DisputeReasonAndNotFound(t *testing.T) {
	sc := statementSvc(t, "partner")
	l := NewPartnerStatementLogic(wbCtx(), sc)

	// 理由过长 → 400
	if _, err := l.Dispute(&types.DisputeStatementRequest{Id: 1, Reason: string(make([]byte, 300))}, "Bearer t"); err == nil {
		t.Fatal("long reason = nil error, want 400")
	}
	// 不存在 → 404（Get/Confirm/Dispute）
	if _, err := l.Get(&types.StatementPathRequest{Id: 99999}, "Bearer t"); err == nil {
		t.Fatal("get not found = nil error, want 404")
	}
	if _, err := l.Confirm(&types.StatementPathRequest{Id: 99999}, "Bearer t"); err == nil {
		t.Fatal("confirm not found = nil error, want 404")
	}
	if _, err := l.Dispute(&types.DisputeStatementRequest{Id: 99999, Reason: "ab"}, "Bearer t"); err == nil {
		t.Fatal("dispute not found = nil error, want 404")
	}
}
