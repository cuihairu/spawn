package handler

import (
	"net/http"

	"github.com/tappi/tappi/services/user-service/internal/logic"
	"github.com/tappi/tappi/services/user-service/internal/svc"
	"github.com/tappi/tappi/services/user-service/internal/types"

	"github.com/zeromicro/go-zero/rest/httpx"
)

func GetUserRecommendationsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.GetUserRecommendationsRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := logic.NewGetUserRecommendationsLogic(r.Context(), svcCtx)
		resp, err := l.GetUserRecommendations(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
