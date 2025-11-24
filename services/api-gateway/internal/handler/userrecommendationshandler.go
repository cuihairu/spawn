package handler

import (
	"net/http"
	"strings"

	"github.com/tappi/tappi/services/api-gateway/internal/logic"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/tappi/tappi/services/api-gateway/internal/types"

	"github.com/zeromicro/go-zero/rest/httpx"
)

func UserRecommendationsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.RecommendationRequest
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		token := extractToken(r.Header.Get("Authorization"))

		l := logic.NewUserRecommendationsLogic(r.Context(), svcCtx)
		resp, err := l.UserRecommendations(&req, token)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

func extractToken(header string) string {
	if header == "" {
		return ""
	}
	if strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(header[7:])
	}
	return strings.TrimSpace(header)
}
