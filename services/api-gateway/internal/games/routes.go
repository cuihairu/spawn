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
		// GP-201 区服目录/充值目标：公开读 + 合作方维护（鉴权在 game-catalog 侧）
		{Method: http.MethodGet, Path: "/games/:id/servers", Handler: h},
		{Method: http.MethodPost, Path: "/partner/games/:id/servers", Handler: h},
		{Method: http.MethodGet, Path: "/partner/games/:id/servers", Handler: h},
		{Method: http.MethodPatch, Path: "/partner/games/:id/servers/:serverId", Handler: h},
		// GP-306 开服预约：玩家预约/取消/我的预约（Authorization 头透传，
		// 登录与角色在 game-catalog 侧；S2S /internal/* 不经网关暴露）
		{Method: http.MethodPost, Path: "/games/:id/servers/:serverId/reserve", Handler: h},
		{Method: http.MethodDelete, Path: "/games/:id/servers/:serverId/reserve", Handler: h},
		{Method: http.MethodGet, Path: "/games/:id/reservations/me", Handler: h},
		// GP-401 游戏评价（F04）：长评公开读 + 评分/发评/改评/举报；
		// 开发者回复与举报处置在合作方/管理路由，鉴权均在 game-catalog 侧
		{Method: http.MethodGet, Path: "/games/:id/reviews", Handler: h},
		{Method: http.MethodPost, Path: "/games/:id/rating", Handler: h},
		{Method: http.MethodPost, Path: "/games/:id/reviews", Handler: h},
		{Method: http.MethodPut, Path: "/games/:id/reviews/:reviewId", Handler: h},
		{Method: http.MethodPost, Path: "/games/:id/reviews/:reviewId/report", Handler: h},
		{Method: http.MethodPost, Path: "/partner/games/:id/reviews/:reviewId/reply", Handler: h},
		{Method: http.MethodGet, Path: "/admin/review-reports", Handler: h},
		{Method: http.MethodPost, Path: "/admin/review-reports/:id/handle", Handler: h},
		// GP-401 收藏游戏（F05）：收藏/取消幂等 + 我的收藏
		{Method: http.MethodPost, Path: "/games/:id/favorite", Handler: h},
		{Method: http.MethodDelete, Path: "/games/:id/favorite", Handler: h},
		{Method: http.MethodGet, Path: "/me/favorites", Handler: h},
	})

	// GP-105 包体通道：latest 元数据是小 JSON，走普通反代；上传/下载为
	// GB 级流量，走流式透传（不受 1 MiB 缓冲/整体超时约束），鉴权、
	// 角色与大小上限都在 game-catalog 侧强制。
	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/games/:id/versions/latest", Handler: h},
	})
	stream := upstream.StreamingHandler()
	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/games/:id/versions/:versionId/download", Handler: stream},
		{Method: http.MethodPost, Path: "/partner/games/:id/versions/:versionId/package", Handler: stream},
	})
}
