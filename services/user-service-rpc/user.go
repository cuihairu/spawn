package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/tappi/tappi/services/user-service-rpc/internal/config"
	"github.com/tappi/tappi/services/user-service-rpc/internal/server"
	"github.com/tappi/tappi/services/user-service-rpc/internal/svc"
	"github.com/tappi/tappi/services/user-service-rpc/user"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/user.yaml", "the config file")

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "user-service-rpc exited: %v\n", err)
		os.Exit(1)
	}
}

// run 装配并启动 zRPC 服务；配置加载失败以错误返回（替代原先的 MustLoad panic），
// 使 main 能以非零退出码结束。与其余服务的 main→run 模式一致。
// 服务器启动后阻塞直至进程退出。
func run() error {
	flag.Parse()

	var c config.Config
	if err := conf.Load(*configFile, &c); err != nil {
		return fmt.Errorf("load config %s: %w", *configFile, err)
	}
	ctx := svc.NewServiceContext(c)

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		user.RegisterUserServer(grpcServer, server.NewUserServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
	return nil
}
