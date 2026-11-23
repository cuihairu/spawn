package model

// 经营分析事件模型失败臂收口。三层手法：
//  1. 真 SQLite 无表库 → 各聚合 Query 错误臂与 Ingest 错误臂；
//  2. 真 SQLite 裸表（缺列/脏行）→ 建表补列迁移、索引失败、TEXT 脏值
//     Scan 失败臂（SQLite 类型序 TEXT > INTEGER，'abc' > 0 为真）；
//  3. 脚本化 driver（sql.OpenDB + Queryer/Execer）→ MySQL 方言回退臂与
//     行迭代失败臂（真 SQLite 驱动无法产生这些错误形态，stub 是这些
//     防御臂唯一诚实的触发方式）。

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ---- 脚本化 driver ----

// stubRule 按 query 子串分流（按序首中即用）。
type stubRule struct {
	match string
	reply stubReply
}

// stubReply 单条脚本：err 非 nil 则 Exec/Query 失败；Query 成功时回
// cols+rows；Exec 成功时回 rowsAffected；nextErr 使行迭代中途失败
// （驱动行级错误 → rows.Err）。
type stubReply struct {
	err          error
	cols         []string
	rows         [][]driver.Value
	rowsAffected int64
	nextErr      error
}

type stubBackend struct{ rules []stubRule }

func (b stubBackend) reply(q string) stubReply {
	for _, r := range b.rules {
		if strings.Contains(q, r.match) {
			return r.reply
		}
	}
	return stubReply{err: fmt.Errorf("stub: unexpected query: %.80s", q)}
}

type stubConnector struct{ backend stubBackend }

func (c stubConnector) Connect(context.Context) (driver.Conn, error) { return stubConn{c.backend}, nil }
func (c stubConnector) Driver() driver.Driver                        { return stubDrv{} }

type stubDrv struct{}

func (stubDrv) Open(string) (driver.Conn, error) { return nil, errors.New("use sql.OpenDB") }

type stubConn struct{ backend stubBackend }

func (stubConn) Prepare(string) (driver.Stmt, error) { return nil, errors.New("use QueryerContext") }
func (stubConn) Close() error                        { return nil }
func (stubConn) Begin() (driver.Tx, error)           { return nil, errors.New("tx unsupported") }

func (c stubConn) ExecContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Result, error) {
	r := c.backend.reply(q)
	if r.err != nil {
		return nil, r.err
	}
	return stubResult{n: r.rowsAffected}, nil
}

func (c stubConn) QueryContext(_ context.Context, q string, _ []driver.NamedValue) (driver.Rows, error) {
	r := c.backend.reply(q)
	if r.err != nil {
		return nil, r.err
	}
	return &stubRows{cols: r.cols, rows: r.rows, nextErr: r.nextErr}, nil
}

type stubResult struct{ n int64 }

func (r stubResult) LastInsertId() (int64, error) { return 0, nil }
func (r stubResult) RowsAffected() (int64, error) { return r.n, nil }

// stubRows nextErr 非 nil 时迭代即失败（驱动行级错误 → rows.Err）。
type stubRows struct {
	cols    []string
	rows    [][]driver.Value
	i       int
	nextErr error
}

func (r *stubRows) Columns() []string { return r.cols }
func (r *stubRows) Close() error      { return nil }
func (r *stubRows) Next(dest []driver.Value) error {
	if r.nextErr != nil {
		return r.nextErr
	}
	if r.i >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.i])
	r.i++
	return nil
}

func newStubAnalyticsModel(rules ...stubRule) *AnalyticsEventsModel {
	return NewAnalyticsEventsModel(sql.OpenDB(stubConnector{stubBackend{rules}}))
}

// ---- 真 SQLite：无表库 ----

// TestAnalyticsAggregates_NoTableQueryErrors 表缺失时各聚合与摄入显式报错
// （不静默返空——空表与坏库必须可区分）。
func TestAnalyticsAggregates_NoTableQueryErrors(t *testing.T) {
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/empty.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	m := NewAnalyticsEventsModel(db)

	if _, err := m.Overview(nil, "2026-01-01", "2026-12-31", ""); err == nil {
		t.Error("overview: want error")
	}
	if _, err := m.DailyActiveUsers("register", "2026-01-01", "2026-12-31"); err == nil {
		t.Error("daily active: want error")
	}
	if _, err := m.ActivityEffect(1); err == nil {
		t.Error("activity effect: want error")
	}
	if _, err := m.Reconciliation("2026-01-01", "2026-12-31"); err == nil {
		t.Error("reconciliation: want error")
	}
	if _, err := m.ChannelEffect("2026-01-01", "2026-12-31", ""); err == nil {
		t.Error("channel effect: want error")
	}
	if _, err := m.PartnerOverview("2026-01-01", "2026-12-31", 1, nil); err == nil {
		t.Error("partner overview: want error")
	}
	if _, err := m.PartnerDailyTrend("2026-01-01", "2026-12-31", 1, nil); err == nil {
		t.Error("partner trend: want error")
	}
	if _, err := m.PartnerDailyActiveUsers("register", "2026-01-01", "2026-12-31", 1, nil); err == nil {
		t.Error("partner dau: want error")
	}
	if _, err := m.PartnerByGame("2026-01-01", "2026-12-31", 1, nil); err == nil {
		t.Error("partner by game: want error")
	}
	if _, err := m.IngestEvents([]*AnalyticsEvent{ev("e1", "register", 1, "g1", 0, "", 0, "")}, "2026-06-01T00:00:00Z"); err == nil {
		t.Error("ingest: want error")
	}
}

// ---- 真 SQLite：裸表（可控列集合） ----

// bareEventsTable 建裸表（不走 CreateAnalyticsTable 的完整 DDL）。
func bareEventsTable(t *testing.T, ddl string) *AnalyticsEventsModel {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/bare.db")
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(ddl); err != nil {
		t.Fatalf("exec ddl: %v", err)
	}
	return NewAnalyticsEventsModel(db)
}

// TestAnalyticsMigrate_BareTableAddsMissingColumns 存量裸表（带 channel、
// 缺 partner_id/currency——channel 被索引引用必须先存在）：CREATE IF NOT
// EXISTS 跳过、索引建成、缺列逐个 ALTER 补齐，迁移后按新列摄入查询贯通。
func TestAnalyticsMigrate_BareTableAddsMissingColumns(t *testing.T) {
	m := bareEventsTable(t, `CREATE TABLE platform_events (
		id INTEGER PRIMARY KEY, event_id TEXT, event_type TEXT, game_id TEXT,
		user_id INTEGER, activity_id INTEGER, order_id TEXT, amount_cents INTEGER,
		occurred_at TEXT, created_at TEXT, channel TEXT)`)
	if err := m.CreateAnalyticsTable(); err != nil {
		t.Fatalf("create on bare table: %v", err)
	}
	// 补列后可按新列摄入与合作方查询
	if n, err := m.IngestEvents([]*AnalyticsEvent{ev("e1", EventRegister, 1, "g1", 0, "", 0, "2026-06-01T00:00:00Z")}, "2026-06-01T00:00:00Z"); err != nil || n != 1 {
		t.Fatalf("ingest after migrate: n=%d err=%v", n, err)
	}
	if _, err := m.PartnerOverview("2026-01-01", "2026-12-31", 1, nil); err != nil {
		t.Fatalf("partner overview after migrate: %v", err)
	}
}

// TestAnalyticsMigrate_AltersLegacyColumnsByPragma PRAGMA 枚举无目标列 →
// 逐列 ALTER 兜底（channel 的索引先于迁移引用它，真实 SQLite 上
// channel-ALTER 臂只有 stub 可达；此处三列迁移补齐全走通）。
func TestAnalyticsMigrate_AltersLegacyColumnsByPragma(t *testing.T) {
	pragma := stubReply{cols: []string{"cid", "name"}, rows: [][]driver.Value{
		{"0", "id"}, {"1", "event_type"}, {"2", "occurred_at"},
	}}
	m := newStubAnalyticsModel(
		stubRule{match: "CREATE TABLE", reply: stubReply{rowsAffected: 0}},
		stubRule{match: "CREATE INDEX", reply: stubReply{rowsAffected: 0}},
		stubRule{match: "PRAGMA table_info", reply: pragma},
		stubRule{match: "ALTER TABLE", reply: stubReply{rowsAffected: 0}},
	)
	if err := m.CreateAnalyticsTable(); err != nil {
		t.Fatalf("legacy columns via pragma: %v", err)
	}
}

// TestAnalyticsMigrate_IndexFailOnMissingColumn 索引引用列缺失 → 建表显式
// 报错（存量表结构异常必须暴露，不带病运行）。
func TestAnalyticsMigrate_IndexFailOnMissingColumn(t *testing.T) {
	m := bareEventsTable(t, `CREATE TABLE platform_events (
		id INTEGER PRIMARY KEY, occurred_at TEXT)`)
	if err := m.CreateAnalyticsTable(); err == nil {
		t.Fatal("missing indexed column must fail table creation")
	}
}

// TestAnalyticsUserPairs_TextUserIdScanError TEXT 脏 user_id（SQLite 类型序
// TEXT>INTEGER 使 'abc' 通过 user_id>0 过滤）→ Scan 失败显式报错。
func TestAnalyticsUserPairs_TextUserIdScanError(t *testing.T) {
	m := bareEventsTable(t, `CREATE TABLE platform_events (
		event_type TEXT, user_id TEXT, occurred_at TEXT, game_id TEXT)`)
	if _, err := m.db.Exec(`INSERT INTO platform_events VALUES
		('register', 'abc', '2026-06-01T00:00:00Z', 'g1')`); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if _, err := m.DailyActiveUsers("register", "2026-01-01", "2026-12-31"); err == nil {
		t.Error("daily active: text user_id must fail scan")
	}
	if _, err := m.PartnerDailyActiveUsers("register", "2026-01-01", "2026-12-31", 0, []string{"g1"}); err == nil {
		t.Error("partner dau: text user_id must fail scan")
	}
}

// ---- 脚本化 driver：方言回退臂 ----

const syntaxErr = `near "AUTOINCREMENT": syntax error`

// TestAnalyticsMigrate_MySQLCreateFallback SQLite 方言建表失败 → MySQL
// 方言成功 → 逐条建索引 → 启动完成。
func TestAnalyticsMigrate_MySQLCreateFallback(t *testing.T) {
	m := newStubAnalyticsModel(
		stubRule{match: "AUTOINCREMENT", reply: stubReply{err: errors.New(syntaxErr)}},
		stubRule{match: "AUTO_INCREMENT", reply: stubReply{rowsAffected: 0}},
		stubRule{match: "CREATE INDEX", reply: stubReply{rowsAffected: 0}},
	)
	if err := m.CreateAnalyticsTable(); err != nil {
		t.Fatalf("mysql fallback path: %v", err)
	}
}

// TestAnalyticsMigrate_MySQLCreateAlsoFails 双方言建表皆败 → 启动显式报错。
func TestAnalyticsMigrate_MySQLCreateAlsoFails(t *testing.T) {
	m := newStubAnalyticsModel(
		stubRule{match: "AUTOINCREMENT", reply: stubReply{err: errors.New(syntaxErr)}},
		stubRule{match: "AUTO_INCREMENT", reply: stubReply{err: errors.New("no innodb")}},
	)
	if err := m.CreateAnalyticsTable(); err == nil {
		t.Fatal("both dialects failing must return error")
	}
}

// TestAnalyticsMigrate_MySQLFallbackIndexFail 回退建表成功但索引失败 →
// 启动显式报错（索引缺失影响查询性能，不静默）。
func TestAnalyticsMigrate_MySQLFallbackIndexFail(t *testing.T) {
	m := newStubAnalyticsModel(
		stubRule{match: "AUTOINCREMENT", reply: stubReply{err: errors.New(syntaxErr)}},
		stubRule{match: "AUTO_INCREMENT", reply: stubReply{rowsAffected: 0}},
		stubRule{match: "CREATE INDEX", reply: stubReply{err: errors.New("no such column")}},
	)
	if err := m.CreateAnalyticsTable(); err == nil {
		t.Fatal("index failure must return error")
	}
}

// TestAnalyticsMigrate_ShowColumnsPath PRAGMA 不可用（MySQL 场景）→
// SHOW COLUMNS 枚举列存在性（方言回退正路径）。
func TestAnalyticsMigrate_ShowColumnsPath(t *testing.T) {
	m := newStubAnalyticsModel(
		stubRule{match: "CREATE TABLE", reply: stubReply{rowsAffected: 0}},
		stubRule{match: "CREATE INDEX", reply: stubReply{rowsAffected: 0}},
		stubRule{match: "PRAGMA table_info", reply: stubReply{err: errors.New("no pragma in mysql")}},
		stubRule{match: "SHOW COLUMNS", reply: stubReply{cols: []string{"Field"},
			rows: [][]driver.Value{{"channel"}, {"partner_id"}, {"currency"}}}},
	)
	if err := m.CreateAnalyticsTable(); err != nil {
		t.Fatalf("show columns path: %v", err)
	}
}

// TestColumnPresent_NameColOutOfRange 列枚举行宽不足 → 判列缺失（不越界）。
func TestColumnPresent_NameColOutOfRange(t *testing.T) {
	m := newStubAnalyticsModel(
		stubRule{match: "CREATE TABLE", reply: stubReply{rowsAffected: 0}},
		stubRule{match: "CREATE INDEX", reply: stubReply{rowsAffected: 0}},
		stubRule{match: "PRAGMA table_info", reply: stubReply{cols: []string{"cid"},
			rows: [][]driver.Value{{int64(0)}}}},
		stubRule{match: "ALTER TABLE", reply: stubReply{rowsAffected: 0}},
	)
	if err := m.CreateAnalyticsTable(); err != nil {
		t.Fatalf("narrow column enumeration: %v", err)
	}
}

// TestColumnPresent_ScanRowError 枚举行值不可收（chan 等任何转换路径都不
// 支持的类型 → NullString.Scan 报错；int64/bool/float/time 均会被 sql 包
// 自动转串，不可用）→ 判列缺失、走 ALTER 兜底（枚举失败跳过不阻断）。
func TestColumnPresent_ScanRowError(t *testing.T) {
	m := newStubAnalyticsModel(
		stubRule{match: "CREATE TABLE", reply: stubReply{rowsAffected: 0}},
		stubRule{match: "CREATE INDEX", reply: stubReply{rowsAffected: 0}},
		stubRule{match: "PRAGMA table_info", reply: stubReply{cols: []string{"cid", "name"},
			rows: [][]driver.Value{{make(chan int), "cid"}}}},
		stubRule{match: "ALTER TABLE", reply: stubReply{rowsAffected: 0}},
	)
	if err := m.CreateAnalyticsTable(); err != nil {
		t.Fatalf("scan-error enumeration: %v", err)
	}
}

// ---- 脚本化 driver：摄入方言回退臂 ----

// TestIngest_MySQLInsertFallback INSERT OR IGNORE 方言失败 → INSERT IGNORE
// 回落成功（accepted 仍按 RowsAffected 计）。
func TestIngest_MySQLInsertFallback(t *testing.T) {
	m := newStubAnalyticsModel(
		stubRule{match: "INSERT OR IGNORE",
			reply: stubReply{err: errors.New(`near "OR IGNORE": syntax error`)}},
		stubRule{match: "INSERT IGNORE", reply: stubReply{rowsAffected: 1}},
	)
	n, err := m.IngestEvents([]*AnalyticsEvent{ev("e1", EventRegister, 1, "g1", 0, "", 0, "")}, "now")
	if err != nil || n != 1 {
		t.Fatalf("ingest fallback: n=%d err=%v", n, err)
	}
}

// TestIngest_MySQLInsertFallbackFails 回落亦失败 → 显式报错（不静默丢事件）。
func TestIngest_MySQLInsertFallbackFails(t *testing.T) {
	m := newStubAnalyticsModel(
		stubRule{match: "INSERT OR IGNORE",
			reply: stubReply{err: errors.New(`near "OR IGNORE": syntax error`)}},
		stubRule{match: "INSERT IGNORE", reply: stubReply{err: errors.New("lock timeout")}},
	)
	if _, err := m.IngestEvents([]*AnalyticsEvent{ev("e1", EventRegister, 1, "g1", 0, "", 0, "")}, "now"); err == nil {
		t.Fatal("fallback failure must return error")
	}
}

// ---- 脚本化 driver：聚合行类型污染 Scan 失败臂 ----

// TestAnalyticsAggregates_HostileRowScanErrors 驱动回传与聚合列类型不符的
// 行（int 目标列顶 time.Time——sql 包把 int64/bool/float/time 都自动转成
// string，唯有 time→*int64 不可转换）→ 各聚合 Scan 显式报错。规则按序
// 首中，前六条精确命中各自查询形状，兜底条命中 Overview。
func TestAnalyticsAggregates_HostileRowScanErrors(t *testing.T) {
	hostile := func(n, timeIdx int) stubReply {
		row := make([]driver.Value, n)
		for i := range row {
			row[i] = int64(i + 1)
		}
		row[timeIdx] = time.Now()
		return stubReply{cols: make([]string, n), rows: [][]driver.Value{row}}
	}
	m := newStubAnalyticsModel(
		stubRule{match: "AS amount_cents", reply: hostile(4, 1)},            // PartnerOverview
		stubRule{match: "AS paid_cents", reply: hostile(4, 1)},              // ChannelEffect
		stubRule{match: "NULLIF(currency", reply: hostile(6, 2)},            // Reconciliation
		stubRule{match: "AS day, COUNT", reply: hostile(3, 1)},              // PartnerDailyTrend
		stubRule{match: "game_id, event_type, COUNT", reply: hostile(5, 2)}, // PartnerByGame
		stubRule{match: "WHERE activity_id", reply: hostile(3, 0)},          // ActivityEffect
		stubRule{match: "SELECT event_type, COUNT", reply: hostile(4, 1)},   // Overview
	)

	if _, err := m.PartnerOverview("a", "b", 1, nil); err == nil {
		t.Error("partner overview: hostile row must fail scan")
	}
	if _, err := m.ChannelEffect("a", "b", ""); err == nil {
		t.Error("channel effect: hostile row must fail scan")
	}
	if _, err := m.Reconciliation("a", "b"); err == nil {
		t.Error("reconciliation: hostile row must fail scan")
	}
	if _, err := m.PartnerDailyTrend("a", "b", 1, nil); err == nil {
		t.Error("partner trend: hostile row must fail scan")
	}
	if _, err := m.PartnerByGame("a", "b", 1, nil); err == nil {
		t.Error("partner by game: hostile row must fail scan")
	}
	if _, err := m.ActivityEffect(1); err == nil {
		t.Error("activity effect: hostile row must fail scan")
	}
	if _, err := m.Overview(nil, "a", "b", ""); err == nil {
		t.Error("overview: hostile row must fail scan")
	}
}

// ---- 脚本化 driver：战绩行迭代失败臂 ----

// TestPlayerStatsListGames_DriverRowIterationError 行迭代中途驱动失败 →
// rows.Err 显式报错（部分数据不静默当全量返回）。
func TestPlayerStatsListGames_DriverRowIterationError(t *testing.T) {
	m := &PlayerStatsModel{db: sql.OpenDB(stubConnector{stubBackend{rules: []stubRule{
		{match: "SELECT COUNT(*) FROM player_stats",
			reply: stubReply{cols: []string{"c"}, rows: [][]driver.Value{{int64(1)}}}},
		{match: "ORDER BY matches DESC",
			reply: stubReply{cols: []string{"game_id"}, nextErr: errors.New("driver boom mid-iterate")}},
	}}})}

	if _, _, err := m.ListGames(7, 50, 0); err == nil || !strings.Contains(err.Error(), "遍历战绩行失败") {
		t.Fatalf("err = %v, want row iteration failure", err)
	}
}
