package logic

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/tappi/tappi/services/payments/internal/analytics"
	"github.com/tappi/tappi/services/payments/internal/channel"
	"github.com/tappi/tappi/services/payments/internal/promotion"
	"github.com/tappi/tappi/services/payments/internal/svc"

	"github.com/zeromicro/go-zero/core/logx"
)

// CallbackLogic 渠道 S2S 回调：验签先于 body 解析（payment.md 安全边界 2）。
//
// 沙箱：X-Payment-Signature = HMAC-SHA256(sandboxSecret, rawBody)，与 Courier
// teller 沙箱共享密钥，验签通过后原样转发（teller 再验一遍）。
// 真实渠道：实现 ChannelAdapter 用原生验签（平台证书/RSA2），随后以共享
// Courier 渠道密钥构造 HMAC 转发；未接入时返回 501，**不拿沙箱 HMAC 冒充**。
type CallbackLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewCallbackLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CallbackLogic {
	return &CallbackLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

// Handle 处理渠道回调；channelName 选择适配器（sandbox / real）。
func (l *CallbackLogic) Handle(channelName string, rawBody []byte, signature string, meta map[string]string) error {
	var adapter channel.ChannelAdapter
	switch channelName {
	case "sandbox":
		adapter = l.svcCtx.Sandbox
	default:
		adapter = l.svcCtx.Real
	}
	if err := adapter.VerifyNativeSignature(rawBody, signature, meta); err != nil {
		l.Errorf("channel %s verify signature failed: %v", channelName, err)
		if errors.Is(err, channel.ErrRealChannelNotConfigured) {
			return newAPIError(http.StatusNotImplemented, "真实渠道未接入")
		}
		return newAPIError(http.StatusForbidden, "回调签名无效")
	}
	// 构造 Courier 期望的 HMAC（沙箱 = 传入签名本身；真实渠道用共享密钥重算）
	expected := l.hmacCourier(rawBody)
	// 转发 Courier 回调（scope 由订单归属在 Courier 侧解析，此处无需 header）
	if err := l.svcCtx.Courier.ForwardCallback(rawBody, expected); err != nil {
		l.Errorf("forward callback to courier failed: %v", err)
		return newAPIError(http.StatusBadGateway, "转发支付回调失败")
	}
	// 订单投影 PAID（事实副本；状态值来自 Courier 接受回调后，非本服务推进）
	l.projectPaid(rawBody)
	// GP-402 经营分析：order_paid 事实事件（event_id 由订单号派生，重放幂等）
	l.emitOrderPaid(rawBody)
	// GP-305 券核销：订单关联的 locked 券 → used（幂等，无关联即 no-op）
	l.redeemOrderCoupons(rawBody)
	// GP-302 活动奖励评估入队（促销服务未部署时行为不变）
	l.enqueueRewardEvaluation(rawBody, channelName)
	return nil
}

// redeemOrderCoupons 从回调体解析 orderId 并核销关联券（locked → used）。
// 解析失败仅记日志，不影响回调结果（券仍锁于订单，可经退款返还回收）。
func (l *CallbackLogic) redeemOrderCoupons(rawBody []byte) {
	var body struct {
		OrderId string `json:"orderId"`
	}
	if err := json.Unmarshal(rawBody, &body); err != nil || body.OrderId == "" {
		return
	}
	if err := NewCouponLogic(l.ctx, l.svcCtx).RedeemOrderCoupons(body.OrderId); err != nil {
		l.Errorf("redeem coupons for order=%s failed: %v", body.OrderId, err)
	}
}

// projectPaid 从回调体解析 orderId 并投影 PAID（解析失败仅记日志，不影响
// 回调结果——回调已幂等接受）。
func (l *CallbackLogic) projectPaid(rawBody []byte) {
	var body struct {
		OrderId string `json:"orderId"`
		PaidAt  string `json:"paidAt"`
	}
	if err := json.Unmarshal(rawBody, &body); err != nil || body.OrderId == "" {
		l.Errorf("parse callback for projection failed: %s", rawBody)
		return
	}
	if err := l.svcCtx.ProjectionDB.MarkPaid(body.OrderId, body.PaidAt); err != nil {
		l.Errorf("project paid order=%s failed: %v", body.OrderId, err)
	}
}

// emitOrderPaid GP-402/F24：PAID 后向数据面板投 order_paid 事实事件。
// 金额/游戏/账号一律取订单投影（交易事实），不由回调体携带；event_id 由
// 订单号派生（"order_paid:<orderId>"），回调重放时数据面板按 UNIQUE 忽略。
// 失败仅记日志（fire-and-forget，数据面板未部署时 Enabled=false 不拼装）。
func (l *CallbackLogic) emitOrderPaid(rawBody []byte) {
	if !l.svcCtx.AnalyticsSvc.Enabled() {
		return
	}
	var body struct {
		OrderId string `json:"orderId"`
	}
	if err := json.Unmarshal(rawBody, &body); err != nil || body.OrderId == "" {
		return
	}
	proj, err := l.svcCtx.ProjectionDB.Get(body.OrderId)
	if err != nil {
		l.Errorf("load projection for analytics order=%s failed: %v", body.OrderId, err)
		return
	}
	l.svcCtx.AnalyticsSvc.Emit(analytics.Event{
		EventId:     "order_paid:" + proj.OrderId,
		EventType:   analytics.EventOrderPaid,
		UserId:      analytics.UserIDOf(proj.AccountId),
		GameId:      proj.GameId,
		PartnerId:   proj.PartnerId,
		OrderId:     proj.OrderId,
		AmountCents: proj.AmountCents,
		OccurredAt:  proj.PaidAt,
	})
}

// enqueueRewardEvaluation GP-302：PAID 后按投影+发货账本事实入队（基础量
// 为下单时服务端商品固化值）。失败仅记日志（队列扫描会兜底重投；未入队
// 的极端场景由投影/账本事实可查，不阻塞回调）。
func (l *CallbackLogic) enqueueRewardEvaluation(rawBody []byte, channelName string) {
	if !l.svcCtx.PromotionBridge.Enabled() {
		return
	}
	var body struct {
		OrderId string `json:"orderId"`
		PaidAt  string `json:"paidAt"`
	}
	if err := json.Unmarshal(rawBody, &body); err != nil || body.OrderId == "" {
		return
	}
	proj, err := l.svcCtx.ProjectionDB.Get(body.OrderId)
	if err != nil {
		l.Errorf("load projection for reward eval order=%s failed: %v", body.OrderId, err)
		return
	}
	serverID, roleID := int64(0), ""
	if d, err := l.svcCtx.DeliveryDB.Get(body.OrderId); err == nil {
		serverID, roleID = d.ServerId, d.RoleId
	}
	facts := promotion.Facts{
		OrderId:      proj.OrderId,
		GameId:       proj.GameId,
		ServerId:     serverID,
		RoleId:       roleID,
		AccountId:    proj.AccountId,
		SkuId:        proj.SkuId,
		ProductId:    proj.ProductId,
		Channel:      channelName,
		PaidCents:    proj.AmountCents,
		Currency:     proj.Currency,
		PaidAt:       proj.PaidAt,
		BaseQuantity: proj.BaseQuantity,
	}
	// GP-303 首充/累充事实：投影口径计算（首充=同账号同游戏无更早
	// PAID/DELIVERED 订单；累充净额=PAID/DELIVERED 实付 − EXECUTED 退款）。
	// 投影查询失败即 fail-closed 不入队（不推送错误事实；记日志可查）。
	firstPurchase, err := l.svcCtx.ProjectionDB.HasEarlierPaidOrder(proj.AccountId, proj.GameId, proj.OrderId)
	if err != nil {
		l.Errorf("first purchase fact order=%s failed: %v", proj.OrderId, err)
		return
	}
	facts.FirstPurchase = !firstPurchase
	cumulative, err := l.svcCtx.ProjectionDB.AccountGameNetCents(proj.AccountId, proj.GameId)
	if err != nil {
		l.Errorf("cumulative fact order=%s failed: %v", proj.OrderId, err)
		return
	}
	facts.CumulativeCents = cumulative
	if err := l.svcCtx.PromotionBridge.Enqueue(body.OrderId, channelName, facts); err != nil {
		l.Errorf("enqueue reward eval order=%s failed: %v", body.OrderId, err)
	}
}

// hmacCourier 计算 Courier 渠道密钥的 HMAC-SHA256（沙箱共享密钥）。
func (l *CallbackLogic) hmacCourier(rawBody []byte) string {
	mac := hmac.New(sha256.New, []byte(l.svcCtx.Config.Channel.SandboxSecret))
	mac.Write(rawBody)
	return hex.EncodeToString(mac.Sum(nil))
}
