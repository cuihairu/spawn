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

	// 公开详情只露出已上架游戏；提审中/驳回/下架由运营或合作方后台查看。
	if game.Status != model.GameStatusApproved {
		return nil, newAPIError(http.StatusNotFound, "游戏不存在")
	}

	return &types.GameDetailResponse{
		Game: toTypesGame(game),
	}, nil
}
