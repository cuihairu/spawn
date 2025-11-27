// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package svc

import (
	"fmt"
	"path/filepath"

	"github.com/tappi/tappi/services/content-service/internal/config"
	"github.com/tappi/tappi/services/content-service/model"
)

type ServiceContext struct {
	Config            config.Config
	GuideRepository   *model.GuideRepository
	CommentRepository *model.CommentRepository
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

	return &ServiceContext{
		Config:            c,
		GuideRepository:   guideRepo,
		CommentRepository: commentRepo,
	}
}
