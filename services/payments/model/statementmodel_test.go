package model

// GP-404 对账单异议轮次留痕单测：轮次递增、最新一轮结案（复核人/时间）、
// 时间线升序回读、无未结轮次不报错。

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func newStatementModel(t *testing.T) (*StatementModel, int64) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "statement.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	m := NewStatementModel(db)
	if err := m.CreateTables(); err != nil {
		t.Fatalf("create tables: %v", err)
	}
	s := &VendorStatement{
		Vendor: "sandbox", PartnerId: 5,
		PeriodStart: "2026-11-01T00:00:00Z", PeriodEnd: "2026-11-30T23:59:59Z",
		SplitRateBp: 7000, Status: StatementStatusDraft, Diff: 700,
	}
	if err := m.UpsertStatement(s); err != nil {
		t.Fatalf("upsert statement: %v", err)
	}
	got, err := m.GetByKey("sandbox", 5, s.PeriodStart, s.PeriodEnd)
	if err != nil || got.Id == 0 {
		t.Fatalf("get statement: %+v err=%v", got, err)
	}
	return m, got.Id
}

func TestStatementDisputeRounds_RecordResolveAndTimeline(t *testing.T) {
	m, id := newStatementModel(t)

	// 第一轮异议：轮次 1，diff 快照落表
	round, err := m.RecordDisputeRound(id, 2, "结算单与渠道不符", 700)
	if err != nil || round != 1 {
		t.Fatalf("record round1 = %d err=%v, want 1", round, err)
	}
	// 第二轮（第一轮结案前）：轮次递增 2
	round, err = m.RecordDisputeRound(id, 2, "口径需再核对", 850)
	if err != nil || round != 2 {
		t.Fatalf("record round2 = %d err=%v, want 2", round, err)
	}
	rounds, err := m.ListDisputeRounds(id)
	if err != nil || len(rounds) != 2 {
		t.Fatalf("timeline = %d err=%v, want 2", len(rounds), err)
	}
	if rounds[0].RoundNo != 1 || rounds[1].RoundNo != 2 {
		t.Fatalf("order = %d/%d, want 1/2", rounds[0].RoundNo, rounds[1].RoundNo)
	}
	if rounds[0].DiffAtDispute != 700 || rounds[1].DiffAtDispute != 850 {
		t.Fatalf("diff snapshots = %d/%d, want 700/850", rounds[0].DiffAtDispute, rounds[1].DiffAtDispute)
	}

	// 结案最新一轮未结（round 2）：填复核人与时间
	ok, err := m.ResolveLatestDisputeRound(id, 1)
	if err != nil || !ok {
		t.Fatalf("resolve latest = %v err=%v, want true", ok, err)
	}
	rounds, _ = m.ListDisputeRounds(id)
	if rounds[1].ResolvedBy != 1 || rounds[1].ResolvedAt == "" {
		t.Fatalf("round2 resolved = %+v", rounds[1])
	}
	// round 1 仍未结（只结最新）
	if rounds[0].ResolvedBy != 0 || rounds[0].ResolvedAt != "" {
		t.Fatalf("round1 must stay open = %+v", rounds[0])
	}

	// 再结一轮：命中 round 1
	ok, err = m.ResolveLatestDisputeRound(id, 9)
	if err != nil || !ok {
		t.Fatalf("resolve round1 = %v err=%v", ok, err)
	}
	// 无未结轮次：返回 false 不报错
	ok, err = m.ResolveLatestDisputeRound(id, 9)
	if err != nil || ok {
		t.Fatalf("resolve none = %v err=%v, want false", ok, err)
	}

	// 时间线：两轮均结案，轮次与理由保留
	rounds, _ = m.ListDisputeRounds(id)
	if len(rounds) != 2 || rounds[0].ResolvedBy != 9 || rounds[1].ResolvedBy != 1 {
		t.Fatalf("final timeline = %+v", rounds)
	}
}
