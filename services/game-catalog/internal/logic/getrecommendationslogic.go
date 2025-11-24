package logic

import (
	"context"
	"strings"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetRecommendationsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetRecommendationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetRecommendationsLogic {
	return &GetRecommendationsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetRecommendationsLogic) GetRecommendations(req *types.RecommendationsRequest) (*types.RecommendationsResponse, error) {
	var genres []string
	if req.Genres != "" {
		for _, item := range strings.Split(req.Genres, ",") {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			genres = append(genres, item)
		}
	}

	limit := int(req.Limit)
	if limit <= 0 {
		limit = 5
	}

	games := l.svcCtx.GameRepository.Recommend(req.UserId, genres, limit)
	return &types.RecommendationsResponse{
		Games: toTypesGames(games),
	}, nil
}
