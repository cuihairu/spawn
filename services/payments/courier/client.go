package courier

// Courier 支付网关只读客户端（契约 Frozen payment v1.1，见 docs/zh/contract/
// payment.md）：scope 仅经 X-Courier-Game-Id / X-Courier-Env 请求头传递，
// 不在 URL/payload 携带；玩家令牌透传，归属校验在 Courier 侧（非本人 404）。
//
// 本服务不复制订单状态机：订单/金额事实源在 Courier（teller），这里只做
// 下发转发 + 金额/币种/商户归属核验 + 回调验签接缝。

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// APIError Courier 错误（信封 {error:{code,message}} 展开）。
type APIError struct {
	HTTPStatus int
	Code       string
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("courier %s: %s", e.Code, e.Message)
}

// IsCode 判断是否为指定契约错误码（如 PAYMENT_SKU_NOT_FOUND）。
func (e *APIError) IsCode(code string) bool {
	return e.Code == code
}

// Order 订单快照（payment.md v1.1 数据模型；只读投影，不复制状态机）。
type Order struct {
	Id          string `json:"id"`
	Status      string `json:"status"` // CREATED/PAID/DELIVERED/CLOSED
	SkuId       string `json:"skuId"`
	ProductId   string `json:"productId"`
	Form        string `json:"form"`
	AmountCents int64  `json:"amountCents"`
	Currency    string `json:"currency"`
	PayToken    string `json:"payToken"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
	PaidAt      string `json:"paidAt"`
	DeliveredAt string `json:"deliveredAt"`
}

// Sku 可购 SKU 列表项（价格表投影）。
type Sku struct {
	Id          string `json:"id"`
	ProductId   string `json:"productId"`
	Form        string `json:"form"`
	AmountCents int64  `json:"amountCents"`
	Currency    string `json:"currency"`
}

// Client Courier 支付网关 HTTP 客户端。
type Client struct {
	baseURL string
	http    *http.Client
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		http:    &http.Client{Timeout: timeout},
	}
}

type envelope struct {
	Data  json.RawMessage `json:"data"`
	Error *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// ListSkus GET /v1/payments/skus（Bearer 透传，scope 仅 header）。
func (c *Client) ListSkus(gameID, env, bearer string) ([]Sku, error) {
	var out []Sku
	if err := c.call(http.MethodGet, c.baseURL+"/v1/payments/skus", gameID, env, bearer, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// CreateOrder POST /v1/payments/orders（请求体仅 {skuId}，服务端定价）。
func (c *Client) CreateOrder(gameID, env, bearer, skuID string) (*Order, error) {
	var out Order
	body := map[string]string{"skuId": skuID}
	if err := c.call(http.MethodPost, c.baseURL+"/v1/payments/orders", gameID, env, bearer, body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetOrder GET /v1/payments/orders/{orderId}。
func (c *Client) GetOrder(gameID, env, bearer, orderID string) (*Order, error) {
	var out Order
	if err := c.call(http.MethodGet, c.baseURL+"/v1/payments/orders/"+url.PathEscape(orderID), gameID, env, bearer, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListOrders GET /v1/payments/orders（我的订单，updatedAt 降序；契约无分页
// 查询参数，响应 {items, nextCursor}）。
func (c *Client) ListOrders(gameID, env, bearer string) ([]Order, error) {
	var page struct {
		Items      []Order `json:"items"`
		NextCursor string  `json:"nextCursor"`
	}
	if err := c.call(http.MethodGet, c.baseURL+"/v1/payments/orders", gameID, env, bearer, nil, &page); err != nil {
		return nil, err
	}
	return page.Items, nil
}

// ForwardCallback POST /v1/payments/callback（S2S，无 Bearer）：以 X-Payment-
// Signature（HMAC-SHA256 十六进制）转发渠道回调 raw body。验签由本服务
// 先做（原生渠道签名），这里构造 Courier 期望的 HMAC。
//
// gatewayID/gatewayEnv 是订单归属的 Courier scope（映射解析结果）：网关
// scope 中间件先于 auth 生效，对 /v1/payments/* 含 callback 一律要求
// X-Courier-Game-Id / X-Courier-Env，缺失即 COMMON_INVALID_ARGUMENT。
// 两者必须非空（调用方按订单投影解析后提供；空即本层拒绝，防静默回退
// 成必 400 的转发）。
func (c *Client) ForwardCallback(rawBody []byte, hmacHex, gatewayID, gatewayEnv string) error {
	if gatewayID == "" || gatewayEnv == "" {
		return errors.New("courier 回调转发缺 scope（X-Courier-Game-Id/Env）")
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/v1/payments/callback", bytes.NewReader(rawBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Payment-Signature", hmacHex)
	req.Header.Set("X-Courier-Game-Id", gatewayID)
	req.Header.Set("X-Courier-Env", gatewayEnv)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var env2 envelope
	if err := json.Unmarshal(raw, &env2); err != nil {
		return fmt.Errorf("courier 响应非信封: %w", err)
	}
	if env2.Error != nil {
		return &APIError{HTTPStatus: resp.StatusCode, Code: env2.Error.Code, Message: env2.Error.Message}
	}
	if resp.StatusCode >= 300 {
		return &APIError{HTTPStatus: resp.StatusCode, Code: "HTTP_" + http.StatusText(resp.StatusCode), Message: string(raw)}
	}
	return nil
}

// ---- 管理面（Courier payment v1.2，/v1/payments/admin/*，S2S 运营面）----
//
// 管理端点认证 = Authorization: Bearer <AdminToken>（空 token fail-closed 401，
// 网关侧不配置不裸奔）；端点本身不经玩家 Bearer-scope 路由，但网关全局 scope
// 中间件先于业务校验仍强制两 header（缺失即 400），故方法签名与玩家面一致
// 保留 gameID/env——调用方按订单/商品所属游戏解析 courier scope 传入。
// 管理凭证与玩家令牌不混用：adminToken 独立于 bearer。

// AdminSkuReq 管理面 SKU 登记/改价请求（v1.2 管理端点；服务端定价事实）。
// GameID/Env（契约 v1.3 SKU scope 隔离）让登记按 scope 隔离：带 gameId 的
// SKU 只在其 scope 可见可购，跨 scope 按不存在处理；空 = 全局。旧网关把
// 未知字段当缺省忽略，行为不变。
type AdminSkuReq struct {
	SkuId       string `json:"skuId"`
	ProductId   string `json:"productId"`
	Form        string `json:"form"`
	AmountCents int64  `json:"amountCents"`
	Currency    string `json:"currency"`
	GameID      string `json:"gameId,omitempty"`
	Env         string `json:"env,omitempty"`
}

// AdminOrder 管理面订单视图（v1.2；不下发 payToken，含断单态 PAID 标识与退款登记事实）。
type AdminOrder struct {
	Id          string           `json:"id"`
	Status      string           `json:"status"`
	AccountId   string           `json:"accountId"`
	GameId      string           `json:"gameId"`
	SkuId       string           `json:"skuId"`
	ProductId   string           `json:"productId"`
	Form        string           `json:"form"`
	AmountCents int64            `json:"amountCents"`
	Currency    string           `json:"currency"`
	CreatedAt   string           `json:"createdAt"`
	UpdatedAt   string           `json:"updatedAt"`
	PaidAt      string           `json:"paidAt,omitempty"`
	DeliveredAt string           `json:"deliveredAt,omitempty"`
	Refunds     []AdminRefundEnt `json:"refunds,omitempty"`
}

// AdminOrderPage 管理面全局检索分页（cursor keyset = updatedAt+orderID base64）。
type AdminOrderPage struct {
	Items      []AdminOrder `json:"items"`
	NextCursor string       `json:"nextCursor"`
}

// AdminRefundEnt 退款登记事实条目（v1.2 管理面）。
type AdminRefundEnt struct {
	Action      string `json:"action"`
	AmountCents int64  `json:"amountCents"`
	Reason      string `json:"reason"`
	At          string `json:"at"`
}

// AdminRefund 管理面退款登记结果（幂等：同订单同动作重复登记返回既有）。
type AdminRefund struct {
	OrderId string         `json:"orderId"`
	Refund  AdminRefundEnt `json:"refund"`
}

// AdminRedeliver 管理面断单（停 PAID）对账重发结果。
type AdminRedeliver struct {
	Order       AdminOrder `json:"order"`
	Redelivered bool       `json:"redelivered"`
}

// AdminOrderQuery 管理面检索筛选（gameId/status/accountId + 有界分页）。
type AdminOrderQuery struct {
	GameId    string
	Status    string
	AccountId string
	Limit     int
	Cursor    string
}

// adminBearer 管理凭证封装（Authorization: Bearer <token>）；空 token = 不设
// Authorization 头，网关 fail-closed 401 COMMON_UNAUTHENTICATED 原样上抛
// （APIError），调用方据此判定管理面未接线。
func adminBearer(token string) string {
	if token == "" {
		return ""
	}
	return "Bearer " + token
}

// AdminSku POST /v1/payments/admin/skus（登记/改价 SKU；重复登记 = 改价，已建
// 订单不受影响）。服务端定价事实，接入方按 game 归属登记。
func (c *Client) AdminSku(gameID, env, adminToken string, req *AdminSkuReq) (*Sku, error) {
	var out struct {
		Sku Sku `json:"sku"`
	}
	if err := c.call(http.MethodPost, c.baseURL+"/v1/payments/admin/skus", gameID, env, adminBearer(adminToken), req, &out); err != nil {
		return nil, err
	}
	return &out.Sku, nil
}

// AdminListOrders GET /v1/payments/admin/orders（全局检索 + 有界分页）。
func (c *Client) AdminListOrders(gameID, env, adminToken string, q AdminOrderQuery) (*AdminOrderPage, error) {
	u, err := url.Parse(c.baseURL + "/v1/payments/admin/orders")
	if err != nil {
		return nil, err
	}
	qs := u.Query()
	if q.GameId != "" {
		qs.Set("gameId", q.GameId)
	}
	if q.Status != "" {
		qs.Set("status", q.Status)
	}
	if q.AccountId != "" {
		qs.Set("accountId", q.AccountId)
	}
	if q.Limit > 0 {
		qs.Set("limit", strconv.Itoa(q.Limit))
	}
	if q.Cursor != "" {
		qs.Set("cursor", q.Cursor)
	}
	u.RawQuery = qs.Encode()
	var out AdminOrderPage
	if err := c.call(http.MethodGet, u.String(), gameID, env, adminBearer(adminToken), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminGetOrder GET /v1/payments/admin/orders/{orderId}（管理详情，无 payToken）。
func (c *Client) AdminGetOrder(gameID, env, adminToken, orderID string) (*AdminOrder, error) {
	var out AdminOrder
	if err := c.call(http.MethodGet, c.baseURL+"/v1/payments/admin/orders/"+url.PathEscape(orderID), gameID, env, adminBearer(adminToken), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminRedeliver POST /v1/payments/admin/orders/{orderId}/redeliver（断单对账
// 重发）：仅停 PAID 可重发；DELIVERED/CLOSED 409 PAYMENT_ORDER_STATE。
func (c *Client) AdminRedeliver(gameID, env, adminToken, orderID string) (*AdminRedeliver, error) {
	var out AdminRedeliver
	if err := c.call(http.MethodPost, c.baseURL+"/v1/payments/admin/orders/"+url.PathEscape(orderID)+"/redeliver", gameID, env, adminBearer(adminToken), nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// AdminRefund POST /v1/payments/admin/orders/{orderId}/refund（退款/冲正登记；
// action REFUND/CORRECTION）。金额 ≤ 实付（≤0 记全额，网关侧兜底）；幂等：
// 同订单同动作重复登记返回既有。
func (c *Client) AdminRefund(gameID, env, adminToken, orderID, action string, amountCents int64, reason string) (*AdminRefund, error) {
	body := map[string]any{"action": action, "amountCents": amountCents}
	if reason != "" {
		body["reason"] = reason
	}
	var out AdminRefund
	if err := c.call(http.MethodPost, c.baseURL+"/v1/payments/admin/orders/"+url.PathEscape(orderID)+"/refund", gameID, env, adminBearer(adminToken), body, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (c *Client) call(method, urlStr, gameID, env, bearer string, reqBody any, out any) error {
	var body io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, urlStr, body)
	if err != nil {
		return err
	}
	// scope 仅经 header（scope.md 传输节）；Bearer 原样透传（调用方传完整
	// Authorization 头，如 "Bearer <token>"）
	req.Header.Set("X-Courier-Game-Id", gameID)
	req.Header.Set("X-Courier-Env", env)
	if bearer != "" {
		req.Header.Set("Authorization", bearer)
	}
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return err
	}
	var env2 envelope
	if err := json.Unmarshal(raw, &env2); err != nil {
		return fmt.Errorf("courier 响应非信封: %w", err)
	}
	if env2.Error != nil {
		return &APIError{HTTPStatus: resp.StatusCode, Code: env2.Error.Code, Message: env2.Error.Message}
	}
	if resp.StatusCode >= 300 {
		return &APIError{HTTPStatus: resp.StatusCode, Code: "HTTP_" + http.StatusText(resp.StatusCode), Message: string(raw)}
	}
	if out != nil && len(env2.Data) > 0 {
		if err := json.Unmarshal(env2.Data, out); err != nil {
			return err
		}
	}
	return nil
}

// ErrUpstream 上游不可达包装判定。
var ErrUpstream = errors.New("courier upstream error")
