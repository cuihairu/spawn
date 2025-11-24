package logic

import (
	"context"

	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/tappi/tappi/services/api-gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type FeaturedGamesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewFeaturedGamesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FeaturedGamesLogic {
	return &FeaturedGamesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *FeaturedGamesLogic) FeaturedGames(req *types.FeaturedRequest) (*types.FeaturedResponse, error) {
	limit := req.Limit
	if limit <= 0 {
		limit = 6
	}

	payload, err := l.svcCtx.GameCatalog.GetFeatured(l.ctx, limit)
	if err != nil {
		return nil, err
	}

	games, _ := payload["games"].([]interface{})

	return &types.FeaturedResponse{
		Games: games,
	}, nil
}
