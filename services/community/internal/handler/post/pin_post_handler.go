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

// PinPostHandler GP-307 置顶/取消置顶（仅审核管理员）。
func PinPostHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.PinPostReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := post.NewPinPostLogic(r.Context(), svcCtx)
		if err := l.PinPost(&req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, map[string]string{"message": "ok"})
		}
	}
}

// ListAnnouncementsHandler GP-307 公告入口：官方帖聚合（置顶优先），公开读。
func ListAnnouncementsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.AnnouncementsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := post.NewListAnnouncementsLogic(r.Context(), svcCtx)
		resp, err := l.ListAnnouncements(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
