// realtime-hub：统一实时推送通道（WebSocket）。见 docs/realtime-hub.md。
//
// 刻意采用 stdlib net/http 而非 go-zero rest：WS upgrade 需要裸 Hijack，
// 避开框架 ResponseWriter 包装层的风险。
package main

import (
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/tappi/tappi/services/realtime-hub/internal/auth"
	"github.com/tappi/tappi/services/realtime-hub/internal/config"
	"github.com/tappi/tappi/services/realtime-hub/internal/handler"
	"github.com/tappi/tappi/services/realtime-hub/internal/hub"
)

var configFile = flag.String("f", "etc/realtime-hub.yaml", "the config file")

func main() {
	flag.Parse()
	if err := run(*configFile); err != nil {
		fmt.Fprintf(os.Stderr, "realtime-hub exited: %v\n", err)
		os.Exit(1)
	}
}

// run 装配并启动服务；配置加载失败以错误返回（主进程以非零退出码结束）。
// 不设 server-level WriteTimeout——WS 为长连接，超时控制由 hub 的
// ping/write deadline 负责。
func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("load config %s: %w", configPath, err)
	}

	h := hub.New(hub.Options{
		QueueSize:  cfg.SendQueueSize,
		WriteWait:  cfg.WriteTimeout(),
		PingPeriod: cfg.PingInterval(),
	})
	if cfg.RedisAddr != "" {
		relay, err := hub.NewRedisRelay(cfg.RedisAddr, cfg.RedisPassword, hub.RedisChannel, h.DeliverLocal)
		if err != nil {
			return fmt.Errorf("redis relay: %w", err)
		}
		defer relay.Close()
		h.SetRelay(relay)
	}
	if cfg.PushWebhook != "" {
		h.SetNotifier(hub.NewWebhookNotifier(cfg.PushWebhook, cfg.PushToken))
	}
	srv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler.NewServer(h, auth.NewVerifier(cfg.JwtSecret), cfg.InternalToken),
		ReadHeaderTimeout: 5 * time.Second,
	}

	ln, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		return fmt.Errorf("listen %s: %w", cfg.Listen, err)
	}
	return srv.Serve(ln)
}
