package types

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
