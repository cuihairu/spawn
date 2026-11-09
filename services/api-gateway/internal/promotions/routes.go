package promotions

import (
	"net/http"
	"time"

	"github.com/tappi/tappi/services/api-gateway/internal/proxy"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/zeromicro/go-zero/rest"
)

// RegisterPromotionsRoutes GP-301 活动/促销配置服务反代（BFF：mobile 统一
// 入口）。活动配置管理面均为小 JSON，走普通反代；Authorization 头透传，
// 鉴权与角色在 promotions 侧（经 game-catalog /me 权威判定）。未配置上游
// 则不注册路由，网关其余功能不受影响。
func RegisterPromotionsRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	cfg := serverCtx.Config.Upstreams.Promotions
	if cfg.BaseURL == "" {
		return
	}
	timeout := time.Millisecond * time.Duration(cfg.Timeout)
	upstream, err := proxy.NewUpstream(cfg.BaseURL, timeout)
	if err != nil {
		panic(err)
	}
	h := upstream.Handler()

	server.AddRoutes([]rest.Route{
		// 活动配置管理面：状态机 draft→published→disabled→revise + 版本快照。
		{Method: http.MethodPost, Path: "/admin/activities", Handler: h},
		{Method: http.MethodPut, Path: "/admin/activities/:id", Handler: h},
		{Method: http.MethodPost, Path: "/admin/activities/:id/publish", Handler: h},
		{Method: http.MethodPost, Path: "/admin/activities/:id/disable", Handler: h},
		{Method: http.MethodPost, Path: "/admin/activities/:id/revise", Handler: h},
		{Method: http.MethodGet, Path: "/admin/activities", Handler: h},
		{Method: http.MethodGet, Path: "/admin/activities/:id", Handler: h},
		{Method: http.MethodGet, Path: "/admin/activities/:id/versions", Handler: h},
		{Method: http.MethodGet, Path: "/admin/activity-audit", Handler: h},
		// GP-302 奖励发放账本查询。/internal/reward-evaluations 为 payments
		// S2S 专用（HMAC 签名），不进网关。
		{Method: http.MethodGet, Path: "/admin/reward-grants", Handler: h},
		// GP-304 礼包：管理面 + 玩家侧展示/领取/我的领取。核销回调
		// /internal/gift-pack-redemptions 为游戏服 S2S（HMAC），不进网关。
		{Method: http.MethodPost, Path: "/admin/gift-packs", Handler: h},
		{Method: http.MethodPut, Path: "/admin/gift-packs/:id", Handler: h},
		{Method: http.MethodPost, Path: "/admin/gift-packs/:id/publish", Handler: h},
		{Method: http.MethodPost, Path: "/admin/gift-packs/:id/disable", Handler: h},
		{Method: http.MethodPost, Path: "/admin/gift-packs/:id/revise", Handler: h},
		{Method: http.MethodGet, Path: "/admin/gift-packs", Handler: h},
		{Method: http.MethodGet, Path: "/admin/gift-packs/:id", Handler: h},
		{Method: http.MethodPost, Path: "/admin/gift-packs/:id/codes", Handler: h},
		{Method: http.MethodGet, Path: "/admin/gift-packs/:id/codes", Handler: h},
		{Method: http.MethodGet, Path: "/admin/gift-packs/:id/claims", Handler: h},
		{Method: http.MethodGet, Path: "/admin/gift-claims", Handler: h},
		{Method: http.MethodGet, Path: "/gift-packs", Handler: h},
		{Method: http.MethodGet, Path: "/gift-packs/:id", Handler: h},
		{Method: http.MethodPost, Path: "/gift-packs/:id/claim", Handler: h},
		{Method: http.MethodGet, Path: "/gift-claims/me", Handler: h},
	})
}
