package model

// GP-402 经营分析事件域（F24）：平台级事实事件（注册/活跃/下载/付费/
// 发奖/退款）追加写入 + 指标聚合。与战绩域（player_stats，玩家视角只读
// 展示）明确分离：本域为经营分析数据面，仅服务端事实写入（S2S 摄入
// event_id 幂等），金额字段一律来自产生事实的服务（账单从交易事实核对，
// 客户端不可上报金额）。

import (
	"database/sql"
	"fmt"
	"strings"
)

// 事件类型口径（F24）：register 注册、login 活跃（登录即活跃心跳）、
// download 包体下载、order_paid 支付成功、refund_requested 退款请求
// （执行器未接，EXECUTED 事件待 Courier 退款端点后补）、reward_granted 活动发奖。
const (
	EventRegister        = "register"
	EventLogin           = "login"
	EventDownload        = "download"
	EventOrderPaid       = "order_paid"
	EventRefundRequested = "refund_requested"
	EventRewardGranted   = "reward_granted"
)

var eventTypes = map[string]bool{
	EventRegister: true, EventLogin: true, EventDownload: true,
	EventOrderPaid: true, EventRefundRequested: true, EventRewardGranted: true,
}

// ValidEventType 事件类型白名单（摄入侧校验，未知类型拒绝）。
func ValidEventType(t string) bool { return eventTypes[t] }

// AnalyticsEvent 单条平台事实事件（event_id 为幂等锚，重复摄入忽略）。
type AnalyticsEvent struct {
	EventId     string `json:"event_id"`
	EventType   string `json:"event_type"`
	UserId      int64  `json:"user_id"`
	GameId      string `json:"game_id"`
	ActivityId  int64  `json:"activity_id"`
	OrderId     string `json:"order_id"`
	AmountCents int64  `json:"amount_cents"`
	// Channel GP-403 渠道标识（单层渠道 code，如 my-app.com 的推广渠道）。
	// 注册/下载等事件可带渠道，用于按渠道看漏斗；空 = 未归因/自然流量。
	Channel    string `json:"channel"`
	OccurredAt string `json:"occurred_at"` // RFC3339；空则由摄入侧补当前时间
}

// TypeCounts 单类型计数（指标查询中间行）。
type TypeCounts struct {
	EventType string
	Events    int64
	Users     int64
	Amount    int64
}

// OverviewRow 经营总览行：按事件类型聚合计数/人数/金额。
type OverviewRow struct {
	From    string       `json:"from"`
	To      string       `json:"to"`
	GameId  string       `json:"game_id,omitempty"`
	ByTypes []TypeCounts `json:"by_types"`
}

// DailyUserPair 留存计算输入：某日活跃的去重用户（日期取 occurred_at 前 10 位）。
type DailyUserPair struct {
	Day    string
	UserId int64
}

// ActivityEffectRow 活动效果（F24）：按 activity_id 的事件事实口径。
// reward_cents = reward_granted 事件上报的金额合计，即该活动发放所对应
// 订单实付合计（活动带动 GMV / ROI 口径）；该字段不参与账单对账聚合
// （对账只统计 order_paid / refund_requested）。
type ActivityEffectRow struct {
	ActivityId   int64 `json:"activity_id"`
	Grants       int64 `json:"grants"`
	Participants int64 `json:"participants"`
	RewardCents  int64 `json:"reward_cents"`
}

// ChannelEffectRow 渠道效果行（F22 渠道包追踪/归因口径，GP-403）：
// 按渠道标识聚合注册人数与付费金额。channel 空串代表自然流量（未归因）。
type ChannelEffectRow struct {
	Channel       string `json:"channel"`
	Registrations int64  `json:"registrations"`
	Users         int64  `json:"users"`
	PaidCents     int64  `json:"paid_cents"`
}

// ReconciliationRow 账单核对行（从交易事实核对，F24）：按游戏聚合
// 付费/退款事件金额与笔数；与 payments 对账单（order_projection 事实）
// 比对即可核对（两端均源自同一支付事实，事件侧仅做转发副本）。
type ReconciliationRow struct {
	GameId      string `json:"game_id"`
	PaidCount   int64  `json:"paid_count"`
	PaidCents   int64  `json:"paid_cents"`
	RefundCount int64  `json:"refund_count"`
	RefundCents int64  `json:"refund_cents"`
}

// AnalyticsStore 经营分析事件仓储接口。
type AnalyticsStore interface {
	CreateAnalyticsTable() error
	IngestEvents(events []*AnalyticsEvent, now string) (int, error)
	Overview(eventTypes []string, from, to, gameId string) ([]TypeCounts, error)
	DailyActiveUsers(eventType string, from, to string) ([]DailyUserPair, error)
	ActivityEffect(activityId int64) (*ActivityEffectRow, error)
	Reconciliation(from, to string) ([]ReconciliationRow, error)
	ChannelEffect(from, to string, gameId string) ([]ChannelEffectRow, error)
}

var _ AnalyticsStore = (*AnalyticsEventsModel)(nil)

// AnalyticsEventsModel 经营分析事件模型；追加写 + 聚合读，连接池钳制为 1
// （SQLite 单写者，与战绩域同款）。
type AnalyticsEventsModel struct {
	db *sql.DB
}

// NewAnalyticsEventsModel 创建经营分析事件模型。
func NewAnalyticsEventsModel(db *sql.DB) *AnalyticsEventsModel {
	db.SetMaxOpenConns(1)
	return &AnalyticsEventsModel{db: db}
}

// CreateAnalyticsTable 建事件表（幂等；SQLite 先行，MySQL 回落）。
func (m *AnalyticsEventsModel) CreateAnalyticsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS platform_events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id VARCHAR(64) NOT NULL,
			event_type VARCHAR(32) NOT NULL,
			user_id INTEGER NOT NULL DEFAULT 0,
			game_id VARCHAR(64) NOT NULL DEFAULT '',
			activity_id INTEGER NOT NULL DEFAULT 0,
			order_id VARCHAR(64) NOT NULL DEFAULT '',
			amount_cents INTEGER NOT NULL DEFAULT 0,
			channel VARCHAR(64) NOT NULL DEFAULT '',
			occurred_at VARCHAR(32) NOT NULL,
			created_at VARCHAR(32) NOT NULL,
			UNIQUE (event_id)
		)
	`
	if _, err := m.db.Exec(query); err != nil {
		query = `
			CREATE TABLE IF NOT EXISTS platform_events (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				event_id VARCHAR(64) NOT NULL,
				event_type VARCHAR(32) NOT NULL,
				user_id BIGINT NOT NULL DEFAULT 0,
				game_id VARCHAR(64) NOT NULL DEFAULT '',
				activity_id BIGINT NOT NULL DEFAULT 0,
				order_id VARCHAR(64) NOT NULL DEFAULT '',
				amount_cents BIGINT NOT NULL DEFAULT 0,
				channel VARCHAR(64) NOT NULL DEFAULT '',
				occurred_at VARCHAR(32) NOT NULL,
				created_at VARCHAR(32) NOT NULL,
				UNIQUE KEY uk_event_id (event_id),
				KEY idx_events_type_time (event_type, occurred_at),
				KEY idx_events_game_time (game_id, occurred_at),
				KEY idx_events_user_time (user_id, occurred_at),
				KEY idx_events_activity (activity_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		if _, err := m.db.Exec(query); err != nil {
			return fmt.Errorf("创建平台事件表失败（尝试了 SQLite 和 MySQL 格式）: %w", err)
		}
		// SQLite 侧索引（IF NOT EXISTS 方言兼容）
		for _, idx := range []string{
			`CREATE INDEX IF NOT EXISTS idx_events_type_time ON platform_events (event_type, occurred_at)`,
			`CREATE INDEX IF NOT EXISTS idx_events_game_time ON platform_events (game_id, occurred_at)`,
			`CREATE INDEX IF NOT EXISTS idx_events_user_time ON platform_events (user_id, occurred_at)`,
			`CREATE INDEX IF NOT EXISTS idx_events_activity ON platform_events (activity_id)`,
			`CREATE INDEX IF NOT EXISTS idx_events_channel_time ON platform_events (channel, occurred_at)`,
		} {
			if _, err := m.db.Exec(idx); err != nil {
				return fmt.Errorf("创建平台事件索引失败: %w", err)
			}
		}
		return nil
	}
	// SQLite 建表成功路径同样补索引
	for _, idx := range []string{
		`CREATE INDEX IF NOT EXISTS idx_events_type_time ON platform_events (event_type, occurred_at)`,
		`CREATE INDEX IF NOT EXISTS idx_events_game_time ON platform_events (game_id, occurred_at)`,
		`CREATE INDEX IF NOT EXISTS idx_events_user_time ON platform_events (user_id, occurred_at)`,
		`CREATE INDEX IF NOT EXISTS idx_events_activity ON platform_events (activity_id)`,
		`CREATE INDEX IF NOT EXISTS idx_events_channel_time ON platform_events (channel, occurred_at)`,
	} {
		if _, err := m.db.Exec(idx); err != nil {
			return fmt.Errorf("创建平台事件索引失败: %w", err)
		}
	}
	// 存量库补列（GP-403 渠道标识）：CREATE TABLE IF NOT EXISTS 不会给已有
	// 表加列，故补一次 ALTER（列已存在视为成功，不阻断启动）。
	return m.migrateChannelColumn()
}

// migrateChannelColumn 为存量 platform_events 补 channel 列（幂等；
// SQLite PRAGMA / MySQL SHOW COLUMNS 枚举，枚举失败跳过不阻断）。
func (m *AnalyticsEventsModel) migrateChannelColumn() error {
	has := false
	const name = "channel"
	// SQLite：PRAGMA table_info（第 2 列为列名）
	if rows, err := m.db.Query(`PRAGMA table_info(platform_events)`); err == nil {
		has = columnPresent(rows, 1, name)
		rows.Close()
	} else if rows, err := m.db.Query(`SHOW COLUMNS FROM platform_events`); err == nil {
		// MySQL：SHOW COLUMNS（第 1 列为列名）
		has = columnPresent(rows, 0, name)
		rows.Close()
	}
	if has {
		return nil
	}
	if _, err := m.db.Exec(`ALTER TABLE platform_events ADD COLUMN channel VARCHAR(64) NOT NULL DEFAULT ''`); err != nil {
		// 列已存在（并发）或方言差异：忽略，不阻断
	}
	return nil
}

// columnPresent 在列枚举结果里按列名位置判断某列是否已存在。
// nameCol 为列名列的下标（PRAGMA table_info=1，SHOW COLUMNS=0）。
func columnPresent(rows *sql.Rows, nameCol int, name string) bool {
	for rows.Next() {
		cols, err := rows.Columns()
		if err != nil || nameCol >= len(cols) {
			return false
		}
		vals := make([]any, len(cols))
		for i := range vals {
			vals[i] = new(sql.NullString)
		}
		if err := rows.Scan(vals...); err != nil {
			return false
		}
		if ns, ok := vals[nameCol].(*sql.NullString); ok && ns.String == name {
			return true
		}
	}
	return false
}

// IngestEvents 批量摄入（event_id 幂等：重复整行忽略）。逐行 INSERT OR
// IGNORE（SQLite）→ 失败回落 INSERT IGNORE（MySQL）；返回实际新增行数。
// 事件时间空缺由调用方补 now（摄入时刻），金额域不在本层校验（S2S 事实
// 由产生方负责，handler 侧仅做类型/字段卫生检查）。
func (m *AnalyticsEventsModel) IngestEvents(events []*AnalyticsEvent, now string) (int, error) {
	accepted := 0
	for _, e := range events {
		occurred := e.OccurredAt
		if occurred == "" {
			occurred = now
		}
		var res sql.Result
		var err error
		res, err = m.db.Exec(
			`INSERT OR IGNORE INTO platform_events (event_id, event_type, user_id, game_id, activity_id, order_id, amount_cents, channel, occurred_at, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			e.EventId, e.EventType, e.UserId, e.GameId, e.ActivityId, e.OrderId, e.AmountCents, e.Channel, occurred, now,
		)
		if err != nil {
			if !strings.Contains(err.Error(), "syntax") && !strings.Contains(err.Error(), "near") {
				return accepted, fmt.Errorf("写入平台事件失败: %w", err)
			}
			res, err = m.db.Exec(
				`INSERT IGNORE INTO platform_events (event_id, event_type, user_id, game_id, activity_id, order_id, amount_cents, channel, occurred_at, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				e.EventId, e.EventType, e.UserId, e.GameId, e.ActivityId, e.OrderId, e.AmountCents, e.Channel, occurred, now,
			)
			if err != nil {
				return accepted, fmt.Errorf("写入平台事件失败: %w", err)
			}
		}
		if n, _ := res.RowsAffected(); n == 1 {
			accepted++
		}
	}
	return accepted, nil
}

// Overview 期间内按事件类型聚合（types 空为全部已知类型；game_id 空=全站）。
// 纯聚合 SQL 双方言一致（COUNT/SUM/CASE），金额只对携带金额的事件有意义。
func (m *AnalyticsEventsModel) Overview(typesFilter []string, from, to, gameId string) ([]TypeCounts, error) {
	q := `SELECT event_type, COUNT(*), COUNT(DISTINCT CASE WHEN user_id > 0 THEN user_id END), COALESCE(SUM(amount_cents), 0)
		  FROM platform_events WHERE occurred_at >= ? AND occurred_at <= ?`
	args := []any{from, to}
	if gameId != "" {
		q += ` AND game_id = ?`
		args = append(args, gameId)
	}
	if len(typesFilter) > 0 {
		q += ` AND event_type IN (` + placeholders(len(typesFilter)) + `)`
		for _, t := range typesFilter {
			args = append(args, t)
		}
	}
	q += ` GROUP BY event_type ORDER BY event_type`
	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("聚合平台事件失败: %w", err)
	}
	defer rows.Close()
	out := make([]TypeCounts, 0, 8)
	for rows.Next() {
		var r TypeCounts
		if err := rows.Scan(&r.EventType, &r.Events, &r.Users, &r.Amount); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// DailyActiveUsers 期间内某事件类型的 (日期, user) 去重对（留存计算的
// 便携输入；日期取 occurred_at 前 10 位，RFC3339 字典序与时间序一致）。
// user_id <= 0 的事件不参与（匿名事件无留存语义）。
func (m *AnalyticsEventsModel) DailyActiveUsers(eventType, from, to string) ([]DailyUserPair, error) {
	rows, err := m.db.Query(
		`SELECT DISTINCT substr(occurred_at, 1, 10), user_id FROM platform_events
		 WHERE event_type = ? AND user_id > 0 AND occurred_at >= ? AND occurred_at <= ? ORDER BY 1, 2`,
		eventType, from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("查询日活对失败: %w", err)
	}
	defer rows.Close()
	out := make([]DailyUserPair, 0, 64)
	for rows.Next() {
		var p DailyUserPair
		if err := rows.Scan(&p.Day, &p.UserId); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ActivityEffect 活动效果：发奖事件事实（笔数/参与人数/奖励金额）。
func (m *AnalyticsEventsModel) ActivityEffect(activityId int64) (*ActivityEffectRow, error) {
	row := m.db.QueryRow(
		`SELECT COUNT(*), COUNT(DISTINCT CASE WHEN user_id > 0 THEN user_id END), COALESCE(SUM(amount_cents), 0)
		 FROM platform_events WHERE activity_id = ? AND event_type = ?`,
		activityId, EventRewardGranted,
	)
	var r ActivityEffectRow
	if err := row.Scan(&r.Grants, &r.Participants, &r.RewardCents); err != nil {
		return nil, fmt.Errorf("聚合活动效果失败: %w", err)
	}
	r.ActivityId = activityId
	return &r, nil
}

// Reconciliation 账单核对聚合：期间内按游戏的付费/退款事件笔数与金额。
func (m *AnalyticsEventsModel) Reconciliation(from, to string) ([]ReconciliationRow, error) {
	rows, err := m.db.Query(
		`SELECT game_id,
			COALESCE(SUM(CASE WHEN event_type = '`+EventOrderPaid+`' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN event_type = '`+EventOrderPaid+`' THEN amount_cents ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN event_type = '`+EventRefundRequested+`' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN event_type = '`+EventRefundRequested+`' THEN amount_cents ELSE 0 END), 0)
		 FROM platform_events
		 WHERE event_type IN ('`+EventOrderPaid+`','`+EventRefundRequested+`') AND occurred_at >= ? AND occurred_at <= ?
		 GROUP BY game_id ORDER BY game_id`,
		from, to,
	)
	if err != nil {
		return nil, fmt.Errorf("聚合账单核对失败: %w", err)
	}
	defer rows.Close()
	out := make([]ReconciliationRow, 0, 8)
	for rows.Next() {
		var r ReconciliationRow
		if err := rows.Scan(&r.GameId, &r.PaidCount, &r.PaidCents, &r.RefundCount, &r.RefundCents); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// ChannelEffect 渠道效果（F22/GP-403）：期间内按渠道标识聚合注册人数与付费金额。
// channel 为空串的行即自然流量（未带渠道的注册）。gameId 非空时只统计该游戏。
func (m *AnalyticsEventsModel) ChannelEffect(from, to string, gameId string) ([]ChannelEffectRow, error) {
	// 聚合列必须显式起别名：ORDER BY 依赖的列名在 SQLite 里只认 AS 别名
	// （裸 COUNT/SUM 表达式无列名，会报 no such column），MySQL 侧同理。
	q := `SELECT channel,
			COUNT(CASE WHEN event_type = '` + EventRegister + `' THEN 1 END) AS registrations,
			COUNT(DISTINCT CASE WHEN user_id > 0 THEN user_id END) AS users,
			COALESCE(SUM(CASE WHEN event_type = '` + EventOrderPaid + `' THEN amount_cents ELSE 0 END), 0) AS paid_cents
		 FROM platform_events
		 WHERE event_type IN ('` + EventRegister + `','` + EventOrderPaid + `') AND occurred_at >= ? AND occurred_at <= ?`
	args := []any{from, to}
	if gameId != "" {
		q += ` AND game_id = ?`
		args = append(args, gameId)
	}
	q += ` GROUP BY channel ORDER BY registrations DESC, channel`
	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, fmt.Errorf("聚合渠道效果失败: %w", err)
	}
	defer rows.Close()
	out := make([]ChannelEffectRow, 0, 8)
	for rows.Next() {
		var r ChannelEffectRow
		if err := rows.Scan(&r.Channel, &r.Registrations, &r.Users, &r.PaidCents); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
