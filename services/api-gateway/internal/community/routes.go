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
		{Method: http.MethodPost, Path: "/api/v1/posts", Handler: h},
		{Method: http.MethodPut, Path: "/api/v1/posts/:id", Handler: h},
		{Method: http.MethodDelete, Path: "/api/v1/posts/:id", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/posts/:id/like", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/posts/:id/share", Handler: h},

		{Method: http.MethodPost, Path: "/api/v1/users/:user_id/follow", Handler: h},
		{Method: http.MethodDelete, Path: "/api/v1/users/:user_id/follow", Handler: h},
	})
}
