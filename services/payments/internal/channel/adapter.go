package channel

// 渠道适配接缝（payment.md v1.1 安全边界 2）：回调只认渠道签名。
//
// 真实渠道（WeChat Pay v3 / Alipay 等）必须实现 VerifyNativeSignature 用
// 渠道原生验签（平台证书/RSA2 等），并以环境变量注入渠道密钥；
// 不得用沙箱 HMAC 冒充支付机构验签。SandboxHMAC 仅限开发/沙箱 teller。

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
)

// ErrRealChannelNotConfigured 真实渠道适配未接入（无渠道密钥/实现）。
var ErrRealChannelNotConfigured = errors.New("real channel adapter not configured")

// ChannelAdapter 支付渠道适配：Name 标识渠道；VerifyNativeSignature 对 raw
// body 做渠道原生签名校验（真实渠道用平台证书/RSA 等，非 HMAC）。
type ChannelAdapter interface {
	Name() string
	// VerifyNativeSignature 校验渠道回调签名；meta 携带渠道特定参数
	// （如 platform-serial-no、timestamp、nonce），实现自行解析。
	VerifyNativeSignature(rawBody []byte, signature string, meta map[string]string) error
}

// SandboxHMAC 开发/沙箱专用：HMAC-SHA256（teller 沙箱渠道回调用）。
// 仅沙箱环境使用；真实渠道不得走此路径（见 ErrRealChannelNotConfigured）。
type SandboxHMAC struct {
	Secret []byte
}

func (s *SandboxHMAC) Name() string { return "sandbox-hmac" }

func (s *SandboxHMAC) VerifyNativeSignature(rawBody []byte, signature string, _ map[string]string) error {
	mac := hmac.New(sha256.New, s.Secret)
	mac.Write(rawBody)
	want := hex.EncodeToString(mac.Sum(nil))
	if !hmac.Equal([]byte(want), []byte(signature)) {
		return errors.New("sandbox hmac mismatch")
	}
	return nil
}

// RealChannel 真实渠道适配占位：未接入渠道时所有验签失败，
// 明确不把沙箱 HMAC 当真实渠道验签（Name 实现接口，字段存渠道名）。
type RealChannel struct {
	name string
}

func NewRealChannel(name string) *RealChannel { return &RealChannel{name: name} }

func (r *RealChannel) Name() string { return r.name }

func (r *RealChannel) VerifyNativeSignature(_ []byte, _ string, _ map[string]string) error {
	return ErrRealChannelNotConfigured
}
