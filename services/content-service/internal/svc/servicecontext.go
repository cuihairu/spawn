package svc

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/tappi/tappi/services/content-service/client"
	"github.com/tappi/tappi/services/content-service/internal/config"
	"github.com/tappi/tappi/services/content-service/model"
	"github.com/tappi/tappi/services/content-service/utils"
)

type ServiceContext struct {
	Config            config.Config
	GuideRepository   *model.GuideRepository
	CommentRepository *model.CommentRepository
	Auth              *utils.Auth
	GameCatalogClient *client.GameCatalogClient
}

func NewServiceContext(c config.Config) *ServiceContext {
	guidesSource := c.DataSource.GuidesFile
	if !filepath.IsAbs(guidesSource) {
		guidesSource = filepath.Clean(guidesSource)
	}

	commentsSource := c.DataSource.CommentsFile
	if !filepath.IsAbs(commentsSource) {
		commentsSource = filepath.Clean(commentsSource)
	}

	guideRepo, err := model.NewGuideRepository(guidesSource)
	if err != nil {
		panic(fmt.Sprintf("load guide data: %v", err))
	}

	commentRepo, err := model.NewCommentRepository(commentsSource)
	if err != nil {
		panic(fmt.Sprintf("load comment data: %v", err))
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
		GuideRepository:   guideRepo,
		CommentRepository: commentRepo,
		Auth:              auth,
		GameCatalogClient: gameCatalogClient,
	}
}
