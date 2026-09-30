package logic

import (
	"context"
	"testing"

	"github.com/tappi/tappi/services/user-service-rpc/internal/config"
	"github.com/tappi/tappi/services/user-service-rpc/internal/svc"
	"github.com/tappi/tappi/services/user-service-rpc/user"
)

// TestPingLogic_ReturnsEmptyResponse 骨架契约：任何 Request 返回空 Response 且无错误。
func TestPingLogic_ReturnsEmptyResponse(t *testing.T) {
	l := NewPingLogic(context.Background(), svc.NewServiceContext(config.Config{}))
	if l.Logger == nil {
		t.Fatal("logic must carry a logger")
	}

	resp, err := l.Ping(&user.Request{Ping: "ping"})
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if resp == nil {
		t.Fatal("resp must not be nil")
	}
	if resp.Pong != "" {
		t.Fatalf("Pong = %q, want empty (skeleton contract)", resp.Pong)
	}
}
