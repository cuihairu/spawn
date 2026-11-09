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
func (c *Client) ForwardCallback(rawBody []byte, hmacHex string) error {
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/v1/payments/callback", bytes.NewReader(rawBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Payment-Signature", hmacHex)
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
