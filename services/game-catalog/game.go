// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package main

import (
	"flag"
	"fmt"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
	"github.com/tappi/tappi/services/game-catalog/internal/handler"
	"github.com/tappi/tappi/services/game-catalog/internal/logic"
	"github.com/tappi/tappi/services/game-catalog/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/rest"
	"github.com/zeromicro/go-zero/rest/httpx"
)

var configFile = flag.String("f", "etc/game-api.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c)

	// logic 层 apiError 的状态码（404/500）渲染为真实 HTTP 状态
	httpx.SetErrorHandlerCtx(logic.ErrorHandler)

	server := rest.MustNewServer(c.RestConf)
	defer server.Stop()

	ctx := svc.NewServiceContext(c)
	handler.RegisterHandlers(server, ctx)

	fmt.Printf("Starting server at %s:%d...\n", c.Host, c.Port)
	server.Start()
}
