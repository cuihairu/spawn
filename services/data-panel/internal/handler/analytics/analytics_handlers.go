// Package analytics 承载 GP-402 经营分析端点 handler：S2S 摄入
// （/internal/events）与管理查询（漏斗/留存/活动效果/账单核对）。
package analytics

import (
	"net/http"

	alogic "github.com/tappi/tappi/services/data-panel/internal/logic/analytics"
	"github.com/tappi/tappi/services/data-panel/internal/svc"
	"github.com/tappi/tappi/services/data-panel/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// IngestEventsHandler POST /internal/events（S2S：X-Internal-Token 严格比对）
func IngestEventsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := alogic.CheckInternalToken(svcCtx, r.Header.Get("X-Internal-Token")); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		var req types.IngestAnalyticsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := alogic.NewIngestLogic(r.Context(), svcCtx)
		resp, err := l.Ingest(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// AnalyticsOverviewHandler GET /api/v1/analytics/overview（名单管理员）
func AnalyticsOverviewHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AnalyticsOverviewReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := alogic.NewOverviewLogic(r.Context(), svcCtx)
		resp, err := l.Overview(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// AnalyticsFunnelHandler GET /api/v1/analytics/funnel（名单管理员）
func AnalyticsFunnelHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AnalyticsFunnelReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := alogic.NewFunnelLogic(r.Context(), svcCtx)
		resp, err := l.Funnel(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// AnalyticsRetentionHandler GET /api/v1/analytics/retention（名单管理员）
func AnalyticsRetentionHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AnalyticsRetentionReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := alogic.NewRetentionLogic(r.Context(), svcCtx)
		resp, err := l.Retention(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// AnalyticsActivityEffectHandler GET /api/v1/analytics/activities/:id/effect（名单管理员）
func AnalyticsActivityEffectHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AnalyticsActivityReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := alogic.NewActivityEffectLogic(r.Context(), svcCtx)
		resp, err := l.Effect(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// AnalyticsReconciliationHandler GET /api/v1/analytics/reconciliation（名单管理员）
func AnalyticsReconciliationHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AnalyticsReconciliationReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := alogic.NewReconciliationLogic(r.Context(), svcCtx)
		resp, err := l.Reconcile(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// AnalyticsChannelEffectHandler GET /api/v1/analytics/channels（名单管理员）：
// 渠道效果（F22/GP-403），按渠道标识聚合注册人数与付费金额。
func AnalyticsChannelEffectHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AnalyticsChannelEffectReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}
		l := alogic.NewChannelEffectLogic(r.Context(), svcCtx)
		resp, err := l.Query(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
