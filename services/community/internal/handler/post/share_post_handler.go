// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package post

import (
	"net/http"

	"community/internal/logic/post"
	"community/internal/svc"
	"github.com/zeromicro/go-zero/rest/httpx"
)

func SharePostHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := post.NewSharePostLogic(r.Context(), svcCtx)
		resp, err := l.SharePost()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
