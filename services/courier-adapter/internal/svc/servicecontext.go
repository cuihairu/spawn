package svc

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/tappi/tappi/services/courier-adapter/courier"
	"github.com/tappi/tappi/services/courier-adapter/internal/config"
	"github.com/tappi/tappi/services/courier-adapter/internal/playeract"
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
	// ActivityReport GP-303 回流判定源上报（游戏会话签发成功 = 账号×游戏
	// 活跃事实 → promotions）；nil = PROMOTIONS_URL 未配置（no-op）。
	ActivityReport *playeract.Reporter
	// ActivityRelay GP-303 活跃上报 outbox 重投循环；nil = 未配置（不启动）。
	ActivityRelay *playeract.Relay
	// ActivityOutbox GP-303 活跃上报 outbox（始终建表，未启用时不写行）。
	ActivityOutbox *model.ActivityOutboxModel
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

	// GP-303 活跃上报 outbox（始终建表，后续部署接促销服务时免迁移；未启用
	// 不写行，Relay 返回 nil 不启动扫描）。
	activityOutbox := model.NewActivityOutboxModel(db)
	if err := activityOutbox.CreateTables(); err != nil {
		panic(fmt.Sprintf("建活跃上报 outbox 表失败: %v", err))
	}
	activityReporter := playeract.NewReporter(c.Promotions.BaseURL, c.Promotions.InternalToken, c.Promotions.Timeout, activityOutbox)
	activityRelay := playeract.NewRelay(activityOutbox, playeract.NewClient(c.Promotions.BaseURL, c.Promotions.InternalToken, c.Promotions.Timeout),
		time.Second*time.Duration(c.Promotions.OutboxIntervalSeconds),
		int(c.Promotions.OutboxMaxAttempts),
		time.Hour*time.Duration(c.Promotions.OutboxRetentionHours))

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
		// TTLSeconds 是秒（配置注释同口径）——曾误走 msDuration 把 120s
		// 当 120ms，票据出票即过期、兑票恒 404（2026-11-26 活走查实测）。
		Tickets:        ticket.NewStore(time.Second * time.Duration(c.Ticket.TTLSeconds)),
		DB:             db,
		ActivityReport: activityReporter,
		ActivityRelay:  activityRelay,
		ActivityOutbox: activityOutbox,
	}
}

func msDuration(ms int64) time.Duration {
	if ms <= 0 {
		ms = 5000
	}
	return time.Millisecond * time.Duration(ms)
}
