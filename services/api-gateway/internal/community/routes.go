package community

import (
	"net/http"
	"time"

	"github.com/tappi/tappi/services/api-gateway/internal/proxy"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/zeromicro/go-zero/rest"
)

func RegisterCommunityProxyRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	baseURL := serverCtx.Config.Upstreams.Community.BaseURL
	timeout := time.Millisecond * time.Duration(serverCtx.Config.Upstreams.Community.Timeout)
	upstream, err := proxy.NewUpstream(baseURL, timeout)
	if err != nil {
		panic(err)
	}
	h := upstream.Handler()

	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/api/v1/topics", Handler: h},
		{Method: http.MethodGet, Path: "/api/v1/topics/:id", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/topics", Handler: h},
		{Method: http.MethodGet, Path: "/api/v1/topics/following", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/topics/:topic_id/follow", Handler: h},
		{Method: http.MethodDelete, Path: "/api/v1/topics/:topic_id/follow", Handler: h},

		{Method: http.MethodGet, Path: "/api/v1/posts", Handler: h},
		{Method: http.MethodGet, Path: "/api/v1/posts/:id", Handler: h},
		{Method: http.MethodGet, Path: "/api/v1/posts/hot", Handler: h},
		{Method: http.MethodGet, Path: "/api/v1/posts/followed", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/posts", Handler: h},
		{Method: http.MethodPut, Path: "/api/v1/posts/:id", Handler: h},
		{Method: http.MethodDelete, Path: "/api/v1/posts/:id", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/posts/:id/like", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/posts/:id/share", Handler: h},

		// 帖子评论（M3 web 对等补齐）：发评/删评登录语义由 community 校验，
		// 列表公开；删评挂帖子子资源（/api/v1/comments/:id 已属 content-service）。
		{Method: http.MethodGet, Path: "/api/v1/posts/:id/comments", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/posts/:id/comments", Handler: h},
		{Method: http.MethodDelete, Path: "/api/v1/posts/:id/comments/:cid", Handler: h},

		{Method: http.MethodGet, Path: "/api/v1/users/following", Handler: h},
		{Method: http.MethodGet, Path: "/api/v1/users/likes", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/users/:user_id/follow", Handler: h},
		{Method: http.MethodDelete, Path: "/api/v1/users/:user_id/follow", Handler: h},

		// 站内通知（登录语义由 community 校验）
		{Method: http.MethodGet, Path: "/api/v1/notifications", Handler: h},
		{Method: http.MethodGet, Path: "/api/v1/notifications/unread-count", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/notifications/read-all", Handler: h},

		// 内容审核：用户举报 + 管理员队列查看/处置（登录与管理员语义由 community 校验）
		{Method: http.MethodPost, Path: "/api/v1/posts/:id/report", Handler: h},
		{Method: http.MethodGet, Path: "/api/v1/moderation/reports", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/moderation/reports/:id/handle", Handler: h},
	})
}
