package handler

// GP-202 原生验签 HTTP 接缝集成：已注入验签材料（自签测试证书）的真实渠道
// 回调走 POST /callback/:channel 全链——微信原生头提取（Wechatpay-Signature/
// -Timestamp/-Nonce/-Serial）→ 平台证书验签 → 以独立 Courier 渠道密钥重算
// HMAC 转发（绝非沙箱密钥；未配置 fail-closed 503）；
// 支付宝表单 sign 放行分支（无签名头、适配器解析 body）→ RSA2 验签 → 转发；
// 篡改/缺元数据 403、未接入渠道仍 501（注册表隔离，绝不回退沙箱 HMAC）。
//
// 验签只用公开材料（平台证书/公钥），测试密钥自签即可，无需真实渠道凭据；
// 真实渠道商户凭据注入后的正式链路验收仍归 GP-206（GP-202 保持未勾）。

import (
	"bytes"
	"crypto"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/tappi/tappi/services/payments/internal/config"
	"github.com/tappi/tappi/services/payments/model"
)

// genNativeCertPem 自签 RSA 测试证书 PEM（CERTIFICATE 载体，充当微信平台
// 证书）与对应私钥；与 channel 包 rsatesthelper 同法，包外须另备一份。
func genNativeCertPem(t *testing.T) (string, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa.GenerateKey: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "native-integration-platform-cert"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), key
}

// nativePubPem 导出 RSA 公钥 PEM（PUBLIC KEY 载体，充当支付宝公钥）。
func nativePubPem(t *testing.T, key *rsa.PrivateKey) string {
	t.Helper()
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))
}

// signNativeB64 RSA-PKCS1v15 SHA-256 签名的 base64（微信签名头/支付宝 sign 同载体）。
func signNativeB64(t *testing.T, key *rsa.PrivateKey, msg []byte) string {
	t.Helper()
	hashed := sha256.Sum256(msg)
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hashed[:])
	if err != nil {
		t.Fatalf("SignPKCS1v15: %v", err)
	}
	return base64.StdEncoding.EncodeToString(sig)
}

func TestRoutes_CallbackNativeChannels(t *testing.T) {
	certPEM, key := genNativeCertPem(t)
	env := newPaymentsServer(t, func(c *config.Config) {
		c.Channel.WechatPlatformCert = certPEM
		c.Channel.AlipayPublicKey = nativePubPem(t, key)
		// GP-202 密钥分离：真实渠道转发腿必须用独立 Courier 渠道密钥
		// （与沙箱密钥不同值），存根记录命中方供断言。
		c.Channel.CourierSecret = testCourierChannelSecret
	})
	seed := func(orderID string) {
		t.Helper()
		if err := env.svcCtx.ProjectionDB.UpsertFromOrder(&model.OrderProjection{
			OrderId: orderID, GameId: "game-a", AccountId: "7", SkuId: "sku-native-1",
			ProductId: "native-1", Form: "DIRECT_PURCHASE", AmountCents: 1000,
			Currency: "CNY", Status: "CREATED",
		}); err != nil {
			t.Fatalf("seed projection %s: %v", orderID, err)
		}
	}
	seed("o_wx_native_1")
	seed("o_ali_native_1")

	// 1. 微信 v3 合法回调：原生头齐备 → 平台证书验签通过 → 重算 HMAC 转发
	// Courier（存根验签通过才计 callbackSeen）→ 200。
	wxBody := `{"orderId":"o_wx_native_1","paidAt":"2026-11-27T00:00:00Z"}`
	wxTs, wxNonce := "1764184800", "native-it-nonce"
	wxSig := signNativeB64(t, key, []byte(wxTs+"\n"+wxNonce+"\n"+wxBody+"\n"))
	req, _ := http.NewRequest(http.MethodPost, env.base+"/callback/wechat-pay-v3", strings.NewReader(wxBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Wechatpay-Signature", wxSig)
	req.Header.Set("Wechatpay-Timestamp", wxTs)
	req.Header.Set("Wechatpay-Nonce", wxNonce)
	req.Header.Set("Wechatpay-Serial", "PUB_TEST_001")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("wechat callback: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("wechat valid callback = %d, want 200", resp.StatusCode)
	}
	if env.callbackSeen != 1 {
		t.Fatalf("courier callback seen = %d, want 1（微信验签过必转发）", env.callbackSeen)
	}
	if len(env.callbackSigSecretSeen) != 1 || env.callbackSigSecretSeen[0] != "courier" {
		t.Fatalf("wechat forward signed with %v, want [courier]（绝不以沙箱密钥代发）", env.callbackSigSecretSeen)
	}

	// 2. 微信回调体被篡改（合法签名配改动后的 body）→ RSA 验签不匹配 403，
	// 不再转发。
	wxBadBody := `{"orderId":"o_wx_native_1","paidAt":"2026-11-27T00:00:01Z"}`
	req2, _ := http.NewRequest(http.MethodPost, env.base+"/callback/wechat-pay-v3", strings.NewReader(wxBadBody))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("Wechatpay-Signature", wxSig)
	req2.Header.Set("Wechatpay-Timestamp", wxTs)
	req2.Header.Set("Wechatpay-Nonce", wxNonce)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("wechat tampered callback: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusForbidden {
		t.Fatalf("wechat tampered = %d, want 403", resp2.StatusCode)
	}
	if env.callbackSeen != 1 {
		t.Fatalf("courier callback seen = %d, want 1（验签失败绝不转发）", env.callbackSeen)
	}

	// 3. 微信缺 Wechatpay-Timestamp（meta 无 timestamp 也无 X-Payment-* 兼容头）
	// → 适配器拒验 403。
	req3, _ := http.NewRequest(http.MethodPost, env.base+"/callback/wechat-pay-v3", strings.NewReader(wxBody))
	req3.Header.Set("Content-Type", "application/json")
	req3.Header.Set("Wechatpay-Signature", wxSig)
	req3.Header.Set("Wechatpay-Nonce", wxNonce)
	resp3, err := http.DefaultClient.Do(req3)
	if err != nil {
		t.Fatalf("wechat missing timestamp: %v", err)
	}
	resp3.Body.Close()
	if resp3.StatusCode != http.StatusForbidden {
		t.Fatalf("wechat missing timestamp = %d, want 403", resp3.StatusCode)
	}

	// 4. 非支付宝渠道无签名头 → 403 签名缺失（Wechatpay-Signature 缺席且
	// X-Payment-Signature 空时处理器拒收，不进适配器）。
	req4, _ := http.NewRequest(http.MethodPost, env.base+"/callback/wechat-pay-v3", strings.NewReader(wxBody))
	req4.Header.Set("Content-Type", "application/json")
	resp4, err := http.DefaultClient.Do(req4)
	if err != nil {
		t.Fatalf("wechat no signature header: %v", err)
	}
	resp4.Body.Close()
	if resp4.StatusCode != http.StatusForbidden {
		t.Fatalf("wechat no signature header = %d, want 403", resp4.StatusCode)
	}

	// 5. 支付宝合法回调：无签名头（表单 sign 承载，处理器按渠道放行）→
	// 适配器解析表单、规范串（除 sign/sign_type 按 key 升序）RSA2 验签 →
	// 转发 Courier → 200。
	aliCanonical := "orderId=o_ali_native_1&paidAt=20261127T000000Z"
	aliSig := signNativeB64(t, key, []byte(aliCanonical))
	aliBody := "orderId=o_ali_native_1&paidAt=20261127T000000Z&sign=" + url.QueryEscape(aliSig) + "&sign_type=RSA2"
	req5, _ := http.NewRequest(http.MethodPost, env.base+"/callback/alipay", strings.NewReader(aliBody))
	req5.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp5, err := http.DefaultClient.Do(req5)
	if err != nil {
		t.Fatalf("alipay callback: %v", err)
	}
	resp5.Body.Close()
	if resp5.StatusCode != http.StatusOK {
		t.Fatalf("alipay valid callback = %d, want 200", resp5.StatusCode)
	}
	if env.callbackSeen != 2 {
		t.Fatalf("courier callback seen = %d, want 2（支付宝验签过必转发）", env.callbackSeen)
	}
	if len(env.callbackSigSecretSeen) != 2 || env.callbackSigSecretSeen[1] != "courier" {
		t.Fatalf("alipay forward signed with %v, want [courier courier]（绝不以沙箱密钥代发）", env.callbackSigSecretSeen)
	}

	// 6. 支付宝签名与规范串不符（sign 对错误参数串签）→ 403。
	wrongCanonical := "orderId=o_ali_native_1&paidAt=WRONG&"
	aliBadSig := signNativeB64(t, key, []byte(wrongCanonical))
	aliBadBody := "orderId=o_ali_native_1&paidAt=20261127T000000Z&sign=" + url.QueryEscape(aliBadSig) + "&sign_type=RSA2"
	req6, _ := http.NewRequest(http.MethodPost, env.base+"/callback/alipay", strings.NewReader(aliBadBody))
	req6.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp6, err := http.DefaultClient.Do(req6)
	if err != nil {
		t.Fatalf("alipay tampered callback: %v", err)
	}
	resp6.Body.Close()
	if resp6.StatusCode != http.StatusForbidden {
		t.Fatalf("alipay tampered = %d, want 403", resp6.StatusCode)
	}
	if env.callbackSeen != 2 {
		t.Fatalf("courier callback seen = %d, want 2（验签失败绝不转发）", env.callbackSeen)
	}

	// 7. 未接入渠道仍 501（已注入微信/支付宝材料不改变注册表隔离——未知渠道
	// 回落占位适配器，绝不拿沙箱 HMAC 冒充）。
	req7, _ := http.NewRequest(http.MethodPost, env.base+"/callback/real", strings.NewReader(wxBody))
	req7.Header.Set("Content-Type", "application/json")
	req7.Header.Set("X-Payment-Signature", "placeholder-sig")
	resp7, err := http.DefaultClient.Do(req7)
	if err != nil {
		t.Fatalf("unknown channel callback: %v", err)
	}
	resp7.Body.Close()
	if resp7.StatusCode != http.StatusNotImplemented {
		t.Fatalf("unknown channel with materials injected = %d, want 501", resp7.StatusCode)
	}
}

// TestRoutes_CallbackNativeCourierSecretMissing GP-202 密钥分离 fail-closed
// 回归：已注入验签材料但 Courier 渠道密钥未配置时，真实渠道回调验签通过也
// 必须拒绝转发（503、存根零触达）——绝不以沙箱密钥代发；同环境沙箱渠道
// 转发腿不受影响（沙箱共享密钥照常转发）。
func TestRoutes_CallbackNativeCourierSecretMissing(t *testing.T) {
	certPEM, key := genNativeCertPem(t)
	env := newPaymentsServer(t, func(c *config.Config) {
		c.Channel.WechatPlatformCert = certPEM
		c.Channel.AlipayPublicKey = nativePubPem(t, key)
		// Channel.CourierSecret 刻意留空 = 真实渠道转发腿未配置。
	})
	if err := env.svcCtx.ProjectionDB.UpsertFromOrder(&model.OrderProjection{
		OrderId: "o_wx_native_nc", GameId: "game-a", AccountId: "7", SkuId: "sku-native-nc",
		ProductId: "native-nc", Form: "DIRECT_PURCHASE", AmountCents: 1000,
		Currency: "CNY", Status: "CREATED",
	}); err != nil {
		t.Fatalf("seed projection: %v", err)
	}

	// 1. 微信合法回调（验签必过）→ 503 Courier 渠道密钥未配置，不转发。
	wxBody := `{"orderId":"o_wx_native_nc","paidAt":"2026-11-27T00:00:00Z"}`
	wxTs, wxNonce := "1764184800", "native-nc-nonce"
	wxSig := signNativeB64(t, key, []byte(wxTs+"\n"+wxNonce+"\n"+wxBody+"\n"))
	req, _ := http.NewRequest(http.MethodPost, env.base+"/callback/wechat-pay-v3", strings.NewReader(wxBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Wechatpay-Signature", wxSig)
	req.Header.Set("Wechatpay-Timestamp", wxTs)
	req.Header.Set("Wechatpay-Nonce", wxNonce)
	req.Header.Set("Wechatpay-Serial", "PUB_TEST_001")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("wechat callback without courier secret: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("wechat callback without courier secret = %d, want 503", resp.StatusCode)
	}
	if env.callbackSeen != 0 {
		t.Fatalf("courier callback seen = %d, want 0（密钥未配置绝不转发）", env.callbackSeen)
	}

	// 2. 同环境沙箱渠道照常转发（沙箱共享密钥，存根命中 sandbox）。
	cbBody := `{"orderId":"o_wx_native_nc","paidAt":"2026-11-27T00:00:01Z"}`
	mac := hmac.New(sha256.New, []byte(testSandboxSecret))
	mac.Write([]byte(cbBody))
	goodSig := hex.EncodeToString(mac.Sum(nil))
	req2, _ := http.NewRequest(http.MethodPost, env.base+"/callback/sandbox", bytes.NewReader([]byte(cbBody)))
	req2.Header.Set("X-Payment-Signature", goodSig)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("sandbox callback: %v", err)
	}
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("sandbox callback without courier secret = %d, want 200（沙箱腿不受影响）", resp2.StatusCode)
	}
	if env.callbackSeen != 1 || len(env.callbackSigSecretSeen) != 1 || env.callbackSigSecretSeen[0] != "sandbox" {
		t.Fatalf("sandbox forward = seen %d secrets %v, want 1 [sandbox]", env.callbackSeen, env.callbackSigSecretSeen)
	}
}
