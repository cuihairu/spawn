package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tappi/tappi/services/courier-adapter/internal/config"
	"github.com/tappi/tappi/services/courier-adapter/internal/handler"
	"github.com/tappi/tappi/services/courier-adapter/internal/logic"
	"github.com/tappi/tappi/services/courier-adapter/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

var configFile = flag.String("f", "etc/adapter-api.yaml", "the config file")

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "courier-adapter exited: %v\n", err)
		os.Exit(1)
	}
}

// run 装配并启动服务（与 game-catalog 同款口径）；启动失败以非零码退出。
func run() error {
	flag.Parse()

	var c config.Config
	if err := conf.Load(*configFile, &c); err != nil {
		return fmt.Errorf("load config %s: %w", *configFile, err)
	}

	// logic 层 apiError 的状态码（404/403/502…）渲染为真实 HTTP 状态
	httpx.SetErrorHandlerCtx(logic.ErrorHandler)

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)

	fmt.Printf("Starting server at %s:%d...\n", c.Host, c.Port)
	server.Start()
	return nil
}
