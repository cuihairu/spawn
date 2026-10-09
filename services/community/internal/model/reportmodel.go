package model

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/tappi/tappi/services/community/internal/types"
)

// ReportStore 举报记录仓储接口（与 CommentStore 同款双驱动）。
// 去噪契约：UNIQUE (reporter_id, target_type, target_id) + INSERT OR IGNORE，
// 同一用户对同一目标只保留一条举报。
type ReportStore interface {
	CreateReportsTable() error
	Create(r *types.Report) error
	GetByID(id int64) (*types.Report, error)
	ListByStatus(status string, limit, offset int64) ([]*types.Report, int64, error)
	UpdateStatus(id int64, status string, handledBy int64) error
}

type ReportModel struct {
	db *sql.DB
}

// NewReportModel 创建举报模型（连接池钳制为 1，与其他模型共享同池）
func NewReportModel(db *sql.DB) *ReportModel {
	db.SetMaxOpenConns(1)
	return &ReportModel{db: db}
}

const reportColumns = `id, reporter_id, target_type, target_id, reason, status, handled_by, created_at, handled_at`

func (m *ReportModel) CreateReportsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS reports (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			reporter_id INTEGER NOT NULL,
			target_type VARCHAR(16) NOT NULL,
			target_id INTEGER NOT NULL,
			reason VARCHAR(256) NOT NULL DEFAULT '',
			status VARCHAR(16) NOT NULL DEFAULT 'pending',
			handled_by INTEGER NOT NULL DEFAULT 0,
			created_at VARCHAR(32) NOT NULL,
			handled_at VARCHAR(32) NOT NULL DEFAULT '',
			UNIQUE (reporter_id, target_type, target_id)
		)
	`
	if _, err := m.db.Exec(query); err != nil {
		query = `
			CREATE TABLE IF NOT EXISTS reports (
				id BIGINT AUTO_INCREMENT PRIMARY KEY,
				reporter_id BIGINT NOT NULL,
				target_type VARCHAR(16) NOT NULL,
				target_id BIGINT NOT NULL,
				reason VARCHAR(256) NOT NULL DEFAULT '',
				status VARCHAR(16) NOT NULL DEFAULT 'pending',
				handled_by BIGINT NOT NULL DEFAULT 0,
				created_at VARCHAR(32) NOT NULL,
				handled_at VARCHAR(32) NOT NULL DEFAULT '',
				UNIQUE KEY uk_report_dedup (reporter_id, target_type, target_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci
		`
		if _, err := m.db.Exec(query); err != nil {
			return fmt.Errorf("创建举报表失败（尝试了 SQLite 和 MySQL 格式）: %w", err)
		}
	}
	return nil
}

// Create 幂等写入举报：唯一键冲突静默忽略（与通知写入同款策略）。
func (m *ReportModel) Create(r *types.Report) error {
	_, err := m.db.Exec(
		`INSERT OR IGNORE INTO reports (reporter_id, target_type, target_id, reason, status, handled_by, created_at, handled_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ReporterId, r.TargetType, r.TargetId, r.Reason, r.Status, r.HandledBy, r.CreatedAt, r.HandledAt,
	)
	if err == nil {
		return nil
	}
	if _, retryErr := m.db.Exec(
		`INSERT IGNORE INTO reports (reporter_id, target_type, target_id, reason, status, handled_by, created_at, handled_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ReporterId, r.TargetType, r.TargetId, r.Reason, r.Status, r.HandledBy, r.CreatedAt, r.HandledAt,
	); retryErr != nil {
		return fmt.Errorf("写入举报失败（SQLite: %v；MySQL: %w）", err, retryErr)
	}
	return nil
}

func (m *ReportModel) GetByID(id int64) (*types.Report, error) {
	row := m.db.QueryRow(`SELECT `+reportColumns+` FROM reports WHERE id = ?`, id)
	r := &types.Report{}
	err := row.Scan(
		&r.Id, &r.ReporterId, &r.TargetType, &r.TargetId, &r.Reason, &r.Status, &r.HandledBy, &r.CreatedAt, &r.HandledAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrReportNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("查询举报失败: %w", err)
	}
	return r, nil
}

// ListByStatus 按状态过滤（空串 = 全部状态），id 倒序 + 全量 total。
func (m *ReportModel) ListByStatus(status string, limit, offset int64) ([]*types.Report, int64, error) {
	var total int64
	var countQuery string
	var countArgs []interface{}
	if status == "" {
		countQuery = `SELECT COUNT(*) FROM reports`
	} else {
		countQuery = `SELECT COUNT(*) FROM reports WHERE status = ?`
		countArgs = append(countArgs, status)
	}
	if err := m.db.QueryRow(countQuery, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("统计举报数失败: %w", err)
	}

	query := `SELECT ` + reportColumns + ` FROM reports`
	args := make([]interface{}, 0, 3)
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := m.db.Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("查询举报失败: %w", err)
	}
	defer rows.Close()

	list := make([]*types.Report, 0)
	for rows.Next() {
		r := &types.Report{}
		if err := rows.Scan(
			&r.Id, &r.ReporterId, &r.TargetType, &r.TargetId, &r.Reason, &r.Status, &r.HandledBy, &r.CreatedAt, &r.HandledAt,
		); err != nil {
			return nil, 0, fmt.Errorf("读取举报失败: %w", err)
		}
		list = append(list, r)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("遍历举报失败: %w", err)
	}
	return list, total, nil
}

func (m *ReportModel) UpdateStatus(id int64, status string, handledBy int64) error {
	res, err := m.db.Exec(
		`UPDATE reports SET status = ?, handled_by = ?, handled_at = ? WHERE id = ?`,
		status, handledBy, time.Now().UTC().Format(time.RFC3339), id,
	)
	if err != nil {
		return fmt.Errorf("更新举报状态失败: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrReportNotFound
	}
	return nil
}
