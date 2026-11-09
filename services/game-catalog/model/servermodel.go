package model

// GP-201 区服目录/充值目标：game_servers 表 CRUD。
// 归属校验（合作方只能操作自有游戏的区服）在 logic 层做，这里只管数据。

import (
	"database/sql"
	"errors"
	"strings"
)

const serverColumns = "id, game_id, name, region, status, sort, planned_open_at, created_at, updated_at"

func scanServer(s *GameServer) []any {
	return []any{&s.Id, &s.GameId, &s.Name, &s.Region, &s.Status, &s.Sort, &s.PlannedOpenAt, &s.CreatedAt, &s.UpdatedAt}
}

// CreateGameServer 新建区服；状态由 logic 层校验后传入（空串按 open 落库兜底）。
// PlannedOpenAt 为开服计划时间（GP-306，RFC3339；空 = 未排期）。
func (m *AdminModel) CreateGameServer(gameId string, s *GameServer) (*GameServer, error) {
	now := nowUTC()
	if strings.TrimSpace(s.Status) == "" {
		s.Status = ServerStatusOpen
	}
	res, err := m.db.Exec(
		`INSERT INTO game_servers (game_id, name, region, status, sort, planned_open_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		gameId, s.Name, s.Region, s.Status, s.Sort, s.PlannedOpenAt, now, now,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &GameServer{
		Id: id, GameId: gameId, Name: s.Name, Region: s.Region, Status: s.Status,
		Sort: s.Sort, PlannedOpenAt: s.PlannedOpenAt, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// GetGameServer 按主键查区服。
func (m *AdminModel) GetGameServer(id int64) (*GameServer, error) {
	var s GameServer
	err := m.db.QueryRow(
		`SELECT `+serverColumns+` FROM game_servers WHERE id = ?`, id,
	).Scan(scanServer(&s)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrServerNotFound
	}
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// ListGameServers 列游戏的区服，按 sort 升序、id 稳定排序；
// includeClosed=false 供玩家公开侧使用（closed 下线区服不外露）。
func (m *AdminModel) ListGameServers(gameId string, includeClosed bool) ([]*GameServer, error) {
	q := `SELECT ` + serverColumns + ` FROM game_servers WHERE game_id = ?`
	if !includeClosed {
		q += ` AND status != '` + ServerStatusClosed + `'`
	}
	q += ` ORDER BY sort ASC, id ASC`
	rows, err := m.db.Query(q, gameId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*GameServer, 0, 8)
	for rows.Next() {
		var s GameServer
		if err := rows.Scan(scanServer(&s)...); err != nil {
			return nil, err
		}
		out = append(out, &s)
	}
	return out, rows.Err()
}

// UpdateGameServer 全量更新（logic 层先读旧值合并补丁，保持跨库可移植；
// 不以 RowsAffected 判存在——MySQL 默认只计变更行，幂等更新会误报 0）。
func (m *AdminModel) UpdateGameServer(s *GameServer) (*GameServer, error) {
	now := nowUTC()
	if _, err := m.db.Exec(
		`UPDATE game_servers SET name = ?, region = ?, status = ?, sort = ?, planned_open_at = ?, updated_at = ? WHERE id = ?`,
		s.Name, s.Region, s.Status, s.Sort, s.PlannedOpenAt, now, s.Id,
	); err != nil {
		return nil, err
	}
	s.UpdatedAt = now
	return s, nil
}

// ---- GP-306 开服预约 ----

const reservationColumns = "id, game_id, server_id, user_id, notified_at, created_at"

func scanReservation(r *ServerReservation) []any {
	return []any{&r.Id, &r.GameId, &r.ServerId, &r.UserId, &r.NotifiedAt, &r.CreatedAt}
}

// ReserveServer 开服预约（幂等：UNIQUE(server_id,user_id)，重复预约返回既有行
// 与 created=false）。预约语义由 logic 层校验（区服归属、未开服才可预约）。
func (m *AdminModel) ReserveServer(r *ServerReservation) (*ServerReservation, bool, error) {
	now := nowUTC()
	res, err := m.db.Exec(
		`INSERT OR IGNORE INTO server_reservations (game_id, server_id, user_id, notified_at, created_at)
		 VALUES (?, ?, ?, '', ?)`,
		r.GameId, r.ServerId, r.UserId, now,
	)
	if err != nil {
		if strings.Contains(err.Error(), "syntax") || strings.Contains(err.Error(), "near") {
			res, err = m.db.Exec(
				`INSERT IGNORE INTO server_reservations (game_id, server_id, user_id, notified_at, created_at)
				 VALUES (?, ?, ?, '', ?)`,
				r.GameId, r.ServerId, r.UserId, now,
			)
		}
		if err != nil {
			return nil, false, err
		}
	}
	if n, _ := res.RowsAffected(); n == 1 {
		if id, err := res.LastInsertId(); err == nil && id > 0 {
			r.Id = id
		}
		r.NotifiedAt = ""
		r.CreatedAt = now
		return r, true, nil
	}
	existing, err := m.GetReservation(r.ServerId, r.UserId)
	return existing, false, err
}

// GetReservation 查单条预约。
func (m *AdminModel) GetReservation(serverId, userId int64) (*ServerReservation, error) {
	var r ServerReservation
	err := m.db.QueryRow(
		`SELECT `+reservationColumns+` FROM server_reservations WHERE server_id = ? AND user_id = ?`,
		serverId, userId,
	).Scan(scanReservation(&r)...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrReservationNotFound
	}
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// CancelReservation 取消预约（删行；未预约不报错，幂等取消）。
func (m *AdminModel) CancelReservation(serverId, userId int64) error {
	_, err := m.db.Exec(`DELETE FROM server_reservations WHERE server_id = ? AND user_id = ?`, serverId, userId)
	return err
}

// ListMyReservations 玩家我的预约（新在前）。
func (m *AdminModel) ListMyReservations(userId int64, limit int) ([]*ServerReservation, error) {
	rows, err := m.db.Query(
		`SELECT `+reservationColumns+` FROM server_reservations WHERE user_id = ? ORDER BY id DESC LIMIT ?`,
		userId, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*ServerReservation, 0, 8)
	for rows.Next() {
		var r ServerReservation
		if err := rows.Scan(scanReservation(&r)...); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// ListReservationUsersByGame 游戏维度去重预约人群（GP-306 版本/活动通知受众）。
func (m *AdminModel) ListReservationUsersByGame(gameId string, limit int) ([]int64, error) {
	rows, err := m.db.Query(
		`SELECT DISTINCT user_id FROM server_reservations WHERE game_id = ? ORDER BY user_id LIMIT ?`,
		gameId, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int64, 0, 16)
	for rows.Next() {
		var uid int64
		if err := rows.Scan(&uid); err != nil {
			return nil, err
		}
		out = append(out, uid)
	}
	return out, rows.Err()
}

// ListUnnotifiedReservations 区服维度未提醒预约（开服提醒只发一次）。
func (m *AdminModel) ListUnnotifiedReservations(serverId int64) ([]*ServerReservation, error) {
	rows, err := m.db.Query(
		`SELECT `+reservationColumns+` FROM server_reservations WHERE server_id = ? AND notified_at = '' ORDER BY id ASC`,
		serverId,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]*ServerReservation, 0, 8)
	for rows.Next() {
		var r ServerReservation
		if err := rows.Scan(scanReservation(&r)...); err != nil {
			return nil, err
		}
		out = append(out, &r)
	}
	return out, rows.Err()
}

// MarkReservationsNotified 区服维度批量置已提醒（开服提醒送达后调用；
// notified_at=” 行数即提醒数，幂等）。
func (m *AdminModel) MarkReservationsNotified(serverId int64) error {
	_, err := m.db.Exec(
		`UPDATE server_reservations SET notified_at = ? WHERE server_id = ? AND notified_at = ''`,
		nowUTC(), serverId,
	)
	return err
}
