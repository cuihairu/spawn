package model

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// Open 打开接入域数据库：DSN 含 file:/ 走 SQLite（本地文件，零配置），
// 否则 MySQL；与 game-catalog/payments 同款探测。
func Open(dataSource string) (*sql.DB, error) {
	if dataSource == "" {
		return nil, fmt.Errorf("datasource 不能为空")
	}
	driver, dsn := "sqlite3", dataSource
	if !strings.Contains(dataSource, "file:") {
		driver, dsn = "mysql", dataSource
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, err
	}
	if err := db.Ping(); err != nil {
		return nil, err
	}
	return db, nil
}

func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
