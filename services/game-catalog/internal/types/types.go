package types

// Game 定义客户端可见的游戏结构
type Game struct {
	Id               string   `json:"id"`
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Genres           []string `json:"genres"`
	Platforms        []string `json:"platforms"`
	ReleaseDate      string   `json:"release_date"`
	Developer        string   `json:"developer"`
	Publisher        string   `json:"publisher"`
	Tags             []string `json:"tags"`
	Score            float64  `json:"score"`
	TrendingScore    int      `json:"trending_score"`
	CoverImage       string   `json:"cover_image"`
	Screenshots      []string `json:"screenshots"`
	VideoUrl         string   `json:"video_url"`
	Changelog        string   `json:"changelog"`
	DeveloperWebsite string   `json:"developer_website"`
	StoreLink        string   `json:"store_link"`
	DownloadUrl      string   `json:"download_url"`
	PlayUrl          string   `json:"play_url"`
	Status           string   `json:"status"`
	ReviewReason     string   `json:"review_reason"`
}

type ListGamesRequest struct {
	Keyword  string `form:"keyword,optional"`
	Genre    string `form:"genre,optional"`
	Platform string `form:"platform,optional"`
	Tag      string `form:"tag,optional"`
	Limit    int64  `form:"limit,default=20"`
	Offset   int64  `form:"offset,default=0"`
	Sort     string `form:"sort,default=popularity"`
}

type ListGamesResponse struct {
	Games  []Game `json:"games"`
	Total  int    `json:"total"`
	Limit  int64  `json:"limit"`
	Offset int64  `json:"offset"`
}

type GetGameDetailRequest struct {
	Id string `path:"id"`
}

type FeaturedGamesRequest struct {
	Limit int64 `form:"limit,default=6"`
}

type RecommendationsRequest struct {
	UserId string `form:"userId,optional"`
	Genres string `form:"genres,optional"`
	Limit  int64  `form:"limit,default=5"`
}

type RecommendationsResponse struct {
	Games []Game `json:"games"`
}

type CreateGameRequest struct {
	Title            string   `json:"title"`
	Description      string   `json:"description"`
	Genres           []string `json:"genres"`
	Platforms        []string `json:"platforms"`
	ReleaseDate      string   `json:"release_date"`
	Developer        string   `json:"developer"`
	Publisher        string   `json:"publisher"`
	Tags             []string `json:"tags"`
	Score            float64  `json:"score"`
	CoverImage       string   `json:"cover_image"`
	Screenshots      []string `json:"screenshots,optional"`
	VideoUrl         string   `json:"video_url,optional"`
	Changelog        string   `json:"changelog,optional"`
	DeveloperWebsite string   `json:"developer_website,optional"`
	StoreLink        string   `json:"store_link,optional"`
	DownloadUrl      string   `json:"download_url,optional"`
	PlayUrl          string   `json:"play_url,optional"`
}

// UpdateGameTagsRequest 整体覆盖游戏标签（运营维护入口，如标记「联运」）。
type UpdateGameTagsRequest struct {
	Id   string   `path:"id"`
	Tags []string `json:"tags"`
}

// GameDetailResponse 详情（GP-401 附玩家评分聚合；与运营编辑分 score 分离）。
type GameDetailResponse struct {
	Game   Game              `json:"game"`
	Rating RatingSummaryView `json:"rating"`
}

type FeaturedGamesResponse struct {
	Games []Game `json:"games"`
}

// ==================== GP-103 运营后台 ====================

// Partner 合作方主体（响应视图）。
type Partner struct {
	Id           int64  `json:"id"`
	Name         string `json:"name"`
	ContactEmail string `json:"contact_email"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
}

// GameOwnership 游戏归属（响应视图）。
type GameOwnership struct {
	GameId    string `json:"game_id"`
	PartnerId int64  `json:"partner_id"`
	GrantedBy int64  `json:"granted_by"`
	GrantedAt string `json:"granted_at"`
	Status    string `json:"status"`
}

// GameReview 提审记录（响应视图）。
type GameReview struct {
	Id           int64  `json:"id"`
	GameId       string `json:"game_id"`
	VersionId    string `json:"version_id"`
	SubmittedBy  int64  `json:"submitted_by"`
	SubmittedAt  string `json:"submitted_at"`
	Status       string `json:"status"`
	ReviewerId   int64  `json:"reviewer_id"`
	ReviewReason string `json:"review_reason"`
	ReviewedAt   string `json:"reviewed_at"`
}

// GameVersion 游戏版本/渠道包（响应视图）。Downloadable 为派生字段：
// 包体托管在平台（storage_url 为 package:// 协议）时 true，客户端可走
// 受控下载端点（GP-105）；外部存储走 storage_url/商店链接承接（F08）。
type GameVersion struct {
	Id              string `json:"id"`
	GameId          string `json:"game_id"`
	Version         string `json:"version"`
	Changelog       string `json:"changelog"`
	PackageName     string `json:"package_name"`
	VersionCode     int64  `json:"version_code"`
	SignFingerprint string `json:"sign_fingerprint"`
	SizeBytes       int64  `json:"size_bytes"`
	Hash            string `json:"hash"`
	StorageUrl      string `json:"storage_url"`
	Status          string `json:"status"`
	ReviewReason    string `json:"review_reason"`
	CreatedAt       string `json:"created_at"`
	Downloadable    bool   `json:"downloadable"`
}

// AuditLog 审计记录（响应视图）。
type AuditLog struct {
	Id           int64  `json:"id"`
	ActorId      int64  `json:"actor_id"`
	ActorRole    string `json:"actor_role"`
	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceId   string `json:"resource_id"`
	BeforeJson   string `json:"before_json"`
	AfterJson    string `json:"after_json"`
	Ip           string `json:"ip"`
	CreatedAt    string `json:"created_at"`
}

// GrantRoleRequest 授予/变更平台角色；partner 角色需带所属合作方 PartnerId。
type GrantRoleRequest struct {
	UserId    int64  `json:"user_id"`
	Role      string `json:"role"`
	PartnerId int64  `json:"partner_id,optional"`
}

type GrantRoleResponse struct {
	UserId    int64  `json:"user_id"`
	Role      string `json:"role"`
	PartnerId int64  `json:"partner_id"`
}

// CreatePartnerRequest 新建合作方主体。
type CreatePartnerRequest struct {
	Name         string `json:"name"`
	ContactEmail string `json:"contact_email,optional"`
}

// SetPartnerStatusRequest 挂起/启用合作方。
type SetPartnerStatusRequest struct {
	Id     int64  `path:"id"`
	Status string `json:"status"`
}

type PartnerListResponse struct {
	Partners []Partner `json:"partners"`
}

// AssignOwnershipRequest 指派游戏归属（path id = game id）。
type AssignOwnershipRequest struct {
	Id        string `path:"id"`
	PartnerId int64  `json:"partner_id"`
}

// ListReviewsRequest 审核队列查询。
type ListReviewsRequest struct {
	Status string `form:"status,optional"`
	Limit  int64  `form:"limit,default=20"`
	Offset int64  `form:"offset,default=0"`
}

type ReviewsListResponse struct {
	Reviews []GameReview `json:"reviews"`
	Total   int          `json:"total"`
}

// DecideReviewRequest 审核决定（approve/reject，reject 需 reason）。
type DecideReviewRequest struct {
	Id       int64  `path:"id"`
	Decision string `json:"decision"`
	Reason   string `json:"reason,optional"`
}

// PartnerSubmitRequest 资料/版本提审（path id = game id）。
type PartnerSubmitRequest struct {
	Id        string `path:"id"`
	VersionId string `json:"version_id,optional"`
}

// CreateVersionRequest 新建游戏版本/渠道包（path id = game id）。
type CreateVersionRequest struct {
	Id              string `path:"id"`
	Version         string `json:"version"`
	Changelog       string `json:"changelog,optional"`
	PackageName     string `json:"package_name,optional"`
	VersionCode     int64  `json:"version_code,optional"`
	SignFingerprint string `json:"sign_fingerprint,optional"`
	SizeBytes       int64  `json:"size_bytes,optional"`
	Hash            string `json:"hash,optional"`
	StorageUrl      string `json:"storage_url,optional"`
}

// VersionPathRequest 版本操作（发布/撤回）：path id = game id，path versionId = 版本 id。
type VersionPathRequest struct {
	Id        string `path:"id"`
	VersionId string `path:"versionId"`
}

type VersionListResponse struct {
	Versions []GameVersion `json:"versions"`
}

// LatestVersionResponse 游戏当前已发布版本（GP-105 公开视图）：
// storage_url 是内部托管细节，公开端点一律置空，客户端凭 downloadable/
// hash/size 走受控下载与完整性校验。
type LatestVersionResponse struct {
	Version GameVersion `json:"version"`
}

// ListAuditRequest 审计查询。
type ListAuditRequest struct {
	ResourceType string `form:"resource_type,optional"`
	Limit        int64  `form:"limit,default=50"`
	Offset       int64  `form:"offset,default=0"`
}

// IdPathRequest 仅携带路径 id（上下架 / 版本列表用）。
type IdPathRequest struct {
	Id string `path:"id"`
}

type AuditListResponse struct {
	Logs  []AuditLog `json:"logs"`
	Total int        `json:"total"`
}

// MeResponse 当前登录账号的平台角色（导航/入口权限用）。
type MeResponse struct {
	UserId    int64  `json:"user_id"`
	Username  string `json:"username"`
	Role      string `json:"role"`
	PartnerId int64  `json:"partner_id"`
}

// GP-201 区服目录/充值目标：合作方维护区服，玩家公开读开放区服。
type GameServer struct {
	Id     int64  `json:"id"`
	GameId string `json:"game_id"`
	Name   string `json:"name"`
	Region string `json:"region"`
	Status string `json:"status"`
	Sort   int64  `json:"sort"`
	// GP-306 开服计划：排期开服时间（RFC3339；空 = 未排期），开服表展示。
	PlannedOpenAt string `json:"planned_open_at"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// CreateServerRequest 新建区服（path id = game id）；status 缺省 open，
// 可选 open/maintenance/closed；planned_open_at 为开服计划（可选）。
type CreateServerRequest struct {
	Id            string `path:"id"`
	Name          string `json:"name"`
	Region        string `json:"region,optional"`
	Status        string `json:"status,optional"`
	Sort          int64  `json:"sort,optional"`
	PlannedOpenAt string `json:"planned_open_at,optional"`
}

// UpdateServerRequest 更新区服（改名/迁移/状态变更/开服排期）；空字段保持原值。
type UpdateServerRequest struct {
	Id            string `path:"id"`
	ServerId      int64  `path:"serverId"`
	Name          string `json:"name,optional"`
	Region        string `json:"region,optional"`
	Status        string `json:"status,optional"`
	Sort          int64  `json:"sort,optional"`
	PlannedOpenAt string `json:"planned_open_at,optional"`
}

type ServerListResponse struct {
	Items []GameServer `json:"items"`
}

// GP-306 开服预约：玩家对未开服区服预约，开服时收到提醒（只发一次）。
type ServerPathRequest struct {
	Id       string `path:"id"`
	ServerId int64  `path:"serverId"`
}

type ReserveServerResponse struct {
	ServerId      int64  `json:"server_id"`
	GameId        string `json:"game_id"`
	Status        string `json:"status"`
	PlannedOpenAt string `json:"planned_open_at"`
	Created       bool   `json:"created"`
}

type MyReservationView struct {
	GameId        string `json:"game_id"`
	ServerId      int64  `json:"server_id"`
	ServerName    string `json:"server_name"`
	ServerStatus  string `json:"server_status"`
	PlannedOpenAt string `json:"planned_open_at"`
	NotifiedAt    string `json:"notified_at"`
	CreatedAt     string `json:"created_at"`
}

type MyReservationsResponse struct {
	Items []MyReservationView `json:"items"`
}

// InternalReservationUsersResponse S2S：游戏维度去重预约人群（活动通知受众）。
type InternalReservationUsersResponse struct {
	GameId  string  `json:"game_id"`
	UserIds []int64 `json:"user_ids"`
}

// ==================== GP-401 游戏评价（F04） ====================

// RateGameRequest 玩家评分（1-5 整数星，重复提交覆盖上一评分）。
type RateGameRequest struct {
	Id     string `path:"id"`
	Rating int    `json:"rating"`
}

// RatingSummaryView 评分聚合（average 为 1-5 均值；无评分 count=0 average=0）。
type RatingSummaryView struct {
	Average float64 `json:"average"`
	Count   int64   `json:"count"`
}

type RateGameResponse struct {
	GameId  string            `json:"game_id"`
	Rating  int               `json:"rating"`
	Summary RatingSummaryView `json:"summary"`
}

// CreateGameReviewRequest 发长评（10-4000 字；一游戏一评，重复 409，改评走 PUT）。
type CreateGameReviewRequest struct {
	Id      string `path:"id"`
	Content string `json:"content"`
}

// UpdateGameReviewRequest 作者改评（path reviewId 为长评 id）。
type UpdateGameReviewRequest struct {
	Id       string `path:"id"`
	ReviewId int64  `path:"reviewId"`
	Content  string `json:"content"`
}

// UserReviewView 玩家长评（公开列表视图；Rating 为作者当前评分，0=未评分；
// DevReply 为开发者官方回复，单条覆盖式）。
type UserReviewView struct {
	Id         int64  `json:"id"`
	GameId     string `json:"game_id"`
	UserId     int64  `json:"user_id"`
	Rating     int    `json:"rating"`
	Content    string `json:"content"`
	DevReply   string `json:"dev_reply"`
	DevReplyBy int64  `json:"dev_reply_by"`
	DevReplyAt string `json:"dev_reply_at"`
	CreatedAt  string `json:"created_at"`
	UpdatedAt  string `json:"updated_at"`
}

type GameReviewListRequest struct {
	Id     string `path:"id"`
	Limit  int64  `form:"limit,default=20"`
	Offset int64  `form:"offset,default=0"`
}

type GameReviewListResponse struct {
	Items  []UserReviewView `json:"items"`
	Total  int              `json:"total"`
	Limit  int64            `json:"limit"`
	Offset int64            `json:"offset"`
}

// ReportGameReviewRequest 举报长评（一人一评一报，重复举报返回既有记录）。
type ReportGameReviewRequest struct {
	Id       string `path:"id"`
	ReviewId int64  `path:"reviewId"`
	Reason   string `json:"reason"`
}

type ReportGameReviewResponse struct {
	Id      int64 `json:"id"`
	Created bool  `json:"created"`
}

// ReplyGameReviewRequest 开发者回复长评（合作方仅限自有游戏；覆盖式更新）。
type ReplyGameReviewRequest struct {
	Id       string `path:"id"`
	ReviewId int64  `path:"reviewId"`
	Content  string `json:"content"`
}

// ListReviewReportsRequest 管理举报队列查询（status 空=全部）。
type ListReviewReportsRequest struct {
	Status string `form:"status,optional"`
	Limit  int64  `form:"limit,default=20"`
	Offset int64  `form:"offset,default=0"`
}

// ReviewReportView 举报记录（管理视图）。
type ReviewReportView struct {
	Id         int64  `json:"id"`
	ReviewId   int64  `json:"review_id"`
	GameId     string `json:"game_id"`
	ReporterId int64  `json:"reporter_id"`
	Reason     string `json:"reason"`
	Status     string `json:"status"`
	HandledBy  int64  `json:"handled_by"`
	CreatedAt  string `json:"created_at"`
	HandledAt  string `json:"handled_at"`
}

type ReviewReportsListResponse struct {
	Items []ReviewReportView `json:"items"`
	Total int                `json:"total"`
}

// HandleReviewReportRequest 处置举报：dismiss 驳回（评价保留），
// remove 移除评价（同评其余未决举报一并结案）。
type HandleReviewReportRequest struct {
	Id     int64  `path:"id"`
	Action string `json:"action"`
}

// ==================== GP-401 收藏游戏（F05） ====================

// FavoriteGameResponse 收藏结果（重复收藏幂等 created=false）。
type FavoriteGameResponse struct {
	GameId  string `json:"game_id"`
	Created bool   `json:"created"`
}

// FavoriteItemView 我的收藏条目（收藏时快照不可得，回读游戏现状含状态）。
type FavoriteItemView struct {
	Game        Game   `json:"game"`
	FavoritedAt string `json:"favorited_at"`
}

type MyFavoritesResponse struct {
	Items []FavoriteItemView `json:"items"`
}
