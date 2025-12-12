// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package svc

import (
	"fmt"
	"path/filepath"

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

	topicsSource := c.DataSource.TopicsFile
	if !filepath.IsAbs(topicsSource) {
		topicsSource = filepath.Clean(topicsSource)
	}
	postsSource := c.DataSource.PostsFile
	if !filepath.IsAbs(postsSource) {
		postsSource = filepath.Clean(postsSource)
	}
	followsSource := c.DataSource.FollowsFile
	if !filepath.IsAbs(followsSource) {
		followsSource = filepath.Clean(followsSource)
	}

	topicRepo, err := model.NewTopicRepository(topicsSource)
	if err != nil {
		panic(fmt.Sprintf("load topics: %v", err))
	}
	postRepo, err := model.NewPostRepository(postsSource)
	if err != nil {
		panic(fmt.Sprintf("load posts: %v", err))
	}
	followRepo, err := model.NewFollowRepository(followsSource)
	if err != nil {
		panic(fmt.Sprintf("load follows: %v", err))
	}

	return &ServiceContext{
		Config:     c,
		Auth:       middleware.NewAuthMiddleware(jwtTool).Handle,
		Jwt:        jwtTool,
		PostRepo:   postRepo,
		TopicRepo:  topicRepo,
		FollowRepo: followRepo,
	}
}
