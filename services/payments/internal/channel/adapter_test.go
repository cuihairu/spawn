package channel

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
)

func TestSandboxHMACVerify(t *testing.T) {
	secret := []byte("test-secret")
	body := []byte(`{"orderId":"order_1","paidAt":"2026-11-09T12:00:00.000Z"}`)
	mac := hmac.New(sha256.New, secret)
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	a := &SandboxHMAC{Secret: secret}
	if err := a.VerifyNativeSignature(body, sig, nil); err != nil {
		t.Fatalf("valid sig rejected: %v", err)
	}
	if err := a.VerifyNativeSignature(body, sig+"00", nil); err == nil {
		t.Fatal("tampered sig accepted")
	}
	if err := a.VerifyNativeSignature(body, "", nil); err == nil {
		t.Fatal("missing sig accepted")
	}
	if a.Name() != "sandbox-hmac" {
		t.Fatalf("name = %s", a.Name())
	}
}

func TestRealChannelNotConfigured(t *testing.T) {
	r := NewRealChannel("wechat-pay-v3")
	if err := r.VerifyNativeSignature(nil, "x", nil); !errors.Is(err, ErrRealChannelNotConfigured) {
		t.Fatalf("real channel should not verify without config, got %v", err)
	}
	// 真实渠道未接入时绝不以沙箱 HMAC 兜底：err 必须非 nil
	if err := r.VerifyNativeSignature([]byte("{}"), "anything", nil); err == nil {
		t.Fatal("real channel must never accept sandbox-style signature")
	}
}
