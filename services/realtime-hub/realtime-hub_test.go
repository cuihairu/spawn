package main

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}

// TestRun_LoadConfigError 配置文件不存在 → run 返回错误（不再 panic）。
func TestRun_LoadConfigError(t *testing.T) {
	if err := run(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("missing config must return an error")
	}
}

// TestRun_MissingSecretError 配置缺 JwtSecret → run 返回错误拒绝启动。
func TestRun_MissingSecretError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nosecret.yaml")
	if err := writeFile(path, "Listen: \":8893\"\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := run(path); err == nil {
		t.Fatal("missing jwt secret must return an error")
	}
}

// TestRun_ListenError 端口被占（先占坑）→ run 返回监听错误。
func TestRun_ListenError(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	defer l.Close()
	path := filepath.Join(t.TempDir(), "occupied.yaml")
	if err := writeFile(path, "Listen: \"127.0.0.1:"+strconv.Itoa(l.Addr().(*net.TCPAddr).Port)+"\"\nJwtSecret: s\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := run(path); err == nil {
		t.Fatal("occupied listen addr must return an error")
	}
}

// TestRun_StartsHubService 完整装配：临时配置起服 → /health 可达。
// run() 阻塞于 Serve，放 goroutine 后由测试进程退出回收。
func TestRun_StartsHubService(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	path := filepath.Join(t.TempDir(), "hub.yaml")
	if err := writeFile(path, "Listen: \"127.0.0.1:"+strconv.Itoa(port)+"\"\nJwtSecret: run-secret\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	go func() { _ = run(path) }()

	// 轮询探活（服务在独立 goroutine 启动）
	ok := false
	for i := 0; i < 50 && !ok; i++ {
		conn, err := net.Dial("tcp", "127.0.0.1:"+strconv.Itoa(port))
		if err == nil {
			_ = conn.Close()
			ok = true
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !ok {
		t.Fatal("service did not start listening")
	}
}
