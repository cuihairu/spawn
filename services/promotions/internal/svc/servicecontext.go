package svc

import (
	"fmt"

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

	return &ServiceContext{
		Config:     c,
		Auth:       utils.NewAuth(c.Auth.JWTSecret),
		ActivityDB: activityDB,
		RewardDB:   rewardDB,
		GiftPackDB: giftPackDB,
		Notify:     notify.New(c.Notify.CommunityUrl, c.Notify.CatalogUrl, c.Notify.Token),
	}
}
