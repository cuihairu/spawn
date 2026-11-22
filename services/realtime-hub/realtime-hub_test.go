package main

import (
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
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

// TestRun_RedisRelayError Redis 地址不可达（连接拒绝）→ run 返回错误，
// 不启动半可用实例（坏地址在启动期暴露）。
func TestRun_RedisRelayError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "badredis.yaml")
	if err := writeFile(path, "Listen: \"127.0.0.1:0\"\nJwtSecret: s\nRedisAddr: \"127.0.0.1:1\"\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := run(path); err == nil || !strings.Contains(err.Error(), "redis relay") {
		t.Fatalf("bad redis addr = %v, want redis relay error", err)
	}
}

// TestRun_WebhookNotifierWiring PushWebhook 配置后 SetNotifier 臂在 Serve 前
// 执行：run 阻塞于 Serve，放 goroutine 后探活即可。
func TestRun_WebhookNotifierWiring(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("grab port: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()

	path := filepath.Join(t.TempDir(), "webhook.yaml")
	if err := writeFile(path, "Listen: \"127.0.0.1:"+strconv.Itoa(port)+"\"\nJwtSecret: s\nPushWebhook: \"http://127.0.0.1:9/push\"\nPushToken: tok\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	go func() { _ = run(path) }()

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
		t.Fatal("service with notifier did not start listening")
	}
}

// TestNewApp_RedisRelayWiring 装配成功路径：miniredis 充当跨实例 Redis（ping/
// 订阅均可用）→ relay 建立并挂接，stop 时 relay.Close 释放。
func TestNewApp_RedisRelayWiring(t *testing.T) {
	mr := miniredis.RunT(t)
	path := filepath.Join(t.TempDir(), "relay.yaml")
	if err := writeFile(path, "Listen: \"127.0.0.1:0\"\nJwtSecret: s\nRedisAddr: \""+mr.Addr()+"\"\n"); err != nil {
		t.Fatalf("write: %v", err)
	}
	a, err := newApp(path)
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}
	if a.relay == nil {
		t.Fatal("relay not wired")
	}
	a.stop()
}
