package main

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// TestRun_LoadConfigError 配置文件不存在 → run 返回错误（不再 panic）。
func TestRun_LoadConfigError(t *testing.T) {
	*configFile = filepath.Join(t.TempDir(), "missing.yaml")
	if err := run(); err == nil {
		t.Fatal("missing config must return an error")
	}
}

// TestRun_StartsGateway 完整装配链：加载临时配置 → 构造 ServiceContext →
// 注册全部路由（API + 两类代理）→ 服务器可接受请求。
// 注：run() 阻塞于 server.Start()，测试放行 goroutine 后由进程退出回收；
// 端口取预占空闲端口，无并发冲突。
func TestRun_StartsGateway(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	cfg := filepath.Join(t.TempDir(), "gateway.yaml")
	content := `Name: api-gateway-test
Host: 127.0.0.1
Port: ` + strconv.Itoa(port) + `
Auth:
  JWTSecret: "test-secret"
Upload:
  Dir: "` + t.TempDir() + `"
  MaxBytes: 10485760
Upstreams:
  UserService:
    BaseURL: "http://localhost:18888"
    Timeout: 1000
  GameCatalog:
    BaseURL: "http://localhost:18890"
    Timeout: 1000
  Content:
    BaseURL: "http://localhost:18891"
    Timeout: 1000
  Community:
    BaseURL: "http://localhost:18892"
    Timeout: 1000
  DataPanel:
    BaseURL: "http://localhost:18896"
    Timeout: 1000
`
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	*configFile = cfg

	done := make(chan error, 1)
	go func() { done <- run() }()

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get(base + "/readyz") // 未注册路径：连接成功即视为就绪
		if err == nil {
			resp.Body.Close()
			break
		}
		select {
		case err := <-done:
			t.Fatalf("run exited early: %v", err)
		default:
		}
		if time.Now().After(deadline) {
			t.Fatal("gateway did not become ready")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 代理路由可达性：请求经网关转发到（不可达的）上游 → 502 JSON 错误
	resp, err := http.Get(base + "/api/v1/topics")
	if err != nil {
		t.Fatalf("proxy route: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("proxy status = %d, want 502 (upstream down)", resp.StatusCode)
	}
}
