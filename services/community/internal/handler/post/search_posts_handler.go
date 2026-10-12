// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package post

import (
	"net/http"

	"github.com/tappi/tappi/services/community/internal/logic/post"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// SearchPostsHandler 公开端点（无认证）：帖子关键词搜索。
func SearchPostsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.SearchPostsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := post.NewSearchPostsLogic(r.Context(), svcCtx)
		resp, err := l.SearchPosts(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
