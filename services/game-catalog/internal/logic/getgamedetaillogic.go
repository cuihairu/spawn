package logic

import (
	"context"
	"errors"
	"net/http"

	"github.com/tappi/tappi/services/game-catalog/internal/svc"
	"github.com/tappi/tappi/services/game-catalog/internal/types"
	"github.com/tappi/tappi/services/game-catalog/model"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetGameDetailLogic struct {
	logx.Logger
	ctx    context.Context
	svcCtx *svc.ServiceContext
}

func NewGetGameDetailLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetGameDetailLogic {
	return &GetGameDetailLogic{
		Logger: logx.WithContext(ctx),
		ctx:    ctx,
		svcCtx: svcCtx,
	}
}

func (l *GetGameDetailLogic) GetGameDetail(req *types.GetGameDetailRequest) (*types.GameDetailResponse, error) {
	game, err := l.svcCtx.GameRepository.Get(req.Id)
	if err != nil {
		if errors.Is(err, model.ErrGameNotFound) {
			return nil, newAPIError(http.StatusNotFound, "游戏不存在")
		}
		l.Errorf("query game failed: %v", err)
		return nil, newAPIError(http.StatusInternalServerError, "查询失败")
	}

	return &types.GameDetailResponse{
		Game: toTypesGame(game),
	}, nil
}
