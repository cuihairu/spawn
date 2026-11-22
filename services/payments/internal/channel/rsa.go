package channel

// 真实渠道原生验签的共享加密核（GP-202）：RSA-PKCS1v15 + SHA-256。
//
// 凭据无关：验签只用渠道下发的**公开**材料（微信 v3 平台证书 / 支付宝公钥），
// 不含商户私钥，故可用自签密钥单测；生产由环境变量注入真实材料。

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
)

// parseRSAPublicKeyPEM 解析 PEM 载体里的 RSA 公钥。接受两种载体：
// CERTIFICATE（微信 v3 平台证书）与 PUBLIC KEY（支付宝公钥）。
func parseRSAPublicKeyPEM(pemBlock string) (*rsa.PublicKey, error) {
	der, _ := pem.Decode([]byte(pemBlock))
	if der == nil {
		return nil, errors.New("rsa: pem decode failed")
	}
	var pub any
	if der.Type == "CERTIFICATE" {
		cert, err := x509.ParseCertificate(der.Bytes)
		if err != nil {
			return nil, fmt.Errorf("rsa: parse certificate: %w", err)
		}
		pub = cert.PublicKey
	} else {
		var err error
		pub, err = x509.ParsePKIXPublicKey(der.Bytes)
		if err != nil {
			return nil, fmt.Errorf("rsa: parse public key: %w", err)
		}
	}
	rsaPub, ok := pub.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("rsa: key is not RSA")
	}
	return rsaPub, nil
}

// verifyRSASHA256 校验 sig 是否为 msg 的 RSA-SHA256 签名（PKCS#1 v1.5）。
func verifyRSASHA256(keyPEM string, msg, sig []byte) error {
	pub, err := parseRSAPublicKeyPEM(keyPEM)
	if err != nil {
		return err
	}
	hashed := sha256.Sum256(msg)
	if err := rsa.VerifyPKCS1v15(pub, crypto.SHA256, hashed[:], sig); err != nil {
		return fmt.Errorf("rsa: signature mismatch: %w", err)
	}
	return nil
}
