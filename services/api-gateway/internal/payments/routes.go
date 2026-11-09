package payments

import (
	"net/http"
	"time"

	"github.com/tappi/tappi/services/api-gateway/internal/proxy"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/zeromicro/go-zero/rest"
)

// RegisterPaymentsRoutes GP-202 支付接入服务反代（BFF：mobile 统一入口）。
// 商品配置/下单/订单查询/回调都是小 JSON，走普通反代；Authorization 头
// 透传，鉴权与角色在 payments 侧（管理面角色再经 game-catalog /me 权威判定）。
func RegisterPaymentsRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	cfg := serverCtx.Config.Upstreams.Payments
	if cfg.BaseURL == "" {
		// 未配置（如独立部署不带支付服务）则不注册路由，网关其余功能不受影响
		return
	}
	timeout := time.Millisecond * time.Duration(cfg.Timeout)
	upstream, err := proxy.NewUpstream(cfg.BaseURL, timeout)
	if err != nil {
		panic(err)
	}
	h := upstream.Handler()

	server.AddRoutes([]rest.Route{
		{Method: http.MethodPost, Path: "/games/:gameId/orders", Handler: h},
		{Method: http.MethodGet, Path: "/games/:gameId/orders/:orderId", Handler: h},
		{Method: http.MethodGet, Path: "/games/:gameId/skus", Handler: h},
		{Method: http.MethodPost, Path: "/admin/products/:id", Handler: h},
		{Method: http.MethodPatch, Path: "/admin/products/:id/enabled", Handler: h},
		{Method: http.MethodGet, Path: "/admin/games/:gameId/products", Handler: h},
		// GP-203 发货面：游戏服发货接口配置 + 投递列表/补单（管理面角色守卫
		// 在 payments 侧经 game-catalog /me 权威判定）。/internal/deliveries
		// 为 S2S 专用，**不进网关**（只配密钥的进程内 Deliver 钩子调用）。
		{Method: http.MethodPut, Path: "/admin/games/:gameId/delivery", Handler: h},
		{Method: http.MethodGet, Path: "/admin/games/:gameId/delivery", Handler: h},
		{Method: http.MethodGet, Path: "/admin/deliveries", Handler: h},
		{Method: http.MethodPost, Path: "/admin/deliveries/:orderId/redeliver", Handler: h},
		{Method: http.MethodPatch, Path: "/admin/deliveries/:orderId/target", Handler: h},
		// GP-204 订单面：玩家我的订单列表 + 运营订单投影检索/详情/退款/审计。
		{Method: http.MethodGet, Path: "/games/:gameId/orders", Handler: h},
		{Method: http.MethodGet, Path: "/admin/orders", Handler: h},
		{Method: http.MethodGet, Path: "/admin/orders/:orderId", Handler: h},
		{Method: http.MethodPost, Path: "/admin/orders/:orderId/refund", Handler: h},
		{Method: http.MethodGet, Path: "/admin/refunds", Handler: h},
		{Method: http.MethodGet, Path: "/admin/audit", Handler: h},
		{Method: http.MethodPost, Path: "/callback/:channel", Handler: h},
	})
}
