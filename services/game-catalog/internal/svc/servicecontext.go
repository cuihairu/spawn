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
	"time"

	"github.com/tappi/tappi/services/game-catalog/internal/analytics"
	"github.com/tappi/tappi/services/game-catalog/internal/config"
	"github.com/tappi/tappi/services/game-catalog/internal/courierdownload"
	"github.com/tappi/tappi/services/game-catalog/internal/notify"
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
	// Notify GP-306 业务通知发布口（community /internal/notifications）；
	// COMMUNITY_URL 未配置时为 no-op。
	Notify notify.Publisher
	// AnalyticsSvc GP-402 经营分析事实投递口（data-panel /internal/events）；
	// DATA_PANEL_URL 未配置时为 no-op。
	AnalyticsSvc analytics.Client
	// AnalyticsRelay GP-402 outbox 重投循环；nil = DATA_PANEL_URL 未配置
	// （no-op，不启动）。
	AnalyticsRelay *analytics.Relay
	// AnalyticsOutbox GP-402 事实事件本地补偿队列（始终建表；未启用时不写行）。
	AnalyticsOutbox *model.AnalyticsOutboxModel
	// NotifyRelay GP-306/401 业务通知 outbox 重投循环；nil = COMMUNITY_URL
	// 未配置（no-op，不启动）。
	NotifyRelay *notify.Relay
	// NotifyOutbox GP-306/401 业务通知本地补偿队列（始终建表；未启用时不写行）。
	NotifyOutbox *model.NotifyOutboxModel
	// CourierDownload GP-002 download 能力查询（受控包体下载门）；
	// nil = COURIER_ADAPTER_URL 未配置（no-op，不门控）。
	CourierDownload *courierdownload.Client
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
		// SQLite 单写者：限制单连接串行化全部访问，防并发写触发
		// SQLITE_BUSY（P1-3 修复）；MySQL 由连接池并发（InnoDB 行锁）。
		if err == nil {
			db.SetMaxOpenConns(1)
		}
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
	// 引导管理员：配置的账号 id（逗号分隔）幂等写为 platform_admin
	for _, raw := range strings.Split(c.Auth.AdminUserIds, ",") {
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

	// GP-402 经营分析事实投递（DATA_PANEL_URL 未配置 = no-op）：失败先落
	// outbox（幂等持久化），由 Relay 定时重投；outbox 表始终建（后续部署
	// 补数据面板时免迁移），未启用时不写行。
	analyticsOutbox := model.NewAnalyticsOutboxModel(db)
	if err := analyticsOutbox.CreateTables(); err != nil {
		panic(fmt.Sprintf("create analytics outbox tables: %v", err))
	}
	analyticsClient := analytics.NewClient(c.Analytics.BaseURL, c.Analytics.Token, c.Analytics.Timeout,
		analyticsOutbox)
	analyticsRelay := analytics.NewRelay(analyticsOutbox, analyticsClient,
		time.Second*time.Duration(c.Analytics.OutboxIntervalSeconds),
		int(c.Analytics.OutboxMaxAttempts),
		time.Hour*time.Duration(c.Analytics.OutboxRetentionHours))

	// GP-306/401 业务通知投递（COMMUNITY_URL 未配置 = no-op）：失败先落
	// outbox（幂等持久化），由 Relay 定时重投；outbox 表始终建（后续部署
	// 补 community 时免迁移），未启用时不写行。重投安全由 community 侧
	// UNIQUE(user_id, actor_id, type, target_id) 去重保证。
	notifyOutbox := model.NewNotifyOutboxModel(db)
	if err := notifyOutbox.CreateTables(); err != nil {
		panic(fmt.Sprintf("create notify outbox tables: %v", err))
	}
	notifyPublisher := notify.NewPublisher(c.Notify.Url, c.Notify.Token, notifyOutbox)
	notifyRelay := notify.NewRelay(notifyOutbox, notifyPublisher,
		time.Second*time.Duration(c.Notify.OutboxIntervalSeconds),
		int(c.Notify.OutboxMaxAttempts),
		time.Hour*time.Duration(c.Notify.OutboxRetentionHours))

	// GP-002 download 能力查询（COURIER_ADAPTER_URL 未配置 = nil no-op）。
	// Token 复用 Internal.Token（共享 INTERNAL_TOKEN，与 payments 同款）。
	courierDownload := courierdownload.NewClient(c.CourierAdapter.BaseURL, c.Internal.Token,
		time.Millisecond*time.Duration(c.CourierAdapter.Timeout))

	return &ServiceContext{
		Config:          c,
		GameRepository:  gameModel,
		AdminRepository: adminModel,
		PackageStore:    packageStore,
		Auth:            utils.NewAuth(c.Auth.JWTSecret),
		Notify:          notifyPublisher,
		AnalyticsSvc:    analyticsClient,
		AnalyticsRelay:  analyticsRelay,
		AnalyticsOutbox: analyticsOutbox,
		NotifyRelay:     notifyRelay,
		NotifyOutbox:    notifyOutbox,
		CourierDownload: courierDownload,
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
