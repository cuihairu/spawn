package model

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

// timeNow 独立变量便于测试注入（当前无时钟测试，保留接缝）。
var timeNow = time.Now

// Open 打开数据库连接：DSN 含 file: 或 .db 走 SQLite（开发/测试），
// 否则 MySQL（与 game-catalog/user-service 同款探测）。
func Open(dataSource string) (*sql.DB, error) {
	if strings.Contains(dataSource, "file:") || strings.Contains(dataSource, ".db") {
		if err := ensureSQLiteDir(dataSource); err != nil {
			return nil, err
		}
		db, err := sql.Open("sqlite3", dataSource)
		if err != nil {
			return nil, err
		}
		// SQLite 单写者：限制单连接串行化全部访问，防并发写触发
		// SQLITE_BUSY（P1-3 修复）；MySQL 由连接池并发（InnoDB 行锁）。
		db.SetMaxOpenConns(1)
		return db, nil
	}
	return sql.Open("mysql", dataSource)
}

func ensureSQLiteDir(dsn string) error {
	path := strings.TrimPrefix(dsn, "file:")
	if idx := strings.IndexByte(path, '?'); idx >= 0 {
		path = path[:idx]
	}
	if path == "" || path == ":memory:" {
		return nil
	}
	dir := filepath.Dir(path)
	if dir == "." || dir == "/" {
		return nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create sqlite dir %s: %w", dir, err)
	}
	return nil
}

// CreateTables 建库表（幂等）。映射/绑定 DDL 与接入设计 §1.2/§2.1 一致，
// 另加 courier_secret_enc（代登录随机密码的 AES-GCM 密文，不落明文）与
// 适配审计表（绑定/注销/能力变更写审计，GP-004 口径）。
func CreateTables(db *sql.DB) error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS courier_game_mapping (
		  spawn_game_id   VARCHAR(64) PRIMARY KEY,
		  courier_game_id VARCHAR(64) NOT NULL,
		  env             VARCHAR(16) NOT NULL DEFAULT 'prod',
		  partner_id      INTEGER     NOT NULL DEFAULT 0,
		  capabilities    TEXT        NOT NULL DEFAULT '{"login":true,"payment":false,"download":false}',
		  created_at      VARCHAR(32) NOT NULL,
		  updated_at      VARCHAR(32) NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_mapping_scope ON courier_game_mapping (courier_game_id, env)`,
		`CREATE TABLE IF NOT EXISTS platform_courier_binding (
		  spawn_user_id      INTEGER PRIMARY KEY,
		  courier_account_id VARCHAR(64) NOT NULL,
		  courier_email      VARCHAR(255) NOT NULL DEFAULT '',
		  courier_secret_enc TEXT NOT NULL DEFAULT '',
		  status             VARCHAR(16) NOT NULL DEFAULT 'ACTIVE',
		  bind_method        VARCHAR(16) NOT NULL DEFAULT 'auto',
		  bound_at           VARCHAR(32) NOT NULL,
		  unbound_at         VARCHAR(32) NOT NULL DEFAULT '',
		  created_at         VARCHAR(32) NOT NULL,
		  updated_at         VARCHAR(32) NOT NULL
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uk_binding_courier ON platform_courier_binding (courier_account_id)`,
		`CREATE TABLE IF NOT EXISTS adapter_audit_logs (
		  id            INTEGER PRIMARY KEY AUTOINCREMENT,
		  actor_id      INTEGER NOT NULL,
		  action        VARCHAR(64) NOT NULL,
		  resource_type VARCHAR(32) NOT NULL,
		  resource_id   VARCHAR(64) NOT NULL,
		  detail        TEXT NOT NULL DEFAULT '',
		  ip            VARCHAR(64) NOT NULL DEFAULT '',
		  created_at    VARCHAR(32) NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_adapter_audit_actor ON adapter_audit_logs (actor_id, created_at)`,
	}
	for _, stmt := range stmts {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("create table: %w", err)
		}
	}
	return nil
}

func nowStr() string {
	return timeNow().Format("2006-01-02T15:04:05.000Z")
}
