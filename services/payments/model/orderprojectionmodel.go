package model

// GP-204 订单只读投影 + 审计 + 退款/冲正管理适配。
//
// 投影是订单 id 为键的事实副本（接入设计 §3.1：Spawn 后台经只读投影读
// 订单事实），状态一律来自 Courier 上报（下单快照 CREATED / 渠道回调
// PAID / 发货投递 DELIVERED），本服务**不维护可独立推进的第二状态机**。
// Courier 无全局订单 HTTP 管理 API（调研 §4 边界 6），运营查询/检索经
// 本投影。退款/冲正为管理适配接缝：登记 PENDING 意图 + 状态映射，执行
// 待 Courier 退款端点（提案）接入，**不假设其存在、不伪造执行成功**。

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrOrderProjectionNotFound = errors.New("order projection not found")
	ErrRefundRequestNotFound   = errors.New("refund request not found")
)

// OrderProjection 订单事实副本（状态= Courier 上报值，非本服务推进）。
// PartnerId 为商户归属（下单时从服务端商品快照落定，对账按商户维度聚合）。
type OrderProjection struct {
	OrderId     string `json:"order_id"`
	GameId      string `json:"game_id"`
	PartnerId   int64  `json:"partner_id"`
	AccountId   string `json:"account_id"`
	SkuId       string `json:"sku_id"`
	ProductId   string `json:"product_id"`
	Form        string `json:"form"`
	AmountCents int64  `json:"amount_cents"`
	// BaseQuantity 基础购买量（下单时从服务端商品配置固化，GP-302 倍率
	// 赠送的基础量；后改商品不改变旧订单）
	BaseQuantity int64  `json:"base_quantity"`
	Currency     string `json:"currency"`
	Status       string `json:"status"`
	PaidAt       string `json:"paid_at"`
	DeliveredAt  string `json:"delivered_at"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// OrderAuditLog 运营查询/退款操作审计（GP-004 模型：actor/action/资源/detail）。
type OrderAuditLog struct {
	Id        int64  `json:"id"`
	OrderId   string `json:"order_id"`
	ActorId   int64  `json:"actor_id"`
	ActorRole string `json:"actor_role"`
	Action    string `json:"action"`
	Detail    string `json:"detail"`
	Ip        string `json:"ip"`
	CreatedAt string `json:"created_at"`
}

// RefundRequest 退款/冲正意图（管理适配；执行待 Courier 退款端点接入）。
type RefundRequest struct {
	Id          int64  `json:"id"`
	OrderId     string `json:"order_id"`
	Action      string `json:"action"` // REFUND / CORRECTION
	AmountCents int64  `json:"amount_cents"`
	Reason      string `json:"reason"`
	Status      string `json:"status"` // PENDING / EXECUTED / REJECTED（状态映射）
	Note        string `json:"note"`
	CreatedBy   int64  `json:"created_by"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// 退款意图状态（映射：PENDING → 执行器 → EXECUTED/REJECTED；执行器为
// Courier 退款端点提案，接入前保持 PENDING 不伪造成功）。
const (
	RefundStatusPending  = "PENDING"
	RefundStatusExecuted = "EXECUTED"
	RefundStatusRejected = "REJECTED"
	RefundActionRefund   = "REFUND"
	RefundActionCorrect  = "CORRECTION"
)

const projectionColumns = "order_id, game_id, partner_id, account_id, sku_id, product_id, form, amount_cents, base_quantity, currency, status, paid_at, delivered_at, created_at, updated_at"

func scanProjection(p *OrderProjection) []any {
	return []any{&p.OrderId, &p.GameId, &p.PartnerId, &p.AccountId, &p.SkuId, &p.ProductId, &p.Form,
		&p.AmountCents, &p.BaseQuantity, &p.Currency, &p.Status, &p.PaidAt, &p.DeliveredAt, &p.CreatedAt, &p.UpdatedAt}
}

// OrderProjectionModel 投影/审计/退款数据访问。
type OrderProjectionModel struct {
	db *sql.DB
}

func NewOrderProjectionModel(db *sql.DB) *OrderProjectionModel {
	return &OrderProjectionModel{db: db}
}

func (m *OrderProjectionModel) CreateTables() error {
	tables := []struct{ sqlite, mysql string }{
		{
			`CREATE TABLE IF NOT EXISTS order_projection (
				order_id VARCHAR(128) PRIMARY KEY,
				game_id VARCHAR(64) NOT NULL,
				partner_id INTEGER NOT NULL DEFAULT 0,
				account_id VARCHAR(128) NOT NULL DEFAULT '',
				sku_id VARCHAR(64) NOT NULL DEFAULT '',
				product_id VARCHAR(128) NOT NULL DEFAULT '',
				form VARCHAR(32) NOT NULL DEFAULT '',
				amount_cents INTEGER NOT NULL DEFAULT 0,
				base_quantity INTEGER NOT NULL DEFAULT 0,
				currency VARCHAR(8) NOT NULL DEFAULT '',
				status VARCHAR(16) NOT NULL,
				paid_at VARCHAR(32) NOT NULL DEFAULT '',
				delivered_at VARCHAR(32) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS order_projection (
				order_id VARCHAR(128) PRIMARY KEY,
				game_id VARCHAR(64) NOT NULL,
				partner_id BIGINT NOT NULL DEFAULT 0,
				account_id VARCHAR(128) NOT NULL DEFAULT '',
				sku_id VARCHAR(64) NOT NULL DEFAULT '',
				product_id VARCHAR(128) NOT NULL DEFAULT '',
				form VARCHAR(32) NOT NULL DEFAULT '',
				amount_cents BIGINT NOT NULL DEFAULT 0,
				base_quantity BIGINT NOT NULL DEFAULT 0,
				currency VARCHAR(8) NOT NULL DEFAULT '',
				status VARCHAR(16) NOT NULL,
				paid_at VARCHAR(32) NOT NULL DEFAULT '',
				delivered_at VARCHAR(32) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL,
				KEY idx_projection_game_status (game_id, status),
				KEY idx_projection_account (account_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		{
			`CREATE TABLE IF NOT EXISTS order_audit_logs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				order_id VARCHAR(128) NOT NULL DEFAULT '',
				actor_id INTEGER NOT NULL DEFAULT 0,
				actor_role VARCHAR(32) NOT NULL DEFAULT '',
				action VARCHAR(64) NOT NULL,
				detail VARCHAR(1024) NOT NULL DEFAULT '',
				ip VARCHAR(64) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS order_audit_logs (
				id BIGINT PRIMARY KEY AUTO_INCREMENT,
				order_id VARCHAR(128) NOT NULL DEFAULT '',
				actor_id BIGINT NOT NULL DEFAULT 0,
				actor_role VARCHAR(32) NOT NULL DEFAULT '',
				action VARCHAR(64) NOT NULL,
				detail VARCHAR(1024) NOT NULL DEFAULT '',
				ip VARCHAR(64) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL,
				KEY idx_audit_order (order_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		{
			`CREATE TABLE IF NOT EXISTS refund_requests (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				order_id VARCHAR(128) NOT NULL,
				action VARCHAR(32) NOT NULL,
				amount_cents INTEGER NOT NULL DEFAULT 0,
				reason VARCHAR(512) NOT NULL DEFAULT '',
				status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
				note VARCHAR(512) NOT NULL DEFAULT '',
				created_by INTEGER NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL,
				UNIQUE (order_id, action)
			)`,
			`CREATE TABLE IF NOT EXISTS refund_requests (
				id BIGINT PRIMARY KEY AUTO_INCREMENT,
				order_id VARCHAR(128) NOT NULL,
				action VARCHAR(32) NOT NULL,
				amount_cents BIGINT NOT NULL DEFAULT 0,
				reason VARCHAR(512) NOT NULL DEFAULT '',
				status VARCHAR(16) NOT NULL DEFAULT 'PENDING',
				note VARCHAR(512) NOT NULL DEFAULT '',
				created_by BIGINT NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL,
				UNIQUE KEY uk_refund_order_action (order_id, action)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
	}
	for _, t := range tables {
		if _, err := m.db.Exec(t.sqlite); err != nil {
			if _, err2 := m.db.Exec(t.mysql); err2 != nil {
				return errors.New("创建订单投影表失败（尝试了 SQLite 和 MySQL 格式）")
			}
		}
	}
	return nil
}

// UpsertFromOrder 落订单快照（CREATED 起；幂等）。
func (m *OrderProjectionModel) UpsertFromOrder(p *OrderProjection) error {
	now := nowUTC()
	if p.CreatedAt == "" {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	_, err := m.db.Exec(
		`INSERT INTO order_projection (order_id, game_id, partner_id, account_id, sku_id, product_id, form, amount_cents, base_quantity, currency, status, paid_at, delivered_at, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(order_id) DO UPDATE SET status = excluded.status, paid_at = excluded.paid_at, delivered_at = excluded.delivered_at, updated_at = excluded.updated_at`,
		p.OrderId, p.GameId, p.PartnerId, p.AccountId, p.SkuId, p.ProductId, p.Form, p.AmountCents,
		p.BaseQuantity, p.Currency, p.Status, p.PaidAt, p.DeliveredAt, p.CreatedAt, p.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "syntax") || strings.Contains(err.Error(), "near") {
			// MySQL 回退（ON CONFLICT 为 SQLite 语法）
			_, err2 := m.db.Exec(
				`INSERT INTO order_projection (order_id, game_id, partner_id, account_id, sku_id, product_id, form, amount_cents, base_quantity, currency, status, paid_at, delivered_at, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				 ON DUPLICATE KEY UPDATE status = VALUES(status), paid_at = VALUES(paid_at), delivered_at = VALUES(delivered_at), updated_at = VALUES(updated_at)`,
				p.OrderId, p.GameId, p.PartnerId, p.AccountId, p.SkuId, p.ProductId, p.Form, p.AmountCents,
				p.BaseQuantity, p.Currency, p.Status, p.PaidAt, p.DeliveredAt, p.CreatedAt, p.UpdatedAt,
			)
			return err2
		}
		return err
	}
	return nil
}

// MarkPaid 回调上报 PAID（状态值来自 Courier，非本服务推进）。
func (m *OrderProjectionModel) MarkPaid(orderID, paidAt string) error {
	_, err := m.db.Exec(
		`UPDATE order_projection SET status = 'PAID', paid_at = ?, updated_at = ? WHERE order_id = ?`,
		paidAt, nowUTC(), orderID,
	)
	return err
}

// MarkDelivered 发货投递成功上报 DELIVERED。
func (m *OrderProjectionModel) MarkDelivered(orderID, deliveredAt string) error {
	_, err := m.db.Exec(
		`UPDATE order_projection SET status = 'DELIVERED', delivered_at = ?, updated_at = ? WHERE order_id = ?`,
		deliveredAt, nowUTC(), orderID,
	)
	return err
}

func (m *OrderProjectionModel) Get(orderID string) (*OrderProjection, error) {
	var p OrderProjection
	err := m.db.QueryRow(`SELECT `+projectionColumns+` FROM order_projection WHERE order_id = ?`, orderID).
		Scan(scanProjection(&p)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrOrderProjectionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// ListByQuery 运营检索（投影事实副本；筛选/分页有界）。
func (m *OrderProjectionModel) ListByQuery(gameId, status, accountId string, limit, offset int) ([]*OrderProjection, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := fmt.Sprintf(`SELECT %s FROM order_projection WHERE 1=1`, projectionColumns)
	args := []any{}
	if gameId != "" {
		q += ` AND game_id = ?`
		args = append(args, gameId)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	if accountId != "" {
		q += ` AND account_id = ?`
		args = append(args, accountId)
	}
	q += fmt.Sprintf(` ORDER BY updated_at DESC LIMIT %d OFFSET %d`, limit, offset)
	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*OrderProjection, 0, limit)
	for rows.Next() {
		var p OrderProjection
		if err := rows.Scan(scanProjection(&p)...); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// LogAudit 写操作审计（订单查询/退款请求等管理面动作）。
func (m *OrderProjectionModel) LogAudit(log *OrderAuditLog) error {
	_, err := m.db.Exec(
		`INSERT INTO order_audit_logs (order_id, actor_id, actor_role, action, detail, ip, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		log.OrderId, log.ActorId, log.ActorRole, log.Action, log.Detail, log.Ip, nowUTC(),
	)
	return err
}

// ListAudit 审计列表（orderId 空 = 全部，有界）。
func (m *OrderProjectionModel) ListAudit(orderId string, limit int) ([]*OrderAuditLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, order_id, actor_id, actor_role, action, detail, ip, created_at FROM order_audit_logs WHERE 1=1`
	args := []any{}
	if orderId != "" {
		q += ` AND order_id = ?`
		args = append(args, orderId)
	}
	q += fmt.Sprintf(` ORDER BY id DESC LIMIT %d`, limit)
	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*OrderAuditLog, 0, limit)
	for rows.Next() {
		var a OrderAuditLog
		if err := rows.Scan(&a.Id, &a.OrderId, &a.ActorId, &a.ActorRole, &a.Action, &a.Detail, &a.Ip, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// CreateRefund 登记退款/冲正意图（管理适配接缝；同订单同动作唯一）。
func (m *OrderProjectionModel) CreateRefund(r *RefundRequest) error {
	now := nowUTC()
	r.CreatedAt, r.UpdatedAt = now, now
	_, err := m.db.Exec(
		`INSERT INTO refund_requests (order_id, action, amount_cents, reason, status, note, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.OrderId, r.Action, r.AmountCents, r.Reason, r.Status, r.Note, r.CreatedBy, now, now,
	)
	return err
}

// MarkRefundExecuted 退款执行器置 EXECUTED（Courier 退款端点接入后由执行
// 器调用；现登记保持 PENDING 不伪造成功）。
func (m *OrderProjectionModel) MarkRefundExecuted(orderId, action string) error {
	_, err := m.db.Exec(
		`UPDATE refund_requests SET status = 'EXECUTED', note = ?, updated_at = ? WHERE order_id = ? AND action = ?`,
		"执行完成", nowUTC(), orderId, action,
	)
	return err
}

func (m *OrderProjectionModel) GetRefund(orderId, action string) (*RefundRequest, error) {
	var r RefundRequest
	err := m.db.QueryRow(
		`SELECT id, order_id, action, amount_cents, reason, status, note, created_by, created_at, updated_at FROM refund_requests WHERE order_id = ? AND action = ?`,
		orderId, action,
	).Scan(&r.Id, &r.OrderId, &r.Action, &r.AmountCents, &r.Reason, &r.Status, &r.Note, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrRefundRequestNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// ListRefunds 退款意图列表（状态可筛，有界）。
func (m *OrderProjectionModel) ListRefunds(status string, limit int) ([]*RefundRequest, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, order_id, action, amount_cents, reason, status, note, created_by, created_at, updated_at FROM refund_requests WHERE 1=1`
	args := []any{}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += fmt.Sprintf(` ORDER BY id DESC LIMIT %d`, limit)
	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*RefundRequest, 0, limit)
	for rows.Next() {
		var r RefundRequest
		if err := rows.Scan(&r.Id, &r.OrderId, &r.Action, &r.AmountCents, &r.Reason, &r.Status, &r.Note, &r.CreatedBy, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}
