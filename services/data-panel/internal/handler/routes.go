// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package handler

import (
	"net/http"

	"github.com/tappi/tappi/services/data-panel/internal/handler/analytics"
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

	// GP-402 经营分析（F24）：S2S 事件摄入口（X-Internal-Token 严格比对，
	// 空令牌 fail-closed；/internal/* 不经网关暴露）
	server.AddRoutes(
		[]rest.Route{
			{
				Method:  http.MethodPost,
				Path:    "/internal/events",
				Handler: analytics.IngestEventsHandler(serverCtx),
			},
		},
	)

	// GP-402 经营分析查询：漏斗/留存/活动效果/账单核对，登录 +
	// Analytics.AdminUserIds 名单双闸（名单为空全关）
	server.AddRoutes(
		rest.WithMiddlewares(
			[]rest.Middleware{serverCtx.Auth},
			[]rest.Route{
				{
					Method:  http.MethodGet,
					Path:    "/analytics/overview",
					Handler: analytics.AnalyticsOverviewHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/analytics/funnel",
					Handler: analytics.AnalyticsFunnelHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/analytics/retention",
					Handler: analytics.AnalyticsRetentionHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/analytics/activities/:id/effect",
					Handler: analytics.AnalyticsActivityEffectHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/analytics/reconciliation",
					Handler: analytics.AnalyticsReconciliationHandler(serverCtx),
				},
				{
					Method:  http.MethodGet,
					Path:    "/analytics/channels",
					Handler: analytics.AnalyticsChannelEffectHandler(serverCtx),
				},
			}...,
		),
		rest.WithPrefix("/api/v1"),
	)
}
