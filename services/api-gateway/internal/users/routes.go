package users

import (
	"net/http"
	"time"

	"github.com/tappi/tappi/services/api-gateway/internal/proxy"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/zeromicro/go-zero/rest"
)

// RegisterUserProxyRoutes user-service 反代（BFF 第一阶段：mobile-app 统一入口）。
// 注册/登录均原样透传上游信封（register/login 响应形状与 user-service 永不漂移），
// 另有资料查询。Bearer 原样透传，401 由上游语义保持（App 统一跳登录）。
func RegisterUserProxyRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	baseURL := serverCtx.Config.Upstreams.UserService.BaseURL
	timeout := time.Millisecond * time.Duration(serverCtx.Config.Upstreams.UserService.Timeout)
	upstream, err := proxy.NewUpstream(baseURL, timeout)
	if err != nil {
		panic(err)
	}
	h := upstream.Handler()

	server.AddRoutes([]rest.Route{
		{Method: http.MethodPost, Path: "/auth/register", Handler: h},
		{Method: http.MethodPost, Path: "/auth/login", Handler: h},
		{Method: http.MethodGet, Path: "/users/:id", Handler: h},
	})
}
