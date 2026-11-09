package types

import "encoding/json"

// GP-301 活动基础类型。

// ActivityConfig 活动配置（服务端配置域，不信任客户端可信金额/规则）。
// ServerIds 空 = 全部区服；Channel 空 = 全部渠道；Audience/Trigger/Limits/
// Rewards 为结构化配置（GP-302/303/305 定义各自规格）。
type ActivityConfig struct {
	Name       string         `json:"name"`
	GameId     string         `json:"game_id"`
	Timezone   string         `json:"timezone"`
	StartAt    string         `json:"start_at"`
	EndAt      string         `json:"end_at"`
	ServerIds  []int64        `json:"server_ids,optional"`
	Channel    string         `json:"channel,optional"`
	Audience   map[string]any `json:"audience,optional"`
	Trigger    map[string]any `json:"trigger,optional"`
	Limits     map[string]any `json:"limits,optional"`
	Stackable  bool           `json:"stackable,optional"`
	StackGroup string         `json:"stack_group,optional"`
	Rewards    map[string]any `json:"rewards,optional"`
}

// CreateActivityRequest 创建活动（落草稿）。
type CreateActivityRequest struct {
	ActivityConfig
}

// UpdateActivityRequest 更新活动（仅草稿）。
type UpdateActivityRequest struct {
	Id int64 `path:"id"`
	ActivityConfig
}

type ActivityPathRequest struct {
	Id int64 `path:"id"`
}

type ActivityListRequest struct {
	GameId string `form:"game_id,optional"`
	Status string `form:"status,optional"`
}

// ActivityView 活动配置视图。
type ActivityView struct {
	Id             int64          `json:"id"`
	Name           string         `json:"name"`
	GameId         string         `json:"game_id"`
	Status         string         `json:"status"`
	Timezone       string         `json:"timezone"`
	StartAt        string         `json:"start_at"`
	EndAt          string         `json:"end_at"`
	ServerIds      []int64        `json:"server_ids"`
	Channel        string         `json:"channel"`
	Audience       map[string]any `json:"audience"`
	Trigger        map[string]any `json:"trigger"`
	Limits         map[string]any `json:"limits"`
	Stackable      bool           `json:"stackable"`
	StackGroup     string         `json:"stack_group"`
	Rewards        map[string]any `json:"rewards"`
	CurrentVersion int64          `json:"current_version"`
	CreatedBy      int64          `json:"created_by"`
	CreatedAt      string         `json:"created_at"`
	UpdatedAt      string         `json:"updated_at"`
}

type ActivityResponse struct {
	Activity ActivityView `json:"activity"`
}

type ActivityListResponse struct {
	Items []ActivityView `json:"items"`
}

// VersionView 版本快照视图（不可变）。
type VersionView struct {
	Id          int64          `json:"id"`
	ActivityId  int64          `json:"activity_id"`
	Version     int64          `json:"version"`
	Snapshot    map[string]any `json:"snapshot"`
	PublishedBy int64          `json:"published_by"`
	PublishedAt string         `json:"published_at"`
}

type VersionListResponse struct {
	Items []VersionView `json:"items"`
}

// AuditListRequest 活动审计列表（activity_id 可选筛）。
type AuditListRequest struct {
	ActivityId int64 `form:"activity_id,optional"`
}

type AuditView struct {
	Id         int64  `json:"id"`
	ActivityId int64  `json:"activity_id"`
	ActorId    int64  `json:"actor_id"`
	ActorRole  string `json:"actor_role"`
	Action     string `json:"action"`
	Detail     string `json:"detail"`
	Ip         string `json:"ip"`
	CreatedAt  string `json:"created_at"`
}

type AuditListResponse struct {
	Items []AuditView `json:"items"`
}

// GP-302 倍率赠送评估类型。

// RewardEvaluateRequest 订单事实推送（payments S2S；基础量来自服务端
// 商品配置，非客户端上报）。AudienceType 为玩家人群标签（可选；活动
// 定向人群时须相等才命中）。
type RewardEvaluateRequest struct {
	OrderId      string `json:"order_id"`
	GameId       string `json:"game_id"`
	ServerId     int64  `json:"server_id"`
	RoleId       string `json:"role_id"`
	AccountId    string `json:"account_id"`
	SkuId        string `json:"sku_id"`
	ProductId    string `json:"product_id"`
	Channel      string `json:"channel"`
	PaidCents    int64  `json:"paid_cents"`
	Currency     string `json:"currency"`
	PaidAt       string `json:"paid_at"`
	BaseQuantity int64  `json:"base_quantity"`
	AudienceType string `json:"audience_type,optional"`
	// GP-303 首充/累充订单事实（payments 按投影计算推送）：
	// FirstPurchase=同账号同游戏无更早 PAID/DELIVERED 订单；
	// CumulativeCents=累充净额（PAID/DELIVERED 实付合计 − EXECUTED 退款）
	FirstPurchase   bool  `json:"first_purchase,optional"`
	CumulativeCents int64 `json:"cumulative_cents,optional"`
}

// RewardGrantView 奖励发放账本视图。
type RewardGrantView struct {
	Id              int64           `json:"id"`
	OrderId         string          `json:"order_id"`
	ActivityId      int64           `json:"activity_id"`
	ActivityVersion int64           `json:"activity_version"`
	GameId          string          `json:"game_id"`
	ServerId        int64           `json:"server_id"`
	RoleId          string          `json:"role_id"`
	AccountId       string          `json:"account_id"`
	SkuId           string          `json:"sku_id"`
	PaidCents       int64           `json:"paid_cents"`
	Currency        string          `json:"currency"`
	BaseQuantity    int64           `json:"base_quantity"`
	ExtraQuantity   int64           `json:"extra_quantity"`
	TotalQuantity   int64           `json:"total_quantity"`
	RateMode        string          `json:"rate_mode"`
	RateBp          int64           `json:"rate_bp"`
	RuleType        string          `json:"rule_type"`
	Milestone       int64           `json:"milestone"`
	Status          string          `json:"status"`
	Snapshot        json.RawMessage `json:"snapshot"`
	GrantedAt       string          `json:"granted_at"`
}

// RewardSkip 未发放明细（叠加排除/限次/零赠送）。
type RewardSkip struct {
	ActivityId int64  `json:"activity_id"`
	Reason     string `json:"reason"`
}

// RewardEvaluateResponse 评估结果（Replay=同订单重放，返回已发放事实）。
type RewardEvaluateResponse struct {
	OrderId string            `json:"order_id"`
	Replay  bool              `json:"replay"`
	Grants  []RewardGrantView `json:"grants"`
	Skipped []RewardSkip      `json:"skipped"`
	// Revoked GP-303 退款撤销：累充净额跌破已发档位时置 revoked
	Revoked []RewardGrantView `json:"revoked"`
}

// RewardGrantListRequest 管理面账本查询（全可选过滤）。
type RewardGrantListRequest struct {
	ActivityId int64  `form:"activity_id,optional"`
	OrderId    string `form:"order_id,optional"`
	GameId     string `form:"game_id,optional"`
	AccountId  string `form:"account_id,optional"`
}

// RewardGrantListResponse 管理面账本列表。
type RewardGrantListResponse struct {
	Grants []RewardGrantView `json:"grants"`
}

// GP-304 礼包类型。

// GiftPackConfig 礼包配置（服务端配置域）。CodeRequired=true 时内容经核销码
// 兑换（库存=码池），false 为直发（库存不限，内容发放走游戏服接缝）。
type GiftPackConfig struct {
	Name         string         `json:"name"`
	GameId       string         `json:"game_id"`
	Description  string         `json:"description,optional"`
	Timezone     string         `json:"timezone"`
	StartAt      string         `json:"start_at"`
	EndAt        string         `json:"end_at"`
	ServerIds    []int64        `json:"server_ids,optional"`
	Channel      string         `json:"channel,optional"`
	Audience     map[string]any `json:"audience,optional"`
	PerUserLimit int64          `json:"per_user_limit,optional"`
	CodeRequired bool           `json:"code_required,optional"`
	Content      map[string]any `json:"content"`
}

type CreateGiftPackRequest struct {
	GiftPackConfig
}

type UpdateGiftPackRequest struct {
	Id int64 `path:"id"`
	GiftPackConfig
}

type GiftPackPathRequest struct {
	Id int64 `path:"id"`
}

type GiftPackListRequest struct {
	GameId string `form:"game_id,optional"`
	Status string `form:"status,optional"`
}

// GiftPackView 礼包配置视图。
type GiftPackView struct {
	Id           int64          `json:"id"`
	Name         string         `json:"name"`
	GameId       string         `json:"game_id"`
	Description  string         `json:"description"`
	Status       string         `json:"status"`
	Timezone     string         `json:"timezone"`
	StartAt      string         `json:"start_at"`
	EndAt        string         `json:"end_at"`
	ServerIds    []int64        `json:"server_ids"`
	Channel      string         `json:"channel"`
	Audience     map[string]any `json:"audience"`
	PerUserLimit int64          `json:"per_user_limit"`
	CodeRequired bool           `json:"code_required"`
	Content      map[string]any `json:"content"`
	CreatedBy    int64          `json:"created_by"`
	CreatedAt    string         `json:"created_at"`
	UpdatedAt    string         `json:"updated_at"`
}

type GiftPackResponse struct {
	GiftPack GiftPackView `json:"gift_pack"`
}

type GiftPackListResponse struct {
	Items []GiftPackView `json:"items"`
}

// PlayerGiftPackView 玩家侧展示（含剩余库存与已领次数）。
type PlayerGiftPackView struct {
	GiftPackView
	AvailableCodes int64 `json:"available_codes"`
	ClaimedCount   int64 `json:"claimed_count"`
}

type PlayerGiftPackListResponse struct {
	Items []PlayerGiftPackView `json:"items"`
}

// ImportCodesRequest 批量导入码库存。
type ImportCodesRequest struct {
	Id    int64    `path:"id"`
	Codes []string `json:"codes"`
}

type ImportCodesResponse struct {
	Imported  int `json:"imported"`
	Duplicate int `json:"duplicate"`
}

type CodeListRequest struct {
	Id     int64  `path:"id"`
	Status string `form:"status,optional"`
}

type CodeListResponse struct {
	Codes []CodeView `json:"codes"`
}

type CodeView struct {
	Id        int64  `json:"id"`
	PackId    int64  `json:"pack_id"`
	Code      string `json:"code"`
	Status    string `json:"status"`
	AccountId string `json:"account_id"`
	ClaimId   int64  `json:"claim_id"`
	IssuedAt  string `json:"issued_at"`
	UsedAt    string `json:"used_at"`
	CreatedAt string `json:"created_at"`
}

// ClaimGiftPackRequest 玩家领取（server_id/role_id 为区服角色锚点；
// audience_type 为定向人群标签，定向礼包须匹配；request_id 为幂等键）。
type ClaimGiftPackRequest struct {
	Id           int64  `path:"id"`
	ServerId     int64  `json:"server_id,optional"`
	RoleId       string `json:"role_id,optional"`
	AudienceType string `json:"audience_type,optional"`
	RequestId    string `json:"request_id"`
}

type ClaimGiftPackResponse struct {
	Claim ClaimView `json:"claim"`
	Code  *CodeView `json:"code,omitempty"`
}

type ClaimView struct {
	Id         int64  `json:"id"`
	PackId     int64  `json:"pack_id"`
	AccountId  string `json:"account_id"`
	GameId     string `json:"game_id"`
	ServerId   int64  `json:"server_id"`
	RoleId     string `json:"role_id"`
	ClaimIndex int64  `json:"claim_index"`
	RequestId  string `json:"request_id"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

type ClaimListResponse struct {
	Claims []ClaimView `json:"claims"`
}

// ClaimListAdminRequest 管理面领取记录筛（全可选）。
type ClaimListAdminRequest struct {
	PackId    int64  `form:"pack_id,optional"`
	AccountId string `form:"account_id,optional"`
}

// RedeemGiftPackRequest 游戏服核销回调（S2S 签名）：mark issued → used。
type RedeemGiftPackRequest struct {
	CodeId int64 `json:"code_id"`
	PackId int64 `json:"pack_id"`
}
