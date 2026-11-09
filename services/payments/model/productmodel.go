package model

// GP-202 服务端商品配置（sku/price 表）：客户端下单只带 skuId，金额/币种
// 一律取服务端价格（payment.md v1.1 安全边界 1）。商品归游戏与合作方
// （商户归属），跨游戏/跨厂商越权在 logic 层校验。

import (
	"database/sql"
	"errors"
	"strings"
)

// 四形态（payment.md v1.1）：直购/账号钱包/游戏钱包/外接支付。
const (
	FormDirectPurchase = "DIRECT_PURCHASE"
	FormAccountWallet  = "ACCOUNT_WALLET"
	FormGameWallet     = "GAME_WALLET"
	FormExternalPay    = "EXTERNAL_PAYMENT"
)

var (
	ErrProductNotFound = errors.New("product not found")
	ErrProductExists   = errors.New("product id already exists")
)

// Product 服务端商品配置（价格表条目投影 + 商户归属）。
type Product struct {
	Id          string `json:"id"`         // 接入方定义 SKU 标识（服务端主键）
	ProductId   string `json:"product_id"` // 发货目标（道具/货币包 ID）
	Form        string `json:"form"`       // 四形态之一
	AmountCents int64  `json:"amount_cents"`
	// BaseQuantity 基础购买内容数量（如 100 游戏币；GP-302 倍率赠送的
	// 基础量，服务端配置随订单事实固化，非客户端上报）
	BaseQuantity int64  `json:"base_quantity"`
	Currency     string `json:"currency"`   // ISO 4217，如 CNY
	GameId       string `json:"game_id"`    // 所属游戏（scope）
	PartnerId    int64  `json:"partner_id"` // 商户（合作方）归属
	Enabled      bool   `json:"enabled"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

const productColumns = "id, product_id, form, amount_cents, base_quantity, currency, game_id, partner_id, enabled, created_at, updated_at"

func scanProduct(p *Product) []any {
	return []any{&p.Id, &p.ProductId, &p.Form, &p.AmountCents, &p.BaseQuantity,
		&p.Currency, &p.GameId, &p.PartnerId, &p.Enabled, &p.CreatedAt, &p.UpdatedAt}
}

func (m *ProductModel) CreateTables() error {
	tables := []struct{ sqlite, mysql string }{
		{
			`CREATE TABLE IF NOT EXISTS payment_products (
				id VARCHAR(64) PRIMARY KEY,
				product_id VARCHAR(128) NOT NULL,
				form VARCHAR(32) NOT NULL,
				amount_cents INTEGER NOT NULL,
				base_quantity INTEGER NOT NULL DEFAULT 0,
				currency VARCHAR(8) NOT NULL,
				game_id VARCHAR(64) NOT NULL,
				partner_id INTEGER NOT NULL DEFAULT 0,
				enabled INTEGER NOT NULL DEFAULT 1,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS payment_products (
				id VARCHAR(64) PRIMARY KEY,
				product_id VARCHAR(128) NOT NULL,
				form VARCHAR(32) NOT NULL,
				amount_cents BIGINT NOT NULL,
				base_quantity BIGINT NOT NULL DEFAULT 0,
				currency VARCHAR(8) NOT NULL,
				game_id VARCHAR(64) NOT NULL,
				partner_id BIGINT NOT NULL DEFAULT 0,
				enabled TINYINT(1) NOT NULL DEFAULT 1,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL,
				KEY idx_products_game (game_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
	}
	for _, t := range tables {
		if _, err := m.db.Exec(t.sqlite); err != nil {
			if _, err2 := m.db.Exec(t.mysql); err2 != nil {
				return errors.New("创建商品表失败（尝试了 SQLite 和 MySQL 格式）")
			}
		}
	}
	return nil
}

func (m *ProductModel) Create(p *Product) (*Product, error) {
	now := nowUTC()
	p.Enabled = true
	p.CreatedAt, p.UpdatedAt = now, now
	if _, err := m.db.Exec(
		`INSERT INTO payment_products (id, product_id, form, amount_cents, base_quantity, currency, game_id, partner_id, enabled, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Id, p.ProductId, p.Form, p.AmountCents, p.BaseQuantity, p.Currency,
		p.GameId, p.PartnerId, p.Enabled, now, now,
	); err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "Duplicate") {
			return nil, ErrProductExists
		}
		return nil, err
	}
	return p, nil
}

func (m *ProductModel) Get(id string) (*Product, error) {
	var p Product
	err := m.db.QueryRow(`SELECT `+productColumns+` FROM payment_products WHERE id = ?`, id).
		Scan(scanProduct(&p)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrProductNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListByGame 列某游戏商品（含 disabled，管理面用）；公开侧用 enabledOnly。
func (m *ProductModel) ListByGame(gameID string, enabledOnly bool) ([]*Product, error) {
	q := `SELECT ` + productColumns + ` FROM payment_products WHERE game_id = ?`
	if enabledOnly {
		q += ` AND enabled = 1`
	}
	q += ` ORDER BY id ASC`
	rows, err := m.db.Query(q, gameID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*Product, 0, 8)
	for rows.Next() {
		var p Product
		if err := rows.Scan(scanProduct(&p)...); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

func (m *ProductModel) SetEnabled(id string, enabled bool) (*Product, error) {
	now := nowUTC()
	if _, err := m.db.Exec(
		`UPDATE payment_products SET enabled = ?, updated_at = ? WHERE id = ?`,
		enabled, now, id,
	); err != nil {
		return nil, err
	}
	return m.Get(id)
}

// ProductModel 商品配置数据访问。
type ProductModel struct {
	db *sql.DB
}

func NewProductModel(db *sql.DB) *ProductModel {
	return &ProductModel{db: db}
}
