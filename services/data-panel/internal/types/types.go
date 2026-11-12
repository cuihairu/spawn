package types

// PlayerStat 单个玩家在单款游戏下的累积战绩行。
// 计数为单调累加（增量摄入，永不回退），win_rate/kd 由读侧计算不下库。
type PlayerStat struct {
	GameId       string  `json:"game_id"`
	GameTitle    string  `json:"game_title"`
	Matches      int64   `json:"matches"`
	Wins         int64   `json:"wins"`
	WinRate      float64 `json:"win_rate"`
	Kills        int64   `json:"kills"`
	Deaths       int64   `json:"deaths"`
	Assists      int64   `json:"assists"`
	Kd           float64 `json:"kd"`
	Score        int64   `json:"score"`
	RankPoints   int64   `json:"rank_points"`
	LastPlayedAt string  `json:"last_played_at"`
	CreatedAt    string  `json:"created_at"`
	UpdatedAt    string  `json:"updated_at"`
}

// StatSummary 玩家跨游戏汇总。
type StatSummary struct {
	UserId          int64   `json:"user_id"`
	TotalMatches    int64   `json:"total_matches"`
	TotalWins       int64   `json:"total_wins"`
	WinRate         float64 `json:"win_rate"`
	TotalKills      int64   `json:"total_kills"`
	TotalDeaths     int64   `json:"total_deaths"`
	TotalAssists    int64   `json:"total_assists"`
	Kd              float64 `json:"kd"`
	TotalScore      int64   `json:"total_score"`
	TotalRankPoints int64   `json:"total_rank_points"`
	GameCount       int64   `json:"game_count"`
	LastPlayedAt    string  `json:"last_played_at"`
}

type GetSummaryReq struct {
	UserId int64 `path:"user_id"`
}

type SummaryResp struct {
	Summary StatSummary `json:"summary"`
}

type ListGameStatsReq struct {
	UserId int64 `path:"user_id"`
	Limit  int64 `form:"limit,optional"`
	Offset int64 `form:"offset,optional"`
}

type GameStatsResp struct {
	Games []PlayerStat `json:"games"`
	Total int64        `json:"total"`
}

type GetGameStatReq struct {
	UserId int64  `path:"user_id"`
	GameId string `path:"game_id"`
}

type PlayerStatResp struct {
	Stat PlayerStat `json:"stat"`
}

// RecordStatReq 战绩摄入请求：携带增量 delta（单调累加语义），
// 归属恒取 JWT 的 user_id，body 不接收 user_id。
type RecordStatReq struct {
	GameId     string `json:"game_id"`
	GameTitle  string `json:"game_title,optional"`
	Matches    int64  `json:"matches,optional"`
	Wins       int64  `json:"wins,optional"`
	Kills      int64  `json:"kills,optional"`
	Deaths     int64  `json:"deaths,optional"`
	Assists    int64  `json:"assists,optional"`
	Score      int64  `json:"score,optional"`
	RankPoints int64  `json:"rank_points,optional"`
	PlayedAt   string `json:"played_at,optional"`
}

type RecordedStatResp struct {
	Code    int64      `json:"code"`
	Message string     `json:"message"`
	Stat    PlayerStat `json:"stat"`
}

// ---- GP-402 经营分析事件域（F24）----

// AnalyticsEventIn S2S 摄入的单条平台事实事件：金额/订单等事实字段只由
// 产生该事实的服务上报，摄入侧不做金额校验（账单核对在查询侧做对账）。
type AnalyticsEventIn struct {
	EventId     string `json:"event_id"`
	EventType   string `json:"event_type"`
	UserId      int64  `json:"user_id,optional"`
	GameId      string `json:"game_id,optional"`
	ActivityId  int64  `json:"activity_id,optional"`
	OrderId     string `json:"order_id,optional"`
	AmountCents int64  `json:"amount_cents,optional"`
	Channel     string `json:"channel,optional"`     // GP-403 渠道标识（≤64 字符，白名单外不拒）
	PartnerId   int64  `json:"partner_id,optional"`  // GP-404 合作方归属（产生方落定，行级隔离键）
	OccurredAt  string `json:"occurred_at,optional"` // RFC3339；空则取摄入时刻
}

// IngestAnalyticsReq POST /internal/events 批量摄入（上限 500 条/批，
// event_id 幂等：重复整行忽略）。
type IngestAnalyticsReq struct {
	Events []AnalyticsEventIn `json:"events"`
}

type IngestAnalyticsResp struct {
	Accepted int64 `json:"accepted"`
}

// AnalyticsOverviewReq GET /api/v1/analytics/overview：期间按事件类型聚合
// （漏斗基础数据）。from/to 支持 RFC3339 或 10 位日期（含全天边界）；
// types 逗号分隔过滤，空为全部。
type AnalyticsOverviewReq struct {
	From   string `form:"from,optional"`
	To     string `form:"to,optional"`
	GameId string `form:"game_id,optional"`
	Types  string `form:"types,optional"`
}

type AnalyticsTypeCount struct {
	EventType   string `json:"event_type"`
	Events      int64  `json:"events"`
	Users       int64  `json:"users"`
	AmountCents int64  `json:"amount_cents"`
}

type AnalyticsOverviewResp struct {
	From   string               `json:"from"`
	To     string               `json:"to"`
	GameId string               `json:"game_id,omitempty"`
	Items  []AnalyticsTypeCount `json:"items"`
}

// AnalyticsFunnelReq GET /api/v1/analytics/funnel：固定漏斗阶段
// 注册→活跃→下载→付费（按各阶段去重人数算转化率）。
type AnalyticsFunnelReq struct {
	From   string `form:"from,optional"`
	To     string `form:"to,optional"`
	GameId string `form:"game_id,optional"`
}

type AnalyticsFunnelStage struct {
	Stage       string  `json:"stage"`
	Users       int64   `json:"users"`
	Events      int64   `json:"events"`
	OverallConv float64 `json:"overall_conv"` // 相对首阶段（注册）人数比
	StepConv    float64 `json:"step_conv"`    // 相对上一阶段人数比；首阶段恒 1
}

type AnalyticsFunnelResp struct {
	From   string                 `json:"from"`
	To     string                 `json:"to"`
	GameId string                 `json:"game_id,omitempty"`
	Stages []AnalyticsFunnelStage `json:"stages"`
}

// AnalyticsRetentionReq GET /api/v1/analytics/retention：按 eventType 的
// 日活对计算次日/第 N 日留存（date 算术在 Go 侧做，SQL 保持方言中立）。
// window 天数上限 30。
type AnalyticsRetentionReq struct {
	EventType string `form:"event_type,optional,default=login"`
	From      string `form:"from,optional"`
	To        string `form:"to,optional"`
	Window    int64  `form:"window,optional,default=7"`
}

type AnalyticsRetentionCell struct {
	Retention float64 `json:"retention"`
	Users     int64   `json:"users"`
}

type AnalyticsCohortRow struct {
	Day       string                            `json:"day"`
	Size      int64                             `json:"size"`
	Retention map[string]AnalyticsRetentionCell `json:"retention"`
}

type AnalyticsRetentionResp struct {
	EventType string               `json:"event_type"`
	From      string               `json:"from"`
	To        string               `json:"to"`
	Window    int64                `json:"window"`
	Cohorts   []AnalyticsCohortRow `json:"cohorts"`
}

// AnalyticsActivityReq GET /api/v1/analytics/activities/:id/effect：活动效果
// （发奖笔数/参与人数/奖励金额，来自 reward_granted 事件事实）。
type AnalyticsActivityReq struct {
	Id int64 `path:"id"`
}

type AnalyticsActivityEffect struct {
	ActivityId   int64 `json:"activity_id"`
	Grants       int64 `json:"grants"`
	Participants int64 `json:"participants"`
	RewardCents  int64 `json:"reward_cents"`
}

type AnalyticsActivityResp struct {
	Activity AnalyticsActivityEffect `json:"activity"`
}

// AnalyticsReconciliationReq GET /api/v1/analytics/reconciliation：账单核对
// （F24）——按游戏聚合付费/退款事件笔数与金额，与 payments 对账单核对。
type AnalyticsReconciliationReq struct {
	From string `form:"from,optional"`
	To   string `form:"to,optional"`
}

type AnalyticsReconciliationRow struct {
	GameId      string `json:"game_id"`
	PaidCount   int64  `json:"paid_count"`
	PaidCents   int64  `json:"paid_cents"`
	RefundCount int64  `json:"refund_count"`
	RefundCents int64  `json:"refund_cents"`
}

type AnalyticsReconciliationResp struct {
	From  string                       `json:"from"`
	To    string                       `json:"to"`
	Items []AnalyticsReconciliationRow `json:"items"`
}

// AnalyticsChannelEffectReq GET /api/v1/analytics/channels：渠道效果
// （F22/GP-403）——按渠道标识聚合注册人数与付费金额；channel 空串行即
// 自然流量。game_id 非空时只统计该游戏的付费。
type AnalyticsChannelEffectReq struct {
	From   string `form:"from,optional"`
	To     string `form:"to,optional"`
	GameId string `form:"game_id,optional"`
}

type AnalyticsChannelEffectRow struct {
	Channel       string `json:"channel"`
	Registrations int64  `json:"registrations"`
	Users         int64  `json:"users"`
	PaidCents     int64  `json:"paid_cents"`
}

type AnalyticsChannelEffectResp struct {
	From  string                      `json:"from"`
	To    string                      `json:"to"`
	Items []AnalyticsChannelEffectRow `json:"items"`
}

// AnalyticsPartnerReq GET /api/v1/analytics/partner：合作方自助数据（GP-404
// F12）。角色必须是 game-catalog 绑定的 partner，数据按 partner_id 行级
// 隔离（只统计本人名下事件）；from/to 口径与 overview 一致。
type AnalyticsPartnerReq struct {
	From string `form:"from,optional"`
	To   string `form:"to,optional"`
}

type AnalyticsPartnerResp struct {
	From      string               `json:"from"`
	To        string               `json:"to"`
	PartnerId int64                `json:"partner_id"`
	Items     []AnalyticsTypeCount `json:"items"`
}
