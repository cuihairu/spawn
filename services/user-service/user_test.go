package main

import (
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
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

// TestRun_StartsUserService 完整装配链：加载临时配置 → NewServiceContext
// （SQLite 临时文件库）→ 注册路由 → 认证中间件 → 服务器可接受请求。
// 注：run() 阻塞于 server.Start()，测试放行 goroutine 后由进程退出回收。
func TestRun_StartsUserService(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	cfg := filepath.Join(t.TempDir(), "user-api.yaml")
	content := `Name: user-service-test
Host: 127.0.0.1
Port: ` + strconv.Itoa(port) + `
MySQL:
  DataSource: file:` + filepath.Join(t.TempDir(), "users.db") + `
Auth:
  JWTSecret: run-integration-secret
  TokenExpire: 7
Services:
  GameCatalog:
    BaseURL: "http://localhost:8890"
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

	// 免认证示例路由可达
	resp, err := http.Get(base + "/from/you")
	if err != nil {
		t.Fatalf("from route: %v", err)
	}
	body := readAll(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Hello, you!") {
		t.Fatalf("from/you status=%d body=%s", resp.StatusCode, body)
	}
}

// TestFromNameMe 通过昵称 "me" 访问示例路由
func TestFromNameMe(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	cfg := filepath.Join(t.TempDir(), "user-api.yaml")
	content := `Name: user-service-test
Host: 127.0.0.1
Port: ` + strconv.Itoa(port) + `
MySQL:
  DataSource: file:` + filepath.Join(t.TempDir(), "users.db") + `
Auth:
  JWTSecret: run-integration-secret
  TokenExpire: 7
Services:
  GameCatalog:
    BaseURL: "http://localhost:8890"
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
		resp, err := http.Get(base + "/readyz")
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

	resp, err := http.Get(base + "/from/me")
	if err != nil {
		t.Fatalf("from/me: %v", err)
	}
	body := readAll(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(body, "Hello, me!") {
		t.Fatalf("from/me status=%d body=%s", resp.StatusCode, body)
	}
}

func readAll(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	return string(b)
}
