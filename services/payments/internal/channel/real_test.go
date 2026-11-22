package channel

// RealAdapter 注册/路由测试（GP-202）：只注册已注入公开材料的渠道，未配置渠道
// 回落 NewRealChannel 占位（真实渠道未接入 = 501），绝不误用沙箱 HMAC。

import "testing"

func TestRealAdapterRegistersOnlyConfigured(t *testing.T) {
	certPEM, key := genRSATestCert(t)
	pubPEM := genRSAPubKeyPEM(t, key)

	// 只配微信 → 只注册 wechat-pay-v3
	a := NewRealAdapter(RealConfig{WechatPlatformCert: certPEM, AlipayPublicKey: ""})
	if !a.Has(WechatPayV3Name) {
		t.Fatal("wechat not registered with platform cert")
	}
	if a.Has(AlipayName) {
		t.Fatal("alipay registered without public key")
	}
	// 未配置渠道 → 占位（501）
	if got := a.For(AlipayName); !isRealChannel(got) {
		t.Fatalf("unconfigured alipay = %T, want *RealChannel placeholder", got)
	}

	// 只配支付宝 → 只注册 alipay
	b := NewRealAdapter(RealConfig{WechatPlatformCert: "", AlipayPublicKey: pubPEM})
	if b.Has(WechatPayV3Name) {
		t.Fatal("wechat registered without platform cert")
	}
	if !b.Has(AlipayName) {
		t.Fatal("alipay not registered with public key")
	}

	// 两者皆配 → 双双注册
	c := NewRealAdapter(RealConfig{WechatPlatformCert: certPEM, AlipayPublicKey: pubPEM})
	if !c.Has(WechatPayV3Name) || !c.Has(AlipayName) {
		t.Fatal("both configured, not both registered")
	}
	if len(c.adapters) != 2 {
		t.Fatalf("adapters = %d, want 2", len(c.adapters))
	}
}

func TestRealAdapterUnknownName(t *testing.T) {
	a := NewRealAdapter(RealConfig{})
	if a.Has("no-such-channel") {
		t.Fatal("unknown name reported as registered")
	}
	// 未注册名也回落占位（501），不 panic
	if got := a.For("no-such-channel"); !isRealChannel(got) {
		t.Fatalf("unknown name = %T, want *RealChannel", got)
	}
}

func isRealChannel(a ChannelAdapter) bool {
	_, ok := a.(*RealChannel)
	return ok
}
