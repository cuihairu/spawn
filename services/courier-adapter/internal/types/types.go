package types

import "github.com/tappi/tappi/services/courier-adapter/courier"

// Capabilities 每游戏开通能力（GP-002：与「联运」标签解耦）。
// 字段 optional：go-zero 严格解析要求显式标注，未传即 false。
type Capabilities struct {
	Login    bool `json:"login,optional"`
	Payment  bool `json:"payment,optional"`
	Download bool `json:"download,optional"`
	Support  bool `json:"support,optional"`
}

// UpsertMappingRequest 管理面映射写入（operator/platform_admin）。
type UpsertMappingRequest struct {
	Id            string       `path:"id"`
	CourierGameId string       `json:"courierGameId"`
	Env           string       `json:",default=prod"`
	PartnerId     int64        `json:",default=0"`
	Capabilities  Capabilities `json:"capabilities"`
}

// MappingResponse 映射读回。
type MappingResponse struct {
	SpawnGameId   string       `json:"spawnGameId"`
	CourierGameId string       `json:"courierGameId"`
	Env           string       `json:"env"`
	PartnerId     int64        `json:"partnerId"`
	Capabilities  Capabilities `json:"capabilities"`
}

// GameSessionRequest 进入游戏（发起游戏会话）。
type GameSessionRequest struct {
	Id string `path:"id"`
}

// GameSessionResponse 启动票据（短期、一次性、绑定 game_id）。
type GameSessionResponse struct {
	Ticket    string `json:"ticket"`
	ExpiresAt string `json:"expiresAt"`
	GameId    string `json:"gameId"` // courier_game_id
	Env       string `json:"env"`
}

// RedeemRequest 票据兑换。
type RedeemRequest struct {
	Ticket string `json:"ticket"`
}

// RedeemAccount Courier 账号摘要（sessionDTO.account 口径）。
type RedeemAccount struct {
	Id     string `json:"id"`
	Type   string `json:"type"`
	Status string `json:"status"`
}

// RedeemResponse scope-bound token（一次性下发，二次兑换 404）。
type RedeemResponse struct {
	Account          RedeemAccount `json:"account"`
	AccessToken      string        `json:"accessToken"`
	AccessExpiresAt  string        `json:"accessExpiresAt"`
	RefreshToken     string        `json:"refreshToken"`
	RefreshExpiresAt string        `json:"refreshExpiresAt"`
}

// DeleteAccountRequest 注销同步（Spawn 侧发起）。
type DeleteAccountRequest struct {
	Id string `path:"id"`
}

// CancelDeleteRequest 宽限期撤销注销。
type CancelDeleteRequest struct {
	Id          string `path:"id"`
	CancelToken string `json:"cancelToken"`
}

// DeleteResultResponse Courier 注销受理透传（字段可选）。
type DeleteResultResponse struct {
	Status        string `json:"status"`
	CancelToken   string `json:"cancelToken,omitempty"`
	PendingDelete bool   `json:"pendingDelete,omitempty"`
	DeletedAt     string `json:"deletedAt,omitempty"`
}

// ListAuditResponse 适配域审计（最近记录）。
type ListAuditResponse struct {
	Items []AuditItem `json:"items"`
}

type AuditItem struct {
	Id           int64  `json:"id"`
	ActorId      int64  `json:"actorId"`
	Action       string `json:"action"`
	ResourceType string `json:"resourceType"`
	ResourceId   string `json:"resourceId"`
	Detail       string `json:"detail"`
	Ip           string `json:"ip"`
	CreatedAt    string `json:"createdAt"`
}

// --- GP-307 售后工单（Courier SupportProvider 适配，契约 Frozen v1.1）---

// CreateSupportTicketRequest 提单：title/body/category 走 Courier 契约；
// serverId 由 Spawn 校验区服归属后入 context；orderId 透传；context 为
// 业务自定义键（自动附加键不可被覆盖，Spawn 侧强制）。
type CreateSupportTicketRequest struct {
	Id       string         `path:"id"`
	Title    string         `json:"title"`
	Body     string         `json:"body"`
	Category string         `json:"category,optional"`
	ServerId string         `json:"serverId,optional"`
	OrderId  string         `json:"orderId,optional"`
	Context  map[string]any `json:"context,optional"`
}

// SupportTicketPathRequest 工单路径（游戏 + 工单 id）。
type SupportTicketPathRequest struct {
	Id       string `path:"id"`
	TicketId string `path:"ticketId"`
}

// AppendSupportTicketRequest 追加工单消息。
type AppendSupportTicketRequest struct {
	Id       string `path:"id"`
	TicketId string `path:"ticketId"`
	Body     string `json:"body"`
}

// SupportTicketListRequest 我的工单列表（limit/cursor 契约透传）。
type SupportTicketListRequest struct {
	Id     string `path:"id"`
	Limit  string `form:"limit,optional"`
	Cursor string `form:"cursor,optional"`
}

// SupportFaqRequest FAQ 检索（keyword 空 = 全量）。
type SupportFaqRequest struct {
	Id      string `path:"id"`
	Keyword string `form:"keyword,optional"`
	Limit   string `form:"limit,optional"`
}

// SupportTicketResponse 提单返回（Courier 工单原样）。
type SupportTicketResponse struct {
	Ticket courier.SupportTicket `json:"ticket"`
}

// SupportTicketListResponse 工单列表页。
type SupportTicketListResponse struct {
	Items      []courier.SupportTicket `json:"items"`
	NextCursor string                  `json:"nextCursor"`
}

// SupportTicketDetailResponse 工单详情：处理记录 = messages（升序）。
type SupportTicketDetailResponse struct {
	Ticket   courier.SupportTicket    `json:"ticket"`
	Messages []courier.SupportMessage `json:"messages"`
}

// SupportMessageResponse 追加消息返回。
type SupportMessageResponse struct {
	Message courier.SupportMessage `json:"message"`
}

// SupportFaqResponse FAQ 列表。
type SupportFaqResponse struct {
	Items []courier.SupportFAQ `json:"items"`
}
