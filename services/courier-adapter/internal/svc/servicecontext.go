package svc

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/tappi/tappi/services/courier-adapter/courier"
	"github.com/tappi/tappi/services/courier-adapter/internal/config"
	"github.com/tappi/tappi/services/courier-adapter/internal/ticket"
	"github.com/tappi/tappi/services/courier-adapter/model"
	"github.com/tappi/tappi/services/courier-adapter/utils"

	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config   config.Config
	Auth     *utils.Auth
	Courier  *courier.Client
	Mappings *model.MappingModel
	Bindings *model.BindingModel
	Audits   *model.AuditModel
	Tickets  *ticket.Store
	DB       *sql.DB
}

func NewServiceContext(c config.Config) *ServiceContext {
	// 启动期守卫（GP-004 口径）：空密钥拒绝启动；内置演示值大声告警。
	if err := utils.ValidateJWTSecret(c.Auth.JWTSecret); err != nil {
		panic(err.Error())
	}
	if c.Auth.JWTSecret == utils.WellKnownDevJWTSecret {
		logx.Errorf("正在使用内置演示 JWT 密钥：除本地开发外必须以 JWT_SECRET 环境变量覆盖，否则任何人可伪造登录令牌")
	}
	if err := utils.ValidateCredentialKey(c.Credential.Key); err != nil {
		panic(err.Error())
	}
	if c.Credential.Key == utils.WellKnownDevCredentialKey {
		logx.Errorf("正在使用内置演示凭据加密密钥：除本地开发外必须以 CREDENTIAL_KEY 环境变量覆盖")
	}

	db, err := model.Open(c.MySQL.DataSource)
	if err != nil {
		panic(fmt.Sprintf("连接数据库失败: %v", err))
	}
	if err := db.Ping(); err != nil {
		panic(fmt.Sprintf("数据库连接测试失败: %v", err))
	}
	if err := model.CreateTables(db); err != nil {
		panic(fmt.Sprintf("建表失败: %v", err))
	}

	courierClient, err := courier.NewClient(c.Courier.BaseURL, msDuration(c.Courier.Timeout))
	if err != nil {
		panic(fmt.Sprintf("初始化 Courier 客户端失败: %v", err))
	}

	return &ServiceContext{
		Config:   c,
		Auth:     utils.NewAuth(c.Auth.JWTSecret),
		Courier:  courierClient,
		Mappings: model.NewMappingModel(db),
		Bindings: model.NewBindingModel(db),
		Audits:   model.NewAuditModel(db),
		Tickets:  ticket.NewStore(msDuration(c.Ticket.TTLSeconds)),
		DB:       db,
	}
}

func msDuration(ms int64) time.Duration {
	if ms <= 0 {
		ms = 5000
	}
	return time.Millisecond * time.Duration(ms)
}
