// Command courier-gateway-integrator 是 Spawn 沙箱的 Courier 支付/身份
// 集成网关（GP-005 联合沙箱 / GP-203 部署面装配，2026-11-23 决策）。
//
// 背景：courier 冻结网关二进制（gateway/cmd/gateway）启动不登记 SKU
// （teller.AddSku 仅装配期 Go 方法、无 HTTP 管理端点）也不接发货钩子
// （teller.Options.Deliver 为 nil = 记账即发货成功的沙箱裸跑），全链路
// 卡在价格表为空（下单 404 PAYMENT_SKU_NOT_FOUND，活走查 2026-11-23
// 实测）。GP-203 注记的合法路径：「接入方构建嵌入 teller 的二进制」。
//
// 本装配只组合 courier 的导出包（providers/account + providers/teller +
// auth/middleware/routing），不修改 courier 仓、不复制其 main.go：
//   - 启动期经 COURIER_TELLER_SKUS 登记价格表（与 spawn payments
//     payment_products 的 id/金额/币种对齐，否则下单金额漂移 409）；
//   - Deliver 钩子转发 spawn payments POST /internal/deliveries（指令
//     形状对齐 delivery.Instruction，X-Delivery-Signature = HMAC-SHA256
//     十六进制，密钥同 payments PAYMENTS_DELIVERY_SECRET）；
//   - 仅挂 identity/payments 两域（沙箱链路只经这两域），其余能力域
//     未配置保持降级 501 COMMON_CAPABILITY_DISABLED（routing 语义）。
//
// 生产管理面（SKU HTTP 管理/退款/重发/检索）仍走 Courier 提案
// （docs/zh/architecture/courier-payments-management-proposal.md），
// 本装配不替代提案，只解沙箱/部署面接线。
//
// 环境变量：
//
//	COURIER_GATEWAY_ADDR              监听地址（缺省 :8080）
//	COURIER_OAUTH_DISABLED            =1 关闭 oauth 域（501）
//	COURIER_PAYMENTS_CHANNEL_SECRET   渠道回调 HMAC（空 = fail-closed 403）
//	COURIER_PAYMENTS_STATE            订单状态快照路径（缺省纯内存）
//	COURIER_TELLER_SKUS               价格表 JSON：
//	                                  [{"id","productId","form","amountCents","currency"}]
//	COURIER_PAYMENTS_DELIVER_WEBHOOK  发货 webhook URL（如
//	                                  http://payments:8898/internal/deliveries；空 = 不接线）
//	COURIER_PAYMENTS_DELIVER_SECRET   发货 webhook HMAC（同 payments
//	                                  PAYMENTS_DELIVERY_SECRET；webhook 非空而密钥空 = 不接线）
//	COURIER_PAYMENTS_ADMIN_TOKEN      管理面 S2S 凭证（Bearer；v1.2 管理端点
//	                                  /v1/payments/admin/*；与 spawn payments
//	                                  容器 COURIER_PAYMENTS_ADMIN_TOKEN 同值；
//	                                  空 = fail-closed，管理端点一律 401）
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cuihairu/courier/gateway/auth"
	"github.com/cuihairu/courier/gateway/middleware"
	"github.com/cuihairu/courier/gateway/providers"
	"github.com/cuihairu/courier/gateway/providers/account"
	"github.com/cuihairu/courier/gateway/providers/teller"
	"github.com/cuihairu/courier/gateway/routing"
)

// deliveryPayload 对齐 spawn payments delivery.Instruction 的 camelCase
// JSON 形状（teller.Delivery 无 json tag，直接 Marshal 会得到 Go 字段名）。
type deliveryPayload struct {
	OrderId     string `json:"orderId"`
	AccountId   string `json:"accountId"`
	SkuId       string `json:"skuId"`
	ProductId   string `json:"productId"`
	Form        string `json:"form"`
	AmountCents int64  `json:"amountCents"`
	Currency    string `json:"currency"`
}

// skuSpec COURIER_TELLER_SKUS 的单条价格表声明。
type skuSpec struct {
	ID          string `json:"id"`
	ProductId   string `json:"productId"`
	Form        string `json:"form"`
	AmountCents int64  `json:"amountCents"`
	Currency    string `json:"currency"`
}

func main() {
	addr := os.Getenv("COURIER_GATEWAY_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	skus, err := skusFromEnv()
	if err != nil {
		log.Fatalf("courier-gateway-integrator: COURIER_TELLER_SKUS invalid: %v", err)
	}
	handler, err := assemble(skus)
	if err != nil {
		log.Fatalf("courier-gateway-integrator: assemble failed: %v", err)
	}

	log.Printf("courier-gateway-integrator listening on %s", addr)
	log.Fatal(newServer(addr, handler).ListenAndServe())
}

// assemble 组装 handler（account+teller 两域）。抽出以便测试：装配
// 读环境变量注入 SKUs/Deliver，其余全部组合 courier 导出包。
func assemble(skus []skuSpec) (http.Handler, error) {
	reg := providers.NewRegistry()

	// identity 域：account（代登录/会话/注销的冻结 /v1/identity/* 契约承载）。
	acc := account.New(account.Options{
		DeleteGrace:   account.DefaultDeleteGrace,
		OAuthDisabled: os.Getenv("COURIER_OAUTH_DISABLED") == "1",
	})
	reqAuth := auth.RequireAuth(acc.Verifier())

	// payments 域：teller（SKU 价格表 + 订单状态机 + 回调验签 + Deliver +
	// 管理面凭证（v1.2 退款/重发/检索；GP-204 部署激活）。
	tellerP := teller.New(teller.Options{
		RequireAuth:   reqAuth,
		ChannelSecret: os.Getenv("COURIER_PAYMENTS_CHANNEL_SECRET"),
		StateFile:     os.Getenv("COURIER_PAYMENTS_STATE"),
		Deliver:       deliverHook(),
		AdminToken:    os.Getenv("COURIER_PAYMENTS_ADMIN_TOKEN"),
	})
	for _, s := range skus {
		tellerP.AddSku(s.ID, s.ProductId, s.Form, s.AmountCents, s.Currency)
		log.Printf("courier-gateway-integrator: sku %s registered (%d %s)",
			s.ID, s.AmountCents, s.Currency)
	}

	for _, h := range []providers.Handler{acc, tellerP} {
		if err := reg.Register(h); err != nil {
			return nil, fmt.Errorf("register %s: %w", h.Name(), err)
		}
	}

	// 仅挂 identity/payments 两域；其余能力域未配置 = 降级 501。
	defaults := providers.Config{
		providers.CapIdentity: {Primary: account.DefaultName},
		providers.CapPayments: {Primary: teller.DefaultName},
	}
	chain := middleware.Chain(middleware.NewRateLimiter(120, 60))
	return middleware.MaxBody(middleware.MaxBodyBytes)(
		middleware.Maintenance(noMaintenance)(routing.New(reg, defaults, chain))), nil
}

// deliverHook 构造 teller Deliver 钩子：webhook 与密钥齐备才接线（部分
// 配置不静默降级为沙箱裸跑——要裸跑就显式不配 webhook，日志说清）。
// 非预期状态码/网络错误返回 err，teller 按契约把订单留 PAID 待对账。
func deliverHook() func(teller.Delivery) error {
	webhook := strings.TrimSpace(os.Getenv("COURIER_PAYMENTS_DELIVER_WEBHOOK"))
	secret := os.Getenv("COURIER_PAYMENTS_DELIVER_SECRET")
	if webhook == "" {
		log.Printf("courier-gateway-integrator: COURIER_PAYMENTS_DELIVER_WEBHOOK empty, Deliver not wired (nil = sandbox success)")
		return nil
	}
	if secret == "" {
		log.Printf("courier-gateway-integrator: COURIER_PAYMENTS_DELIVER_WEBHOOK set but secret empty, Deliver not wired (nil = sandbox success)")
		return nil
	}
	client := &http.Client{Timeout: 30 * time.Second}
	return func(d teller.Delivery) error {
		body, err := json.Marshal(deliveryPayload{
			OrderId:     d.OrderID,
			AccountId:   d.AccountID,
			SkuId:       d.SkuID,
			ProductId:   d.ProductID,
			Form:        d.Form,
			AmountCents: d.AmountCents,
			Currency:    d.Currency,
		})
		if err != nil {
			return fmt.Errorf("marshal delivery: %w", err)
		}
		mac := hmac.New(sha256.New, []byte(secret))
		mac.Write(body)
		req, err := http.NewRequest(http.MethodPost, webhook, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("build delivery request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Delivery-Signature", hex.EncodeToString(mac.Sum(nil)))
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("post delivery webhook: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("delivery webhook %s: status %d", webhook, resp.StatusCode)
		}
		return nil
	}
}

// skusFromEnv 解析 COURIER_TELLER_SKUS（JSON 数组）。语法错误属配置
// 错误，返回 error（调用方启动即拒绝，fail-fast，不静默空表）。
func skusFromEnv() ([]skuSpec, error) {
	raw := strings.TrimSpace(os.Getenv("COURIER_TELLER_SKUS"))
	if raw == "" {
		return nil, nil
	}
	var out []skuSpec
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// noMaintenance 最小装配不挂 scribe 维护门，恒不在维护期。
func noMaintenance() bool { return false }

// newServer 显式超时（同 courier 网关：ReadHeader/Read/Idle；WriteTimeout
// 刻意 0，SSE 流式端点是长写连接）。
func newServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}
}
