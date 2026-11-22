package model

// servermodel.go 补漏分支：查询/写入失败臂（关库）、Scan 失败臂（脏列）、
// ReserveServer 的「主 Exec 失败但非方言错误 → 原样返回」臂。LastInsertId/
// RowsAffected 失败臂与 MySQL 方言回退（INSERT IGNORE）属登记不可构造家族，
// 见 coverage 台账。

import (
	"database/sql"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func newServerModel(t *testing.T) (*AdminModel, *sql.DB) {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+t.TempDir()+"/srv.db")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	m := NewAdminModel(db)
	if err := m.CreateAdminTables(); err != nil {
		t.Fatalf("create admin tables: %v", err)
	}
	return m, db
}

// 关库：所有直接 Exec/Query 的失败臂（无前置 SELECT 或前置已成功）。
func TestServerModel_ClosedDBErrors(t *testing.T) {
	m, db := newServerModel(t)
	db.Close()

	if _, err := m.CreateGameServer("g-1", &GameServer{Name: "S"}); err == nil {
		t.Fatal("CreateGameServer closed db = nil error, want error")
	}
	if _, err := m.GetGameServer(1); err == nil {
		t.Fatal("GetGameServer closed db = nil error, want error")
	}
	if _, err := m.ListGameServers("g-1", true); err == nil {
		t.Fatal("ListGameServers closed db = nil error, want error")
	}
	if _, err := m.UpdateGameServer(&GameServer{Id: 1, Name: "S"}); err == nil {
		t.Fatal("UpdateGameServer closed db = nil error, want error")
	}
	if _, err := m.ListScheduledServers(10); err == nil {
		t.Fatal("ListScheduledServers closed db = nil error, want error")
	}
	if _, err := m.OpenServerIfDue(1, ServerStatusClosed, "2026-01-01T00:00:00Z"); err == nil {
		t.Fatal("OpenServerIfDue closed db = nil error, want error")
	}
	if _, err := m.ListMyReservations(7, 10); err == nil {
		t.Fatal("ListMyReservations closed db = nil error, want error")
	}
	if _, err := m.ListReservationUsersByGame("g-1", 10); err == nil {
		t.Fatal("ListReservationUsersByGame closed db = nil error, want error")
	}
	if _, err := m.ListUnnotifiedReservations(1); err == nil {
		t.Fatal("ListUnnotifiedReservations closed db = nil error, want error")
	}
	// ReserveServer：主 Exec 失败且非方言错误 → 走「原样返回」臂（不经回退）。
	if _, err := m.GetReservation(1, 7); err == nil {
		t.Fatal("GetReservation closed db = nil error, want error")
	}
	if _, _, err := m.ReserveServer(&ServerReservation{GameId: "g-1", ServerId: 1, UserId: 7}); err == nil {
		t.Fatal("ReserveServer closed db = nil error, want error")
	}
}

// 纯守卫臂：ListScheduledServers 非法 limit 钳到 200；OpenServerIfDue 非
// closed/无排期直接返回 false（不触达 UPDATE）。
func TestServerModel_Guards(t *testing.T) {
	m, _ := newServerModel(t)
	if _, err := m.ListScheduledServers(0); err != nil {
		t.Fatalf("ListScheduledServers(0): %v", err)
	}
	if ok, err := m.OpenServerIfDue(1, ServerStatusOpen, ""); err != nil || ok {
		t.Fatalf("OpenServerIfDue open status = %v/%v, want false/nil", ok, err)
	}
	if ok, err := m.OpenServerIfDue(1, ServerStatusClosed, ""); err != nil || ok {
		t.Fatalf("OpenServerIfDue empty planned = %v/%v, want false/nil", ok, err)
	}
}

// 脏列 Scan 失败臂：WHERE 过滤列保持干净，破坏 INTEGER 列（非数字文本）
// 以越过 SQLite 列亲和（TEXT 列会把数字转成文本，无法触发 Scan 失败）。
func TestServerModel_ScanDirtyRows(t *testing.T) {
	m, db := newServerModel(t)
	if _, err := m.CreateGameServer("g-1", &GameServer{
		Name: "S1", Status: ServerStatusClosed, PlannedOpenAt: "2026-01-01T00:00:00Z",
	}); err != nil {
		t.Fatalf("create server: %v", err)
	}
	if _, _, err := m.ReserveServer(&ServerReservation{GameId: "g-1", ServerId: 1, UserId: 7}); err != nil {
		t.Fatalf("reserve: %v", err)
	}

	// ListGameServers（includeClosed=true，仅 game_id 过滤）→ 破坏 sort。
	if _, err := db.Exec(`UPDATE game_servers SET sort = 'abc' WHERE id = 1`); err != nil {
		t.Fatalf("corrupt sort: %v", err)
	}
	if _, err := m.ListGameServers("g-1", true); err == nil {
		t.Fatal("ListGameServers dirty sort = nil error, want scan error")
	}
	if _, err := db.Exec(`UPDATE game_servers SET sort = 0 WHERE id = 1`); err != nil {
		t.Fatalf("restore sort: %v", err)
	}

	// ListScheduledServers（过滤 status/planned_open_at）→ 破坏 sort。
	if _, err := db.Exec(`UPDATE game_servers SET sort = 'xyz' WHERE id = 1`); err != nil {
		t.Fatalf("corrupt sort: %v", err)
	}
	if _, err := m.ListScheduledServers(10); err == nil {
		t.Fatal("ListScheduledServers dirty sort = nil error, want scan error")
	}
	if _, err := db.Exec(`UPDATE game_servers SET sort = 0 WHERE id = 1`); err != nil {
		t.Fatalf("restore sort: %v", err)
	}

	// ListMyReservations（过滤 user_id）→ 破坏 server_id。
	if _, err := db.Exec(`UPDATE server_reservations SET server_id = 'abc' WHERE id = 1`); err != nil {
		t.Fatalf("corrupt server_id: %v", err)
	}
	if _, err := m.ListMyReservations(7, 10); err == nil {
		t.Fatal("ListMyReservations dirty server_id = nil error, want scan error")
	}
	if _, err := db.Exec(`UPDATE server_reservations SET server_id = 1 WHERE id = 1`); err != nil {
		t.Fatalf("restore server_id: %v", err)
	}

	// ListReservationUsersByGame（过滤 game_id）→ 破坏 user_id。
	if _, err := db.Exec(`UPDATE server_reservations SET user_id = 'abc' WHERE id = 1`); err != nil {
		t.Fatalf("corrupt user_id: %v", err)
	}
	if _, err := m.ListReservationUsersByGame("g-1", 10); err == nil {
		t.Fatal("ListReservationUsersByGame dirty user_id = nil error, want scan error")
	}
	if _, err := db.Exec(`UPDATE server_reservations SET user_id = 7 WHERE id = 1`); err != nil {
		t.Fatalf("restore user_id: %v", err)
	}

	// ListUnnotifiedReservations（过滤 server_id/notified_at）→ 破坏 user_id。
	if _, err := db.Exec(`UPDATE server_reservations SET user_id = 'zzz' WHERE id = 1`); err != nil {
		t.Fatalf("corrupt unnotified user_id: %v", err)
	}
	if _, err := m.ListUnnotifiedReservations(1); err == nil {
		t.Fatal("ListUnnotifiedReservations dirty user_id = nil error, want scan error")
	}
}
