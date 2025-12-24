package content

import (
	"net/http"
	"time"

	"github.com/tappi/tappi/services/api-gateway/internal/proxy"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/zeromicro/go-zero/rest"
)

func RegisterContentProxyRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	baseURL := serverCtx.Config.Upstreams.Content.BaseURL
	timeout := time.Millisecond * time.Duration(serverCtx.Config.Upstreams.Content.Timeout)
	upstream, err := proxy.NewUpstream(baseURL, timeout)
	if err != nil {
		panic(err)
	}
	h := upstream.Handler()

	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/api/v1/guides", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/guides", Handler: h},
		{Method: http.MethodGet, Path: "/api/v1/guides/:id", Handler: h},
		{Method: http.MethodPut, Path: "/api/v1/guides/:id", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/guides/:id/publish", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/guides/:id/like", Handler: h},

		{Method: http.MethodGet, Path: "/api/v1/comments", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/comments", Handler: h},
		{Method: http.MethodDelete, Path: "/api/v1/comments/:id", Handler: h},
		{Method: http.MethodPost, Path: "/api/v1/comments/:id/like", Handler: h},
	})
}
