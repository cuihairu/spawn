package model

// GP-304 礼包域：礼包展示（gift_packs 配置）、码库存（gift_pack_codes）、
// 领取记录（gift_pack_claims，含 per_user 限领与幂等）。游戏内兑换边界：
// 领取产生领取记录 +（码包）核销码；实际内容发放走游戏服接缝（GP-203
// 同款签名回调）或 Courier /v1/cdk（Frozen v1，按账号+码幂等）——Courier
// F62 已落地，但 Spawn→Courier cdk/mail 客户端接线为独立步骤，见 todo。
//
// 幂等/限领：UNIQUE(pack_id, account_id, request_id) 幂等锚（客户端幂等键，
// 重提交返回既有领取不新增）；UNIQUE(pack_id, account_id, claim_index) 槽位
// 锚——并发抢末位时唯一冲突即限领上限，不超额发放。

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrGiftPackNotFound      = errors.New("gift pack not found")
	ErrGiftPackCodeNotFound  = errors.New("gift pack code not found")
	ErrGiftPackClaimNotFound = errors.New("gift pack claim not found")
)

// GiftPackStatus / CodeStatus / ClaimStatus 状态常量。
const (
	GiftPackStatusDraft     = "draft"
	GiftPackStatusPublished = "published"
	GiftPackStatusDisabled  = "disabled"

	CodeStatusAvailable = "available"
	CodeStatusIssued    = "issued"
	CodeStatusUsed      = "used"
	CodeStatusDisabled  = "disabled"

	ClaimStatusClaimed = "claimed"
	ClaimStatusRevoked = "revoked"
)

// GiftPack 礼包配置（服务端配置域）。
type GiftPack struct {
	Id           int64  `json:"id"`
	Name         string `json:"name"`
	GameId       string `json:"game_id"`
	Description  string `json:"description"`
	Status       string `json:"status"`
	Timezone     string `json:"timezone"`
	StartAt      string `json:"start_at"`
	EndAt        string `json:"end_at"`
	ServerIds    string `json:"server_ids"` // JSON 数组
	Channel      string `json:"channel"`
	Audience     string `json:"audience"` // JSON 对象
	PerUserLimit int64  `json:"per_user_limit"`
	CodeRequired bool   `json:"code_required"`
	Content      string `json:"content"` // JSON 对象（items 等）
	CreatedBy    int64  `json:"created_by"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

// GiftPackCode 核销码（码库存）。
type GiftPackCode struct {
	Id        int64  `json:"id"`
	PackId    int64  `json:"pack_id"`
	Code      string `json:"code"`
	Status    string `json:"status"`
	AccountId string `json:"account_id"`
	ClaimId   int64  `json:"claim_id"`
	IssuedAt  string `json:"issued_at"`
	UsedAt    string `json:"used_at"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt string `json:"created_at"`
}

// GiftPackClaim 领取记录（限领槽位 + 幂等键）。
type GiftPackClaim struct {
	Id         int64  `json:"id"`
	PackId     int64  `json:"pack_id"`
	AccountId  string `json:"account_id"`
	GameId     string `json:"game_id"`
	ServerId   int64  `json:"server_id"`
	RoleId     string `json:"role_id"`
	CodeId     int64  `json:"code_id"`
	ClaimIndex int64  `json:"claim_index"`
	RequestId  string `json:"request_id"`
	Status     string `json:"status"`
	CreatedAt  string `json:"created_at"`
}

// GiftPackModel 礼包域数据访问（三表共用同一 db）。
type GiftPackModel struct {
	db *sql.DB
}

func NewGiftPackModel(db *sql.DB) *GiftPackModel {
	return &GiftPackModel{db: db}
}

func (m *GiftPackModel) CreateTables() error {
	tables := []struct{ sqlite, mysql string }{
		{
			`CREATE TABLE IF NOT EXISTS gift_packs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name VARCHAR(128) NOT NULL,
				game_id VARCHAR(64) NOT NULL,
				description TEXT NOT NULL DEFAULT '',
				status VARCHAR(16) NOT NULL DEFAULT 'draft',
				timezone VARCHAR(64) NOT NULL,
				start_at VARCHAR(32) NOT NULL,
				end_at VARCHAR(32) NOT NULL,
				server_ids TEXT NOT NULL DEFAULT '',
				channel VARCHAR(32) NOT NULL DEFAULT '',
				audience TEXT NOT NULL DEFAULT '{}',
				per_user_limit INTEGER NOT NULL DEFAULT 1,
				code_required INTEGER NOT NULL DEFAULT 0,
				content TEXT NOT NULL,
				created_by INTEGER NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS gift_packs (
				id BIGINT PRIMARY KEY AUTO_INCREMENT,
				name VARCHAR(128) NOT NULL,
				game_id VARCHAR(64) NOT NULL,
				description TEXT NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'draft',
				timezone VARCHAR(64) NOT NULL,
				start_at VARCHAR(32) NOT NULL,
				end_at VARCHAR(32) NOT NULL,
				server_ids TEXT NOT NULL,
				channel VARCHAR(32) NOT NULL DEFAULT '',
				audience TEXT NOT NULL,
				per_user_limit BIGINT NOT NULL DEFAULT 1,
				code_required TINYINT(1) NOT NULL DEFAULT 0,
				content TEXT NOT NULL,
				created_by BIGINT NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL,
				KEY idx_packs_game_status (game_id, status)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		{
			`CREATE TABLE IF NOT EXISTS gift_pack_codes (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				pack_id INTEGER NOT NULL,
				code VARCHAR(128) NOT NULL UNIQUE,
				status VARCHAR(16) NOT NULL DEFAULT 'available',
				account_id VARCHAR(64) NOT NULL DEFAULT '',
				claim_id INTEGER NOT NULL DEFAULT 0,
				issued_at VARCHAR(32) NOT NULL DEFAULT '',
				used_at VARCHAR(32) NOT NULL DEFAULT '',
				expires_at VARCHAR(32) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS gift_pack_codes (
				id BIGINT PRIMARY KEY AUTO_INCREMENT,
				pack_id BIGINT NOT NULL,
				code VARCHAR(128) NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'available',
				account_id VARCHAR(64) NOT NULL DEFAULT '',
				claim_id BIGINT NOT NULL DEFAULT 0,
				issued_at VARCHAR(32) NOT NULL DEFAULT '',
				used_at VARCHAR(32) NOT NULL DEFAULT '',
				expires_at VARCHAR(32) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL,
				UNIQUE KEY uk_pack_code (pack_id, code),
				KEY idx_codes_pack_status (pack_id, status)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		{
			`CREATE TABLE IF NOT EXISTS gift_pack_claims (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				pack_id INTEGER NOT NULL,
				account_id VARCHAR(64) NOT NULL,
				game_id VARCHAR(64) NOT NULL,
				server_id INTEGER NOT NULL DEFAULT 0,
				role_id VARCHAR(128) NOT NULL DEFAULT '',
				code_id INTEGER NOT NULL DEFAULT 0,
				claim_index INTEGER NOT NULL,
				request_id VARCHAR(128) NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'claimed',
				created_at VARCHAR(32) NOT NULL,
				UNIQUE (pack_id, account_id, request_id),
				UNIQUE (pack_id, account_id, claim_index)
			)`,
			`CREATE TABLE IF NOT EXISTS gift_pack_claims (
				id BIGINT PRIMARY KEY AUTO_INCREMENT,
				pack_id BIGINT NOT NULL,
				account_id VARCHAR(64) NOT NULL,
				game_id VARCHAR(64) NOT NULL,
				server_id BIGINT NOT NULL DEFAULT 0,
				role_id VARCHAR(128) NOT NULL DEFAULT '',
				code_id BIGINT NOT NULL DEFAULT 0,
				claim_index BIGINT NOT NULL,
				request_id VARCHAR(128) NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'claimed',
				created_at VARCHAR(32) NOT NULL,
				UNIQUE KEY uk_claim_request (pack_id, account_id, request_id),
				UNIQUE KEY uk_claim_index (pack_id, account_id, claim_index),
				KEY idx_claims_account (account_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
	}
	for _, t := range tables {
		if _, err := m.db.Exec(t.sqlite); err != nil {
			if _, err2 := m.db.Exec(t.mysql); err2 != nil {
				return errors.New("创建礼包表失败（尝试了 SQLite 和 MySQL 格式）")
			}
		}
	}
	return nil
}

const giftPackColumns = "id, name, game_id, description, status, timezone, start_at, end_at, server_ids, channel, audience, per_user_limit, code_required, content, created_by, created_at, updated_at"

func scanGiftPack(p *GiftPack) []any {
	return []any{&p.Id, &p.Name, &p.GameId, &p.Description, &p.Status, &p.Timezone,
		&p.StartAt, &p.EndAt, &p.ServerIds, &p.Channel, &p.Audience, &p.PerUserLimit,
		&p.CodeRequired, &p.Content, &p.CreatedBy, &p.CreatedAt, &p.UpdatedAt}
}

const giftPackCodeColumns = "id, pack_id, code, status, account_id, claim_id, issued_at, used_at, expires_at, created_at"

func scanGiftPackCode(c *GiftPackCode) []any {
	return []any{&c.Id, &c.PackId, &c.Code, &c.Status, &c.AccountId, &c.ClaimId,
		&c.IssuedAt, &c.UsedAt, &c.ExpiresAt, &c.CreatedAt}
}

const giftPackClaimColumns = "id, pack_id, account_id, game_id, server_id, role_id, code_id, claim_index, request_id, status, created_at"

func scanGiftPackClaim(c *GiftPackClaim) []any {
	return []any{&c.Id, &c.PackId, &c.AccountId, &c.GameId, &c.ServerId, &c.RoleId,
		&c.CodeId, &c.ClaimIndex, &c.RequestId, &c.Status, &c.CreatedAt}
}

// Create 建礼包（草稿）。回填自增 ID。
func (m *GiftPackModel) Create(p *GiftPack) error {
	now := nowUTC()
	p.CreatedAt, p.UpdatedAt = now, now
	res, err := m.db.Exec(
		`INSERT INTO gift_packs (name, game_id, description, status, timezone, start_at, end_at, server_ids, channel, audience, per_user_limit, code_required, content, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		p.Name, p.GameId, p.Description, p.Status, p.Timezone, p.StartAt, p.EndAt,
		p.ServerIds, p.Channel, p.Audience, p.PerUserLimit, p.CodeRequired, p.Content,
		p.CreatedBy, now, now,
	)
	if err != nil {
		return err
	}
	if id, err := res.LastInsertId(); err == nil && id > 0 {
		p.Id = id
	}
	return nil
}

// Update 更新草稿配置（不改变 status/created_at）。
func (m *GiftPackModel) Update(p *GiftPack) error {
	_, err := m.db.Exec(
		`UPDATE gift_packs SET name=?, description=?, timezone=?, start_at=?, end_at=?, server_ids=?, channel=?, audience=?, per_user_limit=?, code_required=?, content=?, updated_at=? WHERE id=?`,
		p.Name, p.Description, p.Timezone, p.StartAt, p.EndAt, p.ServerIds, p.Channel,
		p.Audience, p.PerUserLimit, p.CodeRequired, p.Content, nowUTC(), p.Id,
	)
	return err
}

// SetStatus 状态迁移（publish/disable/revise）。
func (m *GiftPackModel) SetStatus(id int64, status string) error {
	_, err := m.db.Exec(`UPDATE gift_packs SET status=?, updated_at=? WHERE id=?`, status, nowUTC(), id)
	return err
}

func (m *GiftPackModel) Get(id int64) (*GiftPack, error) {
	var p GiftPack
	err := m.db.QueryRow(`SELECT `+giftPackColumns+` FROM gift_packs WHERE id=?`, id).
		Scan(scanGiftPack(&p)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGiftPackNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

// List 管理面列表（game_id/status 可筛，有界 100）。
func (m *GiftPackModel) List(gameID, status string, limit int) ([]*GiftPack, error) {
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
	rows, err := m.db.Query(
		`SELECT `+giftPackColumns+` FROM gift_packs WHERE `+strings.Join(where, " AND ")+
			` ORDER BY id DESC LIMIT `+fmt.Sprintf("%d", limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*GiftPack
	for rows.Next() {
		var p GiftPack
		if err := rows.Scan(scanGiftPack(&p)...); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// ListPublishedByGame 玩家侧可见礼包（已发布 + 起止覆盖 now；区服过滤在
// 调用方按 server_ids JSON 判定，与活动匹配同口径）。
func (m *GiftPackModel) ListPublishedByGame(gameID, now string) ([]*GiftPack, error) {
	rows, err := m.db.Query(
		`SELECT `+giftPackColumns+` FROM gift_packs
		 WHERE game_id = ? AND status = ? AND start_at <= ? AND end_at >= ?
		 ORDER BY id DESC`, gameID, GiftPackStatusPublished, now, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*GiftPack
	for rows.Next() {
		var p GiftPack
		if err := rows.Scan(scanGiftPack(&p)...); err != nil {
			return nil, err
		}
		out = append(out, &p)
	}
	return out, rows.Err()
}

// ImportCodes 批量导入码库存（重复码忽略；返回新增/重复数）。
// INSERT OR IGNORE / INSERT IGNORE 的 RowsAffected：SQLite 与 MySQL 对被忽略
// 的重复行均返回 0，故 n==1 即新增。
func (m *GiftPackModel) ImportCodes(packID int64, codes []string) (int, int, error) {
	ok, dup := 0, 0
	for _, c := range codes {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		res, err := m.db.Exec(
			`INSERT OR IGNORE INTO gift_pack_codes (pack_id, code, status, created_at) VALUES (?, ?, ?, ?)`,
			packID, c, CodeStatusAvailable, nowUTC())
		if err != nil {
			if strings.Contains(err.Error(), "syntax") || strings.Contains(err.Error(), "near") {
				res, err = m.db.Exec(
					`INSERT IGNORE INTO gift_pack_codes (pack_id, code, status, created_at) VALUES (?, ?, ?, ?)`,
					packID, c, CodeStatusAvailable, nowUTC())
			}
			if err != nil {
				return ok, dup, err
			}
		}
		if n, _ := res.RowsAffected(); n == 1 {
			ok++
		} else {
			dup++
		}
	}
	return ok, dup, nil
}

// ListCodes 码库存列表（status 可筛，有界 500）。
func (m *GiftPackModel) ListCodes(packID int64, status string, limit int) ([]*GiftPackCode, error) {
	if limit <= 0 || limit > 1000 {
		limit = 500
	}
	where, args := []string{"pack_id = ?"}, []any{packID}
	if status != "" {
		where = append(where, "status = ?")
		args = append(args, status)
	}
	rows, err := m.db.Query(
		`SELECT `+giftPackCodeColumns+` FROM gift_pack_codes WHERE `+strings.Join(where, " AND ")+
			` ORDER BY id DESC LIMIT `+fmt.Sprintf("%d", limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*GiftPackCode
	for rows.Next() {
		var c GiftPackCode
		if err := rows.Scan(scanGiftPackCode(&c)...); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// CountAvailableCodes 可用码数（玩家侧库存展示与领完判定）。
func (m *GiftPackModel) CountAvailableCodes(packID int64) (int64, error) {
	var n int64
	err := m.db.QueryRow(
		`SELECT COUNT(*) FROM gift_pack_codes WHERE pack_id=? AND status=?`,
		packID, CodeStatusAvailable).Scan(&n)
	return n, err
}

// IssueCode 原子签发一个可用码（绑定账号与领取；并发安全：SELECT 可用码后
// 条件 UPDATE，affected=0 说明被并发抢走则重试）。
func (m *GiftPackModel) IssueCode(packID int64, accountID string, claimID int64, attempt int) (int64, string, error) {
	var id int64
	var code string
	err := m.db.QueryRow(
		`SELECT id, code FROM gift_pack_codes WHERE pack_id=? AND status=? ORDER BY id ASC LIMIT 1`,
		packID, CodeStatusAvailable).Scan(&id, &code)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", ErrGiftPackCodeNotFound
	}
	if err != nil {
		return 0, "", err
	}
	res, err := m.db.Exec(
		`UPDATE gift_pack_codes SET status=?, account_id=?, claim_id=?, issued_at=? WHERE id=? AND status=?`,
		CodeStatusIssued, accountID, claimID, nowUTC(), id, CodeStatusAvailable)
	if err != nil {
		return 0, "", err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		if attempt < 3 {
			return m.IssueCode(packID, accountID, claimID, attempt+1)
		}
		return 0, "", ErrGiftPackCodeNotFound
	}
	return id, code, nil
}

// ReleaseCode 释放已签发但领取失败的码（并发/限额冲突时回滚）。
func (m *GiftPackModel) ReleaseCode(id int64) error {
	_, err := m.db.Exec(
		`UPDATE gift_pack_codes SET status=?, account_id='', claim_id=0, issued_at='' WHERE id=? AND status=?`,
		CodeStatusAvailable, id, CodeStatusIssued)
	return err
}

// MarkCodeUsed 游戏服核销回调：issued → used（记录核销事实）。
func (m *GiftPackModel) MarkCodeUsed(codeID int64) error {
	res, err := m.db.Exec(
		`UPDATE gift_pack_codes SET status=?, used_at=? WHERE id=? AND status=?`,
		CodeStatusUsed, nowUTC(), codeID, CodeStatusIssued)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n != 1 {
		return ErrGiftPackCodeNotFound
	}
	return nil
}

func (m *GiftPackModel) GetCode(id int64) (*GiftPackCode, error) {
	var c GiftPackCode
	err := m.db.QueryRow(`SELECT `+giftPackCodeColumns+` FROM gift_pack_codes WHERE id=?`, id).
		Scan(scanGiftPackCode(&c)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGiftPackCodeNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// CountClaims 该账号该礼包已领取次数（限领消费）。
func (m *GiftPackModel) CountClaims(packID int64, accountID string) (int64, error) {
	var n int64
	err := m.db.QueryRow(
		`SELECT COUNT(*) FROM gift_pack_claims WHERE pack_id=? AND account_id=?`,
		packID, accountID).Scan(&n)
	return n, err
}

// GetClaimByRequest 幂等锚查询（同请求键重提交返回既有领取）。
func (m *GiftPackModel) GetClaimByRequest(packID int64, accountID, requestID string) (*GiftPackClaim, error) {
	var c GiftPackClaim
	err := m.db.QueryRow(
		`SELECT `+giftPackClaimColumns+` FROM gift_pack_claims WHERE pack_id=? AND account_id=? AND request_id=?`,
		packID, accountID, requestID).Scan(scanGiftPackClaim(&c)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGiftPackClaimNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// InsertClaim 落领取记录（槽位=现有数+1；唯一冲突交调用方处理幂等/限领）。
// 回填自增 ID（供后续绑定签发码）。
func (m *GiftPackModel) InsertClaim(c *GiftPackClaim) error {
	c.CreatedAt = nowUTC()
	res, err := m.db.Exec(
		`INSERT INTO gift_pack_claims (pack_id, account_id, game_id, server_id, role_id, code_id, claim_index, request_id, status, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		c.PackId, c.AccountId, c.GameId, c.ServerId, c.RoleId, c.CodeId,
		c.ClaimIndex, c.RequestId, c.Status, c.CreatedAt)
	if err != nil {
		return err
	}
	if id, err := res.LastInsertId(); err == nil && id > 0 {
		c.Id = id
	}
	return nil
}

// ReleaseClaim 领取失败回滚：删除该领取行（码包无码可发等场景，避免
// 占用限领槽位）。
func (m *GiftPackModel) ReleaseClaim(claimID int64) error {
	_, err := m.db.Exec(`DELETE FROM gift_pack_claims WHERE id=? AND status=?`, claimID, ClaimStatusClaimed)
	return err
}

// BindClaimCode 领取成功后再绑定签发的码。
func (m *GiftPackModel) BindClaimCode(claimID, codeID int64) error {
	_, err := m.db.Exec(`UPDATE gift_pack_claims SET code_id=? WHERE id=?`, codeID, claimID)
	return err
}

func (m *GiftPackModel) GetClaim(id int64) (*GiftPackClaim, error) {
	var c GiftPackClaim
	err := m.db.QueryRow(`SELECT `+giftPackClaimColumns+` FROM gift_pack_claims WHERE id=?`, id).
		Scan(scanGiftPackClaim(&c)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrGiftPackClaimNotFound
	}
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// ListClaimsByAccount 该账号领取记录（有界 100）。
func (m *GiftPackModel) ListClaimsByAccount(accountID string, limit int) ([]*GiftPackClaim, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := m.db.Query(
		`SELECT `+giftPackClaimColumns+` FROM gift_pack_claims WHERE account_id=? ORDER BY id DESC LIMIT `+fmt.Sprintf("%d", limit),
		accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*GiftPackClaim
	for rows.Next() {
		var c GiftPackClaim
		if err := rows.Scan(scanGiftPackClaim(&c)...); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}

// ListClaims 管理面领取记录（pack_id/account_id 可筛，有界 100）。
func (m *GiftPackModel) ListClaims(packID int64, accountID string, limit int) ([]*GiftPackClaim, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	where, args := []string{"1=1"}, []any{}
	if packID > 0 {
		where = append(where, "pack_id = ?")
		args = append(args, packID)
	}
	if accountID != "" {
		where = append(where, "account_id = ?")
		args = append(args, accountID)
	}
	rows, err := m.db.Query(
		`SELECT `+giftPackClaimColumns+` FROM gift_pack_claims WHERE `+strings.Join(where, " AND ")+
			` ORDER BY id DESC LIMIT `+fmt.Sprintf("%d", limit), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*GiftPackClaim
	for rows.Next() {
		var c GiftPackClaim
		if err := rows.Scan(scanGiftPackClaim(&c)...); err != nil {
			return nil, err
		}
		out = append(out, &c)
	}
	return out, rows.Err()
}
