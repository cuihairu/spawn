// Package message 承载站内私信五个端点的 handler（发送/会话列表/会话消息/
// 会话已读/未读总数）。
package message

import (
	"net/http"

	"github.com/tappi/tappi/services/community/internal/logic/message"
	"github.com/tappi/tappi/services/community/internal/svc"
	"github.com/tappi/tappi/services/community/internal/types"
	"github.com/zeromicro/go-zero/rest/httpx"
)

// SendDirectMessageHandler POST /api/v1/messages（登录）
func SendDirectMessageHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.SendDirectMessageReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := message.NewSendDirectMessageLogic(r.Context(), svcCtx)
		resp, err := l.SendDirectMessage(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// ListDMConversationsHandler GET /api/v1/messages/conversations（登录）
func ListDMConversationsHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := message.NewListDMConversationsLogic(r.Context(), svcCtx)
		resp, err := l.ListDMConversations()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// GetDMThreadHandler GET /api/v1/messages/thread/:peer_id（登录）
func GetDMThreadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DMThreadReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := message.NewGetDMThreadLogic(r.Context(), svcCtx)
		resp, err := l.GetDMThread(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// MarkDMThreadReadHandler POST /api/v1/messages/thread/:peer_id/read（登录）
func MarkDMThreadReadHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req types.DMThreadReadReq
		if err := httpx.Parse(r, &req); err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
			return
		}

		l := message.NewMarkDMThreadReadLogic(r.Context(), svcCtx)
		resp, err := l.MarkDMThreadRead(&req)
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}

// GetDMUnreadCountHandler GET /api/v1/messages/unread-count（登录）
func GetDMUnreadCountHandler(svcCtx *svc.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		l := message.NewGetDMUnreadCountLogic(r.Context(), svcCtx)
		resp, err := l.GetDMUnreadCount()
		if err != nil {
			httpx.ErrorCtx(r.Context(), w, err)
		} else {
			httpx.OkJsonCtx(r.Context(), w, resp)
		}
	}
}
