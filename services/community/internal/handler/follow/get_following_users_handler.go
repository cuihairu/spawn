// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package follow

import (
	"net/http"

	"github.com/tappi/tappi/services/community/internal/logic/follow"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func GetFollowingUsersHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := follow.NewGetFollowingUsersLogic(r.Context(), svcCtx)
		resp, err := l.GetFollowingUsers()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
