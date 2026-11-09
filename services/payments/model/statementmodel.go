package model

// GP-205 最小厂商对账单：从只读投影（order_projection 事实副本）按商户
// 维度聚合实付/优惠承担/退款冲减，算合同分成基数与差异；先人工确认、
// CSV 导出，**不自动打款**（F23）。
//
// 事实源仍是 Courier teller（设计 §3.1 交易唯一事实源），本服务不旁路
// 契约：投影由回调/投递上报驱动（无主动拉取，可能滞后于 Courier 事实，
// 对账走查归 GP-206）。优惠承担暂为 0（优惠券子系统未接，GP-305）；
// 退款冲减只计 EXECUTED（执行器待 Courier 退款端点提案，接入前为 0）。

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// 对账单状态（先人工确认，不自动打款）。
const (
	StatementStatusDraft     = "draft"
	StatementStatusConfirmed = "confirmed"
)

// 已付状态（计入实付与订单数；含断单停 PAID，可对账）。
const statementPaidStatuses = "('PAID','DELIVERED','CLOSED')"

var ErrStatementNotFound = errors.New("vendor statement not found")

// VendorStatement 对账单快照（一厂商+一商户+一周期一条；重新生成更新草稿）。
type VendorStatement struct {
	Id            int64  `json:"id"`
	Vendor        string `json:"vendor"`
	PartnerId     int64  `json:"partner_id"`
	PeriodStart   string `json:"period_start"`
	PeriodEnd     string `json:"period_end"`
	PaidTotal     int64  `json:"paid_total"`
	DiscountTotal int64  `json:"discount_total"`
	RefundTotal   int64  `json:"refund_total"`
	SplitBase     int64  `json:"split_base"`
	SplitRateBp   int64  `json:"split_rate_bp"`
	SplitAmount   int64  `json:"split_amount"`
	Settlement    int64  `json:"settlement"`
	Diff          int64  `json:"diff"`
	OrderCount    int    `json:"order_count"`
	Status        string `json:"status"`
	ConfirmedBy   int64  `json:"confirmed_by"`
	ConfirmedAt   string `json:"confirmed_at"`
	CreatedBy     int64  `json:"created_by"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// StatementPaidAggregate 实付聚合（金额 + 订单数）。
type StatementPaidAggregate struct {
	Amount int64
	Count  int
}

// StatementLineItem 对账单明细行（每笔已付订单）。
type StatementLineItem struct {
	OrderId     string
	GameId      string
	AccountId   string
	SkuId       string
	Status      string
	AmountCents int64
	PaidAt      string
}

// StatementModel 对账单数据访问。
type StatementModel struct {
	db *sql.DB
}

func NewStatementModel(db *sql.DB) *StatementModel {
	return &StatementModel{db: db}
}

func (m *StatementModel) CreateTables() error {
	tables := []struct{ sqlite, mysql string }{
		{
			`CREATE TABLE IF NOT EXISTS vendor_statements (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				vendor VARCHAR(64) NOT NULL,
				partner_id INTEGER NOT NULL,
				period_start VARCHAR(32) NOT NULL,
				period_end VARCHAR(32) NOT NULL,
				paid_total INTEGER NOT NULL DEFAULT 0,
				discount_total INTEGER NOT NULL DEFAULT 0,
				refund_total INTEGER NOT NULL DEFAULT 0,
				split_base INTEGER NOT NULL DEFAULT 0,
				split_rate_bp INTEGER NOT NULL DEFAULT 0,
				split_amount INTEGER NOT NULL DEFAULT 0,
				settlement INTEGER NOT NULL DEFAULT 0,
				diff INTEGER NOT NULL DEFAULT 0,
				order_count INTEGER NOT NULL DEFAULT 0,
				status VARCHAR(16) NOT NULL DEFAULT 'draft',
				confirmed_by INTEGER NOT NULL DEFAULT 0,
				confirmed_at VARCHAR(32) NOT NULL DEFAULT '',
				created_by INTEGER NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL,
				UNIQUE (vendor, partner_id, period_start, period_end)
			)`,
			`CREATE TABLE IF NOT EXISTS vendor_statements (
				id BIGINT PRIMARY KEY AUTO_INCREMENT,
				vendor VARCHAR(64) NOT NULL,
				partner_id BIGINT NOT NULL,
				period_start VARCHAR(32) NOT NULL,
				period_end VARCHAR(32) NOT NULL,
				paid_total BIGINT NOT NULL DEFAULT 0,
				discount_total BIGINT NOT NULL DEFAULT 0,
				refund_total BIGINT NOT NULL DEFAULT 0,
				split_base BIGINT NOT NULL DEFAULT 0,
				split_rate_bp BIGINT NOT NULL DEFAULT 0,
				split_amount BIGINT NOT NULL DEFAULT 0,
				settlement BIGINT NOT NULL DEFAULT 0,
				diff BIGINT NOT NULL DEFAULT 0,
				order_count BIGINT NOT NULL DEFAULT 0,
				status VARCHAR(16) NOT NULL DEFAULT 'draft',
				confirmed_by BIGINT NOT NULL DEFAULT 0,
				confirmed_at VARCHAR(32) NOT NULL DEFAULT '',
				created_by BIGINT NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL,
				UNIQUE KEY uk_statement_key (vendor, partner_id, period_start, period_end),
				KEY idx_statement_partner (partner_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
	}
	for _, t := range tables {
		if _, err := m.db.Exec(t.sqlite); err != nil {
			if _, err2 := m.db.Exec(t.mysql); err2 != nil {
				return errors.New("创建厂商对账单表失败（尝试了 SQLite 和 MySQL 格式）")
			}
		}
	}
	return nil
}

const statementColumns = "id, vendor, partner_id, period_start, period_end, paid_total, discount_total, refund_total, split_base, split_rate_bp, split_amount, settlement, diff, order_count, status, confirmed_by, confirmed_at, created_by, created_at, updated_at"

func scanStatement(s *VendorStatement) []any {
	return []any{&s.Id, &s.Vendor, &s.PartnerId, &s.PeriodStart, &s.PeriodEnd,
		&s.PaidTotal, &s.DiscountTotal, &s.RefundTotal, &s.SplitBase, &s.SplitRateBp,
		&s.SplitAmount, &s.Settlement, &s.Diff, &s.OrderCount, &s.Status,
		&s.ConfirmedBy, &s.ConfirmedAt, &s.CreatedBy, &s.CreatedAt, &s.UpdatedAt}
}

// UpsertStatement 落对账单快照（同 key 更新草稿；已确认的由逻辑层拒绝）。
func (m *StatementModel) UpsertStatement(s *VendorStatement) error {
	now := nowUTC()
	if s.CreatedAt == "" {
		s.CreatedAt = now
	}
	s.UpdatedAt = now
	_, err := m.db.Exec(
		`INSERT INTO vendor_statements (vendor, partner_id, period_start, period_end, paid_total, discount_total, refund_total, split_base, split_rate_bp, split_amount, settlement, diff, order_count, status, confirmed_by, confirmed_at, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(vendor, partner_id, period_start, period_end) DO UPDATE SET
		   paid_total = excluded.paid_total, discount_total = excluded.discount_total,
		   refund_total = excluded.refund_total, split_base = excluded.split_base,
		   split_rate_bp = excluded.split_rate_bp, split_amount = excluded.split_amount,
		   settlement = excluded.settlement, diff = excluded.diff,
		   order_count = excluded.order_count, updated_at = excluded.updated_at`,
		s.Vendor, s.PartnerId, s.PeriodStart, s.PeriodEnd, s.PaidTotal, s.DiscountTotal,
		s.RefundTotal, s.SplitBase, s.SplitRateBp, s.SplitAmount, s.Settlement, s.Diff,
		s.OrderCount, s.Status, s.ConfirmedBy, s.ConfirmedAt, s.CreatedBy, s.CreatedAt, s.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "syntax") || strings.Contains(err.Error(), "near") {
			_, err2 := m.db.Exec(
				`INSERT INTO vendor_statements (vendor, partner_id, period_start, period_end, paid_total, discount_total, refund_total, split_base, split_rate_bp, split_amount, settlement, diff, order_count, status, confirmed_by, confirmed_at, created_by, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
				 ON DUPLICATE KEY UPDATE
				   paid_total = VALUES(paid_total), discount_total = VALUES(discount_total),
				   refund_total = VALUES(refund_total), split_base = VALUES(split_base),
				   split_rate_bp = VALUES(split_rate_bp), split_amount = VALUES(split_amount),
				   settlement = VALUES(settlement), diff = VALUES(diff),
				   order_count = VALUES(order_count), updated_at = VALUES(updated_at)`,
				s.Vendor, s.PartnerId, s.PeriodStart, s.PeriodEnd, s.PaidTotal, s.DiscountTotal,
				s.RefundTotal, s.SplitBase, s.SplitRateBp, s.SplitAmount, s.Settlement, s.Diff,
				s.OrderCount, s.Status, s.ConfirmedBy, s.ConfirmedAt, s.CreatedBy, s.CreatedAt, s.UpdatedAt,
			)
			return err2
		}
		return err
	}
	return nil
}

func (m *StatementModel) Get(id int64) (*VendorStatement, error) {
	var s VendorStatement
	err := m.db.QueryRow(`SELECT `+statementColumns+` FROM vendor_statements WHERE id = ?`, id).Scan(scanStatement(&s)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrStatementNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// GetByKey 按厂商+商户+周期取（重生成检测；不存在返回 ErrStatementNotFound）。
func (m *StatementModel) GetByKey(vendor string, partnerID int64, start, end string) (*VendorStatement, error) {
	var s VendorStatement
	err := m.db.QueryRow(
		`SELECT `+statementColumns+` FROM vendor_statements WHERE vendor = ? AND partner_id = ? AND period_start = ? AND period_end = ?`,
		vendor, partnerID, start, end,
	).Scan(scanStatement(&s)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrStatementNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListStatements 对账单列表（vendor/partner/status 可筛，有界）。
func (m *StatementModel) ListStatements(vendor string, partnerID int64, status string, limit, offset int) ([]*VendorStatement, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := `SELECT ` + statementColumns + ` FROM vendor_statements WHERE 1=1`
	args := []any{}
	if vendor != "" {
		q += ` AND vendor = ?`
		args = append(args, vendor)
	}
	if partnerID > 0 {
		q += ` AND partner_id = ?`
		args = append(args, partnerID)
	}
	if status != "" {
		q += ` AND status = ?`
		args = append(args, status)
	}
	q += fmt.Sprintf(` ORDER BY id DESC LIMIT %d OFFSET %d`, limit, offset)
	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*VendorStatement, 0, limit)
	for rows.Next() {
		var s VendorStatement
		if err := rows.Scan(scanStatement(&s)...); err != nil {
			return nil, err
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

// UpdateSettlement 人工录入渠道结算单金额并重算差异（确认后由逻辑层拒绝）。
func (m *StatementModel) UpdateSettlement(id, settlement, diff int64) error {
	_, err := m.db.Exec(
		`UPDATE vendor_statements SET settlement = ?, diff = ?, updated_at = ? WHERE id = ?`,
		settlement, diff, nowUTC(), id,
	)
	return err
}

// ConfirmStatement 人工确认（不自动打款；先确认后不可改结算单/重生成）。
func (m *StatementModel) ConfirmStatement(id, by int64) error {
	_, err := m.db.Exec(
		`UPDATE vendor_statements SET status = 'confirmed', confirmed_by = ?, confirmed_at = ?, updated_at = ? WHERE id = ?`,
		by, nowUTC(), nowUTC(), id,
	)
	return err
}

// StatementPaidAggregate 周期内实付聚合（已付订单金额与单数）。
func (m *StatementModel) StatementPaidAggregate(partnerID int64, start, end string) (*StatementPaidAggregate, error) {
	var a StatementPaidAggregate
	err := m.db.QueryRow(
		`SELECT COALESCE(SUM(amount_cents), 0), COUNT(*) FROM order_projection
		 WHERE partner_id = ? AND status IN `+statementPaidStatuses+` AND paid_at >= ? AND paid_at <= ?`,
		partnerID, start, end,
	).Scan(&a.Amount, &a.Count)
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// StatementRefundAggregate 周期内已执行退款冲减（EXECUTED；执行器接入前为 0）。
func (m *StatementModel) StatementRefundAggregate(partnerID int64, start, end string) (int64, error) {
	var total int64
	err := m.db.QueryRow(
		`SELECT COALESCE(SUM(r.amount_cents), 0) FROM refund_requests r
		 JOIN order_projection p ON p.order_id = r.order_id
		 WHERE p.partner_id = ? AND r.status = 'EXECUTED' AND r.created_at >= ? AND r.created_at <= ?`,
		partnerID, start, end,
	).Scan(&total)
	return total, err
}

// StatementLineItems 对账单明细（每笔已付订单，按实付时间升序）。
func (m *StatementModel) StatementLineItems(partnerID int64, start, end string) ([]*StatementLineItem, error) {
	rows, err := m.db.Query(
		`SELECT order_id, game_id, account_id, sku_id, status, amount_cents, paid_at
		 FROM order_projection
		 WHERE partner_id = ? AND status IN `+statementPaidStatuses+` AND paid_at >= ? AND paid_at <= ?
		 ORDER BY paid_at ASC`,
		partnerID, start, end,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*StatementLineItem, 0)
	for rows.Next() {
		var it StatementLineItem
		if err := rows.Scan(&it.OrderId, &it.GameId, &it.AccountId, &it.SkuId, &it.Status, &it.AmountCents, &it.PaidAt); err != nil {
			return nil, err
		}
		out = append(out, &it)
	}
	return out, rows.Err()
}
