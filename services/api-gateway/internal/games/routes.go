package games

import (
	"net/http"
	"time"

	"github.com/tappi/tappi/services/api-gateway/internal/proxy"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/zeromicro/go-zero/rest"
)

// RegisterGameProxyRoutes game-catalog 反代（BFF 第一阶段：mobile-app 统一入口）。
// 精选仍走自有聚合 handler（/games/featured，limit 兜底），这里补 App 在用的
// 列表搜索与详情。query（keyword/limit/offset）与路径参数原样透传。
func RegisterGameProxyRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	baseURL := serverCtx.Config.Upstreams.GameCatalog.BaseURL
	timeout := time.Millisecond * time.Duration(serverCtx.Config.Upstreams.GameCatalog.Timeout)
	upstream, err := proxy.NewUpstream(baseURL, timeout)
	if err != nil {
		panic(err)
	}
	h := upstream.Handler()

	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/games", Handler: h},
		{Method: http.MethodGet, Path: "/games/:id", Handler: h},
	})
}
