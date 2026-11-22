package channel

// Alipay 异步通知原生验签测试（GP-202，RSA2）：自签公钥验签，覆盖正常/
// 篡改/缺 sign/未配置/规范串构造分支。测试内用独立构造器重排规范串，
// 避免与被测 alipayCanonical 循环依赖。

import (
	"encoding/base64"
	"errors"
	"net/url"
	"sort"
	"strings"
	"testing"
)

// testCanonical 独立实现支付宝规范串（排除 sign/sign_type，按 key 升序）。
func testCanonical(form url.Values) string {
	keys := make([]string, 0, len(form))
	for k := range form {
		if k == "sign" || k == "sign_type" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+form.Get(k))
	}
	return strings.Join(parts, "&")
}

func TestAlipayVerify(t *testing.T) {
	_, key := genRSATestCert(t) // 复用自签 RSA 私钥
	pubPEM := genRSAPubKeyPEM(t, key)
	a := &Alipay{PublicKey: pubPEM}

	params := map[string]string{
		"out_trade_no": "o1",
		"trade_status": "TRADE_SUCCESS",
		"total_amount": "10.00",
		"app_id":       "2021000000",
	}
	// 独立规范串 + 签名
	form := url.Values{}
	for k, v := range params {
		form.Set(k, v)
	}
	canonical := testCanonical(form)
	sig := base64.StdEncoding.EncodeToString(signSHA256(t, key, []byte(canonical)))
	form.Set("sign", sig)
	form.Set("sign_type", "RSA2")
	body := form.Encode()

	if a.Name() != AlipayName {
		t.Fatalf("name = %s, want %s", a.Name(), AlipayName)
	}
	// 从表单 sign 提取验签
	if err := a.VerifyNativeSignature([]byte(body), "", nil); err != nil {
		t.Fatalf("valid alipay notify rejected: %v", err)
	}
	// 显式传 signature（测试/直传面）
	if err := a.VerifyNativeSignature([]byte(body), sig, nil); err != nil {
		t.Fatalf("explicit signature rejected: %v", err)
	}
}

func TestAlipayTamperedRejected(t *testing.T) {
	_, key := genRSATestCert(t)
	pubPEM := genRSAPubKeyPEM(t, key)
	a := &Alipay{PublicKey: pubPEM}

	form := url.Values{}
	form.Set("out_trade_no", "o1")
	form.Set("total_amount", "10.00")
	canonical := testCanonical(form)
	sig := base64.StdEncoding.EncodeToString(signSHA256(t, key, []byte(canonical)))
	form.Set("sign", sig)
	form.Set("sign_type", "RSA2")

	// 篡改金额
	tampered := url.Values{}
	for k := range form {
		tampered.Set(k, form.Get(k))
	}
	tampered.Set("total_amount", "99999.00")
	if err := a.VerifyNativeSignature([]byte(tampered.Encode()), "", nil); err == nil {
		t.Fatal("tampered amount accepted")
	}
	// 缺 sign
	noSign := url.Values{}
	noSign.Set("out_trade_no", "o1")
	noSign.Set("total_amount", "10.00")
	if err := a.VerifyNativeSignature([]byte(noSign.Encode()), "", nil); err == nil {
		t.Fatal("missing sign accepted")
	}
}

func TestAlipayNotConfigured(t *testing.T) {
	a := &Alipay{}
	err := a.VerifyNativeSignature([]byte("out_trade_no=o1&sign=abc"), "abc", nil)
	if !errors.Is(err, ErrRealChannelNotConfigured) {
		t.Fatalf("empty pubkey = %v, want ErrRealChannelNotConfigured", err)
	}
}

func TestAlipayCanonicalExcludesSignType(t *testing.T) {
	_, key := genRSATestCert(t)
	pubPEM := genRSAPubKeyPEM(t, key)
	a := &Alipay{PublicKey: pubPEM}

	// sign_type 变化不应改变规范串（sign_type 不参与签名）
	base := map[string]string{"out_trade_no": "o1", "total_amount": "10.00"}
	form := url.Values{}
	for k, v := range base {
		form.Set(k, v)
	}
	canonical := testCanonical(form)
	sig := base64.StdEncoding.EncodeToString(signSHA256(t, key, []byte(canonical)))

	// sign_type=RSA2 vs RSA 均可通过（sign_type 不参与签名）
	for _, st := range []string{"RSA2", "RSA", ""} {
		f := url.Values{}
		for k, v := range base {
			f.Set(k, v)
		}
		f.Set("sign", sig)
		if st != "" {
			f.Set("sign_type", st)
		}
		if err := a.VerifyNativeSignature([]byte(f.Encode()), "", nil); err != nil {
			t.Fatalf("sign_type %q rejected: %v", st, err)
		}
	}
}
