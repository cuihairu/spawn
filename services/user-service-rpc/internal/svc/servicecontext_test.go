package svc

import (
	"testing"

	"github.com/tappi/tappi/services/user-service-rpc/internal/config"
)

// TestNewServiceContext 配置原样装配进 ServiceContext。
func TestNewServiceContext(t *testing.T) {
	var c config.Config
	c.ListenOn = "127.0.0.1:18888"

	ctx := NewServiceContext(c)
	if ctx.Config.ListenOn != c.ListenOn {
		t.Fatalf("Config.ListenOn = %q, want %q", ctx.Config.ListenOn, c.ListenOn)
	}
}
