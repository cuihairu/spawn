// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package svc

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
	"github.com/tappi/tappi/services/game-catalog/model"
	"github.com/tappi/tappi/services/game-catalog/utils"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

type ServiceContext struct {
	Config         config.Config
	GameRepository model.GameStore
	Auth           *utils.Auth
}

func NewServiceContext(c config.Config) *ServiceContext {
	// DSN 含 file: 或 .db → SQLite（开发/测试），否则 MySQL（与
	// user-service.NewServiceContext 同款探测）。
	var db *sql.DB
	var err error

	dataSource := c.MySQL.DataSource
	if strings.Contains(dataSource, "file:") || strings.Contains(dataSource, ".db") {
		dataSource = ensureSQLiteDir(dataSource)
		db, err = sql.Open("sqlite3", dataSource)
	} else {
		db, err = sql.Open("mysql", dataSource)
	}
	if err != nil {
		panic(fmt.Sprintf("连接数据库失败: %v", err))
	}

	// 启动期连通性检查，失败快速终止
	if err := db.Ping(); err != nil {
		panic(fmt.Sprintf("数据库连接测试失败: %v", err))
	}

	gameModel := model.NewGameModel(db)

	// 建表 + 空表写入内嵌种子（首次启动初始化演示目录）
	if err := gameModel.CreateGamesTable(); err != nil {
		panic(fmt.Sprintf("创建游戏表失败: %v", err))
	}
	if err := gameModel.SeedIfEmpty(); err != nil {
		panic(fmt.Sprintf("初始化游戏种子数据失败: %v", err))
	}

	return &ServiceContext{
		Config:         c,
		GameRepository: gameModel,
		Auth:           utils.NewAuth(c.Auth.JWTSecret),
	}
}

// ensureSQLiteDir 为 SQLite 文件 DSN 预创建父目录；内存库与无路径 DSN 原样返回。
// 目录创建失败不在此处报错，交给后续 Ping 暴露真实原因。
func ensureSQLiteDir(dsn string) string {
	path := strings.TrimPrefix(dsn, "file:")
	if i := strings.Index(path, "?"); i >= 0 {
		path = path[:i]
	}
	if path == "" || path == ":memory:" {
		return dsn
	}
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	return dsn
}
