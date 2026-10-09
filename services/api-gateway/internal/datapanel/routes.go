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
	})
}
