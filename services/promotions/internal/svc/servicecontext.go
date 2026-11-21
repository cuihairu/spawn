package svc

import (
	"fmt"
	"time"

	"github.com/tappi/tappi/services/promotions/internal/analytics"
	"github.com/tappi/tappi/services/promotions/internal/config"
	"github.com/tappi/tappi/services/promotions/internal/notify"
	"github.com/tappi/tappi/services/promotions/model"
	"github.com/tappi/tappi/services/promotions/utils"

	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config     config.Config
	Auth       *utils.Auth
	ActivityDB *model.ActivityModel
	RewardDB   *model.RewardGrantModel
	GiftPackDB *model.GiftPackModel
	Notify     *notify.Client
	// AnalyticsSvc GP-402 经营分析事实投递口（data-panel /internal/events）；
	// DATA_PANEL_URL 未配置时为 no-op。
	AnalyticsSvc analytics.Client
	// AnalyticsRelay GP-402 outbox 重投循环；nil = DATA_PANEL_URL 未配置
	//（no-op，不启动扫描）。
	AnalyticsRelay *analytics.Relay
	// AnalyticsOutbox GP-402 事实事件 outbox（始终建表，未启用时不写行）。
	AnalyticsOutbox *model.AnalyticsOutboxModel
}

func NewServiceContext(c config.Config) *ServiceContext {
	if err := utils.ValidateJWTSecret(c.Auth.JWTSecret); err != nil {
		panic(err)
	}
	if c.Auth.JWTSecret == utils.WellKnownDevJWTSecret {
		logx.Error("WARN: JWT_SECRET 使用内置演示值，生产必须用环境变量覆盖")
	}

	db, err := model.Open(c.MySQL.DataSource)
	if err != nil {
		panic(fmt.Sprintf("open db: %v", err))
	}
	activityDB := model.NewActivityModel(db)
	if err := activityDB.CreateTables(); err != nil {
		panic(fmt.Sprintf("create activity tables: %v", err))
	}
	rewardDB := model.NewRewardGrantModel(db)
	if err := rewardDB.CreateTables(); err != nil {
		panic(fmt.Sprintf("create reward grant tables: %v", err))
	}
	giftPackDB := model.NewGiftPackModel(db)
	if err := giftPackDB.CreateTables(); err != nil {
		panic(fmt.Sprintf("create gift pack tables: %v", err))
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

	return &ServiceContext{
		Config:          c,
		Auth:            utils.NewAuth(c.Auth.JWTSecret),
		ActivityDB:      activityDB,
		RewardDB:        rewardDB,
		GiftPackDB:      giftPackDB,
		Notify:          notify.New(c.Notify.CommunityUrl, c.Notify.CatalogUrl, c.Notify.Token),
		AnalyticsSvc:    analyticsClient,
		AnalyticsRelay:  analyticsRelay,
		AnalyticsOutbox: analyticsOutbox,
	}
}
