package config

import (
	"os"
	"path/filepath"
	"testing"
)

// TestLoad_MissingFileErrors 配置文件不存在 → 错误返回（主进程非零退出）。
func TestLoad_MissingFileErrors(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatal("missing config must error")
	}
}

// TestLoad_MalformedYamlErrors 非法 yaml → 解析错误。
func TestLoad_MalformedYamlErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(path, []byte("Listen: [unclosed"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := Load(path); err == nil {
		t.Fatal("malformed yaml must error")
	}
}

// TestLoad_MissingSecretErrors 无 JwtSecret（yaml 与环境变量都没有）→ 拒绝启动。
func TestLoad_MissingSecretErrors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nosecret.yaml")
	if err := os.WriteFile(path, []byte("Listen: \":8893\"\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("JWT_SECRET", "")
	if _, err := Load(path); err == nil {
		t.Fatal("missing jwt secret must error")
	}
}

// TestLoad_DefaultsAndEnvOverrides 缺省项补齐、环境变量覆盖 yaml 密钥。
func TestLoad_DefaultsAndEnvOverrides(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	content := "Listen: \"127.0.0.1:0\"\nJwtSecret: yaml-secret\nInternalToken: yaml-token\nRedisAddr: yaml-redis\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	t.Setenv("JWT_SECRET", "env-secret")
	t.Setenv("REALTIME_INTERNAL_TOKEN", "env-token")
	t.Setenv("REALTIME_REDIS_ADDR", "env-redis:6379")
	t.Setenv("REALTIME_PUSH_WEBHOOK", "http://env-bridge/push")
	t.Setenv("REALTIME_PUSH_TOKEN", "env-push-tok")

	c, err := Load(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.JwtSecret != "env-secret" || c.InternalToken != "env-token" {
		t.Fatalf("env override lost: %+v", c)
	}
	if c.RedisAddr != "env-redis:6379" || c.RedisPassword != "" {
		t.Fatalf("redis config = %+v", c)
	}
	if c.PushWebhook != "http://env-bridge/push" || c.PushToken != "env-push-tok" {
		t.Fatalf("push config = %+v", c)
	}
	if c.Listen != "127.0.0.1:0" {
		t.Fatalf("listen = %q, want yaml value kept", c.Listen)
	}
	// 缺省补齐
	if c.SendQueueSize != 16 || c.WriteTimeout() != 10_000_000_000 || c.PingInterval() != 30_000_000_000 {
		t.Fatalf("defaults not applied: %+v", c)
	}
}
