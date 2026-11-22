package channel

// WeChat Pay v3 回调原生验签（GP-202）。
//
// 微信 v3 通知签名规则（公开文档）：
//   - 签名串 = "timestamp\nnonce\nbody\n"（body 为原始请求体字节）
//   - 验签用**平台证书**（Wechatpay-Serial 头选择对应证书），RSA-PKCS1v15 + SHA-256
//   - 签名、时间戳、随机串、证书序列号分别经 Wechatpay-Signature / -Timestamp /
//     -Nonce / -Serial 请求头传递
//
// 平台证书是渠道公开下发材料（无商户私钥），故本适配器凭据无关；证书为空
// 视为渠道未接入（验签恒失败 → 501），绝不回退沙箱 HMAC（payment.md 安全边界 2）。

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// WechatPayV3Name 微信 v3 渠道名（callback/:channel 路径值）。
const WechatPayV3Name = "wechat-pay-v3"

// WechatPayV3 微信支付 v3 通知验签适配器。
type WechatPayV3 struct {
	// PlatformCertPEM 微信平台证书 PEM（CERTIFICATE 载体）。空 = 未接入。
	PlatformCertPEM string
	// CertsBySerial 可选的按证书序列号（Wechatpay-Serial）多证书表；非空时
	// 优先按 Serial 选择，缺失该序列号则验签失败（证书轮换场景）。
	CertsBySerial map[string]string
}

func (a *WechatPayV3) Name() string { return WechatPayV3Name }

func (a *WechatPayV3) VerifyNativeSignature(rawBody []byte, signature string, meta map[string]string) error {
	if strings.TrimSpace(a.PlatformCertPEM) == "" {
		return ErrRealChannelNotConfigured
	}
	ts := meta["timestamp"]
	nonce := meta["nonce"]
	if ts == "" || nonce == "" {
		return errors.New("wechat: timestamp or nonce missing in meta")
	}
	certPEM := a.PlatformCertPEM
	if len(a.CertsBySerial) > 0 {
		serial := meta["platform-serial"]
		if serial == "" {
			return errors.New("wechat: platform-serial missing in meta")
		}
		cert, ok := a.CertsBySerial[serial]
		if !ok {
			return fmt.Errorf("wechat: unknown platform serial %s", serial)
		}
		certPEM = cert
	}
	if signature == "" {
		return errors.New("wechat: signature missing")
	}
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		return fmt.Errorf("wechat: signature base64: %w", err)
	}
	msg := ts + "\n" + nonce + "\n" + string(rawBody) + "\n"
	return verifyRSASHA256(certPEM, []byte(msg), sigBytes)
}
