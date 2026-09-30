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

// TestRun_StartsContentService 完整装配链：加载临时配置 → NewServiceContext
// （数据文件缺失 → 种子回退）→ 注册路由 → 认证中间件 → 服务器可接受请求。
// 注：run() 阻塞于 server.Start()，测试放行 goroutine 后由进程退出回收。
func TestRun_StartsContentService(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	cfg := filepath.Join(t.TempDir(), "content-api.yaml")
	content := `Name: content-service-test
Host: 127.0.0.1
Port: ` + strconv.Itoa(port) + `
DataSource:
  GuidesFile: ` + filepath.Join(t.TempDir(), "guides.json") + `
  CommentsFile: ` + filepath.Join(t.TempDir(), "comments.json") + `
Auth:
  JWTSecret: run-integration-secret
Services:
  GameCatalog:
    BaseURL: "http://localhost:18890"
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
			t.Fatal("service did not become ready")
		}
		time.Sleep(20 * time.Millisecond)
	}

	// 公开读路由可达（种子攻略列表）
	resp, err := http.Get(base + "/api/v1/guides")
	if err != nil {
		t.Fatalf("guides route: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("guides status = %d, want 200", resp.StatusCode)
	}
}
