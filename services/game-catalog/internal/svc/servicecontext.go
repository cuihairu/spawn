// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package svc

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
	"github.com/tappi/tappi/services/game-catalog/internal/store"
	"github.com/tappi/tappi/services/game-catalog/model"
	"github.com/tappi/tappi/services/game-catalog/utils"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
	"github.com/zeromicro/go-zero/core/logx"
)

type ServiceContext struct {
	Config          config.Config
	GameRepository  model.GameStore
	AdminRepository model.AdminStore
	PackageStore    *store.LocalPackageStore
	Auth            *utils.Auth
}

func NewServiceContext(c config.Config) *ServiceContext {
	// GP-004 生产接入：JWT 密钥启动期守卫。空密钥拒绝启动；内置演示密钥
	// （源码可见，可伪造任意用户令牌）大声告警，除本地开发外须以 JWT_SECRET 覆盖。
	if err := utils.ValidateJWTSecret(c.Auth.JWTSecret); err != nil {
		panic(err.Error())
	}
	if c.Auth.JWTSecret == utils.WellKnownDevJWTSecret {
		logx.Errorf("正在使用内置演示 JWT 密钥：除本地开发外必须以 JWT_SECRET 环境变量覆盖，否则任何人可伪造登录令牌")
	}

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
	adminModel := model.NewAdminModel(db)

	// 建表 + 空表写入内嵌种子（首次启动初始化演示目录）
	if err := gameModel.CreateGamesTable(); err != nil {
		panic(fmt.Sprintf("创建游戏表失败: %v", err))
	}
	if err := gameModel.SeedIfEmpty(); err != nil {
		panic(fmt.Sprintf("初始化游戏种子数据失败: %v", err))
	}
	// 运营后台表（GP-103：角色/归属/提审/版本/审计）
	if err := adminModel.CreateAdminTables(); err != nil {
		panic(fmt.Sprintf("创建运营后台表失败: %v", err))
	}
	// 引导管理员：配置的账号 id 幂等写为 platform_admin
	for _, raw := range c.Auth.AdminUserIds {
		uid, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
		if err != nil || uid <= 0 {
			continue
		}
		if err := adminModel.SetRole(uid, model.RolePlatformAdmin, 0); err != nil {
			panic(fmt.Sprintf("引导管理员角色失败（user %d）: %v", uid, err))
		}
	}

	// 包体存储（GP-105）：目录未配置（测试装配）时保持 nil，
	// 上传/下载端点以「包体存储未配置」拒绝，不 panic。
	var packageStore *store.LocalPackageStore
	if c.Package.Dir != "" {
		ps, err := store.NewLocalPackageStore(c.Package.Dir)
		if err != nil {
			panic(fmt.Sprintf("初始化包体存储失败: %v", err))
		}
		packageStore = ps
	}

	return &ServiceContext{
		Config:          c,
		GameRepository:  gameModel,
		AdminRepository: adminModel,
		PackageStore:    packageStore,
		Auth:            utils.NewAuth(c.Auth.JWTSecret),
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
