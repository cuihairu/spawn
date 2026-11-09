package model

// GP-301 活动基础：活动配置 + 发布版本快照（F18）。
//
// 活动生命周期：draft（草稿）→ published（已发布，版本快照落定）→
// disabled（停用）→ revise（回草稿可改，下次发布新版本）。**每次发布生成
// 不可变版本快照**（activity_versions.snapshot），运行时按 current_version
// 引用，旧版本保留可回溯；发布后不可再改当前配置（须 revise 出新版本）。
//
// 服务端配置域（Spawn 拥有）：起止时间/时区、游戏/区服/渠道/人群、触发
// 条件、限次、预算、叠加与奖励均以结构化配置存储；限次/预算的运行时扣减
// 与倍率/优惠计算归 GP-302/303/305，本表只落配置与版本快照。

import (
	"database/sql"
	"errors"
	"fmt"
)

var (
	ErrActivityNotFound = errors.New("activity not found")
	ErrVersionNotFound  = errors.New("activity version not found")
)

// 活动状态。
const (
	ActivityStatusDraft     = "draft"
	ActivityStatusPublished = "published"
	ActivityStatusDisabled  = "disabled"
)

// Activity 活动配置（server_ids/audience/trigger/limits/rewards 为 JSON 文本，
// 逻辑层负责 JSON 编解码；服务端配置域，不信任客户端金额/规则）。
type Activity struct {
	Id             int64  `json:"id"`
	Name           string `json:"name"`
	GameId         string `json:"game_id"`
	Status         string `json:"status"`
	Timezone       string `json:"timezone"`
	StartAt        string `json:"start_at"`
	EndAt          string `json:"end_at"`
	ServerIds      string `json:"server_ids"` // JSON 数组；"" = 全部区服
	Channel        string `json:"channel"`    // "" = 全部渠道
	Audience       string `json:"audience"`   // JSON 人群标签
	TriggerSpec    string `json:"trigger_spec"`
	LimitsSpec     string `json:"limits_spec"`
	Stackable      bool   `json:"stackable"`
	StackGroup     string `json:"stack_group"`
	RewardsSpec    string `json:"rewards_spec"`
	CurrentVersion int64  `json:"current_version"`
	CreatedBy      int64  `json:"created_by"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// ActivityVersion 发布版本快照（不可变；发布即启用）。
type ActivityVersion struct {
	Id          int64  `json:"id"`
	ActivityId  int64  `json:"activity_id"`
	Version     int64  `json:"version"`
	Snapshot    string `json:"snapshot"` // 完整配置 JSON 快照
	PublishedBy int64  `json:"published_by"`
	PublishedAt string `json:"published_at"`
	CreatedAt   string `json:"created_at"`
}

// ActivityModel 活动配置数据访问。
type ActivityModel struct {
	db *sql.DB
}

func NewActivityModel(db *sql.DB) *ActivityModel {
	return &ActivityModel{db: db}
}

func (m *ActivityModel) CreateTables() error {
	tables := []struct{ sqlite, mysql string }{
		{
			`CREATE TABLE IF NOT EXISTS activities (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				name VARCHAR(128) NOT NULL,
				game_id VARCHAR(64) NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'draft',
				timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
				start_at VARCHAR(32) NOT NULL DEFAULT '',
				end_at VARCHAR(32) NOT NULL DEFAULT '',
				server_ids TEXT NOT NULL DEFAULT '',
				channel VARCHAR(32) NOT NULL DEFAULT '',
				audience TEXT NOT NULL DEFAULT '{}',
				trigger_spec TEXT NOT NULL DEFAULT '{}',
				limits_spec TEXT NOT NULL DEFAULT '{}',
				stackable INTEGER NOT NULL DEFAULT 0,
				stack_group VARCHAR(64) NOT NULL DEFAULT '',
				rewards_spec TEXT NOT NULL DEFAULT '{}',
				current_version INTEGER NOT NULL DEFAULT 0,
				created_by INTEGER NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS activities (
				id BIGINT PRIMARY KEY AUTO_INCREMENT,
				name VARCHAR(128) NOT NULL,
				game_id VARCHAR(64) NOT NULL,
				status VARCHAR(16) NOT NULL DEFAULT 'draft',
				timezone VARCHAR(64) NOT NULL DEFAULT 'Asia/Shanghai',
				start_at VARCHAR(32) NOT NULL DEFAULT '',
				end_at VARCHAR(32) NOT NULL DEFAULT '',
				server_ids TEXT NOT NULL,
				channel VARCHAR(32) NOT NULL DEFAULT '',
				audience TEXT NOT NULL,
				trigger_spec TEXT NOT NULL,
				limits_spec TEXT NOT NULL,
				stackable TINYINT NOT NULL DEFAULT 0,
				stack_group VARCHAR(64) NOT NULL DEFAULT '',
				rewards_spec TEXT NOT NULL,
				current_version BIGINT NOT NULL DEFAULT 0,
				created_by BIGINT NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				updated_at VARCHAR(32) NOT NULL,
				KEY idx_activity_game (game_id, status)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		{
			`CREATE TABLE IF NOT EXISTS activity_versions (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				activity_id INTEGER NOT NULL,
				version INTEGER NOT NULL,
				snapshot TEXT NOT NULL,
				published_by INTEGER NOT NULL DEFAULT 0,
				published_at VARCHAR(32) NOT NULL,
				created_at VARCHAR(32) NOT NULL,
				UNIQUE (activity_id, version)
			)`,
			`CREATE TABLE IF NOT EXISTS activity_versions (
				id BIGINT PRIMARY KEY AUTO_INCREMENT,
				activity_id BIGINT NOT NULL,
				version BIGINT NOT NULL,
				snapshot TEXT NOT NULL,
				published_by BIGINT NOT NULL DEFAULT 0,
				published_at VARCHAR(32) NOT NULL,
				created_at VARCHAR(32) NOT NULL,
				UNIQUE KEY uk_activity_version (activity_id, version),
				KEY idx_av_activity (activity_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
		{
			`CREATE TABLE IF NOT EXISTS activity_audit_logs (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				activity_id INTEGER NOT NULL DEFAULT 0,
				actor_id INTEGER NOT NULL DEFAULT 0,
				actor_role VARCHAR(32) NOT NULL DEFAULT '',
				action VARCHAR(64) NOT NULL,
				detail VARCHAR(1024) NOT NULL DEFAULT '',
				ip VARCHAR(64) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS activity_audit_logs (
				id BIGINT PRIMARY KEY AUTO_INCREMENT,
				activity_id BIGINT NOT NULL DEFAULT 0,
				actor_id BIGINT NOT NULL DEFAULT 0,
				actor_role VARCHAR(32) NOT NULL DEFAULT '',
				action VARCHAR(64) NOT NULL,
				detail VARCHAR(1024) NOT NULL DEFAULT '',
				ip VARCHAR(64) NOT NULL DEFAULT '',
				created_at VARCHAR(32) NOT NULL,
				KEY idx_aa_activity (activity_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		},
	}
	for _, t := range tables {
		if _, err := m.db.Exec(t.sqlite); err != nil {
			if _, err2 := m.db.Exec(t.mysql); err2 != nil {
				return errors.New("创建活动表失败（尝试了 SQLite 和 MySQL 格式）")
			}
		}
	}
	return nil
}

const activityColumns = "id, name, game_id, status, timezone, start_at, end_at, server_ids, channel, audience, trigger_spec, limits_spec, stackable, stack_group, rewards_spec, current_version, created_by, created_at, updated_at"

func scanActivity(a *Activity) []any {
	return []any{&a.Id, &a.Name, &a.GameId, &a.Status, &a.Timezone, &a.StartAt, &a.EndAt,
		&a.ServerIds, &a.Channel, &a.Audience, &a.TriggerSpec, &a.LimitsSpec,
		&a.Stackable, &a.StackGroup, &a.RewardsSpec, &a.CurrentVersion,
		&a.CreatedBy, &a.CreatedAt, &a.UpdatedAt}
}

// Create 落活动草稿，回填 Id。
func (m *ActivityModel) Create(a *Activity) error {
	now := nowUTC()
	a.CreatedAt, a.UpdatedAt = now, now
	res, err := m.db.Exec(
		`INSERT INTO activities (name, game_id, status, timezone, start_at, end_at, server_ids, channel, audience, trigger_spec, limits_spec, stackable, stack_group, rewards_spec, current_version, created_by, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.Name, a.GameId, a.Status, a.Timezone, a.StartAt, a.EndAt, a.ServerIds, a.Channel,
		a.Audience, a.TriggerSpec, a.LimitsSpec, a.Stackable, a.StackGroup, a.RewardsSpec,
		a.CurrentVersion, a.CreatedBy, a.CreatedAt, a.UpdatedAt,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	a.Id = id
	return nil
}

// Update 更新活动配置（仅草稿；已发布/停用由逻辑层拒绝）。
func (m *ActivityModel) Update(a *Activity) error {
	_, err := m.db.Exec(
		`UPDATE activities SET name = ?, game_id = ?, timezone = ?, start_at = ?, end_at = ?,
		   server_ids = ?, channel = ?, audience = ?, trigger_spec = ?, limits_spec = ?,
		   stackable = ?, stack_group = ?, rewards_spec = ?, updated_at = ?
		 WHERE id = ?`,
		a.Name, a.GameId, a.Timezone, a.StartAt, a.EndAt, a.ServerIds, a.Channel,
		a.Audience, a.TriggerSpec, a.LimitsSpec, a.Stackable, a.StackGroup, a.RewardsSpec,
		nowUTC(), a.Id,
	)
	return err
}

// PublishSnapshot 发布：落不可变版本快照并推进 current_version（事务原子）。
func (m *ActivityModel) PublishSnapshot(activityID, version int64, snapshot string, by int64) error {
	tx, err := m.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := nowUTC()
	if _, err := tx.Exec(
		`INSERT INTO activity_versions (activity_id, version, snapshot, published_by, published_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		activityID, version, snapshot, by, now, now,
	); err != nil {
		return err
	}
	if _, err := tx.Exec(
		`UPDATE activities SET current_version = ?, status = 'published', updated_at = ? WHERE id = ?`,
		version, now, activityID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// SetStatus 更新状态（disable/revise）。
func (m *ActivityModel) SetStatus(activityID int64, status string) error {
	_, err := m.db.Exec(
		`UPDATE activities SET status = ?, updated_at = ? WHERE id = ?`,
		status, nowUTC(), activityID,
	)
	return err
}

func (m *ActivityModel) Get(id int64) (*Activity, error) {
	var a Activity
	err := m.db.QueryRow(`SELECT `+activityColumns+` FROM activities WHERE id = ?`, id).Scan(scanActivity(&a)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrActivityNotFound
	}
	if err != nil {
		return nil, err
	}
	return &a, nil
}

// List 活动列表（game_id/status 可筛，有界）。
func (m *ActivityModel) List(gameId, status string, limit, offset int) ([]*Activity, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	q := `SELECT ` + activityColumns + ` FROM activities WHERE 1=1`
	args := []any{}
	if gameId != "" {
		q += ` AND game_id = ?`
		args = append(args, gameId)
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
	out := make([]*Activity, 0, limit)
	for rows.Next() {
		var a Activity
		if err := rows.Scan(scanActivity(&a)...); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}

// NextVersion 下一个发布版本号（1 起）。
func (m *ActivityModel) NextVersion(activityID int64) (int64, error) {
	var max int64
	err := m.db.QueryRow(`SELECT COALESCE(MAX(version), 0) FROM activity_versions WHERE activity_id = ?`, activityID).Scan(&max)
	if err != nil {
		return 0, err
	}
	return max + 1, nil
}

// ListVersions 活动版本列表（新→旧）。
func (m *ActivityModel) ListVersions(activityID int64) ([]*ActivityVersion, error) {
	rows, err := m.db.Query(
		`SELECT id, activity_id, version, snapshot, published_by, published_at, created_at
		 FROM activity_versions WHERE activity_id = ? ORDER BY version DESC`,
		activityID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*ActivityVersion, 0)
	for rows.Next() {
		var v ActivityVersion
		if err := rows.Scan(&v.Id, &v.ActivityId, &v.Version, &v.Snapshot, &v.PublishedBy, &v.PublishedAt, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &v)
	}
	return out, rows.Err()
}

// GetVersion 取指定版本快照。
func (m *ActivityModel) GetVersion(activityID, version int64) (*ActivityVersion, error) {
	var v ActivityVersion
	err := m.db.QueryRow(
		`SELECT id, activity_id, version, snapshot, published_by, published_at, created_at
		 FROM activity_versions WHERE activity_id = ? AND version = ?`,
		activityID, version,
	).Scan(&v.Id, &v.ActivityId, &v.Version, &v.Snapshot, &v.PublishedBy, &v.PublishedAt, &v.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrVersionNotFound
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// HasSnapshots 该活动是否已发布过版本。
func (m *ActivityModel) HasSnapshots(activityID int64) (bool, error) {
	version, err := m.NextVersion(activityID)
	if err != nil {
		return false, err
	}
	return version > 1, nil
}

// ActivityAuditLog 活动配置操作审计（actor/action/detail/ip）。
type ActivityAuditLog struct {
	Id         int64  `json:"id"`
	ActivityId int64  `json:"activity_id"`
	ActorId    int64  `json:"actor_id"`
	ActorRole  string `json:"actor_role"`
	Action     string `json:"action"`
	Detail     string `json:"detail"`
	Ip         string `json:"ip"`
	CreatedAt  string `json:"created_at"`
}

// LogAudit 写活动审计。
func (m *ActivityModel) LogAudit(log *ActivityAuditLog) error {
	_, err := m.db.Exec(
		`INSERT INTO activity_audit_logs (activity_id, actor_id, actor_role, action, detail, ip, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		log.ActivityId, log.ActorId, log.ActorRole, log.Action, log.Detail, log.Ip, nowUTC(),
	)
	return err
}

// ListAudit 活动审计列表（activityId 空 = 全部，有界）。
func (m *ActivityModel) ListAudit(activityID int64, limit int) ([]*ActivityAuditLog, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	q := `SELECT id, activity_id, actor_id, actor_role, action, detail, ip, created_at FROM activity_audit_logs WHERE 1=1`
	args := []any{}
	if activityID > 0 {
		q += ` AND activity_id = ?`
		args = append(args, activityID)
	}
	q += fmt.Sprintf(` ORDER BY id DESC LIMIT %d`, limit)
	rows, err := m.db.Query(q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*ActivityAuditLog, 0, limit)
	for rows.Next() {
		var a ActivityAuditLog
		if err := rows.Scan(&a.Id, &a.ActivityId, &a.ActorId, &a.ActorRole, &a.Action, &a.Detail, &a.Ip, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &a)
	}
	return out, rows.Err()
}
