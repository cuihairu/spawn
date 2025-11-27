package handler

import (
	"net/http"

	"github.com/tappi/tappi/services/content-service/internal/logic"
	"github.com/tappi/tappi/services/content-service/internal/svc"
	"github.com/tappi/tappi/services/content-service/internal/types"

	"github.com/zeromicro/go-zero/rest/httpx"
)

func PublishGuideHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.PublishGuideRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := logic.NewPublishGuideLogic(r.Context(), svcCtx)
		resp, err := l.PublishGuide(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
