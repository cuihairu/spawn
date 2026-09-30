package main

import (
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"
)

// TestRun_LoadConfigError 配置文件不存在 → run 返回错误（不再 panic）。
func TestRun_LoadConfigError(t *testing.T) {
	*configFile = filepath.Join(t.TempDir(), "missing.yaml")
	if err := run(); err == nil {
		t.Fatal("missing config must return an error")
	}
}

// TestRun_StartsGameService 完整装配链：加载临时配置 → 构造 ServiceContext →
// 注册全部路由 → 服务器可接受请求（GET /games 匿名公开）。
// 注：run() 阻塞于 server.Start()，测试放行 goroutine 后由进程退出回收；
// 端口取预占空闲端口，无并发冲突。
func TestRun_StartsGameService(t *testing.T) {
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
DataSource:
  File: ` + filepath.Join(t.TempDir(), "games.json") + `
Auth:
  JWTSecret: run-integration-secret
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

	// 路由可达性：GET /games 匿名公开，返回种子游戏列表
	resp, err := http.Get(base + "/games")
	if err != nil {
		t.Fatalf("list route: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("list status = %d, want 200", resp.StatusCode)
	}
}

// TestNewServiceContextBadDataSource 数据文件存在但内容非法 JSON →
// NewServiceContext 以 panic 终止（与生产行为一致，启动期快速失败）。
func TestNewServiceContextBadDataSource(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "games.json")
	if err := os.WriteFile(bad, []byte("{invalid json"), 0o644); err != nil {
		t.Fatalf("write bad data file: %v", err)
	}

	var c config.Config
	c.DataSource.File = bad
	c.Auth.JWTSecret = "panic-test-secret"

	defer func() {
		if recover() == nil {
			t.Fatal("bad game data file must panic in NewServiceContext")
		}
	}()
	svc.NewServiceContext(c)
}
