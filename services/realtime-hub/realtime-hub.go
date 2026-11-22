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

// app 装配完成的推送通道：hub 与 HTTP 服务器配对（relay 关闭随 app 生命周期）。
type app struct {
	srv   *http.Server
	relay hub.Relay
}

func (a *app) stop() {
	if a.relay != nil {
		_ = a.relay.Close()
	}
}

// newApp 装配 hub（可选 Redis 跨实例 relay）与 HTTP 服务器；配置加载失败或
// Redis 连接失败以错误返回，不做静默半可用实例。
func newApp(configPath string) (*app, error) {
	cfg, err := config.Load(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config %s: %w", configPath, err)
	}

	h := hub.New(hub.Options{
		QueueSize:  cfg.SendQueueSize,
		WriteWait:  cfg.WriteTimeout(),
		PingPeriod: cfg.PingInterval(),
	})
	a := &app{}
	if cfg.RedisAddr != "" {
		relay, err := hub.NewRedisRelay(cfg.RedisAddr, cfg.RedisPassword, hub.RedisChannel, h.DeliverLocal)
		if err != nil {
			return nil, fmt.Errorf("redis relay: %w", err)
		}
		a.relay = relay
		h.SetRelay(relay)
	}
	if cfg.PushWebhook != "" {
		h.SetNotifier(hub.NewWebhookNotifier(cfg.PushWebhook, cfg.PushToken))
	}
	a.srv = &http.Server{
		Addr:              cfg.Listen,
		Handler:           handler.NewServer(h, auth.NewVerifier(cfg.JwtSecret), cfg.InternalToken),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return a, nil
}

// run 装配并启动服务；服务器启动后阻塞直至进程退出或出错。
// 不设 server-level WriteTimeout——WS 为长连接，超时控制由 hub 的
// ping/write deadline 负责。
func run(configPath string) error {
	a, err := newApp(configPath)
	if err != nil {
		return err
	}
	defer a.stop()

	ln, err := net.Listen("tcp", a.srv.Addr)
	if err != nil {
		return fmt.Errorf("listen %s: %w", a.srv.Addr, err)
	}
	return a.srv.Serve(ln)
}
