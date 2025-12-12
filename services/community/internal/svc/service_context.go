// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package svc

import (
	"github.com/tappi/tappi/services/community/internal/config"
	"github.com/tappi/tappi/services/community/internal/middleware"
	"github.com/tappi/tappi/services/community/internal/model"
	"github.com/tappi/tappi/services/community/utils"
	"github.com/zeromicro/go-zero/rest"
)

type ServiceContext struct {
	Config config.Config
	Auth   rest.Middleware
	Jwt    *utils.Auth

	PostRepo   *model.PostRepository
	TopicRepo  *model.TopicRepository
	FollowRepo *model.FollowRepository
}

func NewServiceContext(c config.Config) *ServiceContext {
	jwtTool := utils.NewAuth(c.Auth.JWTSecret)
	return &ServiceContext{
		Config:     c,
		Auth:       middleware.NewAuthMiddleware(jwtTool).Handle,
		Jwt:        jwtTool,
		PostRepo:   model.NewPostRepository(),
		TopicRepo:  model.NewTopicRepository(),
		FollowRepo: model.NewFollowRepository(),
	}
}
