package model

import (
	"testing"
)

// TestAnalyticsPartnerOverview GP-404 合作方自助数据口径：只统计 partner_id
// 命中的事件（行级隔离），按类型聚合；未标注归属（平台级）事件不混入。
func TestAnalyticsPartnerOverview(t *testing.T) {
	m := newTestAnalyticsModel(t)
	batch := []*AnalyticsEvent{
		{EventId: "q1", EventType: EventRegister, UserId: 10, PartnerId: 5, OccurredAt: "2026-11-01T09:00:00Z"},
		{EventId: "q2", EventType: EventRegister, UserId: 11, PartnerId: 5, OccurredAt: "2026-11-02T09:00:00Z"},
		{EventId: "q3", EventType: EventOrderPaid, UserId: 10, GameId: "game-a", PartnerId: 5, AmountCents: 900, OccurredAt: "2026-11-01T10:00:00Z"},
		{EventId: "q4", EventType: EventRegister, UserId: 20, PartnerId: 9, OccurredAt: "2026-11-01T11:00:00Z"},
		{EventId: "q5", EventType: EventOrderPaid, UserId: 20, GameId: "game-b", PartnerId: 9, AmountCents: 500, OccurredAt: "2026-11-01T12:00:00Z"},
		// 平台级（未标注归属）+ 窗口外
		{EventId: "q6", EventType: EventRegister, UserId: 30, OccurredAt: "2026-11-01T13:00:00Z"},
		{EventId: "q7", EventType: EventOrderPaid, UserId: 10, PartnerId: 5, AmountCents: 200, OccurredAt: "2026-12-05T09:00:00Z"},
	}
	if accepted, err := m.IngestEvents(batch, "2026-12-05T10:00:00Z"); err != nil || accepted != 7 {
		t.Fatalf("ingest accepted=%d err=%v, want 7", accepted, err)
	}

	rows, err := m.PartnerOverview("2026-11-01T00:00:00Z", "2026-11-30T23:59:59Z", 5, nil)
	if err != nil {
		t.Fatalf("partner overview: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("rows = %+v, want 2 types", rows)
	}
	byType := map[string]TypeCounts{}
	for _, r := range rows {
		byType[r.EventType] = r
	}
	if r := byType[EventRegister]; r.Events != 2 || r.Users != 2 {
		t.Fatalf("register = %+v, want 2 events 2 users", r)
	}
	if r := byType[EventOrderPaid]; r.Events != 1 || r.Users != 1 || r.Amount != 900 {
		t.Fatalf("paid = %+v, want 1 event 1 user 900", r)
	}

	// 另一合作方：口径互不混入
	rows, err = m.PartnerOverview("2026-11-01T00:00:00Z", "2026-11-30T23:59:59Z", 9, nil)
	if err != nil {
		t.Fatalf("partner 9 overview: %v", err)
	}
	got := map[string]TypeCounts{}
	for _, r := range rows {
		got[r.EventType] = r
	}
	if r := got[EventOrderPaid]; r.Events != 1 || r.Amount != 500 {
		t.Fatalf("partner 9 paid = %+v, want 1 event 500", r)
	}

	// 游戏收窄：只余 game-a 付费
	rows, err = m.PartnerOverview("2026-11-01T00:00:00Z", "2026-11-30T23:59:59Z", 5, []string{"game-a"})
	if err != nil {
		t.Fatalf("game filter: %v", err)
	}
	if len(rows) != 1 || rows[0].EventType != EventOrderPaid || rows[0].Amount != 900 {
		t.Fatalf("game-a rows = %+v, want single paid 900", rows)
	}

	// 无效 partner id / 空区间：空结果而非错误
	if none, err := m.PartnerOverview("2026-11-01T00:00:00Z", "2026-11-30T23:59:59Z", 0, nil); err != nil || len(none) != 0 {
		t.Fatalf("zero partner = %+v/%v, want 0 rows", none, err)
	}
	if none, err := m.PartnerOverview("2020-01-01T00:00:00Z", "2020-01-02T00:00:00Z", 5, nil); err != nil || len(none) != 0 {
		t.Fatalf("empty window = %+v/%v, want 0 rows", none, err)
	}
}
