package channel

// Alipay 异步通知原生验签（GP-202，RSA2 = RSA-PKCS1v15 + SHA-256）。
//
// 支付宝异步通知（notify）为 x-www-form-urlencoded，签名在表单字段 sign；
// 规范串 = 除 sign/sign_type 外全部参数按 key 升序拼 "k=v"，以 & 连接，
// 验签用**支付宝公钥**（渠道公开下发材料，无商户私钥）。

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// AlipayName 支付宝渠道名（callback/:channel 路径值）。
const AlipayName = "alipay"

// Alipay 支付宝异步通知验签适配器。
type Alipay struct {
	// PublicKey 支付宝公钥 PEM（PUBLIC KEY 载体）。空 = 未接入。
	PublicKey string
}

func (a *Alipay) Name() string { return AlipayName }

// VerifyNativeSignature 校验签名。signature 非空时优先使用（便于测试/显式传参），
// 否则从 rawBody 的表单字段 sign 提取（支付宝 notify 的真实承载方式）。
func (a *Alipay) VerifyNativeSignature(rawBody []byte, signature string, _ map[string]string) error {
	if strings.TrimSpace(a.PublicKey) == "" {
		return ErrRealChannelNotConfigured
	}
	form, err := url.ParseQuery(string(rawBody))
	if err != nil {
		return fmt.Errorf("alipay: parse notify body: %w", err)
	}
	sig := signature
	if sig == "" {
		sig = form.Get("sign")
	}
	if sig == "" {
		return errors.New("alipay: sign missing")
	}
	sigBytes, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("alipay: sign base64: %w", err)
	}
	canonical, err := alipayCanonical(form)
	if err != nil {
		return err
	}
	return verifyRSASHA256(a.PublicKey, []byte(canonical), sigBytes)
}

// alipayCanonical 生成支付宝规范串：除 sign/sign_type 外按 key 升序拼 k=v。
// 值取首个（支付宝表单键唯一）。sign_type 不参与签名（规范明确）。
func alipayCanonical(form url.Values) (string, error) {
	keys := make([]string, 0, len(form))
	for k := range form {
		if k == "sign" || k == "sign_type" {
			continue
		}
		keys = append(keys, k)
	}
	if len(keys) == 0 {
		return "", errors.New("alipay: no params to sign")
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+form.Get(k))
	}
	return strings.Join(parts, "&"), nil
}
