// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package svc

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/tappi/tappi/services/user-service/internal/analytics"
	"github.com/tappi/tappi/services/user-service/internal/config"
	"github.com/tappi/tappi/services/user-service/internal/integration"
	"github.com/tappi/tappi/services/user-service/internal/promoattr"
	"github.com/tappi/tappi/services/user-service/model"
	"github.com/tappi/tappi/services/user-service/utils"
	"github.com/zeromicro/go-zero/core/logx"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

type ServiceContext struct {
	Config            config.Config
	DB                *sql.DB
	UserModel         model.UserStore
	Auth              *utils.Auth
	GameCatalogClient *integration.GameCatalogClient
	// AnalyticsSvc GP-402 经营分析事实投递口（data-panel /internal/events）；
	// DATA_PANEL_URL 未配置时为 no-op。
	AnalyticsSvc analytics.Client
	// AnalyticsRelay GP-402 outbox 重投循环；nil = DATA_PANEL_URL 未配置
	//（no-op，不启动扫描）。
	AnalyticsRelay *analytics.Relay
	// AnalyticsOutbox GP-402 事实事件 outbox（始终建表，未启用时不写行）。
	AnalyticsOutbox *model.AnalyticsOutboxModel
	// PromoAttrRelay GP-403 渠道归因 outbox 重投循环。
	PromoAttrRelay *promoattr.Relay
	// PromoAttrOutbox GP-403 渠道归因意图 outbox（始终建表，无 click_id 时不写行）。
	PromoAttrOutbox *model.PromoAttrOutboxModel
}

func NewServiceContext(c config.Config) *ServiceContext {
	// JWT 密钥启动期守卫：空密钥拒绝启动；内置演示密钥大声告警
	// （该密钥源码可见，可被用来伪造任意用户的令牌）。
	if err := utils.ValidateJWTSecret(c.Auth.JWTSecret); err != nil {
		panic(err.Error())
	}
	if c.Auth.JWTSecret == utils.WellKnownDevJWTSecret {
		logx.Errorf("正在使用内置演示 JWT 密钥：除本地开发外必须以 JWT_SECRET 环境变量覆盖，否则任何人可伪造登录令牌")
	}

	// 检测数据库类型并连接
	var db *sql.DB
	var err error

	dataSource := c.MySQL.DataSource
	if strings.Contains(dataSource, "file:") || strings.Contains(dataSource, ".db") {
		// SQLite 数据库
		db, err = sql.Open("sqlite3", dataSource)
	} else {
		// MySQL 数据库
		db, err = sql.Open("mysql", dataSource)
	}

	if err != nil {
		panic(fmt.Sprintf("连接数据库失败: %v", err))
	}

	// 测试数据库连接
	if err := db.Ping(); err != nil {
		panic(fmt.Sprintf("数据库连接测试失败: %v", err))
	}

	// 创建用户模型
	userModel := model.NewUserModel(db)

	// 创建认证工具
	auth := utils.NewAuth(c.Auth.JWTSecret, time.Duration(c.Auth.TokenExpire)*24*time.Hour)

	timeout := time.Duration(c.Services.GameCatalog.Timeout) * time.Millisecond
	gameClient := integration.NewGameCatalogClient(c.Services.GameCatalog.BaseURL, timeout)
	// GP-403 渠道归因 S2S 令牌（空 = 点击解析按未授权处理，归因退化为自然流量）
	if c.Services.GameCatalog.InternalToken != "" {
		gameClient.SetInternalToken(c.Services.GameCatalog.InternalToken)
	}

	// 创建用户表（开发环境）
	if err := userModel.CreateUsersTable(); err != nil {
		panic(fmt.Sprintf("创建用户表失败: %v", err))
	}

	// GP-402 经营分析事实投递（DATA_PANEL_URL 未配置 = no-op）：失败先落
	// outbox（幂等持久化），由 Relay 定时重投；outbox 表始终建（后续部署
	// 补数据面板时免迁移），未启用时不写行。
	analyticsOutbox := model.NewAnalyticsOutboxModel(db)
	if err := analyticsOutbox.CreateTables(); err != nil {
		panic(fmt.Sprintf("create analytics outbox tables: %v", err))
	}
	analyticsClient := analytics.NewClient(c.Analytics.BaseURL, c.Analytics.Token, c.Analytics.Timeout,
		analyticsOutbox)
	analyticsRelay := analytics.NewRelay(analyticsOutbox, analyticsClient,
		time.Second*time.Duration(c.Analytics.OutboxIntervalSeconds),
		int(c.Analytics.OutboxMaxAttempts),
		time.Hour*time.Duration(c.Analytics.OutboxRetentionHours))

	// GP-403 渠道归因补偿：归因意图先落 outbox，game-catalog 短时不可达时
	// 失败留 pending 由 Relay 重投（重投锚定原始 registered_at 判定、回写
	// user_id 幂等）；outbox 表始终建，注册不带 click_id 时不写行。
	promoAttrOutbox := model.NewPromoAttrOutboxModel(db)
	if err := promoAttrOutbox.CreateTables(); err != nil {
		panic(fmt.Sprintf("create promo attr outbox tables: %v", err))
	}
	promoAttrRelay := promoattr.NewRelay(promoAttrOutbox, gameClient,
		time.Second*time.Duration(c.PromoAttr.OutboxIntervalSeconds),
		int(c.PromoAttr.OutboxMaxAttempts),
		time.Hour*time.Duration(c.PromoAttr.OutboxRetentionHours))

	return &ServiceContext{
		Config:            c,
		DB:                db,
		UserModel:         userModel,
		Auth:              auth,
		GameCatalogClient: gameClient,
		AnalyticsSvc:      analyticsClient,
		AnalyticsRelay:    analyticsRelay,
		AnalyticsOutbox:   analyticsOutbox,
		PromoAttrRelay:    promoAttrRelay,
		PromoAttrOutbox:   promoAttrOutbox,
	}
}
