package realtime

import (
	"net/http"
	"strings"
	"time"

	"github.com/tappi/tappi/services/api-gateway/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// RegisterRealtimeTunnelRoutes realtime-hub WS 隧道路由。BaseURL 未
// 配置时跳过注册（hub 为可选部署，网关其余功能不受影响）。
func RegisterRealtimeTunnelRoutes(server *rest.Server, serverCtx *svc.ServiceContext) {
	baseURL := serverCtx.Config.Upstreams.RealtimeHub.BaseURL
	if strings.TrimSpace(baseURL) == "" {
		return
	}
	timeout := time.Millisecond * time.Duration(serverCtx.Config.Upstreams.RealtimeHub.Timeout)
	tunnel, err := NewTunnel(baseURL, timeout)
	if err != nil {
		panic(err)
	}
	server.AddRoutes([]rest.Route{
		{Method: http.MethodGet, Path: "/ws", Handler: tunnel.Handler()},
	})
}
