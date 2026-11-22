package channel

// WeChat Pay v3 回调原生验签测试（GP-202）：自签平台证书验签，覆盖正常/
// 篡改/缺证书/缺 meta/证书序列号选择分支。

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestWechatPayV3Verify(t *testing.T) {
	certPEM, key := genRSATestCert(t)
	body := []byte(`{"out_trade_no":"o1","trade_state":"SUCCESS"}`)
	ts, nonce := "1700000000", "nonce123"
	msg := ts + "\n" + nonce + "\n" + string(body) + "\n"
	sig := base64.StdEncoding.EncodeToString(signSHA256(t, key, []byte(msg)))
	meta := map[string]string{"timestamp": ts, "nonce": nonce}

	a := &WechatPayV3{PlatformCertPEM: certPEM}
	if a.Name() != WechatPayV3Name {
		t.Fatalf("name = %s, want %s", a.Name(), WechatPayV3Name)
	}
	if err := a.VerifyNativeSignature(body, sig, meta); err != nil {
		t.Fatalf("valid sig rejected: %v", err)
	}
}

func TestWechatPayV3TamperedRejected(t *testing.T) {
	certPEM, key := genRSATestCert(t)
	body := []byte(`{"out_trade_no":"o1"}`)
	ts, nonce := "1700000000", "n1"
	msg := ts + "\n" + nonce + "\n" + string(body) + "\n"
	sig := base64.StdEncoding.EncodeToString(signSHA256(t, key, []byte(msg)))
	a := &WechatPayV3{PlatformCertPEM: certPEM}

	// 篡改 body（签名针对原 body）
	if err := a.VerifyNativeSignature([]byte(`{"out_trade_no":"o2"}`), sig, map[string]string{"timestamp": ts, "nonce": nonce}); err == nil {
		t.Fatal("tampered body accepted")
	}
	// 篡改 timestamp
	if err := a.VerifyNativeSignature(body, sig, map[string]string{"timestamp": "1699999999", "nonce": nonce}); err == nil {
		t.Fatal("tampered timestamp accepted")
	}
	// 篡改 nonce
	if err := a.VerifyNativeSignature(body, sig, map[string]string{"timestamp": ts, "nonce": "other"}); err == nil {
		t.Fatal("tampered nonce accepted")
	}
}

func TestWechatPayV3NotConfigured(t *testing.T) {
	a := &WechatPayV3{}
	err := a.VerifyNativeSignature([]byte("{}"), "sig", map[string]string{"timestamp": "t", "nonce": "n"})
	if !errors.Is(err, ErrRealChannelNotConfigured) {
		t.Fatalf("empty cert = %v, want ErrRealChannelNotConfigured", err)
	}
}

func TestWechatPayV3MissingMetaOrSignature(t *testing.T) {
	certPEM, _ := genRSATestCert(t)
	a := &WechatPayV3{PlatformCertPEM: certPEM}
	if err := a.VerifyNativeSignature(nil, "sig", nil); err == nil {
		t.Fatal("missing meta accepted")
	}
	if err := a.VerifyNativeSignature(nil, "", map[string]string{"timestamp": "t", "nonce": "n"}); err == nil {
		t.Fatal("missing signature accepted")
	}
	// 非 base64 签名
	if err := a.VerifyNativeSignature(nil, "!!not-b64!!", map[string]string{"timestamp": "t", "nonce": "n"}); err == nil {
		t.Fatal("bad base64 signature accepted")
	}
}

func TestWechatPayV3CertSerialSelection(t *testing.T) {
	certPEM, key := genRSATestCert(t)
	body := []byte(`{"out_trade_no":"o1"}`)
	msg := "1700000000\nn1\n" + string(body) + "\n"
	sig := base64.StdEncoding.EncodeToString(signSHA256(t, key, []byte(msg)))

	// 多证书表：正确序列号命中
	a := &WechatPayV3{PlatformCertPEM: "other-pem", CertsBySerial: map[string]string{"SN1": certPEM}}
	meta := map[string]string{"timestamp": "1700000000", "nonce": "n1", "platform-serial": "SN1"}
	if err := a.VerifyNativeSignature(body, sig, meta); err != nil {
		t.Fatalf("serial-selected cert rejected: %v", err)
	}

	// 未知序列号 → 拒绝（证书轮换场景）
	badMeta := map[string]string{"timestamp": "1700000000", "nonce": "n1", "platform-serial": "SN-unknown"}
	if err := a.VerifyNativeSignature(body, sig, badMeta); err == nil {
		t.Fatal("unknown serial accepted")
	}

	// 缺序列号 → 拒绝
	if err := a.VerifyNativeSignature(body, sig, map[string]string{"timestamp": "1700000000", "nonce": "n1"}); err == nil {
		t.Fatal("missing serial accepted")
	}
}

func TestWechatPayV3SignatureIsBase64RSA256(t *testing.T) {
	// 反向确认：签名串必须是 base64(RSA-SHA256)，不能是 hex 或 raw。
	certPEM, key := genRSATestCert(t)
	body := []byte("{}")
	msg := "t\nn\n{}\n"
	raw := signSHA256(t, key, []byte(msg))
	a := &WechatPayV3{PlatformCertPEM: certPEM}
	meta := map[string]string{"timestamp": "t", "nonce": "n"}
	if err := a.VerifyNativeSignature(body, base64.StdEncoding.EncodeToString(raw), meta); err != nil {
		t.Fatalf("base64 sig rejected: %v", err)
	}
	if err := a.VerifyNativeSignature(body, strings.Repeat("0", 512), meta); err == nil {
		t.Fatal("garbage sig accepted")
	}
}
