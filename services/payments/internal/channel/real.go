package channel

// 真实渠道适配注册表（GP-202）：按 callback/:channel 渠道名选择已配置的
// 原生验签适配器。未配置/未知渠道回退 RealChannel 占位（恒 501）。
//
// 渠道验签材料均为公开材料（平台证书/公钥）经环境变量注入；注入为空 =
// 该渠道未接入（501），与「真实渠道未接入返回 501，绝不拿沙箱 HMAC 冒充」
// 语义一致（payment.md 安全边界 2）。

import (
	"strings"
)

// RealConfig 真实渠道验签材料配置（来自 env，见 etc/payments-api.yaml）。
type RealConfig struct {
	// WechatPlatformCert 微信 v3 平台证书 PEM（CERTIFICATE 载体）。
	WechatPlatformCert string
	// AlipayPublicKey 支付宝公钥 PEM（PUBLIC KEY 载体）。
	AlipayPublicKey string
}

// RealAdapter 渠道名 → 适配器的运行时选择器。
type RealAdapter struct {
	adapters map[string]ChannelAdapter
}

// NewRealAdapter 按配置构建已接入的真实渠道适配器；未配置渠道不入表。
func NewRealAdapter(cfg RealConfig) *RealAdapter {
	a := &RealAdapter{adapters: map[string]ChannelAdapter{}}
	if strings.TrimSpace(cfg.WechatPlatformCert) != "" {
		a.adapters[WechatPayV3Name] = &WechatPayV3{PlatformCertPEM: cfg.WechatPlatformCert}
	}
	if strings.TrimSpace(cfg.AlipayPublicKey) != "" {
		a.adapters[AlipayName] = &Alipay{PublicKey: cfg.AlipayPublicKey}
	}
	return a
}

// For 返回渠道 name 对应的适配器；未接入/未知渠道回退 501 占位。
func (a *RealAdapter) For(name string) ChannelAdapter {
	if ad, ok := a.adapters[name]; ok {
		return ad
	}
	return NewRealChannel(name)
}

// Has 报告渠道 name 是否已接入（注册了验签适配器）。
func (a *RealAdapter) Has(name string) bool {
	_, ok := a.adapters[name]
	return ok
}
