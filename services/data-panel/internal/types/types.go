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
