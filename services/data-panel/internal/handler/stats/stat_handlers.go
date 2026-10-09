// Package stats 承载战绩面板四个端点的 handler（汇总/明细/单游戏/摄入）。
package stats

import (
	"net/http"

	"github.com/tappi/tappi/services/data-panel/internal/logic/stats"
	"github.com/tappi/tappi/services/data-panel/internal/svc"
	"github.com/tappi/tappi/services/data-panel/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// GetSummaryHandler GET /api/v1/stats/users/:user_id/summary（公开）
func GetSummaryHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetSummaryReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := stats.NewGetSummaryLogic(r.Context(), svcCtx)
		resp, err := l.GetSummary(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// ListGameStatsHandler GET /api/v1/stats/users/:user_id/games（公开）
func ListGameStatsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ListGameStatsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := stats.NewListGameStatsLogic(r.Context(), svcCtx)
		resp, err := l.ListGameStats(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// GetGameStatHandler GET /api/v1/stats/users/:user_id/games/:game_id（公开）
func GetGameStatHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetGameStatReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := stats.NewGetGameStatLogic(r.Context(), svcCtx)
		resp, err := l.GetGameStat(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// RecordStatHandler POST /api/v1/stats/records（登录）
func RecordStatHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RecordStatReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := stats.NewRecordStatLogic(r.Context(), svcCtx)
		resp, err := l.Record(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
