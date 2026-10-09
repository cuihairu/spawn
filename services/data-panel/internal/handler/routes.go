// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package handler

import (
	"net/http"

	"github.com/tappi/tappi/services/data-panel/internal/handler/stats"
	"github.com/tappi/tappi/services/data-panel/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

func RegisterHandlers(server *rest.Server, serverCtx *svc.ServiceContext) {
	// 公开读路由：战绩面板为只读展示数据
	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodGet,
				Path:    "/stats/users/:user_id/summary",
				Handler: stats.GetSummaryHandler(serverCtx),
			},
			{
				Method:  http.MethodGet,
				Path:    "/stats/users/:user_id/games",
				Handler: stats.ListGameStatsHandler(serverCtx),
			},
			{
				Method:  http.MethodGet,
				Path:    "/stats/users/:user_id/games/:game_id",
				Handler: stats.GetGameStatHandler(serverCtx),
			},
		},
		rest.WithPrefix("/api/v1"),
	)

	// 受保护路由：战绩摄入需要 Bearer 令牌（由 user-service 签发）
	server.AddRoutes(
		rest.WithMiddlewares(
			[]rest.Middleware{serverCtx.Auth},
			[]rest.Route{
				{
					Method:  http.MethodPost,
					Path:    "/stats/records",
					Handler: stats.RecordStatHandler(serverCtx),
				},
			}...,
		),
		rest.WithPrefix("/api/v1"),
	)
}
