package datapanel

import (
	"testing"

	"github.com/tappi/tappi/services/api-gateway/internal/config"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"

	"github.com/zeromicro/go-zero/rest"
)

// TestRegisterDataPanelProxyRoutesPanics 非法上游地址 → NewUpstream 失败 panic。
func TestRegisterDataPanelProxyRoutesPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("invalid upstream base url must panic at registration")
		}
	}()

	var cfg config.Config
	cfg.Upstreams.DataPanel.BaseURL = "/relative/only"
	cfg.Upstreams.DataPanel.Timeout = 1000
	RegisterDataPanelProxyRoutes(rest.MustNewServer(rest.RestConf{}), &svc.ServiceContext{Config: cfg})
}
