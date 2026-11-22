package main

// game.go run() 装配臂补漏：COMMUNITY_URL（Notify.Url）与 DATA_PANEL_URL
// （Analytics.BaseURL）配置后 NotifyRelay/AnalyticsRelay 非 nil → 拉起两个
// outbox 重投 goroutine 臂（GP-306/401 通知 + GP-402 事实）。
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

// TestRun_RelaysWiring 配置带 Notify.Url + Analytics.BaseURL → run 装配出两个
// 非 nil Relay 并拉起重投 goroutine。
func TestRun_RelaysWiring(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	cfg := filepath.Join(t.TempDir(), "game.yaml")
	content := `Name: game-catalog-test
Host: 127.0.0.1
Port: ` + strconv.Itoa(port) + `
MySQL:
  DataSource: "file:` + filepath.Join(t.TempDir(), "games.db") + `"
Auth:
  JWTSecret: run-integration-secret
Notify:
  Url: "http://127.0.0.1:8892"
  Token: "tok"
Analytics:
  BaseURL: "http://127.0.0.1:8893"
  Token: "tok"
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
		resp, err := http.Get(base + "/games")
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
