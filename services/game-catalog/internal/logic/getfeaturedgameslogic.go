package logic

import (
	"context"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFeaturedGamesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetFeaturedGamesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFeaturedGamesLogic {
	return &GetFeaturedGamesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetFeaturedGamesLogic) GetFeaturedGames(req *types.FeaturedGamesRequest) (*types.FeaturedGamesResponse, error) {
	limit := int(req.Limit)
	if limit <= 0 {
		limit = 6
	}

	games := l.svcCtx.GameRepository.Featured(limit)
	return &types.FeaturedGamesResponse{
		Games: toTypesGames(games),
	}, nil
}
