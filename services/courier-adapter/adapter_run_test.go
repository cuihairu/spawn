package main

// run() 装配链覆盖：配置文件缺失 → 返回错误（不再 panic）；完整配置 →
// ServiceContext 装配 + 路由注册 + 活跃上报 Relay goroutine 启动，
// 服务器可接受请求（GET /healthz）。

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func TestRun_ConfigLoadError(t *testing.T) {
	orig := *configFile
	*configFile = filepath.Join(t.TempDir(), "missing.yaml")
	defer func() { *configFile = orig }()
	if err := run(); err == nil {
		t.Fatal("missing config must return an error")
	}
}

func TestRun_StartsAdapterService(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	cfg := filepath.Join(t.TempDir(), "adapter.yaml")
	content := `Name: courier-adapter-run-test
Host: 127.0.0.1
Port: ` + strconv.Itoa(port) + `
MySQL:
  DataSource: "file:` + filepath.Join(t.TempDir(), "run.db") + `"
Auth:
  JWTSecret: run-integration-secret
Credential:
  Key: run-credential-key
Courier:
  BaseURL: http://127.0.0.1:1
  Timeout: 500
GameCatalog:
  BaseURL: http://127.0.0.1:1
  Timeout: 500
UserService:
  BaseURL: http://127.0.0.1:1
  Timeout: 500
Ticket:
  TTLSeconds: 120
`
	if err := os.WriteFile(cfg, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	orig := *configFile
	*configFile = cfg
	defer func() { *configFile = orig }()

	done := make(chan error, 1)
	go func() { done <- run() }()

	base := "http://127.0.0.1:" + strconv.Itoa(port)
	deadline := time.Now().Add(5 * time.Second)
	for {
		resp, err := http.Get(base + "/healthz")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
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
}
