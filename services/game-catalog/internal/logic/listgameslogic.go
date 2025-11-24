package logic

import (
	"context"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type ListGamesLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewListGamesLogic(ctx context.Context, svcCtx *svc.ServiceContext) *ListGamesLogic {
	return &ListGamesLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *ListGamesLogic) ListGames(req *types.ListGamesRequest) (resp *types.ListGamesResponse, err error) {
	filter := model.GameFilter{
		Keyword:  req.Keyword,
		Genre:    req.Genre,
		Platform: req.Platform,
		Tag:      req.Tag,
		Sort:     req.Sort,
		Limit:    int(req.Limit),
		Offset:   int(req.Offset),
	}

	if filter.Limit <= 0 {
		filter.Limit = 20
	}

	games, total := l.svcCtx.GameRepository.List(filter)

	return &types.ListGamesResponse{
		Games:  toTypesGames(games),
		Total:  total,
		Limit:  req.Limit,
		Offset: req.Offset,
	}, nil
}
