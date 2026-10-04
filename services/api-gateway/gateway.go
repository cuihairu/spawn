// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tappi/tappi/services/api-gateway/internal/community"
	"github.com/tappi/tappi/services/api-gateway/internal/config"
	"github.com/tappi/tappi/services/api-gateway/internal/content"
	"github.com/tappi/tappi/services/api-gateway/internal/games"
	"github.com/tappi/tappi/services/api-gateway/internal/handler"
	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/tappi/tappi/services/api-gateway/internal/users"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
)

var configFile = flag.String("f", "etc/gateway-api.yaml", "the config file")

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "gateway exited: %v\n", err)
		os.Exit(1)
	}
}

// run 装配并启动网关；配置加载失败以错误返回（替代原先的 MustLoad panic），
// 使 main 能以非零退出码结束。服务器启动后阻塞直至进程退出。
func run() error {
	flag.Parse()

	var c config.Config
	if err := conf.Load(*configFile, &c); err != nil {
		return fmt.Errorf("load config %s: %w", *configFile, err)
	}

	server := rest.MustNewServer(c.RestConf, rest.WithCors("*"))
	defer server.Stop()

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)
	content.RegisterContentProxyRoutes(server, ctx)
	community.RegisterCommunityProxyRoutes(server, ctx)
	users.RegisterUserProxyRoutes(server, ctx)
	games.RegisterGameProxyRoutes(server, ctx)

	fmt.Printf("Starting server at %s:%d...\n", c.Host, c.Port)
	server.Start()
	return nil
}
