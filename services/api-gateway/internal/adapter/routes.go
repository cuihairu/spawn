package adapter

import (
	"net/http"
	"time"

	"github.com/tappi/tappi/services/api-gateway/internal/proxy"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/zeromicro/go-zero/rest"
)

// RegisterCourierAdapterRoutes GP-201 Courier 适配服务反代（BFF：mobile
// 统一入口）。游戏会话票据/兑换/注销同步/映射管理面都是小 JSON，走普通
// 反代；Authorization 头透传，鉴权与角色在 courier-adapter 侧（管理面
// 角色再经 game-catalog /me 权威判定）。
func RegisterCourierAdapterRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	cfg := serverCtx.Config.Upstreams.CourierAdapter
	if cfg.BaseURL == "" {
		// 未配置（如独立部署不带适配服务）则不注册路由，网关其余功能不受影响
		return
	}
	timeout := time.Millisecond * time.Duration(cfg.Timeout)
	upstream, err := proxy.NewUpstream(cfg.BaseURL, timeout)
	if err != nil {
		panic(err)
	}
	h := upstream.Handler()

	server.AddRoutes([]rest.Route{
		{Method: http.MethodPost, Path: "/games/:id/session", Handler: h},
		{Method: http.MethodPost, Path: "/session/redeem", Handler: h},
		{Method: http.MethodPost, Path: "/games/:id/account/delete", Handler: h},
		{Method: http.MethodPost, Path: "/games/:id/account/delete/cancel", Handler: h},
		{Method: http.MethodGet, Path: "/admin/games/:id/mapping", Handler: h},
		{Method: http.MethodPut, Path: "/admin/games/:id/mapping", Handler: h},
		{Method: http.MethodGet, Path: "/admin/audit", Handler: h},
	})
}
