// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package svc

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tappi/tappi/services/data-panel/internal/config"
	"github.com/tappi/tappi/services/data-panel/internal/middleware"
	"github.com/tappi/tappi/services/data-panel/internal/model"
	"github.com/tappi/tappi/services/data-panel/utils"
	"github.com/zeromicro/go-zero/rest"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

type ServiceContext struct {
	Config config.Config
	Auth   rest.Middleware
	Jwt    *utils.Auth

	StatsRepo    model.PlayerStatsStore
	AnalyticsSvc model.AnalyticsStore
}

func NewServiceContext(c config.Config) *ServiceContext {
	jwtTool := utils.NewAuth(c.Auth.JWTSecret)

	// DSN 含 file: 或 .db → SQLite（开发/测试），否则 MySQL（与
	// user-service / game-catalog / content-service / community 同款探测）。
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

	statsModel := model.NewPlayerStatsModel(db)

	// 建表 + 空表写入内嵌演示战绩（首次启动初始化面板演示数据）
	if err := statsModel.CreateTable(); err != nil {
		panic(fmt.Sprintf("创建战绩表失败: %v", err))
	}
	if err := statsModel.SeedIfEmpty(); err != nil {
		panic(fmt.Sprintf("初始化战绩种子数据失败: %v", err))
	}

	// GP-402 经营分析事件域：建 platform_events 表（幂等）
	analyticsModel := model.NewAnalyticsEventsModel(db)
	if err := analyticsModel.CreateAnalyticsTable(); err != nil {
		panic(fmt.Sprintf("创建平台事件表失败: %v", err))
	}

	return &ServiceContext{
		Config:       c,
		Auth:         middleware.NewAuthMiddleware(jwtTool).Handle,
		Jwt:          jwtTool,
		StatsRepo:    statsModel,
		AnalyticsSvc: analyticsModel,
	}
}

// IsAnalyticsAdmin GP-402 经营分析查询权限：AdminUserIds 名单内才可查
// 漏斗/留存/活动效果/账单核对（名单为空即全关，fail-closed）。
func (sc *ServiceContext) IsAnalyticsAdmin(userID string) bool {
	for _, id := range sc.Config.Analytics.AdminUserIds {
		if id == userID {
			return true
		}
	}
	return false
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
