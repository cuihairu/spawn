package model

import (
	"database/sql"
	"encoding/json"
	"errors"
)

// ErrMappingNotFound 映射不存在（游戏未接入 Courier）。
var ErrMappingNotFound = errors.New("courier game mapping not found")

// Capabilities 每游戏开通能力（接入设计 §2.1：按需开通，与标签解耦；
// 「联运」标签只作展示，永不联动本表）。Support = GP-307 售后工单
// （Courier /v1/support/* 代登录代理）；存量行无该键 = 未开通（fail-closed）。
type Capabilities struct {
	Login    bool `json:"login"`
	Payment  bool `json:"payment"`
	Download bool `json:"download"`
	Support  bool `json:"support"`
}

// GameMapping spawn_game_id ↔ (courier_game_id, env) 一一映射（GP-002）。
type GameMapping struct {
	SpawnGameId   string
	CourierGameId string
	Env           string
	PartnerId     int64
	Capabilities  Capabilities
	CreatedAt     string
	UpdatedAt     string
}

type MappingModel struct {
	db *sql.DB
}

func NewMappingModel(db *sql.DB) *MappingModel {
	return &MappingModel{db: db}
}

// Get 按 spawn_game_id 查映射；不存在返回 ErrMappingNotFound。
func (m *MappingModel) Get(spawnGameID string) (*GameMapping, error) {
	row := m.db.QueryRow(
		`SELECT spawn_game_id, courier_game_id, env, partner_id, capabilities, created_at, updated_at
		 FROM courier_game_mapping WHERE spawn_game_id = ?`, spawnGameID)
	var g GameMapping
	var caps string
	if err := row.Scan(&g.SpawnGameId, &g.CourierGameId, &g.Env, &g.PartnerId, &caps, &g.CreatedAt, &g.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrMappingNotFound
		}
		return nil, err
	}
	if err := decodeCapabilities(caps, &g.Capabilities); err != nil {
		return nil, err
	}
	return &g, nil
}

// Upsert 写入/更新映射。(courier_game_id, env) 全局唯一（scope.md），
// 冲突时以错误返回，由调用方提示运营修正。
func (m *MappingModel) Upsert(g *GameMapping) error {
	caps, err := encodeCapabilities(g.Capabilities)
	if err != nil {
		return err
	}
	now := nowStr()
	// SQLite 与 MySQL 均可执行的 upsert 口径：先 UPDATE，未命中再 INSERT。
	res, err := m.db.Exec(
		`UPDATE courier_game_mapping
		 SET courier_game_id = ?, env = ?, partner_id = ?, capabilities = ?, updated_at = ?
		 WHERE spawn_game_id = ?`,
		g.CourierGameId, g.Env, g.PartnerId, caps, now, g.SpawnGameId)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		return nil
	}
	_, err = m.db.Exec(
		`INSERT INTO courier_game_mapping
		 (spawn_game_id, courier_game_id, env, partner_id, capabilities, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		g.SpawnGameId, g.CourierGameId, g.Env, g.PartnerId, caps, now, now)
	return err
}

func decodeCapabilities(raw string, out *Capabilities) error {
	// 空值/坏值按全关处理（端点侧再以能力开关拒绝），不让坏配置打断读路径。
	if raw == "" {
		return nil
	}
	return json.Unmarshal([]byte(raw), out)
}

func encodeCapabilities(c Capabilities) (string, error) {
	b, err := json.Marshal(c)
	if err != nil {
		return "", err
	}
	return string(b), nil
}
