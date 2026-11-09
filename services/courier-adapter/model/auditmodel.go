package model

import (
	"database/sql"
)

// AuditEntry 适配域审计（GP-004 口径的适配侧最小实现）：
// 绑定/解绑/注销同步/映射与能力变更均落审计。
type AuditEntry struct {
	ID           int64  `json:"id"`
	ActorID      int64  `json:"actorId"`
	Action       string `json:"action"`
	ResourceType string `json:"resourceType"`
	ResourceID   string `json:"resourceId"`
	Detail       string `json:"detail"`
	IP           string `json:"ip"`
	CreatedAt    string `json:"createdAt"`
}

type AuditModel struct {
	db *sql.DB
}

func NewAuditModel(db *sql.DB) *AuditModel {
	return &AuditModel{db: db}
}

func (m *AuditModel) Insert(actorID int64, action, resourceType, resourceID, detail, ip string) error {
	_, err := m.db.Exec(
		`INSERT INTO adapter_audit_logs
		 (actor_id, action, resource_type, resource_id, detail, ip, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		actorID, action, resourceType, resourceID, detail, ip, nowStr())
	return err
}

// ListRecent 最近 limit 条（管理面只读）。limit<=0 或超上限按默认/上限处理。
func (m *AuditModel) ListRecent(limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 200 {
		limit = 200
	}
	rows, err := m.db.Query(
		`SELECT id, actor_id, action, resource_type, resource_id, detail, ip, created_at
		 FROM adapter_audit_logs ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]AuditEntry, 0, limit)
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.ActorID, &e.Action, &e.ResourceType, &e.ResourceID,
			&e.Detail, &e.IP, &e.CreatedAt); err != nil {
			return nil, err
		}
		items = append(items, e)
	}
	return items, rows.Err()
}
