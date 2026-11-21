// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
	"github.com/tappi/tappi/services/game-catalog/internal/handler"
	"github.com/tappi/tappi/services/game-catalog/internal/logic"
	"github.com/tappi/tappi/services/game-catalog/internal/openscan"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

var configFile = flag.String("f", "etc/game-api.yaml", "the config file")

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "game-catalog exited: %v\n", err)
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

	// logic 层 apiError 的状态码（404/500）渲染为真实 HTTP 状态
	httpx.SetErrorHandlerCtx(logic.ErrorHandler)

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)

	// GP-306 排期到点自动开服：周期扫描（账号信号面，与 payments 发货恢复
	// 同款），ctx 取消即退出。
	scanCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go openscan.NewRunner(ctx, time.Second*time.Duration(c.OpenScan.IntervalSeconds)).Run(scanCtx)

	// GP-402 经营分析 outbox 重投：data-panel 短时不可达时补偿投递（本地
	// 持久化，重启续扫），ctx 取进程信号面；DATA_PANEL_URL 未配置时
	// NewRelay 返回 nil，不启动。
	if ctx.AnalyticsRelay != nil {
		go ctx.AnalyticsRelay.Run(scanCtx)
	}

	fmt.Printf("Starting server at %s:%d...\n", c.Host, c.Port)
	server.Start()
	return nil
}
