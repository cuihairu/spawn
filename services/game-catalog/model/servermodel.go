package model

// GP-201 区服目录/充值目标：game_servers 表 CRUD。
// 归属校验（合作方只能操作自有游戏的区服）在 logic 层做，这里只管数据。

import (
	"database/sql"
	"errors"
	"strings"
)

const serverColumns = "id, game_id, name, region, status, sort, created_at, updated_at"

func scanServer(s *GameServer) []any {
	return []any{&s.Id, &s.GameId, &s.Name, &s.Region, &s.Status, &s.Sort, &s.CreatedAt, &s.UpdatedAt}
}

// CreateGameServer 新建区服；状态由 logic 层校验后传入（空串按 open 落库兜底）。
func (m *AdminModel) CreateGameServer(gameId string, s *GameServer) (*GameServer, error) {
	now := nowUTC()
	if strings.TrimSpace(s.Status) == "" {
		s.Status = ServerStatusOpen
	}
	res, err := m.db.Exec(
		`INSERT INTO game_servers (game_id, name, region, status, sort, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		gameId, s.Name, s.Region, s.Status, s.Sort, now, now,
	)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	return &GameServer{
		Id: id, GameId: gameId, Name: s.Name, Region: s.Region,
		Status: s.Status, Sort: s.Sort, CreatedAt: now, UpdatedAt: now,
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
		`UPDATE game_servers SET name = ?, region = ?, status = ?, sort = ?, updated_at = ? WHERE id = ?`,
		s.Name, s.Region, s.Status, s.Sort, now, s.Id,
	); err != nil {
		return nil, err
	}
	s.UpdatedAt = now
	return s, nil
}
