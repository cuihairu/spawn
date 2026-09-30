package server

import (
	"context"
	"testing"

	"github.com/tappi/tappi/services/user-service-rpc/internal/config"
	"github.com/tappi/tappi/services/user-service-rpc/internal/svc"
	"github.com/tappi/tappi/services/user-service-rpc/user"
)

// TestUserServer_PingDelegatesToLogic server 层只做转发：
// 持有的 ServiceContext 原样保留，Ping 委托给 logic 并透传结果。
func TestUserServer_PingDelegatesToLogic(t *testing.T) {
	svcCtx := svc.NewServiceContext(config.Config{})
	s := NewUserServer(svcCtx)
	if s.svcCtx != svcCtx {
		t.Fatal("NewUserServer must keep the given ServiceContext")
	}

	resp, err := s.Ping(context.Background(), &user.Request{Ping: "ping"})
	if err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if resp == nil || resp.Pong != "" {
		t.Fatalf("resp = %+v, want empty Response", resp)
	}
}
