package types

// GP-202 请求/响应类型。

// Product 商品配置视图（服务端价格 + 商户归属）。
type Product struct {
	Id          string `json:"id"`
	ProductId   string `json:"product_id"`
	Form        string `json:"form"`
	AmountCents int64  `json:"amount_cents"`
	// BaseQuantity 基础购买内容数量（GP-302 倍率赠送的基础量）
	BaseQuantity int64  `json:"base_quantity"`
	Currency     string `json:"currency"`
	GameId       string `json:"game_id"`
	PartnerId    int64  `json:"partner_id"`
	Enabled      bool   `json:"enabled"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
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
	// BaseQuantity 基础购买内容数量（可选；缺省 0 = 商品无数值内容量）
	BaseQuantity int64  `json:"base_quantity,optional"`
	Currency     string `json:"currency"`
	GameId       string `json:"game_id"`
	PartnerId    int64  `json:"partner_id,optional"`
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

// CreateOrderRequest 玩家下单：scope = path gameId；请求体仅 skuId + 可选
// coupon_ids（服务端定价与券报价，payment.md 安全边界 1——客户端不下发
// 可信金额）；env 缺省 prod。server_id/role_id 为充值目标（GP-201 区服目录；
// 服务端校验区服归属与开放状态）。coupon_ids 为已领取券 id（GP-305）：
// 服务端按券报价（门槛/折扣/叠加），金额一律本侧计算。
type CreateOrderRequest struct {
	GameId    string  `path:"gameId"`
	SkuId     string  `json:"sku_id"`
	CouponIds []int64 `json:"coupon_ids,optional"`
	ServerId  int64   `json:"server_id,optional"`
	RoleId    string  `json:"role_id,optional"`
	Env       string  `json:"env,optional"`
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
// DiscountCents/PayAmountCents 为 GP-305 服务端券报价（折让与应付款项，
// 信息面；Courier 实收仍为列表价，折让为本侧优惠承担账本）。
type OrderResponse struct {
	Order          Order           `json:"order"`
	DiscountCents  int64           `json:"discount_cents,omitempty"`
	PayAmountCents int64           `json:"pay_amount_cents,omitempty"`
	Coupons        []CouponApplied `json:"coupons,omitempty"`
}

// CouponApplied 订单实际应用的券（报价快照）。
type CouponApplied struct {
	CouponId      int64  `json:"coupon_id"`
	Code          string `json:"code"`
	DiscountCents int64  `json:"discount_cents"`
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
	PartnerId   int64  `json:"partner_id"`
	AccountId   string `json:"account_id"`
	SkuId       string `json:"sku_id"`
	ProductId   string `json:"product_id"`
	Form        string `json:"form"`
	AmountCents int64  `json:"amount_cents"`
	// BaseQuantity 基础购买量（下单时从服务端商品配置固化）
	BaseQuantity int64  `json:"base_quantity"`
	Currency     string `json:"currency"`
	Status       string `json:"status"`
	PaidAt       string `json:"paid_at"`
	DeliveredAt  string `json:"delivered_at"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
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

// ---- GP-205 厂商对账单 ----

// GenerateStatementRequest 生成对账单（一厂商+一商户+一周期；split_rate_bp
// 为合同分成比例基点，如 7000 = 70%）。
type GenerateStatementRequest struct {
	Vendor      string `json:"vendor"`
	PartnerId   int64  `json:"partner_id"`
	PeriodStart string `json:"period_start"`
	PeriodEnd   string `json:"period_end"`
	SplitRateBp int64  `json:"split_rate_bp"`
}

type StatementPathRequest struct {
	Id int64 `path:"id"`
}

type StatementListRequest struct {
	Vendor    string `form:"vendor,optional"`
	PartnerId int64  `form:"partner_id,optional"`
	Status    string `form:"status,optional"`
}

type UpdateSettlementRequest struct {
	Id         int64 `path:"id"`
	Settlement int64 `json:"settlement"`
}

// StatementView 对账单快照（实付/优惠承担/退款冲减/分成基数与金额/差异）。
type StatementView struct {
	Id            int64  `json:"id"`
	Vendor        string `json:"vendor"`
	PartnerId     int64  `json:"partner_id"`
	PeriodStart   string `json:"period_start"`
	PeriodEnd     string `json:"period_end"`
	PaidTotal     int64  `json:"paid_total"`
	DiscountTotal int64  `json:"discount_total"`
	RefundTotal   int64  `json:"refund_total"`
	SplitBase     int64  `json:"split_base"`
	SplitRateBp   int64  `json:"split_rate_bp"`
	SplitAmount   int64  `json:"split_amount"`
	Settlement    int64  `json:"settlement"`
	Diff          int64  `json:"diff"`
	OrderCount    int    `json:"order_count"`
	Status        string `json:"status"`
	ConfirmedBy   int64  `json:"confirmed_by"`
	ConfirmedAt   string `json:"confirmed_at"`
	// DisputeBy/DisputeAt/DisputeReason 结算差异异议（GP-404）：非零/非空表示
	// 合作方对该周期差异提出过异议，待运营复核。
	DisputeBy     int64  `json:"dispute_by"`
	DisputeAt     string `json:"dispute_at"`
	DisputeReason string `json:"dispute_reason"`
	CreatedBy     int64  `json:"created_by"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

type StatementResponse struct {
	Statement StatementView `json:"statement"`
}

type StatementListResponse struct {
	Items []StatementView `json:"items"`
}

// ---- GP-404 合作方自助账单确认 / 结算差异 ----

// DisputeStatementRequest 合作方对差异提异议（异议理由必填，留痕）。
type DisputeStatementRequest struct {
	Id     int64  `path:"id"`
	Reason string `json:"reason"`
}

// PartnerStatementListRequest 合作方自助账单列表（只读本人名下单据）。
type PartnerStatementListRequest struct {
	Vendor string `form:"vendor,optional"`
	Status string `form:"status,optional"`
}

// ---- GP-305 优惠券 ----

// CouponConfig 优惠券定义（服务端配置域）。discount_type=fixed（discount_value
// 直减分）/ percent_bp（discount_value=基点，10000=100%）；threshold_cents
// 为使用门槛（list ≥ threshold）；stackable=true 可与其他可叠券合并。
type CouponConfig struct {
	Code           string `json:"code"`
	Name           string `json:"name"`
	GameId         string `json:"game_id"`
	SkuId          string `json:"sku_id,optional"` // 空 = 全部 sku
	ThresholdCents int64  `json:"threshold_cents,optional"`
	DiscountType   string `json:"discount_type"`
	DiscountValue  int64  `json:"discount_value"`
	Stackable      bool   `json:"stackable,optional"`
	PerUserLimit   int64  `json:"per_user_limit,optional"`
	TotalCount     int64  `json:"total_count,optional"`
	StartAt        string `json:"start_at"`
	EndAt          string `json:"end_at"`
}

type CreateCouponRequest struct {
	CouponConfig
}

type UpdateCouponRequest struct {
	Id int64 `path:"id"`
	CouponConfig
}

type CouponPathRequest struct {
	Id int64 `path:"id"`
}

type CouponListRequest struct {
	GameId string `form:"game_id,optional"`
	Status string `form:"status,optional"`
}

type CouponView struct {
	Id             int64  `json:"id"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	GameId         string `json:"game_id"`
	SkuId          string `json:"sku_id"`
	ThresholdCents int64  `json:"threshold_cents"`
	DiscountType   string `json:"discount_type"`
	DiscountValue  int64  `json:"discount_value"`
	Stackable      bool   `json:"stackable"`
	PerUserLimit   int64  `json:"per_user_limit"`
	TotalCount     int64  `json:"total_count"`
	StartAt        string `json:"start_at"`
	EndAt          string `json:"end_at"`
	Status         string `json:"status"`
	UsedCount      int64  `json:"used_count"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

type CouponResponse struct {
	Coupon CouponView `json:"coupon"`
}

type CouponListResponse struct {
	Items []CouponView `json:"items"`
}

// ClaimCouponRequest 玩家领取（coupon_id 路径；request_id 幂等键）。
type ClaimCouponRequest struct {
	Id        int64  `path:"id"`
	RequestId string `json:"request_id"`
}

type UserCouponView struct {
	Id            int64  `json:"id"`
	CouponId      int64  `json:"coupon_id"`
	Code          string `json:"code"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	ClaimedAt     string `json:"claimed_at"`
	LockedOrderId string `json:"locked_order_id,omitempty"`
	UsedAt        string `json:"used_at"`
}

type ClaimCouponResponse struct {
	Coupon UserCouponView `json:"coupon"`
}

type MyCouponsResponse struct {
	Items []UserCouponView `json:"items"`
}
