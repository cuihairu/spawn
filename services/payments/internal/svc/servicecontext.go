package svc

import (
	"fmt"
	"time"

	"github.com/tappi/tappi/services/payments/courier"
	"github.com/tappi/tappi/services/payments/internal/analytics"
	"github.com/tappi/tappi/services/payments/internal/channel"
	"github.com/tappi/tappi/services/payments/internal/config"
	"github.com/tappi/tappi/services/payments/internal/delivery"
	"github.com/tappi/tappi/services/payments/internal/promotion"
	"github.com/tappi/tappi/services/payments/model"
	"github.com/tappi/tappi/services/payments/utils"

	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config          config.Config
	Auth            *utils.Auth
	ProductDB       *model.ProductModel
	DeliveryDB      *model.DeliveryModel
	ProjectionDB    *model.OrderProjectionModel
	StatementDB     *model.StatementModel
	EvalDB          *model.PromotionEvalModel
	CouponDB        *model.CouponModel
	Courier         *courier.Client
	Sandbox         *channel.SandboxHMAC
	Real            *channel.RealChannel
	Dispatcher      *delivery.Dispatcher
	PromotionBridge *promotion.Bridge
	AnalyticsSvc    analytics.Client
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
	if c.Delivery.Secret == "" {
		// fail-closed：不注入演示值，发货分发全部 403（生产必须显式配置）
		logx.Error("WARN: PAYMENTS_DELIVERY_SECRET 未配置，发货分发 fail-closed（全部 403）")
	}

	db, err := model.Open(c.MySQL.DataSource)
	if err != nil {
		panic(fmt.Sprintf("open db: %v", err))
	}
	productDB := model.NewProductModel(db)
	if err := productDB.CreateTables(); err != nil {
		panic(fmt.Sprintf("create tables: %v", err))
	}
	deliveryDB := model.NewDeliveryModel(db)
	if err := deliveryDB.CreateTables(); err != nil {
		panic(fmt.Sprintf("create delivery tables: %v", err))
	}
	projectionDB := model.NewOrderProjectionModel(db)
	if err := projectionDB.CreateTables(); err != nil {
		panic(fmt.Sprintf("create order projection tables: %v", err))
	}
	statementDB := model.NewStatementModel(db)
	if err := statementDB.CreateTables(); err != nil {
		panic(fmt.Sprintf("create statement tables: %v", err))
	}
	evalDB := model.NewPromotionEvalModel(db)
	if err := evalDB.CreateTables(); err != nil {
		panic(fmt.Sprintf("create promotion eval tables: %v", err))
	}
	couponDB := model.NewCouponModel(db)
	if err := couponDB.CreateTables(); err != nil {
		panic(fmt.Sprintf("create coupon tables: %v", err))
	}
	// GP-302 奖励评估桥（PROMOTIONS_URL 未配置 = 不入队不分发）
	bridge := promotion.NewBridge(evalDB, c.Promotions.BaseURL, c.Promotions.Secret,
		c.Promotions.Timeout)
	// GP-402 经营分析事实投递（DATA_PANEL_URL 未配置 = no-op）
	analyticsClient := analytics.NewClient(c.Analytics.BaseURL, c.Analytics.Token, c.Analytics.Timeout)

	timeout := time.Millisecond * time.Duration(c.Courier.Timeout)
	courierClient := courier.NewClient(c.Courier.BaseURL, timeout)
	// 投递成功 → 订单投影 DELIVERED（只读事实副本，状态值来自投递面）
	dispatcher := delivery.NewDispatcher(deliveryDB, productDB, c.Delivery.TimeoutMs,
		time.Second*time.Duration(c.Delivery.RetryIntervalSeconds),
		func(orderID string) { _ = projectionDB.MarkDelivered(orderID, time.Now().UTC().Format(time.RFC3339)) })

	return &ServiceContext{
		Config:          c,
		Auth:            utils.NewAuth(c.Auth.JWTSecret),
		ProductDB:       productDB,
		DeliveryDB:      deliveryDB,
		ProjectionDB:    projectionDB,
		StatementDB:     statementDB,
		EvalDB:          evalDB,
		CouponDB:        couponDB,
		Courier:         courierClient,
		Sandbox:         &channel.SandboxHMAC{Secret: []byte(c.Channel.SandboxSecret)},
		Real:            channel.NewRealChannel("real-channel"),
		Dispatcher:      dispatcher,
		PromotionBridge: bridge,
		AnalyticsSvc:    analyticsClient,
	}
}
