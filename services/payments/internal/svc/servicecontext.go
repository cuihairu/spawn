package svc

import (
	"fmt"
	"time"

	"github.com/tappi/tappi/services/payments/courier"
	"github.com/tappi/tappi/services/payments/internal/analytics"
	"github.com/tappi/tappi/services/payments/internal/channel"
	"github.com/tappi/tappi/services/payments/internal/config"
	"github.com/tappi/tappi/services/payments/internal/courierscope"
	"github.com/tappi/tappi/services/payments/internal/delivery"
	"github.com/tappi/tappi/services/payments/internal/notify"
	"github.com/tappi/tappi/services/payments/internal/promotion"
	"github.com/tappi/tappi/services/payments/internal/refundexec"
	"github.com/tappi/tappi/services/payments/model"
	"github.com/tappi/tappi/services/payments/utils"

	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config       config.Config
	Auth         *utils.Auth
	ProductDB    *model.ProductModel
	DeliveryDB   *model.DeliveryModel
	ProjectionDB *model.OrderProjectionModel
	StatementDB  *model.StatementModel
	EvalDB       *model.PromotionEvalModel
	CouponDB     *model.CouponModel
	Courier      *courier.Client
	// ScopeRes GP-202 集成缺口：spawn_game_id → courier scope 解析客户端
	//（courier-adapter /internal/games/:id/mapping，X-Internal-Token）。
	ScopeRes *courierscope.Client
	Sandbox  *channel.SandboxHMAC
	// Real 真实渠道适配注册表（按渠道名选择原生验签适配器，GP-202）。
	Real            *channel.RealAdapter
	Dispatcher      *delivery.Dispatcher
	PromotionBridge *promotion.Bridge
	AnalyticsSvc    analytics.Client
	AnalyticsRelay  *analytics.Relay // nil = DATA_PANEL_URL 未配置（no-op，不启动）
	AnalyticsOutbox *model.AnalyticsOutboxModel
	NotifySvc       notify.Client
	// NotifyRelay GP-404 业务通知 outbox 重投循环；nil = COMMUNITY_URL 未配置
	//（no-op，不启动扫描）。
	NotifyRelay *notify.Relay
	// NotifyOutbox GP-404 业务通知 outbox（始终建表，未启用时不写行）。
	NotifyOutbox *model.NotifyOutboxModel
	// RefundExec GP-204 退款执行器（PENDING → Courier 管理面退款端点 →
	// EXECUTED/REJECTED）；nil = COURIER_PAYMENTS_ADMIN_TOKEN 未配置（no-op，
	// 退款保持 PENDING，不伪造成功）。
	RefundExec *refundexec.Executor
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
	// GP-404 异议复核结果回流合作方（COMMUNITY_URL 未配置 = no-op）：失败先落
	// outbox（幂等持久化），由 Relay 定时重投；outbox 表始终建（后续部署
	// 补 community 时免迁移），未启用时不写行。重投安全由 community 侧
	// UNIQUE(user,actor,type,target) 去重保证。
	notifyOutbox := model.NewNotifyOutboxModel(db)
	if err := notifyOutbox.CreateTables(); err != nil {
		panic(fmt.Sprintf("create notify outbox tables: %v", err))
	}
	notifyClient := notify.NewClient(c.Notify.BaseURL, c.Notify.Token, c.Notify.Timeout, notifyOutbox)
	notifyRelay := notify.NewRelay(notifyOutbox, notifyClient,
		time.Second*time.Duration(c.Notify.OutboxIntervalSeconds),
		int(c.Notify.OutboxMaxAttempts),
		time.Hour*time.Duration(c.Notify.OutboxRetentionHours))

	timeout := time.Millisecond * time.Duration(c.Courier.Timeout)
	courierClient := courier.NewClient(c.Courier.BaseURL, timeout)
	// GP-202 集成缺口：spawn_game_id → courier scope 解析（courier-adapter
	// 内部端点）；BaseURL 未配置 = 订单面 fail-closed（不静默回退 spawn id）。
	scopeClient := courierscope.NewClient(c.CourierAdapter.BaseURL, c.Internal.Token,
		time.Millisecond*time.Duration(c.CourierAdapter.Timeout))
	// 投递成功 → 订单投影 DELIVERED（只读事实副本，状态值来自投递面）
	dispatcher := delivery.NewDispatcher(deliveryDB, productDB, c.Delivery.TimeoutMs,
		time.Second*time.Duration(c.Delivery.RetryIntervalSeconds),
		func(orderID string) { _ = projectionDB.MarkDelivered(orderID, time.Now().UTC().Format(time.RFC3339)) })
	// GP-204 退款执行器（PENDING → Courier 管理面退款端点；AdminToken 空 =
	// nil no-op，退款保持 PENDING 不伪造成功）。
	refundExec := refundexec.NewExecutor(projectionDB, courierClient, scopeClient,
		c.Courier.AdminToken, time.Second*time.Duration(c.Courier.RefundExecIntervalSeconds))

	return &ServiceContext{
		Config:       c,
		Auth:         utils.NewAuth(c.Auth.JWTSecret),
		ProductDB:    productDB,
		DeliveryDB:   deliveryDB,
		ProjectionDB: projectionDB,
		StatementDB:  statementDB,
		EvalDB:       evalDB,
		CouponDB:     couponDB,
		Courier:      courierClient,
		ScopeRes:     scopeClient,
		Sandbox:      &channel.SandboxHMAC{Secret: []byte(c.Channel.SandboxSecret)},
		// 真实渠道适配注册表（GP-202）：已注入验签材料的渠道可原生验签，
		// 未配置/未知渠道回退 501 占位（RealChannel）。
		Real: channel.NewRealAdapter(channel.RealConfig{
			WechatPlatformCert: c.Channel.WechatPlatformCert,
			AlipayPublicKey:    c.Channel.AlipayPublicKey,
		}),
		Dispatcher:      dispatcher,
		PromotionBridge: bridge,
		AnalyticsSvc:    analyticsClient,
		AnalyticsRelay:  analyticsRelay,
		AnalyticsOutbox: analyticsOutbox,
		NotifySvc:       notifyClient,
		NotifyRelay:     notifyRelay,
		NotifyOutbox:    notifyOutbox,
		RefundExec:      refundExec,
	}
}
