// Code scaffolded by goctl. Safe to edit.
// goctl 1.9.2

package svc

import (
	"fmt"
	"path/filepath"

	"github.com/tappi/tappi/services/game-catalog/internal/config"
	"github.com/tappi/tappi/services/game-catalog/model"
	"github.com/tappi/tappi/services/game-catalog/utils"
)

type ServiceContext struct {
	Config         config.Config
	GameRepository model.GameStore
	Auth           *utils.Auth
}

func NewServiceContext(c config.Config) *ServiceContext {
	source := c.DataSource.File
	if !filepath.IsAbs(source) {
		source = filepath.Clean(source)
	}

	repo, err := model.NewGameRepository(source)
	if err != nil {
		panic(fmt.Sprintf("load game data: %v", err))
	}

	return &ServiceContext{
		Config:         c,
		GameRepository: repo,
		Auth:           utils.NewAuth(c.Auth.JWTSecret),
	}
}
