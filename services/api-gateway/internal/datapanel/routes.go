package datapanel

import (
	"net/http"
	"time"

	"github.com/tappi/tappi/services/api-gateway/internal/proxy"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/zeromicro/go-zero/rest"
)

// RegisterDataPanelProxyRoutes data-panel 反代：战绩查询公开透传；
// POST /stats/records 的摄入鉴权（Bearer 透传）由 data-panel 校验，
// 网关只做转发，与 community 评论路由同口径。
func RegisterDataPanelProxyRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	baseURL := serverCtx.Config.Upstreams.DataPanel.BaseURL
	timeout := time.Millisecond * time.Duration(serverCtx.Config.Upstreams.DataPanel.Timeout)
	upstream, err := proxy.NewUpstream(baseURL, timeout)
	if err != nil {
		panic(err)
	}
	h := upstream.Handler()

	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/api/v1/stats/users/:user_id/summary", Handler: h},
		{Method: http.MethodGet, Path: "/api/v1/stats/users/:user_id/games", Handler: h},
		{Method: http.MethodGet, Path: "/api/v1/stats/users/:user_id/games/:game_id", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/stats/records", Handler: h},
		// GP-402 经营分析查询（F24）：漏斗/留存/活动效果/账单核对。
		// Authorization 头透传，登录与 AdminUserIds 名单鉴权在 data-panel 侧；
		// S2S 摄入口 /internal/events 不经网关暴露。
		{Method: http.MethodGet, Path: "/api/v1/analytics/overview", Handler: h},
		{Method: http.MethodGet, Path: "/api/v1/analytics/funnel", Handler: h},
		{Method: http.MethodGet, Path: "/api/v1/analytics/retention", Handler: h},
		{Method: http.MethodGet, Path: "/api/v1/analytics/activities/:id/effect", Handler: h},
		{Method: http.MethodGet, Path: "/api/v1/analytics/reconciliation", Handler: h},
		// GP-403 渠道效果（F22）：按渠道汇总注册/去重用户/付费，对照渠道包
		// 追踪口径（登录与 AdminUserIds 名单鉴权在 data-panel 侧）
		{Method: http.MethodGet, Path: "/api/v1/analytics/channels", Handler: h},
		// GP-404 合作方自助数据（角色 partner + partner_id 行级隔离在 data-panel 侧）
		{Method: http.MethodGet, Path: "/api/v1/analytics/partner", Handler: h},
	})
}
