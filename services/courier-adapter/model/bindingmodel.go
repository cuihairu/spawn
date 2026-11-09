package model

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// 绑定状态（镜像 Courier 账号生命周期，接入设计 §1.2）。
const (
	BindingActive        = "ACTIVE"
	BindingUnbound       = "UNBOUND"
	BindingPendingDelete = "PENDING_DELETE"
)

var ErrBindingNotFound = errors.New("platform courier binding not found")

// Binding 一个平台用户 ↔ 一个 Courier 公司级账号（一对一）。
// courier_secret_enc 为代登录随机密码的 AES-GCM 密文（不落明文）。
type Binding struct {
	SpawnUserID      int64
	CourierAccountID string
	CourierEmail     string
	SecretEnc        string
	Status           string
	BindMethod       string
	BoundAt          string
	UnboundAt        string
	CreatedAt        string
	UpdatedAt        string
}

type BindingModel struct {
	db *sql.DB
}

func NewBindingModel(db *sql.DB) *BindingModel {
	return &BindingModel{db: db}
}

func (m *BindingModel) GetByUser(userID int64) (*Binding, error) {
	row := m.db.QueryRow(
		`SELECT spawn_user_id, courier_account_id, courier_email, courier_secret_enc,
		        status, bind_method, bound_at, unbound_at, created_at, updated_at
		 FROM platform_courier_binding WHERE spawn_user_id = ?`, userID)
	return scanBinding(row)
}

// GetByCourierAccount 以 Courier 账号反查（唯一键约束的保护性读取）。
func (m *BindingModel) GetByCourierAccount(accountID string) (*Binding, error) {
	row := m.db.QueryRow(
		`SELECT spawn_user_id, courier_account_id, courier_email, courier_secret_enc,
		        status, bind_method, bound_at, unbound_at, created_at, updated_at
		 FROM platform_courier_binding WHERE courier_account_id = ?`, accountID)
	return scanBinding(row)
}

func scanBinding(row *sql.Row) (*Binding, error) {
	var b Binding
	if err := row.Scan(&b.SpawnUserID, &b.CourierAccountID, &b.CourierEmail, &b.SecretEnc,
		&b.Status, &b.BindMethod, &b.BoundAt, &b.UnboundAt, &b.CreatedAt, &b.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrBindingNotFound
		}
		return nil, err
	}
	return &b, nil
}

// Create 首次绑定（auto/ex explicit）。
func (m *BindingModel) Create(b *Binding) error {
	now := nowStr()
	if b.Status == "" {
		b.Status = BindingActive
	}
	if b.BindMethod == "" {
		b.BindMethod = "auto"
	}
	b.BoundAt, b.CreatedAt, b.UpdatedAt = now, now, now
	_, err := m.db.Exec(
		`INSERT INTO platform_courier_binding
		 (spawn_user_id, courier_account_id, courier_email, courier_secret_enc,
		  status, bind_method, bound_at, unbound_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, '', ?, ?)`,
		b.SpawnUserID, b.CourierAccountID, b.CourierEmail, b.SecretEnc,
		b.Status, b.BindMethod, b.BoundAt, b.CreatedAt, b.UpdatedAt)
	return err
}

// UpdateStatus 状态流转（注销受理 → PENDING_DELETE；撤销 → ACTIVE；
// 终态删除 → UNBOUND 并记 unbound_at）。
func (m *BindingModel) UpdateStatus(userID int64, status string) error {
	var unbound string
	if status == BindingUnbound {
		unbound = nowStr()
	}
	res, err := m.db.Exec(
		`UPDATE platform_courier_binding
		 SET status = ?, unbound_at = ?, updated_at = ?
		 WHERE spawn_user_id = ?`,
		status, unbound, nowStr(), userID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n == 0 {
		return ErrBindingNotFound
	}
	return nil
}

// --- 凭据加密（AES-GCM；密钥 = SHA-256(config key ‖ 固定域串) 32B。
// 保护对象是适配器自产随机密码，域分离避免与其他用途撞键）---

var credKDFDomain = []byte("spawn-courier-adapter-cred-v1")

func deriveKey(configKey string) []byte {
	h := sha256.New()
	_, _ = h.Write([]byte(configKey))
	_, _ = h.Write(credKDFDomain)
	return h.Sum(nil)
}

// EncryptSecret 加密代登录随机密码。
func EncryptSecret(configKey, plaintext string) (string, error) {
	block, err := aes.NewCipher(deriveKey(configKey))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

// DecryptSecret 还元代登录随机密码。
func DecryptSecret(configKey, sealed string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		return "", fmt.Errorf("decode secret: %w", err)
	}
	block, err := aes.NewCipher(deriveKey(configKey))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", fmt.Errorf("open secret: %w", err)
	}
	return string(plain), nil
}

// GenerateAppPassword 生成代登录随机密码（24B → base64url 32 字符，
// 满足 Courier 契约 8–128 字符；明文只在加密前短暂存在于内存）。
func GenerateAppPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
