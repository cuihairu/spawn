package types

// GP-202 请求/响应类型。

// Product 商品配置视图（服务端价格 + 商户归属）。
type Product struct {
	Id          string `json:"id"`
	ProductId   string `json:"product_id"`
	Form        string `json:"form"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	GameId      string `json:"game_id"`
	PartnerId   int64  `json:"partner_id"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// Order 订单快照（Courier 只读投影；不复制状态机）。
type Order struct {
	Id          string `json:"id"`
	Status      string `json:"status"`
	SkuId       string `json:"sku_id"`
	ProductId   string `json:"product_id"`
	Form        string `json:"form"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	PayToken    string `json:"pay_token"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
	PaidAt      string `json:"paid_at"`
	DeliveredAt string `json:"delivered_at"`
}

// CreateProductRequest 新建商品（path id = SKU 标识）。
type CreateProductRequest struct {
	Id          string `path:"id"`
	ProductId   string `json:"product_id"`
	Form        string `json:"form"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	GameId      string `json:"game_id"`
	PartnerId   int64  `json:"partner_id,optional"`
}

// SetProductEnabledRequest 上下架商品（path id = SKU 标识）。
type SetProductEnabledRequest struct {
	Id      string `path:"id"`
	Enabled bool   `json:"enabled"`
}

// ProductPathRequest path id = SKU 标识。
type ProductPathRequest struct {
	Id string `path:"id"`
}

type ProductListResponse struct {
	Items []Product `json:"items"`
}

// CreateOrderRequest 玩家下单：scope = path gameId；请求体仅 skuId（服务端
// 定价，payment.md 安全边界 1）；env 缺省 prod。server_id/role_id 为充值
// 目标（GP-201 区服目录；服务端校验区服归属与开放状态，不下发可信金额）。
type CreateOrderRequest struct {
	GameId   string `path:"gameId"`
	SkuId    string `json:"sku_id"`
	ServerId int64  `json:"server_id,optional"`
	RoleId   string `json:"role_id,optional"`
	Env      string `json:"env,optional"`
}

// OrderQueryRequest 玩家订单查询：scope = path gameId。
type OrderQueryRequest struct {
	GameId  string `path:"gameId"`
	OrderId string `path:"orderId"`
	Env     string `json:"env,optional"`
}

// OrderListRequest 玩家我的订单列表：scope = path gameId（无 orderId）。
type OrderListRequest struct {
	GameId string `path:"gameId"`
	Env    string `json:"env,optional"`
}

// OrderResponse 订单快照（Courier 只读投影 + 归属核验后原样透传）。
type OrderResponse struct {
	Order Order `json:"order"`
}

// CallbackRequest 渠道回调：S2S，签名经 X-Payment-Signature（原生渠道验签接缝）。
type CallbackRequest struct {
	OrderId string `json:"orderId"`
	PaidAt  string `json:"paidAt"`
}

// GamePathRequest path gameId（scope 载体）。
type GamePathRequest struct {
	GameId string `path:"gameId"`
}

// ChannelPathRequest path channel（渠道适配名：sandbox / real）。
type ChannelPathRequest struct {
	Channel string `path:"channel"`
}

// SetDeliveryEndpointRequest 配置游戏服发货接口（GP-203；管理面）。secret 为
// 本平台对游戏服签名发货请求的 HMAC 密钥（X-Game-Delivery-Signature）。
type SetDeliveryEndpointRequest struct {
	GameId   string `path:"gameId"`
	Endpoint string `json:"endpoint"`
	Secret   string `json:"secret"`
	Enabled  *bool  `json:"enabled,optional"`
}

// DeliveryEndpointPathRequest path gameId。
type DeliveryEndpointPathRequest struct {
	GameId string `path:"gameId"`
}

// DeliveryEndpointView 发货接口配置视图（密钥只回是否已设置，不下发明文）。
type DeliveryEndpointView struct {
	GameId    string `json:"game_id"`
	Endpoint  string `json:"endpoint"`
	SecretSet bool   `json:"secret_set"`
	Enabled   bool   `json:"enabled"`
	UpdatedAt string `json:"updated_at"`
}

type DeliveryEndpointResponse struct {
	Endpoint DeliveryEndpointView `json:"endpoint"`
}

// DeliveryListRequest 管理面投递列表筛选（query 可选）。
type DeliveryListRequest struct {
	GameId string `form:"game_id,optional"`
	Status string `form:"status,optional"`
}

// DeliveryOrderPathRequest path orderId（发货账本键）。
type DeliveryOrderPathRequest struct {
	OrderId string `path:"orderId"`
}

// BackfillTargetRequest 补单回填充值目标（区服/角色）。
type BackfillTargetRequest struct {
	OrderId  string `path:"orderId"`
	ServerId int64  `json:"server_id"`
	RoleId   string `json:"role_id,optional"`
}

// DeliveryOrderView 发货账本视图（投递执行事实，非订单状态机）。
type DeliveryOrderView struct {
	OrderId     string `json:"order_id"`
	GameId      string `json:"game_id"`
	ServerId    int64  `json:"server_id"`
	RoleId      string `json:"role_id"`
	AccountId   string `json:"account_id"`
	SkuId       string `json:"sku_id"`
	ProductId   string `json:"product_id"`
	Form        string `json:"form"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
	Attempts    int    `json:"attempts"`
	LastError   string `json:"last_error"`
	NextRetryAt string `json:"next_retry_at"`
	DeliveredAt string `json:"delivered_at"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type DeliveryListResponse struct {
	Items []DeliveryOrderView `json:"items"`
}

// OrderListResponse 玩家我的订单列表（Courier 只读投影，updatedAt 降序）。
type OrderListResponse struct {
	Items      []Order `json:"items"`
	NextCursor string  `json:"next_cursor"`
}

// AdminOrderQueryRequest 运营订单检索（投影事实副本；filter 可选）。
type AdminOrderQueryRequest struct {
	GameId    string `form:"game_id,optional"`
	Status    string `form:"status,optional"`
	AccountId string `form:"account_id,optional"`
	Limit     int64  `form:"limit,optional"`
	Offset    int64  `form:"offset,optional"`
}

// OrderProjectionView 订单事实副本视图（状态= Courier 上报值）。
type OrderProjectionView struct {
	OrderId     string `json:"order_id"`
	GameId      string `json:"game_id"`
	AccountId   string `json:"account_id"`
	SkuId       string `json:"sku_id"`
	ProductId   string `json:"product_id"`
	Form        string `json:"form"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
	PaidAt      string `json:"paid_at"`
	DeliveredAt string `json:"delivered_at"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type AdminOrderListResponse struct {
	Items []OrderProjectionView `json:"items"`
}

// OrderPathRequest path orderId（投影/退款操作键）。
type OrderPathRequest struct {
	OrderId string `path:"orderId"`
}

// CreateRefundRequest 退款/冲正意图登记（管理适配；amount 缺省 = 订单全额）。
type CreateRefundRequest struct {
	OrderId     string `path:"orderId"`
	Action      string `json:"action,optional"` // REFUND（缺省）/ CORRECTION
	AmountCents int64  `json:"amount_cents,optional"`
	Reason      string `json:"reason"`
}

type RefundView struct {
	Id          int64  `json:"id"`
	OrderId     string `json:"order_id"`
	Action      string `json:"action"`
	AmountCents int64  `json:"amount_cents"`
	Reason      string `json:"reason"`
	Status      string `json:"status"`
	Note        string `json:"note"`
	CreatedBy   int64  `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

type RefundResponse struct {
	Refund    RefundView `json:"refund"`
	Execution string     `json:"execution"` // pending-courier-refund-endpoint：执行待提案接入
}

type RefundListResponse struct {
	Items []RefundView `json:"items"`
}

// RefundListRequest 退款意图列表（status 可选筛）。
type RefundListRequest struct {
	Status string `form:"status,optional"`
}

// AuditListRequest 审计列表（orderId 可选筛）。
type AuditListRequest struct {
	OrderId string `form:"order_id,optional"`
}

type AuditView struct {
	Id        int64  `json:"id"`
	OrderId   string `json:"order_id"`
	ActorId   int64  `json:"actor_id"`
	ActorRole string `json:"actor_role"`
	Action    string `json:"action"`
	Detail    string `json:"detail"`
	Ip        string `json:"ip"`
	CreatedAt string `json:"created_at"`
}

type AuditListResponse struct {
	Items []AuditView `json:"items"`
}
