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

type GameDetailResponse struct {
	Game Game `json:"game"`
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
