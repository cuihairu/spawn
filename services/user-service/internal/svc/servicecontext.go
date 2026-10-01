// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package svc

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/tappi/tappi/services/user-service/internal/config"
	"github.com/tappi/tappi/services/user-service/internal/integration"
	"github.com/tappi/tappi/services/user-service/model"
	"github.com/tappi/tappi/services/user-service/utils"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

type ServiceContext struct {
	Config            config.Config
	DB                *sql.DB
	UserModel         model.UserStore
	Auth              *utils.Auth
	GameCatalogClient *integration.GameCatalogClient
}

func NewServiceContext(c config.Config) *ServiceContext {
	// 检测数据库类型并连接
	var db *sql.DB
	var err error

	dataSource := c.MySQL.DataSource
	if strings.Contains(dataSource, "file:") || strings.Contains(dataSource, ".db") {
		// SQLite 数据库
		db, err = sql.Open("sqlite3", dataSource)
	} else {
		// MySQL 数据库
		db, err = sql.Open("mysql", dataSource)
	}

	if err != nil {
		panic(fmt.Sprintf("连接数据库失败: %v", err))
	}

	// 测试数据库连接
	if err := db.Ping(); err != nil {
		panic(fmt.Sprintf("数据库连接测试失败: %v", err))
	}

	// 创建用户模型
	userModel := model.NewUserModel(db)

	// 创建认证工具
	auth := utils.NewAuth(c.Auth.JWTSecret, time.Duration(c.Auth.TokenExpire)*24*time.Hour)

	timeout := time.Duration(c.Services.GameCatalog.Timeout) * time.Millisecond
	gameClient := integration.NewGameCatalogClient(c.Services.GameCatalog.BaseURL, timeout)

	// 创建用户表（开发环境）
	if err := userModel.CreateUsersTable(); err != nil {
		panic(fmt.Sprintf("创建用户表失败: %v", err))
	}

	return &ServiceContext{
		Config:            c,
		DB:                db,
		UserModel:         userModel,
		Auth:              auth,
		GameCatalogClient: gameClient,
	}
}
