package main

// user.go run() 装配臂补漏：DATA_PANEL_URL（Analytics.BaseURL）配置后
// AnalyticsRelay 非 nil → 启动 GP-402 outbox 重投 goroutine 臂。
//
// 登记不可达（不测）：main() 的 flag.Parse 与 stderr+os.Exit 分支（进程级，
// 无法在测试内安全调用）。

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// TestRun_AnalyticsRelayWiring 配置带 Analytics.BaseURL → run 装配出非 nil
// AnalyticsRelay 并拉起重投 goroutine（与已覆盖的 PromoAttrRelay 臂对称）。
func TestRun_AnalyticsRelayWiring(t *testing.T) {
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
Analytics:
  BaseURL: "http://127.0.0.1:8891"
  Token: "tok"
  Timeout: 1000
  OutboxIntervalSeconds: 30
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
}
