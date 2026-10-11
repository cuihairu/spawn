// Package config 装配 realtime-hub 运行配置：etc yaml 结构项 + 环境变量
// 密钥覆盖（compose 注入口径与各服务一致）。
package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 服务配置。JwtSecret 必填（yaml 或 JWT_SECRET 环境变量）；
// InternalToken 未配置时 /internal/publish 恒 403（默认拒绝）。
type Config struct {
	Listen         string `yaml:"Listen"`
	WriteTimeoutMs int    `yaml:"WriteTimeoutMs"`
	PingIntervalMs int    `yaml:"PingIntervalMs"`
	SendQueueSize  int    `yaml:"SendQueueSize"`
	JwtSecret      string `yaml:"JwtSecret"`
	InternalToken  string `yaml:"InternalToken"`
}

// Load 读取 yaml 并应用默认值与环境变量覆盖。
func Load(path string) (Config, error) {
	var c Config
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config %s: %w", path, err)
	}
	if err := yaml.Unmarshal(b, &c); err != nil {
		return Config{}, fmt.Errorf("parse config %s: %w", path, err)
	}

	if v := os.Getenv("JWT_SECRET"); v != "" {
		c.JwtSecret = v
	}
	if v := os.Getenv("REALTIME_INTERNAL_TOKEN"); v != "" {
		c.InternalToken = v
	}

	if c.Listen == "" {
		c.Listen = ":8893"
	}
	if c.WriteTimeoutMs <= 0 {
		c.WriteTimeoutMs = int((10 * time.Second).Milliseconds())
	}
	if c.PingIntervalMs <= 0 {
		c.PingIntervalMs = int((30 * time.Second).Milliseconds())
	}
	if c.SendQueueSize <= 0 {
		c.SendQueueSize = 16
	}
	if c.JwtSecret == "" {
		return Config{}, errors.New("JwtSecret required (yaml JwtSecret or JWT_SECRET env)")
	}
	return c, nil
}

// WriteTimeout / PingInterval 时长访问器（毫秒配置项的语义出口）。
func (c Config) WriteTimeout() time.Duration {
	return time.Duration(c.WriteTimeoutMs) * time.Millisecond
}
func (c Config) PingInterval() time.Duration {
	return time.Duration(c.PingIntervalMs) * time.Millisecond
}
