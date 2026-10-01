package svc

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/tappi/tappi/services/content-service/client"
	"github.com/tappi/tappi/services/content-service/internal/config"
	"github.com/tappi/tappi/services/content-service/model"
	"github.com/tappi/tappi/services/content-service/utils"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

type ServiceContext struct {
	Config            config.Config
	GuideRepository   model.GuideStore
	CommentRepository model.CommentStore
	Auth              *utils.Auth
	GameCatalogClient *client.GameCatalogClient
}

func NewServiceContext(c config.Config) *ServiceContext {
	// DSN 含 file: 或 .db → SQLite（开发/测试），否则 MySQL（与
	// user-service.NewServiceContext / game-catalog 同款探测）。
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

	guideModel := model.NewGuideModel(db)
	commentModel := model.NewCommentModel(db)

	// 建表 + 空表写入内嵌种子（首次启动初始化演示内容）
	if err := guideModel.CreateGuidesTable(); err != nil {
		panic(fmt.Sprintf("创建攻略表失败: %v", err))
	}
	if err := guideModel.SeedIfEmpty(); err != nil {
		panic(fmt.Sprintf("初始化攻略种子数据失败: %v", err))
	}
	if err := commentModel.CreateCommentsTable(); err != nil {
		panic(fmt.Sprintf("创建评论表失败: %v", err))
	}
	if err := commentModel.SeedIfEmpty(); err != nil {
		panic(fmt.Sprintf("初始化评论种子数据失败: %v", err))
	}

	// 初始化认证工具
	auth := utils.NewAuth(c.Auth.JWTSecret)

	// 初始化游戏目录服务客户端
	gameCatalogClient := client.NewGameCatalogClient(
		c.Services.GameCatalog.BaseURL,
		time.Duration(c.Services.GameCatalog.Timeout)*time.Millisecond,
	)

	return &ServiceContext{
		Config:            c,
		GuideRepository:   guideModel,
		CommentRepository: commentModel,
		Auth:              auth,
		GameCatalogClient: gameCatalogClient,
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
