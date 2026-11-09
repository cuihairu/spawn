package svc

import (
	"fmt"
	"time"

	"github.com/tappi/tappi/services/payments/courier"
	"github.com/tappi/tappi/services/payments/internal/channel"
	"github.com/tappi/tappi/services/payments/internal/config"
	"github.com/tappi/tappi/services/payments/model"
	"github.com/tappi/tappi/services/payments/utils"

	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config    config.Config
	Auth      *utils.Auth
	ProductDB *model.ProductModel
	Courier   *courier.Client
	Sandbox   *channel.SandboxHMAC
	Real      *channel.RealChannel
}

func NewServiceContext(c config.Config) *ServiceContext {
	// 生产密钥守护：空密钥 panic，内置演示值大声告警（F28 生产接入要求）
	if err := utils.ValidateJWTSecret(c.Auth.JWTSecret); err != nil {
		panic(err)
	}
	if c.Auth.JWTSecret == utils.WellKnownDevJWTSecret {
		logx.Error("WARN: JWT_SECRET 使用内置演示值，生产必须用环境变量覆盖")
	}
	if c.Channel.SandboxSecret == "" {
		c.Channel.SandboxSecret = utils.WellKnownSandboxSecret
	}
	if c.Channel.SandboxSecret == utils.WellKnownSandboxSecret {
		logx.Error("WARN: 沙箱渠道 HMAC 使用内置演示密钥，仅限开发/沙箱")
	}

	db, err := model.Open(c.MySQL.DataSource)
	if err != nil {
		panic(fmt.Sprintf("open db: %v", err))
	}
	productDB := model.NewProductModel(db)
	if err := productDB.CreateTables(); err != nil {
		panic(fmt.Sprintf("create tables: %v", err))
	}

	timeout := time.Millisecond * time.Duration(c.Courier.Timeout)
	courierClient := courier.NewClient(c.Courier.BaseURL, timeout)

	return &ServiceContext{
		Config:    c,
		Auth:      utils.NewAuth(c.Auth.JWTSecret),
		ProductDB: productDB,
		Courier:   courierClient,
		Sandbox:   &channel.SandboxHMAC{Secret: []byte(c.Channel.SandboxSecret)},
		Real:      channel.NewRealChannel("real-channel"),
	}
}
