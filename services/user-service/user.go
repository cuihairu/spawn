// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/tappi/tappi/services/user-service/internal/config"
	"github.com/tappi/tappi/services/user-service/internal/handler"
	"github.com/tappi/tappi/services/user-service/internal/httperr"
	"github.com/tappi/tappi/services/user-service/internal/svc"
	"github.com/tappi/tappi/services/user-service/middleware"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

var configFile = flag.String("f", "etc/user-api.yaml", "the config file")

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "user-service exited: %v\n", err)
		os.Exit(1)
	}
}

// run 装配并启动服务；配置加载失败以错误返回（替代原先的 MustLoad panic），
// 使 main 能以非零退出码结束。服务器启动后阻塞直至进程退出。
func run() error {
	flag.Parse()

	var c config.Config
	if err := conf.Load(*configFile, &c); err != nil {
		return fmt.Errorf("load config %s: %w", *configFile, err)
	}

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	// 错误信封映射：认证失败等按携带的 HTTP 状态回信封（与其余服务同款）
	httpx.SetErrorHandlerCtx(httperr.ErrorHandler)

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)

	// 添加 JWT 认证中间件
	server.Use(func(next http.HandlerFunc) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			middleware.AuthMiddleware(ctx)(next).ServeHTTP(w, r)
		}
	})

	// GP-402 经营分析 outbox 重投：data-panel 短时不可达时补偿投递（本地
	// 持久化，重启续扫），ctx 取进程信号面；DATA_PANEL_URL 未配置时
	// NewRelay 返回 nil，不启动。
	bgCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if ctx.AnalyticsRelay != nil {
		go ctx.AnalyticsRelay.Run(bgCtx)
	}

	// GP-403 渠道归因 outbox 重投：game-catalog 短时不可达时补偿解析（本地
	// 持久化，重启续扫），ctx 取进程信号面；客户端 nil 时 NewRelay 返回 nil，
	// 不启动。
	if ctx.PromoAttrRelay != nil {
		go ctx.PromoAttrRelay.Run(bgCtx)
	}

	fmt.Printf("Starting server at %s:%d...\n", c.Host, c.Port)
	server.Start()
	return nil
}
