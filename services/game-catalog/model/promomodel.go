package model

// GP-403 单层渠道推广（F22）：渠道标识登记、推广链接点击、归因窗口、
// 渠道包（下载）追踪、变更历史（复用 audit_logs）。表建在 CreateAdminTables
// （与评价/收藏同批），CRUD 在本文件（userreviewmodel.go 同款：方法挂
// *AdminModel，归属与角色校验在 logic 层）。
//
// 口径边界：只做单层归因（渠道→用户），不引入多级代理（无 parent_id 列，
// 代理层级不随本任务默认引入）。

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

// 渠道状态：disabled 渠道的推广链接不再计点击、不再归因（存量点击不受影响）。
const (
	PromoChannelActive   = "active"
	PromoChannelDisabled = "disabled"
)

// 点击类型：click = 推广链接落地点击（归因窗口起算点）；download = 渠道包
// 下载（渠道包追踪计数）。
const (
	PromoClickClick    = "click"
	PromoClickDownload = "download"
)

// PromoChannel 推广渠道（单层）。Code 为渠道标识（推广链接
// ?channel=<code>），全局唯一；AttributionWindowHours 为归因窗口小时数，
// 点击后超过窗口的注册不计入该渠道。
type PromoChannel struct {
	Id                     int64  `json:"id"`
	Code                   string `json:"code"`
	Name                   string `json:"name"`
	AttributionWindowHours int64  `json:"attribution_window_hours"`
	Status                 string `json:"status"`
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`
}

// PromoClick 推广点击/下载记录。ClickId 唯一（client 携带，重放幂等）；
// Channel 为渠道标识冗余，渠道改名后历史仍可读；UserId 归因命中时回写。
type PromoClick struct {
	Id         int64  `json:"id"`
	ClickId    string `json:"click_id"`
	ChannelId  int64  `json:"channel_id"`
	Channel    string `json:"channel"`
	UserId     int64  `json:"user_id"`
	Kind       string `json:"kind"`
	GameId     string `json:"game_id"`
	OccurredAt string `json:"occurred_at"`
	CreatedAt  string `json:"created_at"`
}

// ChannelStats 渠道包/渠道效果计数（窗口内）。
type ChannelStats struct {
	Clicks     int64 `json:"clicks"`
	Downloads  int64 `json:"downloads"`
	Attributes int64 `json:"attributed_users"`
}

// ---- 渠道登记 ----

// CreatePromoChannel 新建渠道；code 撞唯一键由 logic 映射 409。
func (m *AdminModel) CreatePromoChannel(code, name string, windowHours int64) (*PromoChannel, error) {
	now := nowUTC()
	res, err := m.db.Exec(
		`INSERT INTO promo_channels (code, name, attribution_window_hours, status, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		code, name, windowHours, PromoChannelActive, now, now,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &PromoChannel{
		Id: id, Code: code, Name: name,
		AttributionWindowHours: windowHours, Status: PromoChannelActive,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// GetPromoChannel 按主键取渠道（不存在 → ErrPromoChannelNotFound）。
func (m *AdminModel) GetPromoChannel(id int64) (*PromoChannel, error) {
	return scanPromoChannel(m.db.QueryRow(
		`SELECT id, code, name, attribution_window_hours, status, created_at, updated_at
		 FROM promo_channels WHERE id = ?`, id))
}

// GetPromoChannelByCode 按渠道标识取渠道（推广链接解析入口）。
func (m *AdminModel) GetPromoChannelByCode(code string) (*PromoChannel, error) {
	return scanPromoChannel(m.db.QueryRow(
		`SELECT id, code, name, attribution_window_hours, status, created_at, updated_at
		 FROM promo_channels WHERE code = ?`, code))
}

// ListPromoChannels 渠道列表（新→旧）。
func (m *AdminModel) ListPromoChannels(limit, offset int) ([]*PromoChannel, error) {
	rows, err := m.db.Query(
		`SELECT id, code, name, attribution_window_hours, status, created_at, updated_at
		 FROM promo_channels ORDER BY id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []*PromoChannel{}
	for rows.Next() {
		var c PromoChannel
		if err := rows.Scan(&c.Id, &c.Code, &c.Name, &c.AttributionWindowHours,
			&c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, &c)
	}
	return list, rows.Err()
}

// UpdatePromoChannel 改渠道名/归因窗口；窗口收紧后，既有点击按旧窗口已记录的
// 事实不回改（历史结算不变，只影响其后新点击）。
func (m *AdminModel) UpdatePromoChannel(id int64, name string, windowHours int64) (*PromoChannel, error) {
	now := nowUTC()
	if _, err := m.db.Exec(
		`UPDATE promo_channels SET name = ?, attribution_window_hours = ?, updated_at = ? WHERE id = ?`,
		name, windowHours, now, id); err != nil {
		return nil, err
	}
	return m.GetPromoChannel(id)
}

// SetPromoChannelStatus 启用/停用渠道。
func (m *AdminModel) SetPromoChannelStatus(id int64, status string) (*PromoChannel, error) {
	now := nowUTC()
	if _, err := m.db.Exec(
		`UPDATE promo_channels SET status = ?, updated_at = ? WHERE id = ?`, status, now, id); err != nil {
		return nil, err
	}
	return m.GetPromoChannel(id)
}

func scanPromoChannel(row *sql.Row) (*PromoChannel, error) {
	var c PromoChannel
	if err := row.Scan(&c.Id, &c.Code, &c.Name, &c.AttributionWindowHours,
		&c.Status, &c.CreatedAt, &c.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPromoChannelNotFound
		}
		return nil, err
	}
	return &c, nil
}

// ---- 点击与归因 ----

// RecordPromoClick 记一条点击/下载；click_id 唯一键幂等（重放返回既有记录）。
func (m *AdminModel) RecordPromoClick(clickId string, ch *PromoChannel, kind, gameId string, now string) (*PromoClick, error) {
	if now == "" {
		now = nowUTC()
	}
	res, err := m.db.Exec(
		`INSERT OR IGNORE INTO promo_clicks (click_id, channel_id, channel, kind, game_id, occurred_at, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		clickId, ch.Id, ch.Code, kind, gameId, now, now,
	)
	if err != nil {
		// MySQL：无 OR IGNORE 语法，回落 INSERT IGNORE（按语法错误判定，
		// 与 servermodel/userreviewmodel 同款）
		msg := err.Error()
		if !strings.Contains(msg, "syntax") && !strings.Contains(msg, "near") {
			return nil, err
		}
		res, err = m.db.Exec(
			`INSERT IGNORE INTO promo_clicks (click_id, channel_id, channel, kind, game_id, occurred_at, created_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			clickId, ch.Id, ch.Code, kind, gameId, now, now,
		)
		if err != nil {
			return nil, err
		}
	}
	var id int64
	var affected int64
	if affected, err = res.RowsAffected(); err != nil {
		return nil, err
	}
	if affected == 0 {
		// 幂等命中：回读既有记录（重放不改归因结果）
		return m.GetPromoClick(clickId)
	}
	if id, err = res.LastInsertId(); err != nil {
		return nil, err
	}
	return &PromoClick{
		Id: id, ClickId: clickId, ChannelId: ch.Id, Channel: ch.Code,
		Kind: kind, GameId: gameId, OccurredAt: now, CreatedAt: now,
	}, nil
}

// GetPromoClick 按 click_id 取点击记录（不存在 → ErrPromoClickNotFound）。
func (m *AdminModel) GetPromoClick(clickId string) (*PromoClick, error) {
	return scanPromoClick(m.db.QueryRow(
		`SELECT id, click_id, channel_id, channel, user_id, kind, game_id, occurred_at, created_at
		 FROM promo_clicks WHERE click_id = ?`, clickId))
}

// MarkClickAttributed 归因命中：回写归因到该点击的用户。user_id 未占位
// （=0）时可归因；已归因到同一用户时视为幂等命中（返回 true 不再写）；
// 已归因到他人则不覆盖（一点击一归因，渠道间不抢注）。
func (m *AdminModel) MarkClickAttributed(clickId string, userId int64) (bool, error) {
	cur, err := m.GetPromoClick(clickId)
	if err != nil {
		return false, err
	}
	if cur.UserId == userId {
		return true, nil
	}
	if cur.UserId > 0 {
		return false, nil
	}
	res, err := m.db.Exec(
		`UPDATE promo_clicks SET user_id = ? WHERE click_id = ? AND user_id = 0`,
		userId, clickId)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func scanPromoClick(row *sql.Row) (*PromoClick, error) {
	var c PromoClick
	if err := row.Scan(&c.Id, &c.ClickId, &c.ChannelId, &c.Channel, &c.UserId,
		&c.Kind, &c.GameId, &c.OccurredAt, &c.CreatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrPromoClickNotFound
		}
		return nil, err
	}
	return &c, nil
}

// ChannelStats 渠道包/渠道效果计数（occurred_at 区间，闭区间字符串比较：
// occurred_at 为定宽 RFC3339，字典序即时间序）。
//
// 口径注记：kind 常量以 SQL 字面量内联而非占位符——SELECT 中的 ? 在语句文本里
// 先于 WHERE 出现，若全部走占位符，channel/from/to 的参数会被 CASE 吃掉
// （SQLite 按 ? 的文本位置编号），条件永远不命中且聚合返回 NULL。
// COALESCE 兜底空集（无记录时 SUM 为 NULL）。
func (m *AdminModel) ChannelStats(channelCode string, from, to string) (*ChannelStats, error) {
	s := &ChannelStats{}
	args := []any{channelCode}
	where := `WHERE channel = ?`
	if from != "" {
		where += ` AND occurred_at >= ?`
		args = append(args, from)
	}
	if to != "" {
		where += ` AND occurred_at <= ?`
		args = append(args, to)
	}
	quote := func(v string) string { return "'" + strings.ReplaceAll(v, "'", "''") + "'" }
	query := `SELECT
			COALESCE(SUM(CASE WHEN kind = ` + quote(PromoClickClick) + ` THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN kind = ` + quote(PromoClickDownload) + ` THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN kind = ` + quote(PromoClickClick) + ` AND user_id > 0 THEN 1 ELSE 0 END), 0)
		 FROM promo_clicks ` + where
	if err := m.db.QueryRow(query, args...).Scan(&s.Clicks, &s.Downloads, &s.Attributes); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return s, nil
		}
		return nil, err
	}
	return s, nil
}

// IsClickWithinWindow 归因窗口判定：点击时刻 + 窗口小时数 ≥ 发生时刻。
// occurred 为空视为"刚刚"（调用方兜底）。解析失败按窗口外处理（fail-closed，
// 避免把无法定界的注册算进渠道业绩）。
func IsClickWithinWindow(clickAt string, windowHours int64, at string) bool {
	clicked, err := time.Parse(time.RFC3339, clickAt)
	if err != nil {
		return false
	}
	if at == "" {
		return true
	}
	happened, err := time.Parse(time.RFC3339, at)
	if err != nil {
		return false
	}
	return !happened.After(clicked.Add(time.Duration(windowHours) * time.Hour))
}
