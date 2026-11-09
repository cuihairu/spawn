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
// 定价，payment.md 安全边界 1）；env 缺省 prod。
type CreateOrderRequest struct {
	GameId string `path:"gameId"`
	SkuId  string `json:"sku_id"`
	Env    string `json:"env,optional"`
}

// OrderQueryRequest 玩家订单查询：scope = path gameId。
type OrderQueryRequest struct {
	GameId  string `path:"gameId"`
	OrderId string `path:"orderId"`
	Env     string `json:"env,optional"`
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
