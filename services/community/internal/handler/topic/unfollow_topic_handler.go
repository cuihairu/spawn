// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package topic

import (
	"net/http"

	"community/internal/logic/topic"
	"community/internal/svc"
	"community/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func UnfollowTopicHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.FollowTopicReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := topic.NewUnfollowTopicLogic(r.Context(), svcCtx)
		resp, err := l.UnfollowTopic(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
