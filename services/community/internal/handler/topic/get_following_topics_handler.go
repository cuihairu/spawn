// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package topic

import (
	"net/http"

	"github.com/tappi/tappi/services/community/internal/logic/topic"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func GetFollowingTopicsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := topic.NewGetFollowingTopicsLogic(r.Context(), svcCtx)
		resp, err := l.GetFollowingTopics()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
