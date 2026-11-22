package model

// GP-203 发货账本：teller Deliver 钩子（进程内注入）的落地面。订单状态机
// 仍归 Courier teller（不复制 CREATED/PAID/DELIVERED），本表只记发货执行
// 事实：充值目标（区服/角色）+ 按订单幂等的投递状态。
// 幂等锚点 = order_id 唯一（payment.md v1.1 安全边界 3：同订单不二次发货）。

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// 发货执行状态（非订单状态机）：pending 待投递 / delivered 已投递 /
// failed 投递失败（可补单，定时恢复重投）。
const (
	DeliveryStatusPending   = "pending"
	DeliveryStatusDelivered = "delivered"
	DeliveryStatusFailed    = "failed"
)

var (
	ErrDeliveryOrderNotFound    = errors.New("delivery order not found")
	ErrDeliveryOrderExists      = errors.New("delivery order already exists")
	ErrDeliveryEndpointNotFound = errors.New("delivery endpoint not configured")
)

// DeliveryOrder 发货账本行：订单 → 充值目标（区服/角色）+ 投递事实。
type DeliveryOrder struct {
	OrderId     string `json:"order_id"`
	GameId      string `json:"game_id"`
	ServerId    int64  `json:"server_id"`
	RoleId      string `json:"role_id"`
	AccountId   string `json:"account_id"`
	SkuId       string `json:"sku_id"`
	ProductId   string `json:"product_id"`
	Form        string `json:"form"`
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	Status      string `json:"status"`
	Attempts    int    `json:"attempts"`
	LastError   string `json:"last_error"`
	NextRetryAt string `json:"next_retry_at"`
	DeliveredAt string `json:"delivered_at"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// DeliveryEndpoint 游戏服发货接口配置（每游戏一条；管理面维护）。
type DeliveryEndpoint struct {
	GameId    string `json:"game_id"`
	Endpoint  string `json:"endpoint"`
	Secret    string `json:"-"`
	Enabled   bool   `json:"enabled"`
	UpdatedAt string `json:"updated_at"`
}

const deliveryOrderColumns = "order_id, game_id, server_id, role_id, account_id, sku_id, product_id, form, amount_cents, currency, status, attempts, last_error, next_retry_at, delivered_at, created_at, updated_at"

func scanDeliveryOrder(o *DeliveryOrder) []any {
	return []any{&o.OrderId, &o.GameId, &o.ServerId, &o.RoleId, &o.AccountId, &o.SkuId,
		&o.ProductId, &o.Form, &o.AmountCents, &o.Currency, &o.Status, &o.Attempts,
		&o.LastError, &o.NextRetryAt, &o.DeliveredAt, &o.CreatedAt, &o.UpdatedAt}
}

// DeliveryModel 发货账本数据访问。
type DeliveryModel struct {
	db *sql.DB
}

func NewDeliveryModel(db *sql.DB) *DeliveryModel {
	return &DeliveryModel{db: db}
}

func (m *DeliveryModel) CreateTables() error {
	tables := []struct{ sqlite, mysql string }{
		{
			`CREATE TABLE IF NOT EXISTS delivery_orders (
				order_id VARCHAR(128) PRIMARY KEY,
				game_id VARCHAR(64) NOT NULL DEFAULT '',
				server_id INTEGER NOT NULL DEFAULT 0,
				role_id VARCHAR(128) NOT NULL DEFAULT '',
				account_id VARCHAR(128) NOT NULL DEFAULT '',
				sku_id VARCHAR(64) NOT NULL DEFAULT '',
				product_id VARCHAR(128) NOT NULL DEFAULT '',
				form VARCHAR(32) NOT NULL DEFAULT '',
				amount_cents INTEGER NOT NULL DEFAULT 0,
				currency VARCHAR(8) NOT NULL DEFAULT '',
				status VARCHAR(16) NOT NULL DEFAULT 'pending',
				attempts INTEGER NOT NULL DEFAULT 0,
				last_error VARCHAR(512) NOT NULL DEFAULT '',
				next_retry_at VARCHAR(40) NOT NULL DEFAULT '',
				delivered_at VARCHAR(32) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS delivery_orders (
				order_id VARCHAR(128) PRIMARY KEY,
				game_id VARCHAR(64) NOT NULL DEFAULT '',
				server_id BIGINT NOT NULL DEFAULT 0,
				role_id VARCHAR(128) NOT NULL DEFAULT '',
				account_id VARCHAR(128) NOT NULL DEFAULT '',
				sku_id VARCHAR(64) NOT NULL DEFAULT '',
				product_id VARCHAR(128) NOT NULL DEFAULT '',
				form VARCHAR(32) NOT NULL DEFAULT '',
				amount_cents BIGINT NOT NULL DEFAULT 0,
				currency VARCHAR(8) NOT NULL DEFAULT '',
				status VARCHAR(16) NOT NULL DEFAULT 'pending',
				attempts INT NOT NULL DEFAULT 0,
				last_error VARCHAR(512) NOT NULL DEFAULT '',
				next_retry_at VARCHAR(40) NOT NULL DEFAULT '',
				delivered_at VARCHAR(32) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL,
				KEY idx_delivery_retry (status, next_retry_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		{
			`CREATE TABLE IF NOT EXISTS game_delivery_endpoints (
				game_id VARCHAR(64) PRIMARY KEY,
				endpoint VARCHAR(512) NOT NULL,
				secret VARCHAR(256) NOT NULL,
				enabled INTEGER NOT NULL DEFAULT 1,
				updated_at VARCHAR(32) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS game_delivery_endpoints (
				game_id VARCHAR(64) PRIMARY KEY,
				endpoint VARCHAR(512) NOT NULL,
				secret VARCHAR(256) NOT NULL,
				enabled TINYINT(1) NOT NULL DEFAULT 1,
				updated_at VARCHAR(32) NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
	}
	for _, t := range tables {
		if _, err := m.db.Exec(t.sqlite); err != nil {
			if _, err2 := m.db.Exec(t.mysql); err2 != nil {
				return errors.New("创建发货账本表失败（尝试了 SQLite 和 MySQL 格式）")
			}
		}
	}
	return nil
}

// CreateTarget 建账本行（下单时落充值目标；同 order_id 视为幂等命中）。
func (m *DeliveryModel) CreateTarget(o *DeliveryOrder) error {
	now := nowUTC()
	if o.Status == "" {
		o.Status = DeliveryStatusPending
	}
	o.CreatedAt, o.UpdatedAt = now, now
	if o.NextRetryAt == "" {
		o.NextRetryAt = now
	}
	_, err := m.db.Exec(
		`INSERT INTO delivery_orders (order_id, game_id, server_id, role_id, account_id, sku_id, product_id, form, amount_cents, currency, status, attempts, last_error, next_retry_at, delivered_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, '', ?, '', ?, ?)`,
		o.OrderId, o.GameId, o.ServerId, o.RoleId, o.AccountId, o.SkuId, o.ProductId,
		o.Form, o.AmountCents, o.Currency, o.Status, o.NextRetryAt, o.CreatedAt, o.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "Duplicate") {
			return ErrDeliveryOrderExists
		}
		return err
	}
	return nil
}

func (m *DeliveryModel) Get(orderId string) (*DeliveryOrder, error) {
	var o DeliveryOrder
	err := m.db.QueryRow(`SELECT `+deliveryOrderColumns+` FROM delivery_orders WHERE order_id = ?`, orderId).
		Scan(scanDeliveryOrder(&o)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDeliveryOrderNotFound
	}
	if err != nil {
		return nil, err
	}
	return &o, nil
}

// SetTarget 补单面：回填充值目标（已发货订单由 logic 层守卫拒绝）。
func (m *DeliveryModel) SetTarget(orderId string, serverId int64, roleId string) error {
	_, err := m.db.Exec(
		`UPDATE delivery_orders SET server_id = ?, role_id = ?, status = ?, updated_at = ? WHERE order_id = ?`,
		serverId, roleId, DeliveryStatusPending, nowUTC(), orderId,
	)
	return err
}

func (m *DeliveryModel) MarkDelivered(orderId string) error {
	now := nowUTC()
	_, err := m.db.Exec(
		`UPDATE delivery_orders SET status = ?, delivered_at = ?, updated_at = ? WHERE order_id = ?`,
		DeliveryStatusDelivered, now, now, orderId,
	)
	return err
}

// MarkFailed 记失败：attempts 累加，按指数退避定下次重投时间（断单停
// PAID 可对账，本表只记投递事实）。
func (m *DeliveryModel) MarkFailed(orderId, lastErr, nextRetryAt string) error {
	_, err := m.db.Exec(
		`UPDATE delivery_orders SET status = ?, attempts = attempts + 1, last_error = ?, next_retry_at = ?, updated_at = ? WHERE order_id = ?`,
		DeliveryStatusFailed, lastErr, nextRetryAt, nowUTC(), orderId,
	)
	return err
}

// ListRetry 到期待投递行（pending/failed，按 next_retry_at 升序，有界）。
func (m *DeliveryModel) ListRetry(now string, limit int) ([]*DeliveryOrder, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := fmt.Sprintf(`SELECT %s FROM delivery_orders WHERE status IN ('%s', '%s') AND next_retry_at <= ? ORDER BY next_retry_at ASC LIMIT %d`,
		deliveryOrderColumns, DeliveryStatusPending, DeliveryStatusFailed, limit)
	rows, err := m.db.Query(q, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*DeliveryOrder, 0, limit)
	for rows.Next() {
		var o DeliveryOrder
		if err := rows.Scan(scanDeliveryOrder(&o)...); err != nil {
			return nil, err
		}
		out = append(out, &o)
	}
	return out, rows.Err()
}

// ListByGame 管理面投递列表（gameId 空 = 全部；status 空 = 全部）。
func (m *DeliveryModel) ListByGame(gameId, status string, limit int) ([]*DeliveryOrder, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	q := fmt.Sprintf(`SELECT %s FROM delivery_orders WHERE 1=1`, deliveryOrderColumns)
	args := []any{}
	if gameId != "" {
		q += ` AND game_id = ?`
		args = append(args, gameId)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += fmt.Sprintf(` ORDER BY updated_at DESC LIMIT %d`, limit)
	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*DeliveryOrder, 0, limit)
	for rows.Next() {
		var o DeliveryOrder
		if err := rows.Scan(scanDeliveryOrder(&o)...); err != nil {
			return nil, err
		}
		out = append(out, &o)
	}
	return out, rows.Err()
}

// UpsertEndpoint 配置游戏服发货接口（每游戏一条）。
func (m *DeliveryModel) UpsertEndpoint(e *DeliveryEndpoint) error {
	now := nowUTC()
	e.UpdatedAt = now
	// SQLite 走 ON CONFLICT；MySQL 语法不同，先试 SQLite 再试 MySQL（与建表同款回退）
	if _, err := m.db.Exec(
		`INSERT INTO game_delivery_endpoints (game_id, endpoint, secret, enabled, updated_at)
		 VALUES (?, ?, ?, ?, ?)
		 ON CONFLICT(game_id) DO UPDATE SET endpoint = excluded.endpoint, secret = excluded.secret, enabled = excluded.enabled, updated_at = excluded.updated_at`,
		e.GameId, e.Endpoint, e.Secret, e.Enabled, now,
	); err != nil {
		if _, err2 := m.db.Exec(
			`INSERT INTO game_delivery_endpoints (game_id, endpoint, secret, enabled, updated_at)
			 VALUES (?, ?, ?, ?, ?)
			 ON DUPLICATE KEY UPDATE endpoint = VALUES(endpoint), secret = VALUES(secret), enabled = VALUES(enabled), updated_at = VALUES(updated_at)`,
			e.GameId, e.Endpoint, e.Secret, e.Enabled, now,
		); err2 != nil {
			return err2
		}
	}
	return nil
}

func (m *DeliveryModel) GetEndpoint(gameId string) (*DeliveryEndpoint, error) {
	var e DeliveryEndpoint
	err := m.db.QueryRow(
		`SELECT game_id, endpoint, secret, enabled, updated_at FROM game_delivery_endpoints WHERE game_id = ?`,
		gameId,
	).Scan(&e.GameId, &e.Endpoint, &e.Secret, &e.Enabled, &e.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrDeliveryEndpointNotFound
	}
	if err != nil {
		return nil, err
	}
	return &e, nil
}
