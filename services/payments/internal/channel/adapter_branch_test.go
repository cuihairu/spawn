package channel

// adapter.go 剩余补漏分支：RealChannel 的 Name 回显与「真实渠道未接入」
// 恒失败的验签语义。

import "testing"

func TestRealChannelNameAndNotConfigured(t *testing.T) {
	rc := NewRealChannel("wechat-pay-v3")
	if rc.Name() != "wechat-pay-v3" {
		t.Fatalf("name = %q, want wechat-pay-v3", rc.Name())
	}
	if err := rc.VerifyNativeSignature([]byte(`{}`), "sig", nil); err != ErrRealChannelNotConfigured {
		t.Fatalf("verify = %v, want ErrRealChannelNotConfigured", err)
	}
}
