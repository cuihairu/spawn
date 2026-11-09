package model

// GP-302 奖励发放账本：按 (order_id, activity_id) 唯一——重复评估/重复事件
// 不重复发放（幂等发奖）。行内保存订单与奖励快照（命中的活动版本、倍率
// 语义、基础/额外/总量、目标与实付），后改活动不改变旧账本；活动退场后
// 查询记录不丢。赠币不是现金收入，不进分成基数。

import (
	"database/sql"
	"errors"
	"strings"
)

const (
	RewardStatusGranted = "granted"
	RewardStatusRevoked = "revoked" // 退款后的奖励撤销归 GP-303，本项预留状态
)

// ErrRewardGrantNotFound 账本行不存在。
var ErrRewardGrantNotFound = errors.New("reward grant not found")

// RewardGrant 奖励发放账本行。
type RewardGrant struct {
	Id              int64  `json:"id"`
	OrderId         string `json:"order_id"`
	ActivityId      int64  `json:"activity_id"`
	ActivityVersion int64  `json:"activity_version"`
	GameId          string `json:"game_id"`
	ServerId        int64  `json:"server_id"`
	RoleId          string `json:"role_id"`
	AccountId       string `json:"account_id"`
	SkuId           string `json:"sku_id"`
	PaidCents       int64  `json:"paid_cents"`
	Currency        string `json:"currency"`
	BaseQuantity    int64  `json:"base_quantity"`
	ExtraQuantity   int64  `json:"extra_quantity"`
	TotalQuantity   int64  `json:"total_quantity"`
	RateMode        string `json:"rate_mode"` // total | extra
	RateBp          int64  `json:"rate_bp"`
	Status          string `json:"status"`
	Snapshot        string `json:"snapshot"` // 订单+奖励快照 JSON（调研 §5）
	GrantedAt       string `json:"granted_at"`
	CreatedAt       string `json:"created_at"`
}

// RewardGrantModel 奖励发放账本。
type RewardGrantModel struct {
	db *sql.DB
}

// NewRewardGrantModel 构造账本模型。
func NewRewardGrantModel(db *sql.DB) *RewardGrantModel {
	return &RewardGrantModel{db: db}
}

// CreateTables 建账本表（SQLite 先行、失败回 MySQL，与 GP-301 同款；
// UNIQUE(order_id, activity_id) 是幂等发奖的锚点）。
func (m *RewardGrantModel) CreateTables() error {
	tables := []struct{ sqlite, mysql string }{
		{
			`CREATE TABLE IF NOT EXISTS reward_grants (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				order_id VARCHAR(64) NOT NULL,
				activity_id INTEGER NOT NULL,
				activity_version INTEGER NOT NULL,
				game_id VARCHAR(64) NOT NULL,
				server_id INTEGER NOT NULL DEFAULT 0,
				role_id VARCHAR(128) NOT NULL DEFAULT '',
				account_id VARCHAR(64) NOT NULL,
				sku_id VARCHAR(128) NOT NULL DEFAULT '',
				paid_cents INTEGER NOT NULL DEFAULT 0,
				currency VARCHAR(8) NOT NULL DEFAULT '',
				base_quantity INTEGER NOT NULL DEFAULT 0,
				extra_quantity INTEGER NOT NULL DEFAULT 0,
				total_quantity INTEGER NOT NULL DEFAULT 0,
				rate_mode VARCHAR(16) NOT NULL,
				rate_bp INTEGER NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'granted',
				snapshot TEXT NOT NULL DEFAULT '',
				granted_at VARCHAR(32) NOT NULL,
				created_at VARCHAR(32) NOT NULL,
				UNIQUE (order_id, activity_id)
			)`,
			`CREATE TABLE IF NOT EXISTS reward_grants (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				order_id VARCHAR(64) NOT NULL,
				activity_id BIGINT NOT NULL,
				activity_version BIGINT NOT NULL,
				game_id VARCHAR(64) NOT NULL,
				server_id BIGINT NOT NULL DEFAULT 0,
				role_id VARCHAR(128) NOT NULL DEFAULT '',
				account_id VARCHAR(64) NOT NULL,
				sku_id VARCHAR(128) NOT NULL DEFAULT '',
				paid_cents BIGINT NOT NULL DEFAULT 0,
				currency VARCHAR(8) NOT NULL DEFAULT '',
				base_quantity BIGINT NOT NULL DEFAULT 0,
				extra_quantity BIGINT NOT NULL DEFAULT 0,
				total_quantity BIGINT NOT NULL DEFAULT 0,
				rate_mode VARCHAR(16) NOT NULL,
				rate_bp BIGINT NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'granted',
				snapshot TEXT NOT NULL,
				granted_at VARCHAR(32) NOT NULL,
				created_at VARCHAR(32) NOT NULL,
				UNIQUE KEY uk_order_activity (order_id, activity_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
	}
	for _, t := range tables {
		if _, err := m.db.Exec(t.sqlite); err != nil {
			if _, err2 := m.db.Exec(t.mysql); err2 != nil {
				return errors.New("创建奖励账本表失败（尝试了 SQLite 和 MySQL 格式）")
			}
		}
	}
	return nil
}

const grantColumns = "id, order_id, activity_id, activity_version, game_id, server_id, role_id, account_id, sku_id, paid_cents, currency, base_quantity, extra_quantity, total_quantity, rate_mode, rate_bp, status, snapshot, granted_at, created_at"

func scanGrant(g *RewardGrant) []any {
	return []any{&g.Id, &g.OrderId, &g.ActivityId, &g.ActivityVersion, &g.GameId,
		&g.ServerId, &g.RoleId, &g.AccountId, &g.SkuId, &g.PaidCents, &g.Currency,
		&g.BaseQuantity, &g.ExtraQuantity, &g.TotalQuantity, &g.RateMode, &g.RateBp,
		&g.Status, &g.Snapshot, &g.GrantedAt, &g.CreatedAt}
}

// InsertGrant 幂等插入：UNIQUE 冲突视为已发放（created=false），不报错。
// 先 SQLite INSERT OR IGNORE，语法错回 MySQL INSERT IGNORE（与对账单同款）。
func (m *RewardGrantModel) InsertGrant(g *RewardGrant) (bool, error) {
	now := nowUTC()
	g.CreatedAt = now
	res, err := m.db.Exec(
		`INSERT OR IGNORE INTO reward_grants (order_id, activity_id, activity_version, game_id, server_id, role_id, account_id, sku_id, paid_cents, currency, base_quantity, extra_quantity, total_quantity, rate_mode, rate_bp, status, snapshot, granted_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		g.OrderId, g.ActivityId, g.ActivityVersion, g.GameId, g.ServerId, g.RoleId,
		g.AccountId, g.SkuId, g.PaidCents, g.Currency, g.BaseQuantity,
		g.ExtraQuantity, g.TotalQuantity, g.RateMode, g.RateBp, g.Status,
		g.Snapshot, g.GrantedAt, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "syntax") || strings.Contains(err.Error(), "near") {
			res, err = m.db.Exec(
				`INSERT IGNORE INTO reward_grants (order_id, activity_id, activity_version, game_id, server_id, role_id, account_id, sku_id, paid_cents, currency, base_quantity, extra_quantity, total_quantity, rate_mode, rate_bp, status, snapshot, granted_at, created_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				g.OrderId, g.ActivityId, g.ActivityVersion, g.GameId, g.ServerId, g.RoleId,
				g.AccountId, g.SkuId, g.PaidCents, g.Currency, g.BaseQuantity,
				g.ExtraQuantity, g.TotalQuantity, g.RateMode, g.RateBp, g.Status,
				g.Snapshot, g.GrantedAt, now,
			)
		}
		if err != nil {
			return false, err
		}
	}
	created, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return created > 0, nil
}

// GetGrant 取单条账本行（幂等重放时返回已发放事实）。
func (m *RewardGrantModel) GetGrant(orderID string, activityID int64) (*RewardGrant, error) {
	row := m.db.QueryRow(
		`SELECT `+grantColumns+` FROM reward_grants WHERE order_id = ? AND activity_id = ?`,
		orderID, activityID)
	var g RewardGrant
	if err := row.Scan(scanGrant(&g)...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrRewardGrantNotFound
		}
		return nil, err
	}
	return &g, nil
}

// ListGrants 管理面查询（全为可选过滤；有界 100 条，id 降序）。
func (m *RewardGrantModel) ListGrants(activityID int64, orderID, gameID, accountID string) ([]RewardGrant, error) {
	where, args := []string{"1=1"}, []any{}
	if activityID > 0 {
		where = append(where, "activity_id = ?")
		args = append(args, activityID)
	}
	if orderID != "" {
		where = append(where, "order_id = ?")
		args = append(args, orderID)
	}
	if gameID != "" {
		where = append(where, "game_id = ?")
		args = append(args, gameID)
	}
	if accountID != "" {
		where = append(where, "account_id = ?")
		args = append(args, accountID)
	}
	rows, err := m.db.Query(
		`SELECT `+grantColumns+` FROM reward_grants WHERE `+strings.Join(where, " AND ")+
			` ORDER BY id DESC LIMIT 100`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RewardGrant
	for rows.Next() {
		var g RewardGrant
		if err := rows.Scan(scanGrant(&g)...); err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

// CountGrantsByUser 单用户单活动已发放条数（per_user_count 限次消费）。
func (m *RewardGrantModel) CountGrantsByUser(activityID int64, accountID string) (int64, error) {
	var n int64
	err := m.db.QueryRow(
		`SELECT COUNT(*) FROM reward_grants WHERE activity_id = ? AND account_id = ? AND status = ?`,
		activityID, accountID, RewardStatusGranted).Scan(&n)
	return n, err
}

// CountGrantsByActivity 单活动已发放总条数（global_count 限次消费）。
func (m *RewardGrantModel) CountGrantsByActivity(activityID int64) (int64, error) {
	var n int64
	err := m.db.QueryRow(
		`SELECT COUNT(*) FROM reward_grants WHERE activity_id = ? AND status = ?`,
		activityID, RewardStatusGranted).Scan(&n)
	return n, err
}
