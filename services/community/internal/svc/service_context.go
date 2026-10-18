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
	"github.com/tappi/tappi/services/community/internal/moderation"
	"github.com/tappi/tappi/services/community/internal/realtime"
	"github.com/tappi/tappi/services/community/utils"
	"github.com/zeromicro/go-zero/core/logx"
	"github.com/zeromicro/go-zero/rest"

	_ "github.com/go-sql-driver/mysql"
	_ "github.com/mattn/go-sqlite3"
)

type ServiceContext struct {
	Config config.Config
	Auth   rest.Middleware
	Jwt    *utils.Auth

	PostRepo         model.PostStore
	TopicRepo        model.TopicStore
	FollowRepo       model.FollowStore
	CommentRepo      model.CommentStore
	NotificationRepo model.NotificationStore
	MessageRepo      model.DirectMessageStore
	ReportRepo       model.ReportStore

	// BlockedWords 归一化后的敏感词表（空配置回落内置默认）；
	// AdminSet 审核管理员 id 集合（配置预计算，举报队列查看与处置用）。
	BlockedWords []string
	AdminSet     map[int64]bool

	// Realtime 实时事件发布口（realtime-hub）；Url 未配置时为 no-op。
	Realtime realtime.Publisher
}

// IsAdmin 判断用户是否为审核管理员。
func (svcCtx *ServiceContext) IsAdmin(userId int64) bool {
	return svcCtx.AdminSet[userId]
}

func NewServiceContext(c config.Config) *ServiceContext {
	// JWT 密钥启动期守卫：空密钥拒绝启动；内置演示密钥大声告警
	// （该密钥源码可见，用它签出的令牌可伪造任意用户）。
	if err := utils.ValidateJWTSecret(c.Auth.JWTSecret); err != nil {
		panic(err.Error())
	}
	if c.Auth.JWTSecret == utils.WellKnownDevJWTSecret {
		logx.Errorf("正在使用内置演示 JWT 密钥：除本地开发外必须以 JWT_SECRET 环境变量覆盖，否则任何人可伪造登录令牌")
	}

	// 审核管理员配置启动期校验（演示默认 [1] 在 MySQL 生产部署下拒绝启动）。
	if err := validateModerationAdmins(c.MySQL.DataSource, c.Moderation.AdminUserIds); err != nil {
		panic(err.Error())
	}

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
	notificationModel := model.NewNotificationModel(db)
	if err := notificationModel.CreateNotificationsTable(); err != nil {
		panic(fmt.Sprintf("创建通知表失败: %v", err))
	}
	reportModel := model.NewReportModel(db)
	if err := reportModel.CreateReportsTable(); err != nil {
		panic(fmt.Sprintf("创建举报表失败: %v", err))
	}
	messageModel := model.NewDirectMessageModel(db)
	if err := messageModel.CreateDirectMessagesTable(); err != nil {
		panic(fmt.Sprintf("创建私信表失败: %v", err))
	}

	// 审核配置：敏感词空配置回落内置默认表；管理员集合预计算成 map 供 O(1) 判断。
	blockedWords := moderation.NormalizeWords(c.Moderation.BlockedWords)
	if len(blockedWords) == 0 {
		blockedWords = moderation.NormalizeWords(moderation.DefaultBlockedWords)
	}
	adminSet := make(map[int64]bool, len(c.Moderation.AdminUserIds))
	for _, id := range c.Moderation.AdminUserIds {
		adminSet[id] = true
	}

	return &ServiceContext{
		Config:           c,
		Auth:             middleware.NewAuthMiddleware(jwtTool).Handle,
		Jwt:              jwtTool,
		PostRepo:         postModel,
		TopicRepo:        topicModel,
		FollowRepo:       followModel,
		CommentRepo:      commentModel,
		NotificationRepo: notificationModel,
		MessageRepo:      messageModel,
		ReportRepo:       reportModel,
		BlockedWords:     blockedWords,
		AdminSet:         adminSet,
		Realtime:         realtime.NewPublisher(c.Realtime.Url, c.Realtime.InternalToken),
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

// validateModerationAdmins 审核管理员配置启动期校验：
//   - MySQL（生产姿态）：演示默认 [1] 意味着首位注册者即审核管理员，
//     拒绝启动；空名单视为显式停用审核处置，仅告警；
//   - SQLite（开发/测试）：宽松放行，命中演示默认时告警。
func validateModerationAdmins(dataSource string, adminIds []int64) error {
	isDemoDefault := len(adminIds) == 1 && adminIds[0] == 1
	isSQLite := strings.Contains(dataSource, "file:") || strings.Contains(dataSource, ".db")

	if isSQLite {
		if isDemoDefault {
			logx.Errorf("Moderation.AdminUserIds 为演示默认 [1]（首位注册者即审核管理员）：仅限本地开发")
		}
		return nil
	}
	if isDemoDefault {
		return fmt.Errorf("Moderation.AdminUserIds 为演示默认 [1]（首位注册者即审核管理员），MySQL 部署必须显式配置真实管理员名单")
	}
	if len(adminIds) == 0 {
		logx.Errorf("Moderation.AdminUserIds 为空：无人可查看举报队列与处置（审核处置停用）")
	}
	return nil
}
