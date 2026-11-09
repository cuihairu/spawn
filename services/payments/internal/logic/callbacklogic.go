package logic

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"

	"github.com/tappi/tappi/services/payments/internal/channel"
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
	return nil
}

// hmacCourier 计算 Courier 渠道密钥的 HMAC-SHA256（沙箱共享密钥）。
func (l *CallbackLogic) hmacCourier(rawBody []byte) string {
	mac := hmac.New(sha256.New, []byte(l.svcCtx.Config.Channel.SandboxSecret))
	mac.Write(rawBody)
	return hex.EncodeToString(mac.Sum(nil))
}
