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
	})
}
