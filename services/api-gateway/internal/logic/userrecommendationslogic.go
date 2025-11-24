package logic

import (
	"context"

	"github.com/tappi/tappi/services/api-gateway/internal/svc"
	"github.com/tappi/tappi/services/api-gateway/internal/types"

	"github.com/zeromicro/go-zero/core/logx"
)

type UserRecommendationsLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewUserRecommendationsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UserRecommendationsLogic {
	return &UserRecommendationsLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *UserRecommendationsLogic) UserRecommendations(req *types.RecommendationRequest, token string) (*types.RecommendationResponse, error) {
	payload, err := l.svcCtx.UserService.GetRecommendations(l.ctx, req.Id, req.Limit, req.Genres, token)
	if err != nil {
		return nil, err
	}

	return &types.RecommendationResponse{
		Code:    payload.Code,
		Message: payload.Message,
		Data:    payload.Data,
	}, nil
}
