package main

import (
	"context"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/tappi/tappi/services/user-service-rpc/user"
	"github.com/tappi/tappi/services/user-service-rpc/userclient"

	"github.com/zeromicro/go-zero/zrpc"
)

// TestRun_LoadConfigError 配置文件不存在 → run 返回错误（不再 panic）。
func TestRun_LoadConfigError(t *testing.T) {
	*configFile = filepath.Join(t.TempDir(), "missing.yaml")
	if err := run(); err == nil {
		t.Fatal("missing config must return an error")
	}
}

// TestRun_ServesPingThroughUserClient 完整装配链（Mode: test → 含 reflection 注册
// 分支）：run() 起真 zRPC 服务，userclient 经 gRPC 端到端 Ping。run() 阻塞于
// Start()，测试放行 goroutine 后由进程退出回收；就绪以轮询 Ping 成功为准
// （客户端 NonBlock 默认开，未就绪时 Ping 报连接错误，属预期重试路径）。
func TestRun_ServesPingThroughUserClient(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	cfg := filepath.Join(t.TempDir(), "user.yaml")
	content := `Name: user-rpc-test
ListenOn: 127.0.0.1:` + strconv.Itoa(port) + `
Mode: test
`
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	*configFile = cfg

	done := make(chan error, 1)
	go func() { done <- run() }()

	cli, err := zrpc.NewClientWithTarget("127.0.0.1:" + strconv.Itoa(port))
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	defer cli.Conn().Close()

	uc := userclient.NewUser(cli)
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := uc.Ping(context.Background(), &user.Request{Ping: "ping"})
		if err == nil {
			if resp == nil || resp.Pong != "" {
				t.Fatalf("resp = %+v, want empty Response", resp)
			}
			break
		}
		select {
		case err := <-done:
			t.Fatalf("run exited early: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatalf("service did not become ready: %v", err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
