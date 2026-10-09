// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package svc

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/tappi/tappi/services/community/internal/config"
	"github.com/tappi/tappi/services/community/internal/middleware"
	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/utils"
	"github.com/zeromicro/go-zero/rest"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

type ServiceContext struct {
	Config config.Config
	Auth   rest.Middleware
	Jwt    *utils.Auth

	PostRepo    model.PostStore
	TopicRepo   model.TopicStore
	FollowRepo  model.FollowStore
	CommentRepo model.CommentStore
}

func NewServiceContext(c config.Config) *ServiceContext {
	jwtTool := utils.NewAuth(c.Auth.JWTSecret)

	// DSN 含 file: 或 .db → SQLite（开发/测试），否则 MySQL（与
	// user-service.NewServiceContext / game-catalog / content-service 同款探测）。
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

	topicModel := model.NewTopicModel(db)
	postModel := model.NewPostModel(db)
	followModel := model.NewFollowModel(db)

	// 建表 + 空表写入内嵌种子（首次启动初始化演示内容）
	if err := topicModel.CreateTopicsTable(); err != nil {
		panic(fmt.Sprintf("创建话题表失败: %v", err))
	}
	if err := topicModel.SeedIfEmpty(); err != nil {
		panic(fmt.Sprintf("初始化话题种子数据失败: %v", err))
	}
	if err := postModel.CreatePostsTable(); err != nil {
		panic(fmt.Sprintf("创建帖子表失败: %v", err))
	}
	if err := postModel.SeedIfEmpty(); err != nil {
		panic(fmt.Sprintf("初始化帖子种子数据失败: %v", err))
	}
	if err := followModel.CreateFollowsTable(); err != nil {
		panic(fmt.Sprintf("创建关注关系表失败: %v", err))
	}
	if err := postModel.CreatePostLikesTable(); err != nil {
		panic(fmt.Sprintf("创建点赞关系表失败: %v", err))
	}
	commentModel := model.NewCommentModel(db)
	if err := commentModel.CreateCommentsTable(); err != nil {
		panic(fmt.Sprintf("创建评论表失败: %v", err))
	}

	return &ServiceContext{
		Config:      c,
		Auth:        middleware.NewAuthMiddleware(jwtTool).Handle,
		Jwt:         jwtTool,
		PostRepo:    postModel,
		TopicRepo:   topicModel,
		FollowRepo:  followModel,
		CommentRepo: commentModel,
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
