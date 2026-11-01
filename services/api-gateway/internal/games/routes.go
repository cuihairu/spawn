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
		// 运营标签维护（PATCH 标签）：Authorization 头透传，鉴权在 game-catalog 侧。
		{Method: http.MethodPatch, Path: "/games/:id/tags", Handler: h},
		{Method: http.MethodGet, Path: "/me", Handler: h},
		// GP-103 运营后台与开发者自助入口：Authorization 头透传，角色鉴权在 game-catalog 侧。
		{Method: http.MethodPost, Path: "/admin/roles", Handler: h},
		{Method: http.MethodPost, Path: "/admin/partners", Handler: h},
		{Method: http.MethodGet, Path: "/admin/partners", Handler: h},
		{Method: http.MethodPatch, Path: "/admin/partners/:id", Handler: h},
		{Method: http.MethodPost, Path: "/admin/games/:id/ownership", Handler: h},
		{Method: http.MethodGet, Path: "/admin/reviews", Handler: h},
		{Method: http.MethodPost, Path: "/admin/reviews/:id/decision", Handler: h},
		{Method: http.MethodPost, Path: "/admin/games/:id/publish", Handler: h},
		{Method: http.MethodPost, Path: "/admin/games/:id/unpublish", Handler: h},
		{Method: http.MethodGet, Path: "/admin/games/:id/versions", Handler: h},
		{Method: http.MethodPost, Path: "/admin/games/:id/versions/:versionId/publish", Handler: h},
		{Method: http.MethodPost, Path: "/admin/games/:id/versions/:versionId/revoke", Handler: h},
		{Method: http.MethodGet, Path: "/admin/audit", Handler: h},
		{Method: http.MethodGet, Path: "/partner/games", Handler: h},
		{Method: http.MethodPost, Path: "/partner/games/:id/submit", Handler: h},
		{Method: http.MethodPost, Path: "/partner/games/:id/versions", Handler: h},
		{Method: http.MethodGet, Path: "/partner/games/:id/versions", Handler: h},
	})
}
