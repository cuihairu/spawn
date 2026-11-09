package model

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// 平台角色（GP-103 最小权限模型，见 docs/zh/architecture/courier-integration-design.md §4）。
// player 为缺省：无角色绑定的平台账号按 player 处理。
const (
	RolePlatformAdmin = "platform_admin"
	RoleOperator      = "operator"
	RolePartner       = "partner"
	RolePlayer        = "player"
)

// 合作方/归属/提审状态。
const (
	PartnerActive    = "active"
	PartnerSuspended = "suspended"

	OwnershipActive  = "active"
	OwnershipRevoked = "revoked"

	ReviewPending = "pending"
	ReviewApprove = "approve"
	ReviewReject  = "reject"

	VersionStatusDraft     = "draft"
	VersionStatusPending   = "pending"
	VersionStatusApproved  = "approved"
	VersionStatusRejected  = "rejected"
	VersionStatusPublished = "published"
	VersionStatusRevoked   = "revoked"

	// 区服状态（GP-201）：open 可进/可充值，maintenance 维护中，closed 下线
	// （玩家侧隐藏，仅合作方后台可见，保留充值对账记录）。
	ServerStatusOpen        = "open"
	ServerStatusMaintenance = "maintenance"
	ServerStatusClosed      = "closed"
)

var (
	ErrPartnerNotFound      = errors.New("partner not found")
	ErrReviewNotFound       = errors.New("review not found")
	ErrVersionNotFound      = errors.New("version not found")
	ErrVersionPackageLocked = errors.New("version package locked")
	ErrOwnershipMissing     = errors.New("game ownership missing")
	ErrServerNotFound       = errors.New("server not found")
)

// Partner 合作方主体（F11）：先最小化人工审核，在线签约后续再考虑。
type Partner struct {
	Id           int64  `json:"id"`
	Name         string `json:"name"`
	ContactEmail string `json:"contact_email"`
	Status       string `json:"status"`
	CreatedAt    string `json:"created_at"`
}

// RoleBinding 平台账号角色绑定（user-service 账号 id → 平台角色）。
// partner 角色额外绑定所属合作方主体（partner_id），用于按合作方限制访问。
type RoleBinding struct {
	UserId    int64  `json:"user_id"`
	Role      string `json:"role"`
	PartnerId int64  `json:"partner_id"`
	UpdatedAt string `json:"updated_at"`
}

// GameOwnership 游戏归属与授权记录：partner 只达自有游戏。
type GameOwnership struct {
	GameId    string `json:"game_id"`
	PartnerId int64  `json:"partner_id"`
	GrantedBy int64  `json:"granted_by"`
	GrantedAt string `json:"granted_at"`
	Status    string `json:"status"`
}

// GameReview 提审记录（资料/版本共用，version_id 空为资料提审）。
type GameReview struct {
	Id           int64  `json:"id"`
	GameId       string `json:"game_id"`
	VersionId    string `json:"version_id"`
	SubmittedBy  int64  `json:"submitted_by"`
	SubmittedAt  string `json:"submitted_at"`
	Status       string `json:"status"`
	ReviewerId   int64  `json:"reviewer_id"`
	ReviewReason string `json:"review_reason"`
	ReviewedAt   string `json:"reviewed_at"`
}

// GameVersion 游戏版本/渠道包记录（GP-104）：包名、versionCode、签名指纹、
// 大小、哈希、存储地址；version 字段兼任 versionName 展示名。
type GameVersion struct {
	Id              string `json:"id"`
	GameId          string `json:"game_id"`
	Version         string `json:"version"`
	Changelog       string `json:"changelog"`
	PackageName     string `json:"package_name"`
	VersionCode     int64  `json:"version_code"`
	SignFingerprint string `json:"sign_fingerprint"`
	SizeBytes       int64  `json:"size_bytes"`
	Hash            string `json:"hash"`
	StorageUrl      string `json:"storage_url"`
	Status          string `json:"status"`
	ReviewReason    string `json:"review_reason"`
	CreatedAt       string `json:"created_at"`
}

// GameServer 游戏区服（GP-201 区服目录）：合作方维护，玩家公开读开放区服；
// 同时作为充值目标（充值按 game_id + server_id 定位区服）。
type GameServer struct {
	Id        int64  `json:"id"`
	GameId    string `json:"game_id"`
	Name      string `json:"name"`
	Region    string `json:"region"`
	Status    string `json:"status"`
	Sort      int64  `json:"sort"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// AuditLog 操作审计（设计 §4：绑定/映射/能力变更均写审计）。
type AuditLog struct {
	Id           int64  `json:"id"`
	ActorId      int64  `json:"actor_id"`
	ActorRole    string `json:"actor_role"`
	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceId   string `json:"resource_id"`
	BeforeJson   string `json:"before_json"`
	AfterJson    string `json:"after_json"`
	Ip           string `json:"ip"`
	CreatedAt    string `json:"created_at"`
}

// AdminStore 运营后台数据访问（MySQL/SQLite 双驱动）。
type AdminStore interface {
	CreateAdminTables() error

	GetRole(userId int64) (string, error)
	GetRoleWithPartner(userId int64) (string, int64, error)
	SetRole(userId int64, role string, partnerId int64) error

	CreatePartner(name, contactEmail string) (*Partner, error)
	GetPartner(id int64) (*Partner, error)
	ListPartners() ([]*Partner, error)
	SetPartnerStatus(id int64, status string) (*Partner, error)

	AssignOwnership(gameId string, partnerId, grantedBy int64) (*GameOwnership, error)
	GetOwnership(gameId string) (*GameOwnership, error)
	ListGamesByPartner(partnerId int64) ([]string, error)

	SubmitReview(gameId, versionId string, submittedBy int64) (*GameReview, error)
	GetReview(id int64) (*GameReview, error)
	DecideReview(id, reviewerId int64, decision, reason string) (*GameReview, error)
	ListReviews(status string, limit, offset int) ([]*GameReview, int, error)

	CreateVersion(v *GameVersion) (*GameVersion, error)
	GetVersion(id string) (*GameVersion, error)
	ListVersionsByGame(gameId string) ([]*GameVersion, error)
	SetVersionStatus(id, status, reason string) (*GameVersion, error)
	// GP-104 发布/撤回：发布校验不覆盖低版本渠道包（version_code 低于已发布版本）。
	PublishVersion(id string) (*GameVersion, error)
	RevokeVersion(id string) (*GameVersion, error)
	// GP-105 包体托管：上传后回填哈希/大小/托管地址；取当前已发布版本。
	UpdateVersionPackage(id, hash string, sizeBytes int64, storageUrl string) (*GameVersion, error)
	GetLatestPublishedVersion(gameId string) (*GameVersion, error)

	// GP-201 区服目录/充值目标：合作方维护，玩家公开读（隐藏 closed）。
	CreateGameServer(gameId string, s *GameServer) (*GameServer, error)
	GetGameServer(id int64) (*GameServer, error)
	ListGameServers(gameId string, includeClosed bool) ([]*GameServer, error)
	UpdateGameServer(s *GameServer) (*GameServer, error)

	WriteAudit(entry *AuditLog) error
	ListAuditLogs(resourceType string, limit, offset int) ([]*AuditLog, int, error)
}

var _ AdminStore = (*AdminModel)(nil)

// AdminModel 运营后台模型；角色查询走主键，最小实现不设缓存。
type AdminModel struct {
	db *sql.DB
}

// NewAdminModel 创建运营后台模型。
func NewAdminModel(db *sql.DB) *AdminModel {
	return &AdminModel{db: db}
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// CreateAdminTables 建运营后台五表（幂等）；先试 SQLite 方言再回落 MySQL。
func (m *AdminModel) CreateAdminTables() error {
	tables := []struct{ sqlite, mysql string }{
		{
			`CREATE TABLE IF NOT EXISTS partners (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name VARCHAR(128) NOT NULL,
				contact_email VARCHAR(128) NOT NULL DEFAULT '',
				status VARCHAR(16) NOT NULL DEFAULT 'active',
				created_at VARCHAR(32) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS partners (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				name VARCHAR(128) NOT NULL,
				contact_email VARCHAR(128) NOT NULL DEFAULT '',
				status VARCHAR(16) NOT NULL DEFAULT 'active',
				created_at VARCHAR(32) NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		{
			`CREATE TABLE IF NOT EXISTS platform_roles (
				user_id INTEGER PRIMARY KEY,
				role VARCHAR(32) NOT NULL,
				partner_id INTEGER NOT NULL DEFAULT 0,
				updated_at VARCHAR(32) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS platform_roles (
				user_id BIGINT PRIMARY KEY,
				role VARCHAR(32) NOT NULL,
				partner_id BIGINT NOT NULL DEFAULT 0,
				updated_at VARCHAR(32) NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		{
			`CREATE TABLE IF NOT EXISTS game_ownership (
				game_id VARCHAR(64) PRIMARY KEY,
				partner_id INTEGER NOT NULL,
				granted_by INTEGER NOT NULL DEFAULT 0,
				granted_at VARCHAR(32) NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'active'
			)`,
			`CREATE TABLE IF NOT EXISTS game_ownership (
				game_id VARCHAR(64) PRIMARY KEY,
				partner_id BIGINT NOT NULL,
				granted_by BIGINT NOT NULL DEFAULT 0,
				granted_at VARCHAR(32) NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'active',
				KEY idx_ownership_partner (partner_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		{
			`CREATE TABLE IF NOT EXISTS game_reviews (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				game_id VARCHAR(64) NOT NULL,
				version_id VARCHAR(64) NOT NULL DEFAULT '',
				submitted_by INTEGER NOT NULL DEFAULT 0,
				submitted_at VARCHAR(32) NOT NULL,
				status VARCHAR(16) NOT NULL,
				reviewer_id INTEGER NOT NULL DEFAULT 0,
				review_reason VARCHAR(1024) NOT NULL DEFAULT '',
				reviewed_at VARCHAR(32) NOT NULL DEFAULT ''
			)`,
			`CREATE TABLE IF NOT EXISTS game_reviews (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				game_id VARCHAR(64) NOT NULL,
				version_id VARCHAR(64) NOT NULL DEFAULT '',
				submitted_by BIGINT NOT NULL DEFAULT 0,
				submitted_at VARCHAR(32) NOT NULL,
				status VARCHAR(16) NOT NULL,
				reviewer_id BIGINT NOT NULL DEFAULT 0,
				review_reason VARCHAR(1024) NOT NULL DEFAULT '',
				reviewed_at VARCHAR(32) NOT NULL DEFAULT '',
				KEY idx_reviews_status (status)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		{
			`CREATE TABLE IF NOT EXISTS game_versions (
				id VARCHAR(64) PRIMARY KEY,
				game_id VARCHAR(64) NOT NULL,
				version VARCHAR(64) NOT NULL,
				changelog VARCHAR(2048) NOT NULL DEFAULT '',
				package_name VARCHAR(128) NOT NULL DEFAULT '',
				version_code INTEGER NOT NULL DEFAULT 0,
				sign_fingerprint VARCHAR(256) NOT NULL DEFAULT '',
				size_bytes INTEGER NOT NULL DEFAULT 0,
				hash VARCHAR(128) NOT NULL DEFAULT '',
				storage_url VARCHAR(512) NOT NULL DEFAULT '',
				status VARCHAR(16) NOT NULL DEFAULT 'draft',
				review_reason VARCHAR(1024) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS game_versions (
				id VARCHAR(64) PRIMARY KEY,
				game_id VARCHAR(64) NOT NULL,
				version VARCHAR(64) NOT NULL,
				changelog VARCHAR(2048) NOT NULL DEFAULT '',
				package_name VARCHAR(128) NOT NULL DEFAULT '',
				version_code BIGINT NOT NULL DEFAULT 0,
				sign_fingerprint VARCHAR(256) NOT NULL DEFAULT '',
				size_bytes BIGINT NOT NULL DEFAULT 0,
				hash VARCHAR(128) NOT NULL DEFAULT '',
				storage_url VARCHAR(512) NOT NULL DEFAULT '',
				status VARCHAR(16) NOT NULL DEFAULT 'draft',
				review_reason VARCHAR(1024) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL,
				KEY idx_versions_game (game_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		{
			`CREATE TABLE IF NOT EXISTS game_servers (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				game_id VARCHAR(64) NOT NULL,
				name VARCHAR(128) NOT NULL,
				region VARCHAR(64) NOT NULL DEFAULT '',
				status VARCHAR(16) NOT NULL DEFAULT 'open',
				sort INTEGER NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS game_servers (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				game_id VARCHAR(64) NOT NULL,
				name VARCHAR(128) NOT NULL,
				region VARCHAR(64) NOT NULL DEFAULT '',
				status VARCHAR(16) NOT NULL DEFAULT 'open',
				sort BIGINT NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL,
				KEY idx_servers_game (game_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		{
			`CREATE TABLE IF NOT EXISTS audit_logs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				actor_id INTEGER NOT NULL DEFAULT 0,
				actor_role VARCHAR(32) NOT NULL DEFAULT '',
				action VARCHAR(64) NOT NULL,
				resource_type VARCHAR(32) NOT NULL,
				resource_id VARCHAR(64) NOT NULL DEFAULT '',
				before_json TEXT NOT NULL DEFAULT '',
				after_json TEXT NOT NULL DEFAULT '',
				ip VARCHAR(64) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS audit_logs (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				actor_id BIGINT NOT NULL DEFAULT 0,
				actor_role VARCHAR(32) NOT NULL DEFAULT '',
				action VARCHAR(64) NOT NULL,
				resource_type VARCHAR(32) NOT NULL,
				resource_id VARCHAR(64) NOT NULL DEFAULT '',
				before_json TEXT NOT NULL,
				after_json TEXT NOT NULL,
				ip VARCHAR(64) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
	}
	for _, t := range tables {
		if _, err := m.db.Exec(t.sqlite); err != nil {
			if _, err2 := m.db.Exec(t.mysql); err2 != nil {
				return fmt.Errorf("创建运营后台表失败（尝试了 SQLite 和 MySQL 格式）: %w", err2)
			}
		}
	}
	// 既有库的 game_versions 无 GP-104 渠道包列：幂等补齐（SQLite PRAGMA → MySQL SHOW COLUMNS）。
	return m.migrateVersionColumns()
}

// versionColumns 渠道包列（迁移与扫描共用定义，顺序即 SELECT 尾列）。
var versionColumns = []struct{ name, ddl string }{
	{"package_name", "VARCHAR(128) NOT NULL DEFAULT ''"},
	{"version_code", "INTEGER NOT NULL DEFAULT 0"},
	{"sign_fingerprint", "VARCHAR(256) NOT NULL DEFAULT ''"},
	{"size_bytes", "INTEGER NOT NULL DEFAULT 0"},
	{"hash", "VARCHAR(128) NOT NULL DEFAULT ''"},
	{"storage_url", "VARCHAR(512) NOT NULL DEFAULT ''"},
}

// migrateVersionColumns 为存量 game_versions 表补齐 GP-104 列（幂等，缺列才 ALTER）。
func (m *AdminModel) migrateVersionColumns() error {
	existing := map[string]bool{}
	if rows, err := m.db.Query(`PRAGMA table_info(game_versions)`); err == nil {
		for rows.Next() {
			var cid int
			var name, ctype string
			var notnull, pk int
			var dflt sql.NullString
			if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err == nil {
				existing[name] = true
			}
		}
		rows.Close()
	} else {
		// MySQL：SHOW COLUMNS
		rows, err2 := m.db.Query(`SHOW COLUMNS FROM game_versions`)
		if err2 != nil {
			return nil // 无法枚举列信息时跳过迁移（不阻断启动）
		}
		for rows.Next() {
			var field, ctype, nullb, key, extra string
			var dflt sql.NullString
			if err := rows.Scan(&field, &ctype, &nullb, &key, &dflt, &extra); err == nil {
				existing[field] = true
			}
		}
		rows.Close()
	}
	for _, c := range versionColumns {
		if existing[c.name] {
			continue
		}
		ddl := c.ddl
		if strings.HasPrefix(ddl, "INTEGER") {
			ddl = "BIGINT" + strings.TrimPrefix(ddl, "INTEGER")
		}
		if _, err := m.db.Exec(
			`ALTER TABLE game_versions ADD COLUMN ` + c.name + ` ` + ddl); err != nil {
			// 列已存在（并发建表）或方言差异：忽略该列，不阻断
			continue
		}
	}
	return nil
}

// ---- 角色 ----

// GetRole 查账号角色；未绑定返回 player（缺省角色，不报错）。
func (m *AdminModel) GetRole(userId int64) (string, error) {
	role, _, err := m.GetRoleWithPartner(userId)
	return role, err
}

// GetRoleWithPartner 查角色与所属合作方（partner_id；非 partner 角色为 0）。
func (m *AdminModel) GetRoleWithPartner(userId int64) (string, int64, error) {
	var role string
	var partnerID int64
	err := m.db.QueryRow(
		`SELECT role, partner_id FROM platform_roles WHERE user_id = ?`, userId).
		Scan(&role, &partnerID)
	if err == sql.ErrNoRows {
		return RolePlayer, 0, nil
	}
	if err != nil {
		return "", 0, fmt.Errorf("查询角色失败: %w", err)
	}
	if !validRole(role) {
		return RolePlayer, 0, nil
	}
	return role, partnerID, nil
}

// SetRole 绑定/更新账号角色（upsert；partnerId 仅 partner 角色使用）。
func (m *AdminModel) SetRole(userId int64, role string, partnerId int64) error {
	if !validRole(role) {
		return fmt.Errorf("非法角色: %s", role)
	}
	if role != RolePartner {
		partnerId = 0
	}
	if _, err := m.db.Exec(
		`INSERT INTO platform_roles (user_id, role, partner_id, updated_at) VALUES (?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE role = VALUES(role), partner_id = VALUES(partner_id),
			updated_at = VALUES(updated_at)`,
		userId, role, partnerId, nowUTC()); err != nil {
		// SQLite 无 ON DUPLICATE KEY，回落 INSERT OR REPLACE
		if _, err2 := m.db.Exec(
			`INSERT OR REPLACE INTO platform_roles (user_id, role, partner_id, updated_at) VALUES (?, ?, ?, ?)`,
			userId, role, partnerId, nowUTC()); err2 != nil {
			return fmt.Errorf("写入角色失败: %w", err2)
		}
	}
	return nil
}

func validRole(role string) bool {
	switch role {
	case RolePlatformAdmin, RoleOperator, RolePartner, RolePlayer:
		return true
	}
	return false
}

// ---- 合作方 ----

func (m *AdminModel) CreatePartner(name, contactEmail string) (*Partner, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, errors.New("合作方名称不能为空")
	}
	res, err := m.db.Exec(
		`INSERT INTO partners (name, contact_email, status, created_at) VALUES (?, ?, ?, ?)`,
		name, strings.TrimSpace(contactEmail), PartnerActive, nowUTC())
	if err != nil {
		return nil, fmt.Errorf("创建合作方失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("读取合作方 id 失败: %w", err)
	}
	return m.GetPartner(id)
}

func (m *AdminModel) GetPartner(id int64) (*Partner, error) {
	p := &Partner{}
	err := m.db.QueryRow(
		`SELECT id, name, contact_email, status, created_at FROM partners WHERE id = ?`, id).
		Scan(&p.Id, &p.Name, &p.ContactEmail, &p.Status, &p.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrPartnerNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询合作方失败: %w", err)
	}
	return p, nil
}

func (m *AdminModel) ListPartners() ([]*Partner, error) {
	rows, err := m.db.Query(`SELECT id, name, contact_email, status, created_at FROM partners ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("查询合作方列表失败: %w", err)
	}
	defer rows.Close()
	var partners []*Partner
	for rows.Next() {
		p := &Partner{}
		if err := rows.Scan(&p.Id, &p.Name, &p.ContactEmail, &p.Status, &p.CreatedAt); err != nil {
			return nil, fmt.Errorf("读取合作方行失败: %w", err)
		}
		partners = append(partners, p)
	}
	return partners, rows.Err()
}

func (m *AdminModel) SetPartnerStatus(id int64, status string) (*Partner, error) {
	if status != PartnerActive && status != PartnerSuspended {
		return nil, errors.New("非法合作方状态")
	}
	if _, err := m.db.Exec(`UPDATE partners SET status = ? WHERE id = ?`, status, id); err != nil {
		return nil, fmt.Errorf("更新合作方状态失败: %w", err)
	}
	return m.GetPartner(id)
}

// ---- 游戏归属 ----

// AssignOwnership 指派/更新游戏归属（upsert，覆盖旧记录；变更历史走审计日志）。
func (m *AdminModel) AssignOwnership(gameId string, partnerId, grantedBy int64) (*GameOwnership, error) {
	if strings.TrimSpace(gameId) == "" {
		return nil, errors.New("游戏 id 不能为空")
	}
	if _, err := m.db.Exec(
		`INSERT INTO game_ownership (game_id, partner_id, granted_by, granted_at, status)
		 VALUES (?, ?, ?, ?, ?)
		 ON DUPLICATE KEY UPDATE partner_id = VALUES(partner_id),
			granted_by = VALUES(granted_by), granted_at = VALUES(granted_at), status = VALUES(status)`,
		gameId, partnerId, grantedBy, nowUTC(), OwnershipActive); err != nil {
		if _, err2 := m.db.Exec(
			`INSERT OR REPLACE INTO game_ownership (game_id, partner_id, granted_by, granted_at, status)
			 VALUES (?, ?, ?, ?, ?)`,
			gameId, partnerId, grantedBy, nowUTC(), OwnershipActive); err2 != nil {
			return nil, fmt.Errorf("写入游戏归属失败: %w", err2)
		}
	}
	return m.GetOwnership(gameId)
}

func (m *AdminModel) GetOwnership(gameId string) (*GameOwnership, error) {
	o := &GameOwnership{}
	err := m.db.QueryRow(
		`SELECT game_id, partner_id, granted_by, granted_at, status FROM game_ownership WHERE game_id = ?`,
		gameId).Scan(&o.GameId, &o.PartnerId, &o.GrantedBy, &o.GrantedAt, &o.Status)
	if err == sql.ErrNoRows {
		return nil, ErrOwnershipMissing
	}
	if err != nil {
		return nil, fmt.Errorf("查询游戏归属失败: %w", err)
	}
	return o, nil
}

func (m *AdminModel) ListGamesByPartner(partnerId int64) ([]string, error) {
	rows, err := m.db.Query(
		`SELECT game_id FROM game_ownership WHERE partner_id = ? AND status = ? ORDER BY game_id`,
		partnerId, OwnershipActive)
	if err != nil {
		return nil, fmt.Errorf("查询合作方游戏失败: %w", err)
	}
	defer rows.Close()
	ids := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("读取合作方游戏行失败: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// ---- 提审 ----

// SubmitReview 登记一条提审（资料 version_id 为空；版本带 version_id）。
// 不做状态前置校验，由 logic 层约束（同游戏重复提审由 logic 先驳回未决记录）。
func (m *AdminModel) SubmitReview(gameId, versionId string, submittedBy int64) (*GameReview, error) {
	res, err := m.db.Exec(
		`INSERT INTO game_reviews (game_id, version_id, submitted_by, submitted_at, status)
		 VALUES (?, ?, ?, ?, ?)`,
		gameId, versionId, submittedBy, nowUTC(), ReviewPending)
	if err != nil {
		return nil, fmt.Errorf("登记提审失败: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, fmt.Errorf("读取提审 id 失败: %w", err)
	}
	return m.GetReview(id)
}

func (m *AdminModel) GetReview(id int64) (*GameReview, error) {
	r := &GameReview{}
	err := m.db.QueryRow(
		`SELECT id, game_id, version_id, submitted_by, submitted_at, status, reviewer_id, review_reason, reviewed_at
		 FROM game_reviews WHERE id = ?`, id).
		Scan(&r.Id, &r.GameId, &r.VersionId, &r.SubmittedBy, &r.SubmittedAt, &r.Status,
			&r.ReviewerId, &r.ReviewReason, &r.ReviewedAt)
	if err == sql.ErrNoRows {
		return nil, ErrReviewNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询提审失败: %w", err)
	}
	return r, nil
}

// DecideReview 审核决定：decision 取 approve/reject，reject 必须带原因（logic 层校验）。
func (m *AdminModel) DecideReview(id, reviewerId int64, decision, reason string) (*GameReview, error) {
	if decision != ReviewApprove && decision != ReviewReject {
		return nil, errors.New("非法审核决定")
	}
	res, err := m.db.Exec(
		`UPDATE game_reviews SET status = ?, reviewer_id = ?, review_reason = ?, reviewed_at = ?
		 WHERE id = ? AND status = ?`,
		decision, reviewerId, reason, nowUTC(), id, ReviewPending)
	if err != nil {
		return nil, fmt.Errorf("更新提审失败: %w", err)
	}
	if affected, err := res.RowsAffected(); err == nil && affected == 0 {
		// 未决记录不存在：区分 404 与已审结
		if _, err2 := m.GetReview(id); err2 != nil {
			return nil, err2
		}
		return nil, errors.New("该提审已审结")
	}
	return m.GetReview(id)
}

func (m *AdminModel) ListReviews(status string, limit, offset int) ([]*GameReview, int, error) {
	where, args := "", []any{}
	if status != "" {
		where = ` WHERE status = ?`
		args = append(args, status)
	}
	var total int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM game_reviews`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计提审失败: %w", err)
	}
	query := `SELECT id, game_id, version_id, submitted_by, submitted_at, status, reviewer_id, review_reason, reviewed_at
		FROM game_reviews` + where + ` ORDER BY id DESC LIMIT ? OFFSET ?`
	rows, err := m.db.Query(query, append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询提审列表失败: %w", err)
	}
	defer rows.Close()
	reviews := []*GameReview{}
	for rows.Next() {
		r := &GameReview{}
		if err := rows.Scan(&r.Id, &r.GameId, &r.VersionId, &r.SubmittedBy, &r.SubmittedAt, &r.Status,
			&r.ReviewerId, &r.ReviewReason, &r.ReviewedAt); err != nil {
			return nil, 0, fmt.Errorf("读取提审行失败: %w", err)
		}
		reviews = append(reviews, r)
	}
	return reviews, total, rows.Err()
}

// ---- 版本 ----

const versionSelectColumns = `id, game_id, version, changelog, package_name, version_code, sign_fingerprint, size_bytes, hash, storage_url, status, review_reason, created_at`

// CreateVersion 新建版本（draft）；渠道包字段随 GP-104 一并记录。
func (m *AdminModel) CreateVersion(v *GameVersion) (*GameVersion, error) {
	if v == nil || strings.TrimSpace(v.GameId) == "" || strings.TrimSpace(v.Version) == "" {
		return nil, errors.New("游戏 id 与版本号不能为空")
	}
	v.Id = fmt.Sprintf("ver-%s", uuid.NewString())
	v.Version = strings.TrimSpace(v.Version)
	v.Status = VersionStatusDraft
	v.CreatedAt = nowUTC()
	if _, err := m.db.Exec(
		`INSERT INTO game_versions (id, game_id, version, changelog, package_name, version_code,
			sign_fingerprint, size_bytes, hash, storage_url, status, review_reason, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '', ?)`,
		v.Id, v.GameId, v.Version, v.Changelog, v.PackageName, v.VersionCode,
		v.SignFingerprint, v.SizeBytes, v.Hash, v.StorageUrl, v.Status, v.CreatedAt); err != nil {
		return nil, fmt.Errorf("创建版本失败: %w", err)
	}
	return v, nil
}

func (m *AdminModel) GetVersion(id string) (*GameVersion, error) {
	v := &GameVersion{}
	err := m.db.QueryRow(
		`SELECT `+versionSelectColumns+` FROM game_versions WHERE id = ?`, id).
		Scan(&v.Id, &v.GameId, &v.Version, &v.Changelog, &v.PackageName, &v.VersionCode,
			&v.SignFingerprint, &v.SizeBytes, &v.Hash, &v.StorageUrl, &v.Status, &v.ReviewReason, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询版本失败: %w", err)
	}
	return v, nil
}

func (m *AdminModel) ListVersionsByGame(gameId string) ([]*GameVersion, error) {
	rows, err := m.db.Query(
		`SELECT `+versionSelectColumns+` FROM game_versions WHERE game_id = ? ORDER BY id`, gameId)
	if err != nil {
		return nil, fmt.Errorf("查询版本列表失败: %w", err)
	}
	defer rows.Close()
	versions := []*GameVersion{}
	for rows.Next() {
		v := &GameVersion{}
		if err := rows.Scan(&v.Id, &v.GameId, &v.Version, &v.Changelog, &v.PackageName, &v.VersionCode,
			&v.SignFingerprint, &v.SizeBytes, &v.Hash, &v.StorageUrl, &v.Status, &v.ReviewReason, &v.CreatedAt); err != nil {
			return nil, fmt.Errorf("读取版本行失败: %w", err)
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

func (m *AdminModel) SetVersionStatus(id, status, reason string) (*GameVersion, error) {
	switch status {
	case VersionStatusDraft, VersionStatusPending, VersionStatusApproved, VersionStatusRejected, VersionStatusPublished, VersionStatusRevoked:
	default:
		return nil, errors.New("非法版本状态")
	}
	if _, err := m.db.Exec(
		`UPDATE game_versions SET status = ?, review_reason = ? WHERE id = ?`, status, reason, id); err != nil {
		return nil, fmt.Errorf("更新版本状态失败: %w", err)
	}
	return m.GetVersion(id)
}

// PublishVersion 发布渠道包：仅已过审版本可发布；version_code 低于当前已发布
// 版本时拒绝（避免低版本包覆盖已发布更新，F07）。version_code 为 0 时跳过比较。
func (m *AdminModel) PublishVersion(id string) (*GameVersion, error) {
	v, err := m.GetVersion(id)
	if err != nil {
		return nil, err
	}
	if v.Status != VersionStatusApproved {
		return nil, errors.New("仅已过审版本可发布")
	}
	if v.VersionCode > 0 {
		var latestCode int64
		err := m.db.QueryRow(
			`SELECT COALESCE(MAX(version_code), 0) FROM game_versions WHERE game_id = ? AND status = ?`,
			v.GameId, VersionStatusPublished).Scan(&latestCode)
		if err == nil && latestCode > v.VersionCode {
			return nil, errors.New("不能发布低于当前已发布版本的渠道包")
		}
		if err != nil && err != sql.ErrNoRows {
			return nil, fmt.Errorf("查询已发布版本失败: %w", err)
		}
	}
	return m.SetVersionStatus(id, VersionStatusPublished, "")
}

// RevokeVersion 撤回渠道包：仅已发布版本可撤回。
func (m *AdminModel) RevokeVersion(id string) (*GameVersion, error) {
	v, err := m.GetVersion(id)
	if err != nil {
		return nil, err
	}
	if v.Status != VersionStatusPublished {
		return nil, errors.New("仅已发布版本可撤回")
	}
	return m.SetVersionStatus(id, VersionStatusRevoked, "")
}

// UpdateVersionPackage 回填包体指纹（GP-105）：合作方上传平台托管包体后，
// 更新哈希/大小/托管地址。仅 draft/pending/rejected 允许换包；approved/
// published/revoked 禁止（审核与发布后不得掉包，与 GP-104 防回退同一原则）。
func (m *AdminModel) UpdateVersionPackage(id, hash string, sizeBytes int64, storageUrl string) (*GameVersion, error) {
	v, err := m.GetVersion(id)
	if err != nil {
		return nil, err
	}
	switch v.Status {
	case VersionStatusDraft, VersionStatusPending, VersionStatusRejected:
	default:
		return nil, ErrVersionPackageLocked
	}
	if _, err := m.db.Exec(
		`UPDATE game_versions SET hash = ?, size_bytes = ?, storage_url = ? WHERE id = ?`,
		hash, sizeBytes, storageUrl, id); err != nil {
		return nil, fmt.Errorf("更新版本包体失败: %w", err)
	}
	return m.GetVersion(id)
}

// GetLatestPublishedVersion 取游戏当前已发布版本（GP-105 受控下载定位）：
// 按 version_code 降序、created_at 兜底；无已发布版本返回 ErrVersionNotFound。
func (m *AdminModel) GetLatestPublishedVersion(gameId string) (*GameVersion, error) {
	v := &GameVersion{}
	err := m.db.QueryRow(
		`SELECT `+versionSelectColumns+` FROM game_versions
		 WHERE game_id = ? AND status = ?
		 ORDER BY version_code DESC, created_at DESC LIMIT 1`,
		gameId, VersionStatusPublished).
		Scan(&v.Id, &v.GameId, &v.Version, &v.Changelog, &v.PackageName, &v.VersionCode,
			&v.SignFingerprint, &v.SizeBytes, &v.Hash, &v.StorageUrl, &v.Status, &v.ReviewReason, &v.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrVersionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询已发布版本失败: %w", err)
	}
	return v, nil
}

// ---- 审计 ----

// WriteAudit 追加审计记录；审计写失败只记日志语义由调用方决定（本层返回错误）。
func (m *AdminModel) WriteAudit(entry *AuditLog) error {
	if entry == nil {
		return errors.New("audit entry required")
	}
	if _, err := m.db.Exec(
		`INSERT INTO audit_logs (actor_id, actor_role, action, resource_type, resource_id, before_json, after_json, ip, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.ActorId, entry.ActorRole, entry.Action, entry.ResourceType, entry.ResourceId,
		entry.BeforeJson, entry.AfterJson, entry.Ip, nowUTC()); err != nil {
		return fmt.Errorf("写入审计失败: %w", err)
	}
	return nil
}

func (m *AdminModel) ListAuditLogs(resourceType string, limit, offset int) ([]*AuditLog, int, error) {
	where, args := "", []any{}
	if resourceType != "" {
		where = ` WHERE resource_type = ?`
		args = append(args, resourceType)
	}
	var total int
	if err := m.db.QueryRow(`SELECT COUNT(*) FROM audit_logs`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计审计失败: %w", err)
	}
	rows, err := m.db.Query(
		`SELECT id, actor_id, actor_role, action, resource_type, resource_id, before_json, after_json, ip, created_at
		 FROM audit_logs`+where+` ORDER BY id DESC LIMIT ? OFFSET ?`,
		append(args, limit, offset)...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询审计列表失败: %w", err)
	}
	defer rows.Close()
	logs := []*AuditLog{}
	for rows.Next() {
		l := &AuditLog{}
		if err := rows.Scan(&l.Id, &l.ActorId, &l.ActorRole, &l.Action, &l.ResourceType, &l.ResourceId,
			&l.BeforeJson, &l.AfterJson, &l.Ip, &l.CreatedAt); err != nil {
			return nil, 0, fmt.Errorf("读取审计行失败: %w", err)
		}
		logs = append(logs, l)
	}
	return logs, total, rows.Err()
}
