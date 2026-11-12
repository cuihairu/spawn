package model

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

// newTestAnalyticsModel 临时 SQLite 库 + 建事件表。
func newTestAnalyticsModel(t *testing.T) *AnalyticsEventsModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+filepath.Join(t.TempDir(), "analytics.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	m := NewAnalyticsEventsModel(db)
	if err := m.CreateAnalyticsTable(); err != nil {
		t.Fatalf("create table: %v", err)
	}
	return m
}

func ev(id, typ string, userId int64, gameId string, actId int64, orderId string, amount int64, at string) *AnalyticsEvent {
	return &AnalyticsEvent{
		EventId: id, EventType: typ, UserId: userId, GameId: gameId,
		ActivityId: actId, OrderId: orderId, AmountCents: amount, OccurredAt: at,
	}
}

// TestAnalyticsIngest_Idempotent 幂等摄入：重复 event_id 整行忽略，
// accepted 只计新增；重复建表不报错。
func TestAnalyticsIngest_Idempotent(t *testing.T) {
	m := newTestAnalyticsModel(t)
	if err := m.CreateAnalyticsTable(); err != nil { // 幂等
		t.Fatalf("recreate table: %v", err)
	}

	batch := []*AnalyticsEvent{
		ev("e1", EventRegister, 1, "", 0, "", 0, "2026-11-01T09:00:00Z"),
		ev("e2", EventLogin, 1, "", 0, "", 0, "2026-11-01T09:05:00Z"),
	}
	accepted, err := m.IngestEvents(batch, "2026-11-01T09:06:00Z")
	if err != nil || accepted != 2 {
		t.Fatalf("first ingest accepted=%d err=%v, want 2", accepted, err)
	}
	// 整批重放：全部幂等忽略
	accepted, err = m.IngestEvents(batch, "2026-11-01T09:07:00Z")
	if err != nil || accepted != 0 {
		t.Fatalf("replay ingest accepted=%d err=%v, want 0", accepted, err)
	}
	// 混合：1 旧 1 新
	accepted, err = m.IngestEvents([]*AnalyticsEvent{
		ev("e2", EventLogin, 1, "", 0, "", 0, "2026-11-01T09:05:00Z"),
		ev("e3", EventDownload, 1, "game-a", 0, "", 0, "2026-11-01T10:00:00Z"),
	}, "2026-11-01T10:00:00Z")
	if err != nil || accepted != 1 {
		t.Fatalf("mixed ingest accepted=%d err=%v, want 1", accepted, err)
	}
}

// TestAnalyticsOverview_Filters 聚合：类型计数/去重人数/金额，类型与游戏过滤。
func TestAnalyticsOverview_Filters(t *testing.T) {
	m := newTestAnalyticsModel(t)
	batch := []*AnalyticsEvent{
		ev("o1", EventOrderPaid, 1, "game-a", 0, "ord-1", 600, "2026-11-01T10:00:00Z"),
		ev("o2", EventOrderPaid, 1, "game-a", 0, "ord-2", 300, "2026-11-02T10:00:00Z"),
		ev("o3", EventOrderPaid, 2, "game-b", 0, "ord-3", 900, "2026-11-03T10:00:00Z"),
		ev("o4", EventRegister, 1, "", 0, "", 0, "2026-11-01T09:00:00Z"),
		ev("o5", EventRegister, 2, "", 0, "", 0, "2026-11-04T09:00:00Z"),
		// 窗口外
		ev("o6", EventRegister, 3, "", 0, "", 0, "2026-10-01T09:00:00Z"),
	}
	if _, err := m.IngestEvents(batch, "2026-11-05T00:00:00Z"); err != nil {
		t.Fatalf("ingest: %v", err)
	}

	rows, err := m.Overview(nil, "2026-11-01T00:00:00Z", "2026-11-30T23:59:59Z", "")
	if err != nil {
		t.Fatalf("overview: %v", err)
	}
	got := map[string]TypeCounts{}
	for _, r := range rows {
		got[r.EventType] = r
	}
	if r := got[EventOrderPaid]; r.Events != 3 || r.Users != 2 || r.Amount != 1800 {
		t.Fatalf("paid row = %+v, want events 3 users 2 amount 1800", r)
	}
	if r := got[EventRegister]; r.Events != 2 || r.Users != 2 {
		t.Fatalf("register row = %+v, want events 2 users 2", r)
	}
	if _, ok := got[EventLogin]; ok {
		t.Fatalf("unexpected login row")
	}

	// 类型过滤
	rows, err = m.Overview([]string{EventOrderPaid}, "2026-11-01T00:00:00Z", "2026-11-30T23:59:59Z", "")
	if err != nil || len(rows) != 1 {
		t.Fatalf("typed overview len=%d err=%v, want 1", len(rows), err)
	}
	// 游戏过滤
	rows, err = m.Overview(nil, "2026-11-01T00:00:00Z", "2026-11-30T23:59:59Z", "game-a")
	if err != nil || len(rows) != 1 || rows[0].Events != 2 || rows[0].Amount != 900 {
		t.Fatalf("game-a overview rows=%+v err=%v, want 2 events 900", rows, err)
	}
	// 窗口过滤：o6 落在 10 月（10 月窗口恰好只含它），9 月窗口为空
	rows, err = m.Overview(nil, "2026-10-01T00:00:00Z", "2026-10-31T23:59:59Z", "")
	if err != nil || len(rows) != 1 || rows[0].Events != 1 {
		t.Fatalf("oct overview rows=%+v err=%v, want 1 event", rows, err)
	}
	rows, err = m.Overview(nil, "2026-09-01T00:00:00Z", "2026-09-30T23:59:59Z", "")
	if err != nil || len(rows) != 0 {
		t.Fatalf("sep overview len=%d err=%v, want 0", len(rows), err)
	}
}

// TestAnalyticsRetentionPairs 留存输入：同日同人去重为一条 (day,user) 对。
func TestAnalyticsRetentionPairs(t *testing.T) {
	m := newTestAnalyticsModel(t)
	batch := []*AnalyticsEvent{
		ev("r1", EventLogin, 1, "", 0, "", 0, "2026-11-01T08:00:00Z"),
		ev("r2", EventLogin, 1, "", 0, "", 0, "2026-11-01T20:00:00Z"), // 同日重复
		ev("r3", EventLogin, 1, "", 0, "", 0, "2026-11-02T09:00:00Z"),
		ev("r4", EventLogin, 2, "", 0, "", 0, "2026-11-01T10:00:00Z"),
		ev("r5", EventLogin, 0, "", 0, "", 0, "2026-11-01T11:00:00Z"), // 匿名事件不参与
	}
	if _, err := m.IngestEvents(batch, "2026-11-02T23:00:00Z"); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	pairs, err := m.DailyActiveUsers(EventLogin, "2026-11-01T00:00:00Z", "2026-11-02T23:59:59Z")
	if err != nil {
		t.Fatalf("daily pairs: %v", err)
	}
	want := []DailyUserPair{{"2026-11-01", 1}, {"2026-11-01", 2}, {"2026-11-02", 1}}
	if len(pairs) != len(want) {
		t.Fatalf("pairs = %+v, want %+v", pairs, want)
	}
	for i := range want {
		if pairs[i] != want[i] {
			t.Fatalf("pairs[%d] = %+v, want %+v", i, pairs[i], want[i])
		}
	}
}

// TestAnalyticsActivityEffect 活动效果：发奖笔数/参与人数/奖励金额。
func TestAnalyticsActivityEffect(t *testing.T) {
	m := newTestAnalyticsModel(t)
	batch := []*AnalyticsEvent{
		ev("a1", EventRewardGranted, 1, "game-a", 5, "ord-1", 100, "2026-11-01T10:00:00Z"),
		ev("a2", EventRewardGranted, 1, "game-a", 5, "ord-2", 150, "2026-11-01T11:00:00Z"),
		ev("a3", EventRewardGranted, 2, "game-b", 5, "ord-3", 200, "2026-11-01T12:00:00Z"),
		ev("a4", EventRewardGranted, 3, "game-b", 6, "ord-4", 500, "2026-11-01T13:00:00Z"), // 其他活动
	}
	if _, err := m.IngestEvents(batch, "2026-11-02T00:00:00Z"); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	row, err := m.ActivityEffect(5)
	if err != nil {
		t.Fatalf("activity effect: %v", err)
	}
	if row.ActivityId != 5 || row.Grants != 3 || row.Participants != 2 || row.RewardCents != 450 {
		t.Fatalf("effect = %+v, want grants 3 participants 2 cents 450", row)
	}
	empty, err := m.ActivityEffect(99)
	if err != nil || empty.Grants != 0 {
		t.Fatalf("empty effect = %+v err=%v", empty, err)
	}
}

// TestAnalyticsReconciliation 账单核对：按游戏聚合付费/退款笔数与金额。
func TestAnalyticsReconciliation(t *testing.T) {
	m := newTestAnalyticsModel(t)
	batch := []*AnalyticsEvent{
		ev("c1", EventOrderPaid, 1, "game-a", 0, "ord-1", 600, "2026-11-01T10:00:00Z"),
		ev("c2", EventOrderPaid, 2, "game-a", 0, "ord-2", 400, "2026-11-02T10:00:00Z"),
		ev("c3", EventRefundRequested, 2, "game-a", 0, "ord-2", 400, "2026-11-03T10:00:00Z"),
		ev("c4", EventOrderPaid, 3, "game-b", 0, "ord-3", 900, "2026-11-04T10:00:00Z"),
		ev("c5", EventRegister, 3, "", 0, "", 0, "2026-11-04T09:00:00Z"), // 不入对账
	}
	if _, err := m.IngestEvents(batch, "2026-11-05T00:00:00Z"); err != nil {
		t.Fatalf("ingest: %v", err)
	}
	rows, err := m.Reconciliation("2026-11-01T00:00:00Z", "2026-11-30T23:59:59Z")
	if err != nil {
		t.Fatalf("reconciliation: %v", err)
	}
	got := map[string]ReconciliationRow{}
	for _, r := range rows {
		got[r.GameId] = r
	}
	a := got["game-a"]
	if a.PaidCount != 2 || a.PaidCents != 1000 || a.RefundCount != 1 || a.RefundCents != 400 {
		t.Fatalf("game-a = %+v, want paid 2/1000 refund 1/400", a)
	}
	b := got["game-b"]
	if b.PaidCount != 1 || b.PaidCents != 900 || b.RefundCount != 0 {
		t.Fatalf("game-b = %+v, want paid 1/900 refund 0", b)
	}
}
