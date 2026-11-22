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

// TestAnalyticsPartnerTrendAndActiveUsers GP-404 合作方自助趋势/留存输入口径：
// 按日聚合（事件量 + 去重用户数）与 (日期,user) 去重对，行级隔离同
// PartnerOverview（partner_id 命中、游戏收窄、空归属空集）。
func TestAnalyticsPartnerTrendAndActiveUsers(t *testing.T) {
	m := newTestAnalyticsModel(t)
	batch := []*AnalyticsEvent{
		{EventId: "t1", EventType: EventRegister, UserId: 10, GameId: "game-a", PartnerId: 5, OccurredAt: "2026-11-01T09:00:00Z"},
		{EventId: "t2", EventType: EventRegister, UserId: 11, GameId: "game-a", PartnerId: 5, OccurredAt: "2026-11-02T09:00:00Z"},
		{EventId: "t3", EventType: EventLogin, UserId: 10, GameId: "game-a", PartnerId: 5, OccurredAt: "2026-11-01T10:00:00Z"},
		{EventId: "t4", EventType: EventOrderPaid, UserId: 20, GameId: "game-x", PartnerId: 9, AmountCents: 500, OccurredAt: "2026-11-01T11:00:00Z"},
		{EventId: "t5", EventType: EventLogin, UserId: 30, OccurredAt: "2026-11-01T12:00:00Z"}, // 平台级
		{EventId: "t6", EventType: EventLogin, UserId: 10, GameId: "game-a", PartnerId: 5, OccurredAt: "2026-11-03T09:00:00Z"},
	}
	if accepted, err := m.IngestEvents(batch, "2026-11-05T10:00:00Z"); err != nil || accepted != 6 {
		t.Fatalf("ingest accepted=%d err=%v, want 6", accepted, err)
	}
	from, to := "2026-11-01T00:00:00Z", "2026-11-30T23:59:59Z"

	// 趋势：partner 5 按日聚合（t1/t3 同日、t2、t6 → 3 天）
	trend, err := m.PartnerDailyTrend(from, to, 5, nil)
	if err != nil || len(trend) != 3 {
		t.Fatalf("trend = %+v/%v, want 3 days", trend, err)
	}
	if trend[0].Day != "2026-11-01" || trend[0].Events != 2 || trend[0].Users != 1 {
		t.Fatalf("day1 = %+v, want 11-01 2 events 1 user", trend[0])
	}
	if trend[1].Day != "2026-11-02" || trend[1].Events != 1 || trend[1].Users != 1 {
		t.Fatalf("day2 = %+v, want 11-02 1 event 1 user", trend[1])
	}
	if trend[2].Day != "2026-11-03" || trend[2].Events != 1 || trend[2].Users != 1 {
		t.Fatalf("day3 = %+v, want 11-03 1 event 1 user", trend[2])
	}

	// 游戏收窄与另一合作方：口径互不混入
	if rows, _ := m.PartnerDailyTrend(from, to, 5, []string{"game-a"}); len(rows) != 3 {
		t.Fatalf("game-a trend = %+v, want 3 days", rows)
	}
	if rows, _ := m.PartnerDailyTrend(from, to, 9, nil); len(rows) != 1 || rows[0].Events != 1 || rows[0].Users != 1 {
		t.Fatalf("partner 9 trend = %+v, want 1 day 1 event", rows)
	}
	// 空归属：空集非错误
	if rows, err := m.PartnerDailyTrend(from, to, 0, nil); err != nil || len(rows) != 0 {
		t.Fatalf("zero partner trend = %+v/%v, want 0", rows, err)
	}

	// 留存输入：login 事件 (日期,user) 去重对（u10 两日）
	pairs, err := m.PartnerDailyActiveUsers(EventLogin, from, to, 5, nil)
	if err != nil || len(pairs) != 2 {
		t.Fatalf("login pairs = %+v/%v, want 2", pairs, err)
	}
	if pairs[0].Day != "2026-11-01" || pairs[0].UserId != 10 || pairs[1].Day != "2026-11-03" || pairs[1].UserId != 10 {
		t.Fatalf("login pairs = %+v, want (11-01,10),(11-03,10)", pairs)
	}
	// 全类型（eventType 空）：同人多事件去重为 (日,user) 对
	all, err := m.PartnerDailyActiveUsers("", from, to, 5, nil)
	if err != nil || len(all) != 3 {
		t.Fatalf("all-type pairs = %+v/%v, want 3", all, err)
	}
	if all[1].UserId != 11 || all[2].UserId != 10 {
		t.Fatalf("all-type order = %+v, want user 11 then 10", all)
	}
	if none, err := m.PartnerDailyActiveUsers(EventLogin, from, to, 0, nil); err != nil || len(none) != 0 {
		t.Fatalf("zero partner pairs = %+v/%v, want 0", none, err)
	}
}

// TestAnalyticsPartnerByGame GP-404 合作方自助按游戏下钻：按 (game_id,
// event_type) 聚合（金额只计 order_paid），多游戏并列、行级隔离同款。
func TestAnalyticsPartnerByGame(t *testing.T) {
	m := newTestAnalyticsModel(t)
	batch := []*AnalyticsEvent{
		{EventId: "g1", EventType: EventRegister, UserId: 10, GameId: "game-a", PartnerId: 5, OccurredAt: "2026-11-01T09:00:00Z"},
		{EventId: "g2", EventType: EventRegister, UserId: 11, GameId: "game-a", PartnerId: 5, OccurredAt: "2026-11-02T09:00:00Z"},
		{EventId: "g3", EventType: EventLogin, UserId: 10, GameId: "game-a", PartnerId: 5, OccurredAt: "2026-11-01T10:00:00Z"},
		{EventId: "g4", EventType: EventOrderPaid, UserId: 10, GameId: "game-b", PartnerId: 5, AmountCents: 700, OccurredAt: "2026-11-02T10:00:00Z"},
		// 他人游戏（合作方 9）：不混入
		{EventId: "g5", EventType: EventOrderPaid, UserId: 20, GameId: "game-x", PartnerId: 9, AmountCents: 500, OccurredAt: "2026-11-01T11:00:00Z"},
	}
	if accepted, err := m.IngestEvents(batch, "2026-11-05T10:00:00Z"); err != nil || accepted != 5 {
		t.Fatalf("ingest accepted=%d err=%v, want 5", accepted, err)
	}
	from, to := "2026-11-01T00:00:00Z", "2026-11-30T23:59:59Z"

	rows, err := m.PartnerByGame(from, to, 5, nil)
	if err != nil || len(rows) != 3 {
		t.Fatalf("by game = %+v/%v, want 3 rows", rows, err)
	}
	// 排序 (game_id, event_type)：game-a/login, game-a/register, game-b/order_paid
	if rows[0].GameId != "game-a" || rows[0].EventType != EventLogin || rows[0].Events != 1 || rows[0].Users != 1 || rows[0].Amount != 0 {
		t.Fatalf("row0 = %+v, want game-a login 1/1/0", rows[0])
	}
	if rows[1].GameId != "game-a" || rows[1].EventType != EventRegister || rows[1].Events != 2 || rows[1].Users != 2 {
		t.Fatalf("row1 = %+v, want game-a register 2/2", rows[1])
	}
	if rows[2].GameId != "game-b" || rows[2].EventType != EventOrderPaid || rows[2].Events != 1 || rows[2].Amount != 700 {
		t.Fatalf("row2 = %+v, want game-b paid 1/700", rows[2])
	}

	// 游戏收窄：仅 game-a 两行
	if rows, _ = m.PartnerByGame(from, to, 5, []string{"game-a"}); len(rows) != 2 || rows[0].GameId != "game-a" || rows[1].GameId != "game-a" {
		t.Fatalf("game-a rows = %+v, want 2 game-a rows", rows)
	}
	// 空归属：空集非错误
	if none, err := m.PartnerByGame(from, to, 0, nil); err != nil || len(none) != 0 {
		t.Fatalf("zero partner = %+v/%v, want 0", none, err)
	}
}
