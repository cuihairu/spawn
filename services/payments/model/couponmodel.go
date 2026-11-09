package model

// GP-305 优惠券域：定义（coupons 服务端配置，不信任客户端金额）、领取
// （user_coupons，request_id 幂等 + per_user_limit）、核销关联
// （coupon_order_links，订单↔券 优惠承担账本）。
//
// 报价语义（兼容 Courier skuId 下单契约）：客户端下单仅提交 sku_id +
// coupon_ids，金额一律服务端算——门槛（list ≥ threshold_cents）、折扣
// （fixed 直减 / percent_bp 按基点向下取整，上限不超 list）、叠加（仅
// stackable 可合并，非 stackable 单张）。Courier Frozen v1.1 下单仅 {skuId}
// 只读，折后价不能下传给 Courier，故折扣记为本侧「优惠承担」账本
// （coupon_order_links.discount_cents），供 GP-205 分成基数（split_base=
// 实付-优惠-退款）冲减——Courier 侧实收仍为列表价，见 todo（实际到账
// 折扣需 Courier 促销支持，Frozen 只读不可改）。
//
// 状态机：claimed（领取）→ locked（下单占位）→ used（PAID 核销）→
// claimed（退款 EXECUTED 返还，券可复用、有效期仍按 coupon.end_at）。

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrCouponNotFound      = errors.New("coupon not found")
	ErrUserCouponNotFound  = errors.New("user coupon not found")
	ErrCouponStateConflict = errors.New("coupon state conflict")
)

// 券状态 / 折扣类型。
const (
	CouponStatusDraft     = "draft"
	CouponStatusPublished = "published"
	CouponStatusDisabled  = "disabled"

	CouponDiscountFixed     = "fixed"      // 直减（discount_value 分）
	CouponDiscountPercentBp = "percent_bp" // 按基点（discount_value = bp，10000 = 100%）

	UserCouponStatusClaimed = "claimed"
	UserCouponStatusLocked  = "locked"
	UserCouponStatusUsed    = "used"
	UserCouponStatusExpired = "expired"
)

// Coupon 优惠券定义（服务端配置域）。
type Coupon struct {
	Id             int64  `json:"id"`
	Code           string `json:"code"`
	Name           string `json:"name"`
	GameId         string `json:"game_id"`
	SkuId          string `json:"sku_id"` // 空 = 全部 sku
	ThresholdCents int64  `json:"threshold_cents"`
	DiscountType   string `json:"discount_type"`
	DiscountValue  int64  `json:"discount_value"`
	Stackable      bool   `json:"stackable"`
	PerUserLimit   int64  `json:"per_user_limit"`
	TotalCount     int64  `json:"total_count"`
	StartAt        string `json:"start_at"`
	EndAt          string `json:"end_at"`
	Status         string `json:"status"`
	CreatedBy      int64  `json:"created_by"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// UserCoupon 已领取券（账号维度）。
type UserCoupon struct {
	Id            int64  `json:"id"`
	CouponId      int64  `json:"coupon_id"`
	UserId        int64  `json:"user_id"`
	RequestId     string `json:"request_id"`
	Status        string `json:"status"`
	ClaimedAt     string `json:"claimed_at"`
	LockedOrderId string `json:"locked_order_id"`
	UsedOrderId   string `json:"used_order_id"`
	UsedAt        string `json:"used_at"`
}

// CouponOrderLink 订单↔券核销关联（优惠承担账本）。
type CouponOrderLink struct {
	Id            int64  `json:"id"`
	OrderId       string `json:"order_id"`
	CouponId      int64  `json:"coupon_id"`
	UserCouponId  int64  `json:"user_coupon_id"`
	DiscountCents int64  `json:"discount_cents"`
	Snapshot      string `json:"snapshot"`
	CreatedAt     string `json:"created_at"`
}

// CouponModel 优惠券域数据访问（三表共用同一 db）。
type CouponModel struct {
	db *sql.DB
}

func NewCouponModel(db *sql.DB) *CouponModel {
	return &CouponModel{db: db}
}

func (m *CouponModel) CreateTables() error {
	tables := []struct{ sqlite, mysql string }{
		{
			`CREATE TABLE IF NOT EXISTS coupons (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				code VARCHAR(64) NOT NULL UNIQUE,
				name VARCHAR(128) NOT NULL,
				game_id VARCHAR(64) NOT NULL,
				sku_id VARCHAR(64) NOT NULL DEFAULT '',
				threshold_cents INTEGER NOT NULL DEFAULT 0,
				discount_type VARCHAR(16) NOT NULL,
				discount_value INTEGER NOT NULL DEFAULT 0,
				stackable INTEGER NOT NULL DEFAULT 0,
				per_user_limit INTEGER NOT NULL DEFAULT 1,
				total_count INTEGER NOT NULL DEFAULT 0,
				start_at VARCHAR(32) NOT NULL,
				end_at VARCHAR(32) NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'draft',
				created_by INTEGER NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS coupons (
				id BIGINT PRIMARY KEY AUTO_INCREMENT,
				code VARCHAR(64) NOT NULL,
				name VARCHAR(128) NOT NULL,
				game_id VARCHAR(64) NOT NULL,
				sku_id VARCHAR(64) NOT NULL DEFAULT '',
				threshold_cents BIGINT NOT NULL DEFAULT 0,
				discount_type VARCHAR(16) NOT NULL,
				discount_value BIGINT NOT NULL DEFAULT 0,
				stackable TINYINT(1) NOT NULL DEFAULT 0,
				per_user_limit BIGINT NOT NULL DEFAULT 1,
				total_count BIGINT NOT NULL DEFAULT 0,
				start_at VARCHAR(32) NOT NULL,
				end_at VARCHAR(32) NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'draft',
				created_by BIGINT NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL,
				UNIQUE KEY uk_coupons_code (code),
				KEY idx_coupons_game_status (game_id, status)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		{
			`CREATE TABLE IF NOT EXISTS user_coupons (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				coupon_id INTEGER NOT NULL,
				user_id INTEGER NOT NULL,
				request_id VARCHAR(128) NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'claimed',
				claimed_at VARCHAR(32) NOT NULL,
				locked_order_id VARCHAR(128) NOT NULL DEFAULT '',
				used_order_id VARCHAR(128) NOT NULL DEFAULT '',
				used_at VARCHAR(32) NOT NULL DEFAULT '',
				UNIQUE (coupon_id, user_id, request_id)
			)`,
			`CREATE TABLE IF NOT EXISTS user_coupons (
				id BIGINT PRIMARY KEY AUTO_INCREMENT,
				coupon_id BIGINT NOT NULL,
				user_id BIGINT NOT NULL,
				request_id VARCHAR(128) NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'claimed',
				claimed_at VARCHAR(32) NOT NULL,
				locked_order_id VARCHAR(128) NOT NULL DEFAULT '',
				used_order_id VARCHAR(128) NOT NULL DEFAULT '',
				used_at VARCHAR(32) NOT NULL DEFAULT '',
				UNIQUE KEY uk_user_coupon_request (coupon_id, user_id, request_id),
				KEY idx_user_coupons_user (user_id, status)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		{
			`CREATE TABLE IF NOT EXISTS coupon_order_links (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				order_id VARCHAR(128) NOT NULL,
				coupon_id INTEGER NOT NULL,
				user_coupon_id INTEGER NOT NULL,
				discount_cents INTEGER NOT NULL DEFAULT 0,
				snapshot TEXT NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL,
				UNIQUE (order_id, user_coupon_id)
			)`,
			`CREATE TABLE IF NOT EXISTS coupon_order_links (
				id BIGINT PRIMARY KEY AUTO_INCREMENT,
				order_id VARCHAR(128) NOT NULL,
				coupon_id BIGINT NOT NULL,
				user_coupon_id BIGINT NOT NULL,
				discount_cents BIGINT NOT NULL DEFAULT 0,
				snapshot TEXT NOT NULL,
				created_at VARCHAR(32) NOT NULL,
				UNIQUE KEY uk_link_order_user (order_id, user_coupon_id),
				KEY idx_links_order (order_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
	}
	for _, t := range tables {
		if _, err := m.db.Exec(t.sqlite); err != nil {
			if _, err2 := m.db.Exec(t.mysql); err2 != nil {
				return errors.New("创建优惠券表失败（尝试了 SQLite 和 MySQL 格式）")
			}
		}
	}
	return nil
}

const couponColumns = "id, code, name, game_id, sku_id, threshold_cents, discount_type, discount_value, stackable, per_user_limit, total_count, start_at, end_at, status, created_by, created_at, updated_at"

func scanCoupon(c *Coupon) []any {
	return []any{&c.Id, &c.Code, &c.Name, &c.GameId, &c.SkuId, &c.ThresholdCents,
		&c.DiscountType, &c.DiscountValue, &c.Stackable, &c.PerUserLimit, &c.TotalCount,
		&c.StartAt, &c.EndAt, &c.Status, &c.CreatedBy, &c.CreatedAt, &c.UpdatedAt}
}

const userCouponColumns = "id, coupon_id, user_id, request_id, status, claimed_at, locked_order_id, used_order_id, used_at"

func scanUserCoupon(u *UserCoupon) []any {
	return []any{&u.Id, &u.CouponId, &u.UserId, &u.RequestId, &u.Status, &u.ClaimedAt,
		&u.LockedOrderId, &u.UsedOrderId, &u.UsedAt}
}

const couponLinkColumns = "id, order_id, coupon_id, user_coupon_id, discount_cents, snapshot, created_at"

func scanCouponLink(l *CouponOrderLink) []any {
	return []any{&l.Id, &l.OrderId, &l.CouponId, &l.UserCouponId, &l.DiscountCents,
		&l.Snapshot, &l.CreatedAt}
}

func (m *CouponModel) CreateCoupon(c *Coupon) error {
	now := nowUTC()
	c.CreatedAt, c.UpdatedAt = now, now
	res, err := m.db.Exec(
		`INSERT INTO coupons (code, name, game_id, sku_id, threshold_cents, discount_type, discount_value, stackable, per_user_limit, total_count, start_at, end_at, status, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.Code, c.Name, c.GameId, c.SkuId, c.ThresholdCents, c.DiscountType, c.DiscountValue,
		c.Stackable, c.PerUserLimit, c.TotalCount, c.StartAt, c.EndAt, c.Status, c.CreatedBy, now, now)
	if err != nil {
		return err
	}
	if id, err := res.LastInsertId(); err == nil && id > 0 {
		c.Id = id
	}
	return nil
}

func (m *CouponModel) UpdateCoupon(c *Coupon) error {
	_, err := m.db.Exec(
		`UPDATE coupons SET code=?, name=?, game_id=?, sku_id=?, threshold_cents=?, discount_type=?, discount_value=?, stackable=?, per_user_limit=?, total_count=?, start_at=?, end_at=?, updated_at=? WHERE id=?`,
		c.Code, c.Name, c.GameId, c.SkuId, c.ThresholdCents, c.DiscountType, c.DiscountValue,
		c.Stackable, c.PerUserLimit, c.TotalCount, c.StartAt, c.EndAt, nowUTC(), c.Id)
	return err
}

func (m *CouponModel) SetCouponStatus(id int64, status string) error {
	_, err := m.db.Exec(`UPDATE coupons SET status=?, updated_at=? WHERE id=?`, status, nowUTC(), id)
	return err
}

func (m *CouponModel) GetCoupon(id int64) (*Coupon, error) {
	var c Coupon
	err := m.db.QueryRow(`SELECT `+couponColumns+` FROM coupons WHERE id=?`, id).
		Scan(scanCoupon(&c)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCouponNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

func (m *CouponModel) GetCouponByCode(code string) (*Coupon, error) {
	var c Coupon
	err := m.db.QueryRow(`SELECT `+couponColumns+` FROM coupons WHERE code=?`, code).
		Scan(scanCoupon(&c)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrCouponNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListCoupons 管理面列表（game_id/status 可筛，有界）。
func (m *CouponModel) ListCoupons(gameID, status string, limit int) ([]*Coupon, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	where, args := []string{"1=1"}, []any{}
	if gameID != "" {
		where = append(where, "game_id = ?")
		args = append(args, gameID)
	}
	if status != "" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	rows, err := m.db.Query(`SELECT `+couponColumns+` FROM coupons WHERE `+
		strings.Join(where, " AND ")+` ORDER BY id DESC LIMIT `+fmt.Sprintf("%d", limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Coupon
	for rows.Next() {
		var c Coupon
		if err := rows.Scan(scanCoupon(&c)...); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// ClaimCoupon 领取（幂等：同 (coupon_id,user_id,request_id) 返回既有）。
// created=true 表示本次确实新插入（供调用方做限领计数）；false 为幂等重放
// 返回既有行。
func (m *CouponModel) ClaimCoupon(c *UserCoupon) (*UserCoupon, bool, error) {
	c.ClaimedAt = nowUTC()
	res, err := m.db.Exec(
		`INSERT OR IGNORE INTO user_coupons (coupon_id, user_id, request_id, status, claimed_at)
		 VALUES (?, ?, ?, ?, ?)`,
		c.CouponId, c.UserId, c.RequestId, UserCouponStatusClaimed, c.ClaimedAt)
	if err != nil {
		if strings.Contains(err.Error(), "syntax") || strings.Contains(err.Error(), "near") {
			res, err = m.db.Exec(
				`INSERT IGNORE INTO user_coupons (coupon_id, user_id, request_id, status, claimed_at)
				 VALUES (?, ?, ?, ?, ?)`,
				c.CouponId, c.UserId, c.RequestId, UserCouponStatusClaimed, c.ClaimedAt)
		}
		if err != nil {
			return nil, false, err
		}
	}
	if n, _ := res.RowsAffected(); n == 1 {
		if id, err := res.LastInsertId(); err == nil && id > 0 {
			c.Id = id
		}
		c.Status = UserCouponStatusClaimed
		return c, true, nil
	}
	existing, err := m.GetUserCouponByRequest(c.CouponId, c.UserId, c.RequestId)
	return existing, false, err
}

func (m *CouponModel) GetUserCoupon(id int64) (*UserCoupon, error) {
	var u UserCoupon
	err := m.db.QueryRow(`SELECT `+userCouponColumns+` FROM user_coupons WHERE id=?`, id).
		Scan(scanUserCoupon(&u)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserCouponNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// GetClaimedUserCoupon 下单锁券取该用户在券下任一 claimed 实例（限领>1 时
// 实例等价，任取其一）；查询按 user_id 收敛，他人持有的实例不可见。
func (m *CouponModel) GetClaimedUserCoupon(couponID, userID int64) (*UserCoupon, error) {
	var u UserCoupon
	err := m.db.QueryRow(
		`SELECT `+userCouponColumns+` FROM user_coupons WHERE coupon_id=? AND user_id=? AND status=? ORDER BY id LIMIT 1`,
		couponID, userID, UserCouponStatusClaimed).Scan(scanUserCoupon(&u)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserCouponNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (m *CouponModel) GetUserCouponByRequest(couponID, userID int64, requestID string) (*UserCoupon, error) {
	var u UserCoupon
	err := m.db.QueryRow(
		`SELECT `+userCouponColumns+` FROM user_coupons WHERE coupon_id=? AND user_id=? AND request_id=?`,
		couponID, userID, requestID).Scan(scanUserCoupon(&u)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrUserCouponNotFound
	}
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (m *CouponModel) CountUserCouponClaims(couponID, userID int64) (int64, error) {
	var n int64
	err := m.db.QueryRow(`SELECT COUNT(*) FROM user_coupons WHERE coupon_id=? AND user_id=?`, couponID, userID).Scan(&n)
	return n, err
}

func (m *CouponModel) CountCouponUsed(couponID int64) (int64, error) {
	var n int64
	err := m.db.QueryRow(`SELECT COUNT(*) FROM user_coupons WHERE coupon_id=? AND status=?`, couponID, UserCouponStatusUsed).Scan(&n)
	return n, err
}

// LockUserCoupon 下单占位：claimed → locked（并发安全：条件 UPDATE）。
func (m *CouponModel) LockUserCoupon(userCouponID int64, orderID string) error {
	res, err := m.db.Exec(
		`UPDATE user_coupons SET status=?, locked_order_id=?, used_at='' WHERE id=? AND status=?`,
		UserCouponStatusLocked, orderID, userCouponID, UserCouponStatusClaimed)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrCouponStateConflict
	}
	return nil
}

// UnlockUserCoupon 下单失败回滚：locked → claimed。
func (m *CouponModel) UnlockUserCoupon(userCouponID int64) error {
	_, err := m.db.Exec(
		`UPDATE user_coupons SET status=?, locked_order_id='' WHERE id=? AND status=?`,
		UserCouponStatusClaimed, userCouponID, UserCouponStatusLocked)
	return err
}

// MarkCouponUsedByOrder PAID 核销：locked → used（订单归属校验）。
func (m *CouponModel) MarkCouponUsedByOrder(orderID string) error {
	_, err := m.db.Exec(
		`UPDATE user_coupons SET status=?, used_order_id=locked_order_id, used_at=? WHERE status=? AND locked_order_id=?`,
		UserCouponStatusUsed, nowUTC(), UserCouponStatusLocked, orderID)
	return err
}

// ReturnCouponsByOrder 退款返还：used/locked → claimed（券可复用）。
func (m *CouponModel) ReturnCouponsByOrder(orderID string) error {
	_, err := m.db.Exec(
		`UPDATE user_coupons SET status=?, locked_order_id='', used_order_id='', used_at='' WHERE (status=? OR status=?) AND (locked_order_id=? OR used_order_id=?)`,
		UserCouponStatusClaimed, UserCouponStatusUsed, UserCouponStatusLocked, orderID, orderID)
	return err
}

func (m *CouponModel) LinkOrderCoupon(l *CouponOrderLink) error {
	l.CreatedAt = nowUTC()
	_, err := m.db.Exec(
		`INSERT OR IGNORE INTO coupon_order_links (order_id, coupon_id, user_coupon_id, discount_cents, snapshot, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		l.OrderId, l.CouponId, l.UserCouponId, l.DiscountCents, l.Snapshot, l.CreatedAt)
	if err != nil {
		if strings.Contains(err.Error(), "syntax") || strings.Contains(err.Error(), "near") {
			_, err = m.db.Exec(
				`INSERT IGNORE INTO coupon_order_links (order_id, coupon_id, user_coupon_id, discount_cents, snapshot, created_at)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				l.OrderId, l.CouponId, l.UserCouponId, l.DiscountCents, l.Snapshot, l.CreatedAt)
		}
		return err
	}
	return nil
}

func (m *CouponModel) ListLinksByOrder(orderID string) ([]*CouponOrderLink, error) {
	rows, err := m.db.Query(`SELECT `+couponLinkColumns+` FROM coupon_order_links WHERE order_id=? ORDER BY id`, orderID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*CouponOrderLink
	for rows.Next() {
		var l CouponOrderLink
		if err := rows.Scan(scanCouponLink(&l)...); err != nil {
			return nil, err
		}
		out = append(out, &l)
	}
	return out, rows.Err()
}

// ListUserCoupons 我的券（status 可筛，有界 100）。
func (m *CouponModel) ListUserCoupons(userID int64, status string, limit int) ([]*UserCoupon, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	where, args := []string{"user_id = ?"}, []any{userID}
	if status != "" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	rows, err := m.db.Query(`SELECT `+userCouponColumns+` FROM user_coupons WHERE `+
		strings.Join(where, " AND ")+` ORDER BY id DESC LIMIT `+fmt.Sprintf("%d", limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*UserCoupon
	for rows.Next() {
		var u UserCoupon
		if err := rows.Scan(scanUserCoupon(&u)...); err != nil {
			return nil, err
		}
		out = append(out, &u)
	}
	return out, rows.Err()
}
