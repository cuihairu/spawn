// Package moderation 承载内容审核三个端点的 handler（举报/队列查看/处置）。
package moderation

import (
	"net/http"

	"github.com/tappi/tappi/services/community/internal/logic/moderation"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// ReportPostHandler POST /api/v1/posts/:id/report（登录）
func ReportPostHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ReportPostReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := moderation.NewReportPostLogic(r.Context(), svcCtx)
		resp, err := l.ReportPost(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// ListReportsHandler GET /api/v1/moderation/reports（管理员）
func ListReportsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ListReportsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := moderation.NewListReportsLogic(r.Context(), svcCtx)
		resp, err := l.ListReports(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// HandleReportHandler POST /api/v1/moderation/reports/:id/handle（管理员）
func HandleReportHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.HandleReportReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := moderation.NewHandleReportLogic(r.Context(), svcCtx)
		resp, err := l.HandleReport(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
