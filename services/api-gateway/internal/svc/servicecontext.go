// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package svc

import (
	"context"
	"time"

	"github.com/tappi/tappi/services/api-gateway/internal/config"
	"github.com/tappi/tappi/services/api-gateway/internal/integration"
)

type UserService interface {
	Login(ctx context.Context, payload integration.LoginPayload) (*integration.LoginResult, error)
	GetRecommendations(ctx context.Context, userID int64, limit int64, genres string, token string) (*integration.RecommendationResponse, error)
}

type GameCatalogService interface {
	GetFeatured(ctx context.Context, limit int64) (map[string]interface{}, error)
}

type ServiceContext struct {
	Config      config.Config
	UserService UserService
	GameCatalog GameCatalogService
}

func NewServiceContext(c config.Config) *ServiceContext {
	userClient := integration.NewUserServiceClient(c.Upstreams.UserService.BaseURL, time.Millisecond*time.Duration(c.Upstreams.UserService.Timeout))
	gameClient := integration.NewGameCatalogClient(c.Upstreams.GameCatalog.BaseURL, time.Millisecond*time.Duration(c.Upstreams.GameCatalog.Timeout))

	return &ServiceContext{
		Config:      c,
		UserService: userClient,
		GameCatalog: gameClient,
	}
}
