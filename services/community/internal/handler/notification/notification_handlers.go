// Package notification 承载站内通知三个端点的 handler（列表/未读数/全部已读）。
package notification

import (
	"net/http"

	"github.com/tappi/tappi/services/community/internal/logic/notification"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// ListNotificationsHandler GET /api/v1/notifications（登录）
func ListNotificationsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.ListNotificationsReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := notification.NewListNotificationsLogic(r.Context(), svcCtx)
		resp, err := l.ListNotifications(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// GetUnreadCountHandler GET /api/v1/notifications/unread-count（登录）
func GetUnreadCountHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := notification.NewGetUnreadCountLogic(r.Context(), svcCtx)
		resp, err := l.GetUnreadCount()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// MarkAllReadHandler POST /api/v1/notifications/read-all（登录）
func MarkAllReadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := notification.NewMarkAllReadLogic(r.Context(), svcCtx)
		resp, err := l.MarkAllRead()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
